package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/agentapi"
	"github.com/wakecomputer/wakecomputer/internal/auth"
	"github.com/wakecomputer/wakecomputer/internal/config"
	"github.com/wakecomputer/wakecomputer/internal/id"
	"github.com/wakecomputer/wakecomputer/internal/job"
	"github.com/wakecomputer/wakecomputer/internal/sse"
	"github.com/wakecomputer/wakecomputer/internal/status"
	"github.com/wakecomputer/wakecomputer/internal/store"
	"github.com/wakecomputer/wakecomputer/internal/version"
)

type ctxKey string

const userIDKey ctxKey = "uid"

type Server struct {
	cfg    *config.Config
	st     *store.Store
	hub    *sse.Hub
	status *status.Service
	jobs   *job.Manager
	agent  *agentapi.Client
	log    *slog.Logger

	loginLimiter  *auth.Limiter
	globalLimiter *auth.Limiter
	discoveryRate *auth.Limiter
	guard         *auth.Guard
	tokenKey      []byte
	trusted       []*net.IPNet
	mux           *http.ServeMux
}

type Options struct {
	Config   *config.Config
	Store    *store.Store
	Hub      *sse.Hub
	Status   *status.Service
	Jobs     *job.Manager
	Agent    *agentapi.Client
	Logger   *slog.Logger
	TokenKey []byte
}

func New(opt Options) *Server {
	s := &Server{
		cfg:           opt.Config,
		st:            opt.Store,
		hub:           opt.Hub,
		status:        opt.Status,
		jobs:          opt.Jobs,
		agent:         opt.Agent,
		log:           opt.Logger,
		loginLimiter:  auth.NewLimiter(5, 5),
		globalLimiter: auth.NewLimiter(600, 120),
		discoveryRate: auth.NewLimiter(4, 2),
		guard:         auth.NewGuard(5, 15*time.Minute),
		tokenKey:      opt.TokenKey,
		trusted:       parseTrusted(opt.Config.TrustedProxies),
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = s.withRecover(h)
	h = s.withRequestID(h)
	h = s.withSecurityHeaders(h)
	h = s.withAccessLog(h)
	return h
}

func (s *Server) routes() {
	m := s.mux

	// Friendly landing response so a browser hit on / is not a bare 404.
	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "WakeComputer",
			"version": version.Version,
			"health":  "/healthz",
			"api":     "/api/v1",
		})
	})

	m.HandleFunc("GET /healthz", s.handleHealth)

	m.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/v1/auth/refresh", s.handleRefresh)
	m.HandleFunc("POST /api/v1/auth/logout", s.auth(s.handleLogout))
	m.HandleFunc("GET /api/v1/auth/me", s.auth(s.handleMe))
	m.HandleFunc("POST /api/v1/auth/password", s.auth(s.handleChangePassword))

	m.HandleFunc("GET /api/v1/hosts", s.auth(s.handleListHosts))
	m.HandleFunc("GET /api/v1/discovery", s.auth(s.handleDiscovery))
	m.HandleFunc("POST /api/v1/hosts", s.auth(s.handleCreateHost))
	m.HandleFunc("GET /api/v1/hosts/{id}", s.auth(s.handleGetHost))
	m.HandleFunc("PATCH /api/v1/hosts/{id}", s.auth(s.handleUpdateHost))
	m.HandleFunc("DELETE /api/v1/hosts/{id}", s.auth(s.handleDeleteHost))
	m.HandleFunc("POST /api/v1/hosts/{id}/wake", s.auth(s.handleWakeHost))
	m.HandleFunc("POST /api/v1/hosts/{id}/power", s.auth(s.handlePowerHost))
	m.HandleFunc("POST /api/v1/hosts/{id}/probe", s.auth(s.handleProbeHost))
	m.HandleFunc("POST /api/v1/hosts/{id}/agent/enroll-code", s.auth(s.handleEnrollCode))
	m.HandleFunc("DELETE /api/v1/hosts/{id}/agent", s.auth(s.handleRemoveAgent))

	m.HandleFunc("GET /api/v1/groups", s.auth(s.handleListGroups))
	m.HandleFunc("POST /api/v1/groups", s.auth(s.handleCreateGroup))
	m.HandleFunc("PATCH /api/v1/groups/{id}", s.auth(s.handleUpdateGroup))
	m.HandleFunc("DELETE /api/v1/groups/{id}", s.auth(s.handleDeleteGroup))

	m.HandleFunc("POST /api/v1/batch", s.auth(s.handleBatch))
	m.HandleFunc("GET /api/v1/jobs/{id}", s.auth(s.handleGetJob))
	m.HandleFunc("POST /api/v1/jobs/{id}/cancel", s.auth(s.handleCancelJob))

	m.HandleFunc("GET /api/v1/audit", s.auth(s.handleAudit))
	m.HandleFunc("GET /api/v1/settings", s.auth(s.handleGetSettings))
	m.HandleFunc("PUT /api/v1/settings", s.auth(s.handlePutSettings))
	m.HandleFunc("GET /api/v1/events", s.auth(s.handleEvents))

	m.HandleFunc("POST /agent/v1/enroll", s.handleAgentEnroll)
	m.HandleFunc("POST /agent/v1/heartbeat", s.handleAgentHeartbeat)

	m.HandleFunc("GET /install.sh", s.handleInstallSh)
	m.HandleFunc("GET /install.ps1", s.handleInstallPs1)
	m.HandleFunc("GET /download/{name}", s.handleDownload)
}

