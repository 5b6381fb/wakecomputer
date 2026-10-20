package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/agentapi"
	"github.com/wakecomputer/wakecomputer/internal/id"
	"github.com/wakecomputer/wakecomputer/internal/store"
	"github.com/wakecomputer/wakecomputer/internal/wol"
)

type hostPayload struct {
	Name             *string   `json:"name"`
	OSType           *string   `json:"osType"`
	MAC              *string   `json:"mac"`
	IP               *string   `json:"ip"`
	BroadcastTargets *[]string `json:"broadcastTargets"`
	WolPort          *int      `json:"wolPort"`
	ProbePort        *int      `json:"probePort"`
	Notes            *string   `json:"notes"`
	SortOrder        *int      `json:"sortOrder"`
	GroupIDs         *[]string `json:"groupIds"`
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.st.ListHosts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "读取主机失败")
		return
	}
	s.status.Apply(hosts)
	if hosts == nil {
		hosts = []*store.Host{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      hosts,
		"serverTime": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleGetHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	s.status.Apply([]*store.Host{h})
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	var p hostPayload
	if !s.decode(w, r, &p) {
		return
	}
	h := &store.Host{ID: id.New(), OSType: "other", WolPort: s.cfg.WolPort, LastStatus: "unknown"}
	if err := s.applyPayload(h, &p, true); err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if len(h.BroadcastTargets) == 0 && s.cfg.WolBroadcast != "" {
		h.BroadcastTargets = []string{s.cfg.WolBroadcast}
	}
	if err := s.st.CreateHost(h); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "NAME_EXISTS", "主机名称已存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "创建主机失败")
		return
	}
	if p.GroupIDs != nil {
		if err := s.validateGroupIDs(*p.GroupIDs); err != nil {
			writeErr(w, http.StatusBadRequest, "INVALID_GROUP", err.Error())
			return
		}
		_ = s.st.SetHostGroups(h.ID, *p.GroupIDs)
	}
	s.status.Apply([]*store.Host{h})
	s.audit(r, "host.create", h.ID, "ok", h.Name)
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	var p hostPayload
	if !s.decode(w, r, &p) {
		return
	}
	if err := s.applyPayload(h, &p, false); err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if err := s.st.UpdateHost(h); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "NAME_EXISTS", "主机名称已存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "更新主机失败")
		return
	}
	if p.GroupIDs != nil {
		if err := s.validateGroupIDs(*p.GroupIDs); err != nil {
			writeErr(w, http.StatusBadRequest, "INVALID_GROUP", err.Error())
			return
		}
		_ = s.st.SetHostGroups(h.ID, *p.GroupIDs)
	}
	h, err = s.st.GetHost(h.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "读取主机失败")
		return
	}
	s.status.Apply([]*store.Host{h})
	s.audit(r, "host.update", h.ID, "ok", h.Name)
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	idv := r.PathValue("id")
	if _, err := s.st.GetHost(idv); err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	if err := s.st.DeleteHost(idv); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "删除主机失败")
		return
	}
	s.audit(r, "host.delete", idv, "ok", "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWakeHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	mac, err := wol.ParseMAC(h.MAC)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_MAC", err.Error())
		return
	}
	targets := h.BroadcastTargets
	if len(targets) == 0 && s.cfg.WolBroadcast != "" {
		targets = []string{s.cfg.WolBroadcast}
	}
	port := h.WolPort
	if port == 0 {
		port = s.cfg.WolPort
	}
	results, err := wol.Send(mac, targets, port)
	if err != nil {
		s.audit(r, "host.wake", h.ID, "failed", err.Error())
		writeErr(w, http.StatusBadGateway, "WOL_FAILED", "魔术包发送失败："+err.Error())
		return
	}
	okCount := 0
	for _, x := range results {
		if x.OK {
			okCount++
		}
	}
	if okCount == 0 {
		s.audit(r, "host.wake", h.ID, "failed", "all targets failed")
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   map[string]string{"code": "WOL_FAILED", "message": "所有广播目标发送失败"},
			"targets": results,
		})
		return
	}
	s.status.MarkWaking(h.ID, 120*time.Second)
	s.audit(r, "host.wake", h.ID, "ok", fmt.Sprintf("%d/%d targets", okCount, len(results)))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":   true,
		"targets":    results,
		"status":     "waking",
		"etaSeconds": 90,
	})
}

