package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/version"
)

type command struct {
	ID           string `json:"id"`
	Action       string `json:"action"`
	DelaySeconds int    `json:"delaySeconds"`
	Reason       string `json:"reason"`
}

type executor struct {
	token   string
	mu      sync.Mutex
	seen    map[string]bool
	order   []string
	pending map[string]context.CancelFunc
	started time.Time
}

func newExecutor(token string) *executor {
	return &executor{
		token:   token,
		seen:    map[string]bool{},
		pending: map[string]context.CancelFunc{},
		started: time.Now(),
	}
}

func runAgent(ctx context.Context, cfg *Config) error {
	ex := newExecutor(cfg.Token)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/command", ex.handleCommand)
	mux.HandleFunc("GET /v1/health", ex.handleHealth)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("agent command server listening on %s", cfg.Listen)
		if err := srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	go heartbeatLoop(ctx, cfg)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	}
}

func (e *executor) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(got), []byte(e.token)) == 1
}

func (e *executor) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !e.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       version.Version,
		"uptimeSeconds": int64(time.Since(e.started).Seconds()),
		"pending":       e.pendingCount(),
	})
}

func (e *executor) handleCommand(w http.ResponseWriter, r *http.Request) {
	if !e.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var cmd command
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&cmd); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if cmd.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	switch cmd.Action {
	case "shutdown", "reboot", "cancel":
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
		return
	}

	if e.markSeen(cmd.ID) {
		writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "duplicate": true})
		return
	}

	if cmd.Action == "cancel" {
		e.cancelAll()
		go func() {
			if err := runShutdown("cancel"); err != nil {
				log.Printf("cancel failed: %v", err)
			}
		}()
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "executeAt": time.Now().UTC().Format(time.RFC3339)})
		return
	}

	delay := cmd.DelaySeconds
	if delay < 5 {
		delay = 5
	}
	if delay > 3600 {
		delay = 3600
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.pending[cmd.ID] = cancel
	e.mu.Unlock()

	executeAt := time.Now().Add(time.Duration(delay) * time.Second)
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.pending, cmd.ID)
			e.mu.Unlock()
		}()
		timer := time.NewTimer(time.Duration(delay) * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			log.Printf("command %s cancelled", cmd.ID)
			return
		case <-timer.C:
		}
		log.Printf("executing %s (command %s)", cmd.Action, cmd.ID)
		if err := runShutdown(cmd.Action); err != nil {
			log.Printf("execute %s failed: %v", cmd.Action, err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":  true,
		"executeAt": executeAt.UTC().Format(time.RFC3339),
	})
}

func (e *executor) markSeen(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.seen[id] {
		return true
	}
	e.seen[id] = true
	e.order = append(e.order, id)
	if len(e.order) > 200 {
		delete(e.seen, e.order[0])
		e.order = e.order[1:]
	}
	return false
}

func (e *executor) cancelAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, cancel := range e.pending {
		cancel()
		delete(e.pending, id)
	}
}

func (e *executor) pendingCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.pending)
}

func heartbeatLoop(ctx context.Context, cfg *Config) {
	client := httpClient(cfg.Insecure, 10*time.Second)
	backoff := 15 * time.Second
	for {
		if err := sendHeartbeat(ctx, client, cfg); err != nil {
			log.Printf("heartbeat failed: %v", err)
			if backoff < 2*time.Minute {
				backoff *= 2
			}
		} else {
			backoff = 15 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func sendHeartbeat(ctx context.Context, client *http.Client, cfg *Config) error {
	hostname, _ := os.Hostname()
	payload, _ := json.Marshal(map[string]any{
		"hostId":    cfg.HostID,
		"version":   version.Version,
		"uptimeSec": int64(bootUptime().Seconds()),
		"hostname":  hostname,
		"ips":       localIPv4(),
	})
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		cfg.ServerURL+"/agent/v1/heartbeat", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func bootUptime() time.Duration {
	// Avoid platform-specific syscalls: process uptime is enough for diagnostics.
	return time.Since(processStart)
}

var processStart = time.Now()

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
