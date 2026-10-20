package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/auth"
	"github.com/wakecomputer/wakecomputer/internal/id"
	"github.com/wakecomputer/wakecomputer/internal/store"
	"github.com/wakecomputer/wakecomputer/internal/version"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type tokenResp struct {
	AccessToken  string  `json:"accessToken"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresIn    int64   `json:"expiresIn"`
	User         userDTO `json:"user"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if locked, until := s.guard.Check("login:" + ip); locked {
		w.Header().Set("Retry-After", auth.RetryAfterSeconds(until))
		writeErr(w, http.StatusTooManyRequests, "LOCKED", "登录失败次数过多，请稍后再试")
		return
	}
	if !s.loginLimiter.Allow(ip) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "RATE_LIMITED", "登录过于频繁，请稍后再试")
		return
	}
	var req loginReq
	if !s.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Username) == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "请输入用户名和密码")
		return
	}

	u, err := s.st.GetUserByUsername(strings.TrimSpace(req.Username))
	if err != nil || !auth.VerifyPassword(req.Password, u.PasswordHash) {
		if err != nil {
			// Keep timing close to a real verification.
			auth.VerifyPassword(req.Password, auth.DummyHash)
		}
		s.guard.Fail("login:" + ip)
		_ = s.st.InsertAudit(&store.Audit{
			Action: "auth.login_failed", Result: "failed",
			Detail: "username=" + req.Username, IP: ip, UserAgent: r.UserAgent(),
		})
		writeErr(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "用户名或密码错误")
		return
	}
	s.guard.Reset("login:" + ip)
	s.issueTokens(w, r, u)
	_ = s.st.InsertAudit(&store.Audit{
		UserID: u.ID, Action: "auth.login", Result: "ok",
		IP: ip, UserAgent: r.UserAgent(),
	})
}

func (s *Server) issueTokens(w http.ResponseWriter, r *http.Request, u *store.User) {
	refresh, err := auth.NewToken(32)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "生成令牌失败")
		return
	}
	now := time.Now()
	sess := &store.Session{
		ID:          id.New(),
		UserID:      u.ID,
		RefreshHash: auth.HashToken(refresh),
		DeviceName:  deviceName(r),
		UserAgent:   r.UserAgent(),
		IP:          s.clientIP(r),
		CreatedAt:   now.UnixMilli(),
		LastUsedAt:  now.UnixMilli(),
		ExpiresAt:   now.Add(s.cfg.RefreshTTL).UnixMilli(),
	}
	if err := s.st.CreateSession(sess); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "创建会话失败")
		return
	}
	access, exp, err := auth.SignJWT(s.cfg.JWTSecret, u.ID, s.cfg.AccessTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "生成令牌失败")
		return
	}
	writeJSON(w, http.StatusOK, tokenResp{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    exp - now.Unix(),
		User:         userDTO{ID: u.ID, Username: u.Username},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "缺少刷新令牌")
		return
	}
	hash := auth.HashToken(req.RefreshToken)
	sess, err := s.st.GetSessionByHash(hash)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "INVALID_REFRESH", "刷新令牌无效")
		return
	}
	if sess.RevokedAt != nil {
		// Token reuse: assume compromise and revoke every session of this user.
		_ = s.st.RevokeUserSessions(sess.UserID, "")
		_ = s.st.InsertAudit(&store.Audit{
			Action: "auth.refresh_reuse", Result: "blocked", UserID: sess.UserID,
			IP: s.clientIP(r), UserAgent: r.UserAgent(),
		})
		writeErr(w, http.StatusUnauthorized, "INVALID_REFRESH", "刷新令牌已失效，请重新登录")
		return
	}
	if time.Now().UnixMilli() > sess.ExpiresAt {
		writeErr(w, http.StatusUnauthorized, "INVALID_REFRESH", "刷新令牌已过期，请重新登录")
		return
	}
	u, err := s.st.GetUserByID(sess.UserID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "INVALID_REFRESH", "账号不存在")
		return
	}
	_ = s.st.RevokeSession(sess.ID)

	refresh, err := auth.NewToken(32)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "生成令牌失败")
		return
	}
	now := time.Now()
	ns := &store.Session{
		ID:          id.New(),
		UserID:      u.ID,
		RefreshHash: auth.HashToken(refresh),
		DeviceName:  sess.DeviceName,
		UserAgent:   r.UserAgent(),
		IP:          s.clientIP(r),
		CreatedAt:   now.UnixMilli(),
		LastUsedAt:  now.UnixMilli(),
		ExpiresAt:   now.Add(s.cfg.RefreshTTL).UnixMilli(),
	}
	if err := s.st.CreateSession(ns); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "创建会话失败")
		return
	}
	access, exp, err := auth.SignJWT(s.cfg.JWTSecret, u.ID, s.cfg.AccessTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "生成令牌失败")
		return
	}
	writeJSON(w, http.StatusOK, tokenResp{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    exp - now.Unix(),
		User:         userDTO{ID: u.ID, Username: u.Username},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = s.decode(w, r, &req)
	if req.RefreshToken != "" {
		if sess, err := s.st.GetSessionByHash(auth.HashToken(req.RefreshToken)); err == nil {
			_ = s.st.RevokeSession(sess.ID)
		}
	}
	s.audit(r, "auth.logout", "", "ok", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := s.st.GetUserByID(userID(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "账号不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":    userDTO{ID: u.ID, Username: u.Username},
		"version": version.Version,
		"server": map[string]any{
			"publicUrl": s.cfg.PublicURL,
			"time":      time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "WEAK_PASSWORD", "新密码至少 8 位")
		return
	}
	u, err := s.st.GetUserByID(userID(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "账号不存在")
		return
	}
	if !auth.VerifyPassword(req.OldPassword, u.PasswordHash) {
		writeErr(w, http.StatusBadRequest, "INVALID_PASSWORD", "原密码错误")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "密码处理失败")
		return
	}
	if err := s.st.UpdatePassword(u.ID, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "更新密码失败")
		return
	}
	_ = s.st.RevokeUserSessions(u.ID, "")
	s.audit(r, "auth.password", "", "ok", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func deviceName(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Device-Name")); v != "" {
		return truncateStr(v, 64)
	}
	ua := r.UserAgent()
	if ua == "" {
		return "unknown"
	}
	return truncateStr(ua, 64)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
