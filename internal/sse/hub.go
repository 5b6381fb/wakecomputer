package sse

import (
	"encoding/json"
	"sync"
)

type Event struct {
	ID   uint64          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type subscriber struct {
	ch chan Event
}

type Hub struct {
	mu      sync.Mutex
	nextID  uint64
	nextSub int64
	subs    map[int64]*subscriber
	history []Event
	maxKeep int
}

func NewHub() *Hub {
	return &Hub{subs: map[int64]*subscriber{}, maxKeep: 100}
}

// Publish marshals data once and fans it out to all subscribers.
// Slow subscribers are skipped (their buffer is full) rather than blocking.
func (h *Hub) Publish(eventType string, data any) Event {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte(`{}`)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	ev := Event{ID: h.nextID, Type: eventType, Data: raw}
	h.history = append(h.history, ev)
	if len(h.history) > h.maxKeep {
		h.history = h.history[len(h.history)-h.maxKeep:]
	}
	for _, s := range h.subs {
		select {
		case s.ch <- ev:
		default:
			// drop oldest by draining one event, then retry once
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- ev:
			default:
			}
		}
	}
	return ev
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	h.nextSub++
	id := h.nextSub
	s := &subscriber{ch: make(chan Event, 32)}
	h.subs[id] = s
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(s.ch)
		}
		h.mu.Unlock()
	}
	return s.ch, cancel
}

// Replay returns events with ID greater than lastID, for Last-Event-ID reconnects.
func (h *Hub) Replay(lastID uint64) []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Event, 0, len(h.history))
	for _, ev := range h.history {
		if ev.ID > lastID {
			out = append(out, ev)
		}
	}
	return out
}

func (h *Hub) LastID() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.nextID
}
