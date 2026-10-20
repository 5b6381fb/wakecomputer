package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/auth"
	"github.com/wakecomputer/wakecomputer/internal/secretbox"
	"github.com/wakecomputer/wakecomputer/internal/store"
)

type enrollReq struct {
	Code        string   `json:"code"`
	Hostname    string   `json:"hostname"`
	OS          string   `json:"os"`
	Arch        string   `json:"arch"`
	Version     string   `json:"version"`
	CertPEM     string   `json:"certPem"`
	Fingerprint string   `json:"fingerprint"`
	IPs         []string `json:"ips"`
}

func (s *Server) handleAgentEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.enrollFromLAN(r) {
		writeErr(w, http.StatusForbidden, "LAN_ONLY", "只能在局域网内绑定 Agent")
		return
	}
	var req enrollReq
	if !s.decode(w, r, &req) {
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if len(req.Code) != 8 {
		writeErr(w, http.StatusBadRequest, "INVALID_CODE", "接入码格式错误")
		return
	}
	block, _ := pem.Decode([]byte(req.CertPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		writeErr(w, http.StatusBadRequest, "INVALID_CERT", "Agent 证书无效")
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_CERT", "Agent 证书无法解析")
		return
	}
	sum := sha256.Sum256(cert.Raw)
	actual := hex.EncodeToString(sum[:])
	claimed := normalizeFP(req.Fingerprint)
	if claimed == "" || subtle.ConstantTimeCompare([]byte(actual), []byte(claimed)) != 1 {
		writeErr(w, http.StatusBadRequest, "CERT_MISMATCH", "证书指纹不匹配")
		return
	}

	codeHash := auth.HashToken(req.Code)
	hostID, err := s.st.ConsumeEnrollment(codeHash)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_CODE", "接入码无效或已过期")
		return
	}
	host, err := s.st.GetHost(hostID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}

	ip := chooseAgentIP(req.IPs, remoteIP(r), s.cfg.HTTPAddr)
	agentURL := "https://" + net.JoinHostPort(ip, "45871")
	token, err := auth.NewToken(32)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "生成 Agent 令牌失败")
		return
	}
	enc, err := secretbox.Seal(s.tokenKey, token)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "保存 Agent 令牌失败")
		return
	}
	if err := s.st.SetHostAgent(host.ID, agentURL, enc, actual); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "保存 Agent 信息失败")
		return
	}
	_ = s.st.SetHostIPIfEmpty(host.ID, ip)
	_ = s.st.InsertAudit(&store.Audit{
		Action: "agent.enroll", HostID: host.ID, Result: "ok",
		Detail: strings.Join([]string{req.Hostname, req.OS, req.Arch, req.Version}, "/"),
		IP:     s.clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"hostId":     host.ID,
		"token":      token,
		"serverTime": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "缺少 Agent 令牌")
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	var req struct {
		HostID  string   `json:"hostId"`
		Version string   `json:"version"`
		Uptime  int64    `json:"uptimeSec"`
		IPs     []string `json:"ips"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	if req.HostID == "" {
		writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "缺少 hostId")
		return
	}
	host, err := s.st.GetHost(req.HostID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "主机不存在")
		return
	}
	if host.AgentTokenEnc == "" {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "Agent 未绑定")
		return
	}
	want, err := secretbox.Open(s.tokenKey, host.AgentTokenEnc)
	if err != nil || subtle.ConstantTimeCompare([]byte(want), []byte(token)) != 1 {
		_ = s.st.InsertAudit(&store.Audit{
			Action: "agent.heartbeat_auth", HostID: host.ID, Result: "failed",
			IP: s.clientIP(r), UserAgent: r.UserAgent(),
		})
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "Agent 令牌无效")
		return
	}
	ip := chooseAgentIP(req.IPs, remoteIP(r), s.cfg.HTTPAddr)
	agentURL := ""
	if ip != "" && isLocalNetwork(ip) {
		agentURL = "https://" + net.JoinHostPort(ip, "45871")
	}
	if err := s.st.UpdateHeartbeat(host.ID, req.Version, ip, agentURL); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "更新心跳失败")
		return
	}
	s.status.Heartbeat(host.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"serverTime": time.Now().UTC().Format(time.RFC3339),
	})
}

// ---------- static downloads / installers ----------

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	if name == "" || name == "." || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.DataDir, "downloads", name)
	http.ServeFile(w, r, path)
}

func (s *Server) handleInstallSh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(installSh))
}

func (s *Server) handleInstallPs1(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(installPs1))
}

func normalizeFP(fp string) string {
	fp = strings.ToLower(strings.TrimSpace(fp))
	fp = strings.TrimPrefix(fp, "sha256:")
	fp = strings.ReplaceAll(fp, ":", "")
	return fp
}

// enrollFromLAN allows a direct private/loopback connection. A trusted reverse
// proxy must also present a private client in X-Forwarded-For, so the public
// hostname cannot be used to bind an agent.
func (s *Server) enrollFromLAN(r *http.Request) bool {
	peer := remoteIP(r)
	if !isLocalNetwork(peer) {
		return false
	}
	if !s.peerTrusted(peer) {
		return true
	}
	xff := firstForwarded(r)
	if xff == "" {
		return true
	}
	return isLocalNetwork(xff)
}

func (s *Server) peerTrusted(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range s.trusted {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func firstForwarded(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(xff, ",")[0])
}

func isLocalNetwork(raw string) bool {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// chooseAgentIP prefers an agent-reported IPv4 in the same /24 as the server,
// then any private IPv4. When enrollment goes through the HTTPS reverse proxy,
// RemoteAddr is the proxy address and must not be used as the agent endpoint.
func chooseAgentIP(reported []string, fallback, serverAddr string) string {
	var serverIP net.IP
	if host, _, err := net.SplitHostPort(serverAddr); err == nil {
		serverIP = net.ParseIP(host)
	}
	var firstPrivate, firstAny string
	for _, raw := range reported {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		if firstAny == "" {
			firstAny = v4.String()
		}
		if s4 := serverIP.To4(); s4 != nil && s4[0] == v4[0] && s4[1] == v4[1] && s4[2] == v4[2] {
			return v4.String()
		}
		if firstPrivate == "" && v4.IsPrivate() {
			firstPrivate = v4.String()
		}
	}
	if firstPrivate != "" {
		return firstPrivate
	}
	if firstAny != "" {
		return firstAny
	}
	return fallback
}
