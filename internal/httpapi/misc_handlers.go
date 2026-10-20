package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/store"
)

// ---------- groups ----------

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.st.ListGroups()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "读取分组失败")
		return
	}
	if groups == nil {
		groups = []store.Group{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": groups})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sortOrder"`
	}
	if !s.decode(w, r, &p) {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len([]rune(p.Name)) > 32 {
		writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "分组名称长度需为 1-32 个字符")
		return
	}
	g, err := s.st.CreateGroup(p.Name, p.SortOrder)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "NAME_EXISTS", "分组名称已存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "创建分组失败")
		return
	}
	s.audit(r, "group.create", "", "ok", g.Name)
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	idv := r.PathValue("id")
	var p struct {
		Name      *string `json:"name"`
		SortOrder *int    `json:"sortOrder"`
	}
	if !s.decode(w, r, &p) {
		return
	}
	groups, err := s.st.ListGroups()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "读取分组失败")
		return
	}
	var current *store.Group
	for i := range groups {
		if groups[i].ID == idv {
			current = &groups[i]
			break
		}
	}
	if current == nil {
		writeErr(w, http.StatusNotFound, "GROUP_NOT_FOUND", "分组不存在")
		return
	}
	name := current.Name
	order := current.SortOrder
	if p.Name != nil {
		name = strings.TrimSpace(*p.Name)
		if name == "" || len([]rune(name)) > 32 {
			writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "分组名称长度需为 1-32 个字符")
			return
		}
	}
	if p.SortOrder != nil {
		order = *p.SortOrder
	}
	if err := s.st.UpdateGroup(idv, name, order); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "NAME_EXISTS", "分组名称已存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "更新分组失败")
		return
	}
	s.audit(r, "group.update", "", "ok", name)
	writeJSON(w, http.StatusOK, store.Group{ID: idv, Name: name, SortOrder: order})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteGroup(r.PathValue("id")); err != nil {
		if err == store.ErrNotFound {
			writeErr(w, http.StatusNotFound, "GROUP_NOT_FOUND", "分组不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "删除分组失败")
		return
	}
	s.audit(r, "group.delete", "", "ok", r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// ---------- audit ----------

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AuditFilter{
		Limit:  atoiDefault(q.Get("limit"), 50),
		Cursor: parseInt64(q.Get("cursor")),
		HostID: q.Get("hostId"),
		Action: q.Get("action"),
		Result: q.Get("result"),
		From:   parseTime(q.Get("from")),
		To:     parseTime(q.Get("to")),
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	items, err := s.st.ListAudit(f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "读取审计日志失败")
		return
	}
	if items == nil {
		items = []store.Audit{}
	}
	var next any
	if len(items) == f.Limit && f.Limit > 0 {
		next = items[len(items)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseTime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// ---------- settings ----------

func (s *Server) settingsDefaults() map[string]any {
	return map[string]any{
		"wolDefaultBroadcast": s.cfg.WolBroadcast,
		"batchConcurrency":    4,
		"auditRetentionDays":  180,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	out := s.settingsDefaults()
	if raw, err := s.st.GetSetting("settings", ""); err == nil && raw != "" {
		var stored map[string]any
		if json.Unmarshal([]byte(raw), &stored) == nil {
			for k, v := range stored {
				out[k] = v
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var p struct {
		WolDefaultBroadcast *string `json:"wolDefaultBroadcast"`
		BatchConcurrency    *int    `json:"batchConcurrency"`
		AuditRetentionDays  *int    `json:"auditRetentionDays"`
	}
	if !s.decode(w, r, &p) {
		return
	}
	out := s.settingsDefaults()
	if raw, err := s.st.GetSetting("settings", ""); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	if p.WolDefaultBroadcast != nil {
		v := strings.TrimSpace(*p.WolDefaultBroadcast)
		if v == "" {
			writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "默认广播地址不能为空")
			return
		}
		out["wolDefaultBroadcast"] = v
	}
	if p.BatchConcurrency != nil {
		if *p.BatchConcurrency < 1 || *p.BatchConcurrency > 16 {
			writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "批量并发需在 1-16 之间")
			return
		}
		out["batchConcurrency"] = *p.BatchConcurrency
	}
	if p.AuditRetentionDays != nil {
		if *p.AuditRetentionDays < 7 || *p.AuditRetentionDays > 3650 {
			writeErr(w, http.StatusBadRequest, "INVALID_INPUT", "审计保留天数需在 7-3650 之间")
			return
		}
		out["auditRetentionDays"] = *p.AuditRetentionDays
	}
	raw, _ := json.Marshal(out)
	if err := s.st.SetSetting("settings", string(raw)); err != nil {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "保存设置失败")
		return
	}
	broadcast, _ := out["wolDefaultBroadcast"].(string)
	concurrency := 4
	if v, ok := out["batchConcurrency"].(float64); ok {
		concurrency = int(v)
	}
	s.jobs.SetDefaults(broadcast, concurrency)
	s.audit(r, "settings.update", "", "ok", string(raw))
	writeJSON(w, http.StatusOK, out)
}

// ---------- batch / jobs ----------

func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var p struct {
		HostIDs      []string `json:"hostIds"`
		Action       string   `json:"action"`
		DelaySeconds int      `json:"delaySeconds"`
		Concurrency  int      `json:"concurrency"`
	}
	if !s.decode(w, r, &p) {
		return
	}
	j, err := s.jobs.Create(r.Context(), p.HostIDs, p.Action, p.DelaySeconds, userID(r), s.clientIP(r), r.UserAgent())
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_BATCH", err.Error())
		return
	}
	s.audit(r, "batch.create", "", "ok", j.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": j.ID, "job": j.Snapshot()})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.jobs.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "JOB_NOT_FOUND", "任务不存在或已过期")
		return
	}
	writeJSON(w, http.StatusOK, j.Snapshot())
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.jobs.Cancel(r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if err == store.ErrNotFound {
			status = http.StatusNotFound
		}
		writeErr(w, status, "CANCEL_FAILED", err.Error())
		return
	}
	s.audit(r, "batch.cancel", "", "ok", j.ID)
	writeJSON(w, http.StatusOK, j.Snapshot())
}