func (s *Server) handlePowerHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	var req struct {
		Action       string `json:"action"`
		DelaySeconds int    `json:"delaySeconds"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	switch req.Action {
	case "shutdown", "reboot", "cancel":
	default:
		writeErr(w, http.StatusBadRequest, "INVALID_ACTION", "action 必须是 shutdown / reboot / cancel")
		return
	}
	if h.AgentURL == "" {
		writeErr(w, http.StatusBadRequest, "AGENT_REQUIRED", "该主机未安装 Agent，无法远程关机或重启")
		return
	}
	if req.Action != "cancel" {
		if req.DelaySeconds <= 0 {
			req.DelaySeconds = 30
		}
		if req.DelaySeconds > 300 {
			req.DelaySeconds = 300
		}
	} else {
		req.DelaySeconds = 0
	}

	cmd := agentapi.Command{
		ID:           id.New(),
		Action:       req.Action,
		DelaySeconds: req.DelaySeconds,
		Reason:       "WakeComputer remote " + req.Action,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := s.agent.SendCommand(ctx, h, cmd); err != nil {
		s.audit(r, "host."+req.Action, h.ID, "failed", err.Error())
		writeErr(w, http.StatusBadGateway, "AGENT_FAILED", "指令下发失败："+err.Error())
		return
	}
	s.audit(r, "host."+req.Action, h.ID, "ok", fmt.Sprintf("delay=%ds", req.DelaySeconds))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":     true,
		"executeAt":    time.Now().Add(time.Duration(req.DelaySeconds) * time.Second).UTC().Format(time.RFC3339),
		"delaySeconds": req.DelaySeconds,
	})
}

func (s *Server) handleProbeHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	st := s.status.Probe(r.Context(), h)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":     h.ID,
		"status": st,
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleEnrollCode(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	code, err := generateEnrollCode()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "生成接入码失败")
		return
	}
	sum := sha256.Sum256([]byte(code))
	expires := time.Now().Add(10 * time.Minute)
	if err := s.st.CreateEnrollment(hex.EncodeToString(sum[:]), h.ID, expires.UnixMilli()); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "保存接入码失败")
		return
	}
	base := s.cfg.PublicURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	linuxCmd := fmt.Sprintf("curl -fsSL %s/install.sh | sudo bash -s -- --server %s --code %s", base, base, code)
	winCmd := fmt.Sprintf(`powershell -NoProfile -ExecutionPolicy Bypass -c "irm %s/install.ps1 -OutFile $env:TEMP\wake-agent-install.ps1; & $env:TEMP\wake-agent-install.ps1 -Server %s -Code %s"`, base, base, code)

	s.audit(r, "agent.enroll_code", h.ID, "ok", "")
	writeJSON(w, http.StatusOK, map[string]any{
		"code":           code,
		"expiresAt":      expires.UTC().Format(time.RFC3339),
		"linuxCommand":   linuxCmd,
		"windowsCommand": winCmd,
		"agentPort":      45871,
	})
}

func (s *Server) handleRemoveAgent(w http.ResponseWriter, r *http.Request) {
	h, err := s.st.GetHost(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "HOST_NOT_FOUND", "主机不存在")
		return
	}
	if err := s.st.ClearHostAgent(h.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "解绑失败")
		return
	}
	s.audit(r, "agent.remove", h.ID, "ok", "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------- validation ----------

func (s *Server) applyPayload(h *store.Host, p *hostPayload, creating bool) error {
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" || len([]rune(name)) > 64 {
			return fmt.Errorf("主机名称长度需为 1-64 个字符")
		}
		h.Name = name
	} else if creating {
		return fmt.Errorf("缺少主机名称")
	}
	if p.OSType != nil {
		switch *p.OSType {
		case "windows", "linux", "other":
			h.OSType = *p.OSType
		default:
			return fmt.Errorf("osType 必须是 windows / linux / other")
		}
	}
	if p.MAC != nil {
		mac, err := wol.ParseMAC(*p.MAC)
		if err != nil {
			return err
		}
		h.MAC = mac.String()
	} else if creating {
		return fmt.Errorf("缺少 MAC 地址")
	}
	if p.IP != nil {
		ip := strings.TrimSpace(*p.IP)
		if ip != "" && net.ParseIP(ip) == nil {
			return fmt.Errorf("IP 地址格式错误")
		}
		h.IP = ip
	}
	if p.BroadcastTargets != nil {
		seen := map[string]bool{}
		targets := make([]string, 0, len(*p.BroadcastTargets))
		for _, t := range *p.BroadcastTargets {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if len(t) > 64 || strings.ContainsAny(t, " \t\r\n") {
				return fmt.Errorf("广播地址格式错误：%s", t)
			}
			if !seen[t] {
				seen[t] = true
				targets = append(targets, t)
			}
		}
		h.BroadcastTargets = targets
	}
	if p.WolPort != nil {
		if *p.WolPort < 1 || *p.WolPort > 65535 {
			return fmt.Errorf("WoL 端口需在 1-65535 之间")
		}
		h.WolPort = *p.WolPort
	}
	if p.ProbePort != nil {
		if *p.ProbePort < 0 || *p.ProbePort > 65535 {
			return fmt.Errorf("探测端口需在 0-65535 之间")
		}
		h.ProbePort = *p.ProbePort
	}
	if p.Notes != nil {
		if len([]rune(*p.Notes)) > 500 {
			return fmt.Errorf("备注不能超过 500 字")
		}
		h.Notes = *p.Notes
	}
	if p.SortOrder != nil {
		h.SortOrder = *p.SortOrder
	}
	if h.WolPort == 0 {
		h.WolPort = s.cfg.WolPort
	}
	return nil
}

func (s *Server) validateGroupIDs(ids []string) error {
	groups, err := s.st.ListGroups()
	if err != nil {
		return fmt.Errorf("读取分组失败")
	}
	valid := map[string]bool{}
	for _, g := range groups {
		valid[g.ID] = true
	}
	for _, idv := range ids {
		if !valid[idv] {
			return fmt.Errorf("分组不存在：%s", idv)
		}
	}
	return nil
}

func generateEnrollCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}
