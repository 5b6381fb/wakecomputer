package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "INTERNAL", "SSE not supported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	lastID, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	ch, cancel := s.hub.Subscribe()
	defer cancel()

	if lastID > 0 {
		for _, ev := range s.hub.Replay(lastID) {
			if !writeEvent(w, fl, ev.ID, ev.Type, ev.Data) {
				return
			}
		}
	}
	if _, err := fmt.Fprint(w, "event: hello\ndata: {\"ok\":true}\n\n"); err != nil {
		return
	}
	fl.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !writeEvent(w, fl, ev.ID, ev.Type, ev.Data) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, fl http.Flusher, id uint64, event string, data []byte) bool {
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, event, data); err != nil {
		return false
	}
	fl.Flush()
	return true
}
