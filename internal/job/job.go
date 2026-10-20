package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/agentapi"
	"github.com/wakecomputer/wakecomputer/internal/id"
	"github.com/wakecomputer/wakecomputer/internal/sse"
	"github.com/wakecomputer/wakecomputer/internal/status"
	"github.com/wakecomputer/wakecomputer/internal/store"
	"github.com/wakecomputer/wakecomputer/internal/wol"
)

type Action string

const (
	ActionWake     Action = "wake"
	ActionShutdown Action = "shutdown"
	ActionReboot   Action = "reboot"
	ActionCancel   Action = "cancel"
)

func ValidAction(a string) bool {
	switch Action(a) {
	case ActionWake, ActionShutdown, ActionReboot, ActionCancel:
		return true
	}
	return false
}

func Destructive(a string) bool {
	return a == string(ActionShutdown) || a == string(ActionReboot)
}

const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusOK       = "ok"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

type Result struct {
	HostID string `json:"hostId"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	At     int64  `json:"at,omitempty"`
}

type Job struct {
	ID           string    `json:"id"`
	Action       string    `json:"action"`
	Status       string    `json:"status"`
	Total        int       `json:"total"`
	Done         int       `json:"done"`
	Succeeded    int       `json:"succeeded"`
	Failed       int       `json:"failed"`
	Canceled     int       `json:"canceled"`
	DelaySeconds int       `json:"delaySeconds"`
	Results      []*Result `json:"results"`
	CreatedAt    int64     `json:"createdAt"`
	FinishedAt   *int64    `json:"finishedAt,omitempty"`
	UserID       string    `json:"-"`
	IP           string    `json:"-"`
	UA           string    `json:"-"`
	cancel       bool
	mu           sync.Mutex
}

type View struct {
	ID           string   `json:"id"`
	Action       string   `json:"action"`
	Status       string   `json:"status"`
	Total        int      `json:"total"`
	Done         int      `json:"done"`
	Succeeded    int      `json:"succeeded"`
	Failed       int      `json:"failed"`
	Canceled     int      `json:"canceled"`
	DelaySeconds int      `json:"delaySeconds"`
	Results      []Result `json:"results"`
	CreatedAt    int64    `json:"createdAt"`
	FinishedAt   *int64   `json:"finishedAt,omitempty"`
}

func (j *Job) Snapshot() View {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := View{
		ID:           j.ID,
		Action:       j.Action,
		Status:       j.Status,
		Total:        j.Total,
		Done:         j.Done,
		Succeeded:    j.Succeeded,
		Failed:       j.Failed,
		Canceled:     j.Canceled,
		DelaySeconds: j.DelaySeconds,
		Results:      make([]Result, 0, len(j.Results)),
		CreatedAt:    j.CreatedAt,
		FinishedAt:   j.FinishedAt,
	}
	for _, r := range j.Results {
		v.Results = append(v.Results, *r)
	}
	return v
}

type Manager struct {
	mu     sync.Mutex
	jobs   map[string]*Job
	hub    *sse.Hub
	st     *store.Store
	agent  *agentapi.Client
	status *status.Service
	log    *slog.Logger

	defaultBroadcast string
	wolPort          int
	concurrency      int
	buffer           time.Duration
	ttl              time.Duration

	cfgMu sync.RWMutex
}

func New(st *store.Store, hub *sse.Hub, ag *agentapi.Client, stt *status.Service, log *slog.Logger,
	defaultBroadcast string, wolPort, concurrency int) *Manager {
	if concurrency <= 0 || concurrency > 16 {
		concurrency = 4
	}
	m := &Manager{
		jobs:             map[string]*Job{},
		hub:              hub,
		st:               st,
		agent:            ag,
		status:           stt,
		log:              log,
		defaultBroadcast: defaultBroadcast,
		wolPort:          wolPort,
		concurrency:      concurrency,
		buffer:           10 * time.Second,
		ttl:              30 * time.Minute,
	}
	go m.cleanupLoop()
	return m
}

// SetDefaults updates runtime defaults coming from the settings API.
func (m *Manager) SetDefaults(broadcast string, concurrency int) {
	if concurrency < 1 || concurrency > 16 {
		concurrency = 4
	}
	m.cfgMu.Lock()
	if broadcast != "" {
		m.defaultBroadcast = broadcast
	}
	m.concurrency = concurrency
	m.cfgMu.Unlock()
}

func (m *Manager) config() (string, int) {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.defaultBroadcast, m.concurrency
}

func (m *Manager) Create(ctx context.Context, hostIDs []string, action string, delaySeconds int, userID, ip, ua string) (*Job, error) {
	if !ValidAction(action) {
		return nil, fmt.Errorf("invalid action")
	}
	if len(hostIDs) == 0 {
		return nil, fmt.Errorf("no hosts selected")
	}
	if len(hostIDs) > 50 {
		return nil, fmt.Errorf("too many hosts (max 50)")
	}
	seen := map[string]bool{}
	hosts := make([]*store.Host, 0, len(hostIDs))
	results := make([]*Result, 0, len(hostIDs))
	for _, hid := range hostIDs {
		if seen[hid] {
			continue
		}
		seen[hid] = true
		h, err := m.st.GetHost(hid)
		if err != nil {
			return nil, fmt.Errorf("host %s not found", hid)
		}
		hosts = append(hosts, h)
		results = append(results, &Result{HostID: h.ID, Name: h.Name, Status: StatusQueued})
	}
	if delaySeconds < 0 {
		delaySeconds = 0
	}
	if delaySeconds > 300 {
		delaySeconds = 300
	}
	if action == string(ActionShutdown) || action == string(ActionReboot) || action == string(ActionCancel) {
		if delaySeconds == 0 && action != string(ActionCancel) {
			delaySeconds = 30
		}
	}

	j := &Job{
		ID:           id.New(),
		Action:       action,
		Status:       StatusRunning,
		Total:        len(results),
		DelaySeconds: delaySeconds,
		Results:      results,
		CreatedAt:    time.Now().UnixMilli(),
		UserID:       userID,
		IP:           ip,
		UA:           ua,
	}
	m.mu.Lock()
	m.jobs[j.ID] = j
	m.mu.Unlock()

	m.publish(j)
	go m.run(j, hosts)
	return j, nil
}

func (m *Manager) Get(jobID string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[jobID]
	return j, ok
}

func (m *Manager) Cancel(jobID string) (*Job, error) {
	m.mu.Lock()
	j, ok := m.jobs[jobID]
	m.mu.Unlock()
	if !ok {
		return nil, store.ErrNotFound
	}
	j.mu.Lock()
	if j.Status != StatusRunning {
		j.mu.Unlock()
		return j, fmt.Errorf("job already finished")
	}
	j.cancel = true
	for _, r := range j.Results {
		if r.Status == StatusQueued {
			r.Status = StatusCanceled
			j.Canceled++
			j.Done++
		}
	}
	j.mu.Unlock()
	m.publish(j)
	return j, nil
}

func (m *Manager) run(j *Job, hosts []*store.Host) {
	if Destructive(j.Action) || j.Action == string(ActionCancel) {
		deadline := time.Now().Add(m.buffer)
		for time.Now().Before(deadline) {
			if j.isCanceled() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if j.isCanceled() {
		m.finalize(j)
		return
	}

	defaultBroadcast, concurrency := m.config()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, h := range hosts {
		r := j.Results[i]
		j.mu.Lock()
		if j.cancel {
			if r.Status == StatusQueued {
				r.Status = StatusCanceled
				j.Canceled++
				j.Done++
			}
			j.mu.Unlock()
			continue
		}
		j.mu.Unlock()

		wg.Add(1)
		sem <- struct{}{}
		go func(h *store.Host, r *Result) {
			defer wg.Done()
			defer func() { <-sem }()
			j.mu.Lock()
			if j.cancel {
				if r.Status == StatusQueued {
					r.Status = StatusCanceled
					j.Canceled++
					j.Done++
				}
				j.mu.Unlock()
				m.publish(j)
				return
			}
			r.Status = StatusRunning
			j.mu.Unlock()
			m.publish(j)

			err := m.execute(j, h, defaultBroadcast)

			j.mu.Lock()
			r.At = time.Now().UnixMilli()
			if err != nil {
				r.Status = StatusFailed
				r.Error = err.Error()
				j.Failed++
			} else {
				r.Status = StatusOK
				j.Succeeded++
			}
			j.Done++
			j.mu.Unlock()
			m.publish(j)
		}(h, r)
	}
	wg.Wait()
	m.finalize(j)
}

func (m *Manager) execute(j *Job, h *store.Host, defaultBroadcast string) error {
	switch Action(j.Action) {
	case ActionWake:
		targets := h.BroadcastTargets
		if len(targets) == 0 && defaultBroadcast != "" {
			targets = []string{defaultBroadcast}
		}
		mac, err := wol.ParseMAC(h.MAC)
		if err != nil {
			return err
		}
		results, err := wol.Send(mac, targets, m.wolPort)
		if err != nil {
			return err
		}
		var lastErr string
		okCount := 0
		for _, r := range results {
			if r.OK {
				okCount++
			} else {
				lastErr = r.Error
			}
		}
		if okCount == 0 {
			return fmt.Errorf("all wake packets failed: %s", lastErr)
		}
		m.status.MarkWaking(h.ID, 120*time.Second)
		return nil

	case ActionShutdown, ActionReboot, ActionCancel:
		if h.AgentURL == "" {
			return fmt.Errorf("主机未安装 Agent，无法执行该操作")
		}
		cmd := agentapi.Command{
			ID:           id.New(),
			Action:       j.Action,
			DelaySeconds: j.DelaySeconds,
			Reason:       "WakeComputer remote " + j.Action,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return m.agent.SendCommand(ctx, h, cmd)
	}
	return fmt.Errorf("unsupported action")
}

func (j *Job) isCanceled() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cancel
}

func (m *Manager) finalize(j *Job) {
	j.mu.Lock()
	finished := time.Now().UnixMilli()
	j.FinishedAt = &finished
	j.Done = 0
	j.Succeeded, j.Failed, j.Canceled = 0, 0, 0
	for _, r := range j.Results {
		j.Done++
		switch r.Status {
		case StatusOK:
			j.Succeeded++
		case StatusFailed:
			j.Failed++
		case StatusCanceled:
			j.Canceled++
		}
	}
	switch {
	case j.cancel:
		if j.Succeeded == 0 {
			j.Status = "canceled"
		} else {
			j.Status = "partial"
		}
	case j.Failed == 0:
		j.Status = "succeeded"
	case j.Succeeded == 0:
		j.Status = "failed"
	default:
		j.Status = "partial"
	}
	detail, _ := json.Marshal(j.Results)
	j.mu.Unlock()

	_ = m.st.InsertAudit(&store.Audit{
		UserID:    j.UserID,
		Action:    "batch." + j.Action,
		Result:    j.Status,
		Detail:    string(detail),
		IP:        j.IP,
		UserAgent: j.UA,
	})
	m.publish(j)
}

func (m *Manager) publish(j *Job) {
	j.mu.Lock()
	results := make([]Result, 0, len(j.Results))
	for _, r := range j.Results {
		results = append(results, *r)
	}
	snapshot := struct {
		ID        string   `json:"id"`
		Action    string   `json:"action"`
		Status    string   `json:"status"`
		Total     int      `json:"total"`
		Done      int      `json:"done"`
		Succeeded int      `json:"succeeded"`
		Failed    int      `json:"failed"`
		Canceled  int      `json:"canceled"`
		Results   []Result `json:"results"`
	}{
		ID: j.ID, Action: j.Action, Status: j.Status, Total: j.Total, Done: j.Done,
		Succeeded: j.Succeeded, Failed: j.Failed, Canceled: j.Canceled, Results: results,
	}
	j.mu.Unlock()
	evType := "job.update"
	if snapshot.Status != StatusRunning {
		evType = "job.done"
	}
	m.hub.Publish(evType, snapshot)
}

func (m *Manager) cleanupLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-m.ttl).UnixMilli()
		m.mu.Lock()
		for k, j := range m.jobs {
			j.mu.Lock()
			finished := j.FinishedAt != nil && *j.FinishedAt < cutoff
			j.mu.Unlock()
			if finished {
				delete(m.jobs, k)
			}
		}
		m.mu.Unlock()
	}
}
