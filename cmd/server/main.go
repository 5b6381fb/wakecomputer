package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/wakecomputer/wakecomputer/internal/agentapi"
	"github.com/wakecomputer/wakecomputer/internal/auth"
	"github.com/wakecomputer/wakecomputer/internal/config"
	"github.com/wakecomputer/wakecomputer/internal/discovery"
	"github.com/wakecomputer/wakecomputer/internal/httpapi"
	"github.com/wakecomputer/wakecomputer/internal/job"
	"github.com/wakecomputer/wakecomputer/internal/secretbox"
	"github.com/wakecomputer/wakecomputer/internal/sse"
	"github.com/wakecomputer/wakecomputer/internal/status"
	"github.com/wakecomputer/wakecomputer/internal/store"
	"github.com/wakecomputer/wakecomputer/internal/version"
)

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
	case "admin":
		if err := adminCmd(args); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
	case "backup":
		if err := backupCmd(args); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
	case "healthcheck":
		if err := healthcheckCmd(); err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck failed:", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("wakecomputer %s (%s, %s)\n", version.Version, version.Commit, version.BuildTime)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(cfg.LogFormat, "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

func openStore(cfg *config.Config) (*store.Store, error) {
	return store.Open(filepath.Join(cfg.DataDir, "wake.db"))
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	st, err := openStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := ensureAdmin(cfg, st, log); err != nil {
		return err
	}
	if cfg.LANSubnet == "" {
		cfg.LANSubnet = discovery.DefaultSubnet(cfg.WolBroadcast)
	}

	tokenKey := secretbox.Key(cfg.JWTSecret, "agent-token")
	hub := sse.NewHub()
	statusSvc := status.New(st, hub, log)
	agentClient := agentapi.New(tokenKey, false, log)
	jobMgr := job.New(st, hub, agentClient, statusSvc, log, cfg.WolBroadcast, cfg.WolPort, 4)
	applyRuntimeSettings(st, jobMgr, cfg)

	api := httpapi.New(httpapi.Options{
		Config:   cfg,
		Store:    st,
		Hub:      hub,
		Status:   statusSvc,
		Jobs:     jobMgr,
		Agent:    agentClient,
		Logger:   log,
		TokenKey: tokenKey,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	statusSvc.Start(ctx)
	go cleanupLoop(ctx, st, log)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("wakecomputer listening",
			"addr", cfg.HTTPAddr, "version", version.Version,
			"publicUrl", cfg.PublicURL, "dataDir", cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("graceful shutdown failed", "err", err)
	}
	return nil
}

func ensureAdmin(cfg *config.Config, st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	password := cfg.AdminPassword
	generated := false
	if password == "" {
		password, err = auth.NewToken(12)
		if err != nil {
			return err
		}
		generated = true
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(cfg.AdminUser, hash); err != nil {
		return err
	}
	log.Info("initial administrator created", "username", cfg.AdminUser)
	if generated {
		fmt.Printf("\n==================================================\n")
		fmt.Printf(" WakeComputer initial admin credentials\n")
		fmt.Printf("   username: %s\n", cfg.AdminUser)
		fmt.Printf("   password: %s\n", password)
		fmt.Printf(" Please store it safely and change it after login.\n")
		fmt.Printf("==================================================\n\n")
	}
	return nil
}

func applyRuntimeSettings(st *store.Store, jobs *job.Manager, cfg *config.Config) {
	broadcast := cfg.WolBroadcast
	concurrency := 4
	if raw, err := st.GetSetting("settings", ""); err == nil && raw != "" {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			if v, ok := m["wolDefaultBroadcast"].(string); ok && v != "" {
				broadcast = v
			}
			if v, ok := m["batchConcurrency"].(float64); ok {
				concurrency = int(v)
			}
		}
	}
	jobs.SetDefaults(broadcast, concurrency)
}

func cleanupLoop(ctx context.Context, st *store.Store, log *slog.Logger) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			retention := 180
			if raw, err := st.GetSetting("settings", ""); err == nil && raw != "" {
				var m map[string]any
				if json.Unmarshal([]byte(raw), &m) == nil {
					if v, ok := m["auditRetentionDays"].(float64); ok && v >= 7 {
						retention = int(v)
					}
				}
			}
			cutoff := time.Now().AddDate(0, 0, -retention).UnixMilli()
			if n, err := st.PurgeAudit(cutoff); err != nil {
				log.Warn("audit cleanup failed", "err", err)
			} else if n > 0 {
				log.Info("audit cleanup", "removed", n, "retentionDays", retention)
			}
			st.CleanupExpired(time.Now().Add(-24 * time.Hour).UnixMilli())
		}
	}
}

func adminCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wakecomputer admin create|reset-password [-username admin] [-password xxx]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("admin "+sub, flag.ExitOnError)
	username := fs.String("username", "admin", "administrator username")
	password := fs.String("password", "", "password (generated when empty)")
	_ = fs.Parse(args[1:])

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := openStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	pw := *password
	if pw == "" {
		pw, err = auth.NewToken(12)
		if err != nil {
			return err
		}
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}

	switch sub {
	case "create":
		if _, err := st.GetUserByUsername(*username); err == nil {
			return fmt.Errorf("user %q already exists", *username)
		}
		if _, err := st.CreateUser(*username, hash); err != nil {
			return err
		}
		fmt.Printf("created user %s\npassword: %s\n", *username, pw)
	case "reset-password":
		u, err := st.GetUserByUsername(*username)
		if err != nil {
			return fmt.Errorf("user %q not found", *username)
		}
		if err := st.UpdatePassword(u.ID, hash); err != nil {
			return err
		}
		if err := st.RevokeUserSessions(u.ID, ""); err != nil {
			return err
		}
		fmt.Printf("password updated for %s\npassword: %s\n", *username, pw)
	default:
		return fmt.Errorf("unknown admin subcommand %q", sub)
	}
	return nil
}

func backupCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: wakecomputer backup <destination.db>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := openStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	dest := args[0]
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := st.Backup(dest); err != nil {
		return err
	}
	fi, _ := os.Stat(dest)
	fmt.Printf("backup written to %s (%d bytes)\n", dest, fi.Size())
	return nil
}

func healthcheckCmd() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return err
	}
	urls := []string{"http://127.0.0.1:" + port + "/healthz"}
	if host, _, err := net.SplitHostPort(cfg.HTTPAddr); err == nil && host != "" && host != "0.0.0.0" && host != "::" {
		urls = append(urls, "http://"+net.JoinHostPort(host, port)+"/healthz")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			lastErr = err
			continue
		}
		var body struct {
			Status string `json:"status"`
			DB     string `json:"db"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && body.DB == "ok" {
			return nil
		}
		lastErr = fmt.Errorf("%s returned %d (db=%s)", u, resp.StatusCode, body.DB)
	}
	return lastErr
}