// ---------- middleware ----------

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "缺少访问令牌")
			return
		}
		uid, err := auth.VerifyJWT(s.cfg.JWTSecret, strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "访问令牌无效或已过期")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, uid)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) allow(r *http.Request) bool {
	ip := s.clientIP(r)
	return s.globalLimiter.Allow(ip)
}

func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "err", fmt.Sprint(rec), "path", r.URL.Path)
				writeErr(w, http.StatusInternalServerError, "INTERNAL", "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := id.New()
		w.Header().Set("X-Request-Id", rid)
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), ctxKey("rid"), rid)))
		s.log.Debug("http",
			"id", rid,
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"ms", time.Since(start).Milliseconds(),
			"ip", s.clientIP(r))
	})
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.globalLimiter.Allow(s.clientIP(r)) {
			writeErr(w, http.StatusTooManyRequests, "RATE_LIMITED", "请求过于频繁")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------- helpers ----------

func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	for _, n := range s.trusted {
		if n.Contains(ip) {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				first := strings.TrimSpace(strings.Split(xff, ",")[0])
				if parsed := net.ParseIP(first); parsed != nil {
					return parsed.String()
				}
			}
			break
		}
	}
	return ip.String()
}

func parseTrusted(list []string) []*net.IPNet {
	var out []*net.IPNet
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !strings.Contains(item, "/") {
			if ip := net.ParseIP(item); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			}
			continue
		}
		if _, n, err := net.ParseCIDR(item); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, "INVALID_JSON", "请求体为空")
			return false
		}
		writeErr(w, http.StatusBadRequest, "INVALID_JSON", "请求体格式错误")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]string{"code": errCode, "message": msg},
	})
}

func userID(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}

func (s *Server) audit(r *http.Request, action, hostID, result, detail string) {
	uid := userID(r)
	if uid == "" {
		uid = ""
	}
	_ = s.st.InsertAudit(&store.Audit{
		UserID:    uid,
		Action:    action,
		HostID:    hostID,
		Result:    result,
		Detail:    detail,
		IP:        s.clientIP(r),
		UserAgent: r.UserAgent(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbOK := "ok"
	if err := s.st.DB().PingContext(r.Context()); err != nil {
		dbOK = "error"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"db":      dbOK,
		"version": version.Version,
		"commit":  version.Commit,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}
