package status

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/sse"
	"github.com/wakecomputer/wakecomputer/internal/store"
)

const (
	Online        = "online"
	OnlineNoAgent = "online_no_agent"
	Waking        = "waking"
	Offline       = "offline"
	Unknown       = "unknown"
)

type Service struct {
	st  *store.Store
	hub *sse.Hub
	log *slog.Logger

	mu      sync.RWMutex
	hb      map[string]int64
	waking  map[string]int64
	current map[string]string

	heartbeatTTL time.Duration
	wakeTTL      time.Duration
	probeTimeout time.Duration
}

func New(st *store.Store, hub *sse.Hub, log *slog.Logger) *Service {
	return &Service{
		st:           st,
		hub:          hub,
		log:          log,
		hb:           map[string]int64{},
		waking:       map[string]int64{},
		current:      map[string]string{},
		heartbeatTTL: 45 * time.Second,
		wakeTTL:      120 * time.Second,
		probeTimeout: 1500 * time.Millisecond,
	}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.Refresh(ctx)
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.Refresh(ctx)
			}
		}
	}()
}

func (s *Service) Heartbeat(hostID string) {
	s.mu.Lock()
	s.hb[hostID] = time.Now().UnixMilli()
	delete(s.waking, hostID)
	s.mu.Unlock()
}

func (s *Service) MarkWaking(hostID string, d time.Duration) {
	s.mu.Lock()
	s.waking[hostID] = time.Now().Add(d).UnixMilli()
	s.current[hostID] = Waking
	s.mu.Unlock()
	s.publish(hostID, "", Waking)
}

func (s *Service) Status(hostID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.current[hostID]; ok {
		return v
	}
	return Unknown
}

// Apply fills runtime status and capability fields on a host list.
func (s *Service) Apply(hosts []*store.Host) {
	s.mu.RLock()
	hbCopy := make(map[string]int64, len(s.hb))
	for k, v := range s.hb {
		hbCopy[k] = v
	}
	wakingCopy := make(map[string]int64, len(s.waking))
	for k, v := range s.waking {
		wakingCopy[k] = v
	}
	curCopy := make(map[string]string, len(s.current))
	for k, v := range s.current {
		curCopy[k] = v
	}
	s.mu.RUnlock()

	now := time.Now().UnixMilli()
	for _, h := range hosts {
		status := h.LastStatus
		if status == "" {
			status = Unknown
		}
		if v, ok := curCopy[h.ID]; ok && v != "" {
			status = v
		}
		if last, ok := hbCopy[h.ID]; ok {
			if now-last <= s.heartbeatTTL.Milliseconds() {
				status = Online
			} else if status == Online {
				// Heartbeat expired: let the next refresh confirm via probe.
				status = Unknown
			}
		}
		if until, ok := wakingCopy[h.ID]; ok && now < until {
			if status != Online && status != OnlineNoAgent {
				status = Waking
			}
		}
		h.Status = status
		h.Capability.Wake = true
		hasAgent := h.AgentURL != ""
		h.Capability.Shutdown = hasAgent
		h.Capability.Reboot = hasAgent
		h.Capability.Cancel = hasAgent
	}
}

// Probe forces a fresh status check for one host.
func (s *Service) Probe(ctx context.Context, h *store.Host) string {
	st := s.compute(ctx, h)
	s.setCurrent(h, st)
	return st
}

func (s *Service) Refresh(ctx context.Context) {
	hosts, err := s.st.ListHosts()
	if err != nil {
		s.log.Warn("status: list hosts", "err", err)
		return
	}
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for _, h := range hosts {
		h := h
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			st := s.compute(ctx, h)
			s.setCurrent(h, st)
		}()
	}
	wg.Wait()
}

func (s *Service) compute(ctx context.Context, h *store.Host) string {
	now := time.Now()
	nowMs := now.UnixMilli()

	s.mu.RLock()
	lastHB := s.hb[h.ID]
	wakeUntil := s.waking[h.ID]
	s.mu.RUnlock()

	if lastHB > 0 && nowMs-lastHB <= s.heartbeatTTL.Milliseconds() {
		return Online
	}
	if addr := probeAddr(h); addr != "" {
		if tcpReachable(ctx, addr, s.probeTimeout) {
			return OnlineNoAgent
		}
	}
	if wakeUntil > 0 && nowMs < wakeUntil {
		return Waking
	}
	if h.LastStatus != "" && h.LastStatus != Unknown {
		return Offline
	}
	return Unknown
}

func probeAddr(h *store.Host) string {
	if h.AgentURL != "" {
		if u, err := url.Parse(h.AgentURL); err == nil {
			host := u.Hostname()
			port := u.Port()
			if port == "" {
				port = "45871"
			}
			if host != "" {
				return net.JoinHostPort(host, port)
			}
		}
	}
	if h.IP != "" && h.ProbePort > 0 {
		return net.JoinHostPort(h.IP, strconv.Itoa(h.ProbePort))
	}
	return ""
}

func tcpReachable(ctx context.Context, addr string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s *Service) setCurrent(h *store.Host, st string) {
	s.mu.Lock()
	prev := s.current[h.ID]
	changed := prev != st
	s.current[h.ID] = st
	s.mu.Unlock()

	if st == Offline {
		s.mu.Lock()
		delete(s.waking, h.ID)
		s.mu.Unlock()
	}
	if !changed {
		return
	}
	if prev != "" || h.LastStatus != st {
		_ = s.st.UpdateHostStatus(h.ID, st)
	}
	s.publish(h.ID, h.Name, st)
}

func (s *Service) publish(hostID, name, st string) {
	if name == "" {
		if h, err := s.st.GetHost(hostID); err == nil {
			name = h.Name
		}
	}
	s.hub.Publish("host.status", map[string]any{
		"hostId":    hostID,
		"name":      name,
		"status":    st,
		"updatedAt": time.Now().UnixMilli(),
	})
}
