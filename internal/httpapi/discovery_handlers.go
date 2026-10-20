package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/wakecomputer/wakecomputer/internal/discovery"
)

func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if !s.discoveryRate.Allow(userID(r)) {
		writeErr(w, http.StatusTooManyRequests, "RATE_LIMITED", "扫描过于频繁，请稍后再试")
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("subnet"))
	if raw == "" {
		raw = s.cfg.LANSubnet
	}
	subnet, err := discovery.ParseSubnet(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "INVALID_SUBNET", err.Error())
		return
	}
	probe := r.URL.Query().Get("scan") != "0"

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	items, err := discovery.Scan(ctx, subnet, 1200*time.Millisecond, probe)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DISCOVERY_FAILED", "扫描局域网失败："+err.Error())
		return
	}
	if items == nil {
		items = []discovery.Device{}
	}

	hosts, err := s.st.ListHosts()
	if err == nil {
		added := make(map[string]bool, len(hosts))
		for _, h := range hosts {
			added[strings.ToUpper(h.MAC)] = true
		}
		for i := range items {
			items[i].Added = added[strings.ToUpper(items[i].MAC)]
		}
	}

	writeJSON(w, http.StatusOK, discovery.Result{
		Subnet:    subnet.String(),
		ScannedAt: time.Now().UTC().Format(time.RFC3339),
		Items:     items,
	})
}
