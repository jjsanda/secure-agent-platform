package dispatch

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is one persisted run event as streamed to Server-Sent-Events clients.
// Its shape matches a row in app.run_event.
type Event struct {
	ID      int64           `json:"id"`
	RunID   string          `json:"runId"`
	Kind    string          `json:"kind"`
	Step    int32           `json:"step"`
	Payload json.RawMessage `json:"payload"`
	At      time.Time       `json:"at"`
}

// Hub fans run events out to SSE subscribers and keeps a short in-memory history
// per run, so a subscriber that connects mid-run still receives the whole trace.
//
// This is single-instance by design (the events are also durably persisted in
// Postgres). A horizontally-scaled deployment would replace the in-memory
// fan-out with Postgres LISTEN/NOTIFY or a message broker — a documented
// limitation, not an oversight.
type Hub struct {
	mu   sync.Mutex
	runs map[string]*runChannel
}

type runChannel struct {
	history []Event
	subs    map[chan Event]struct{}
	closed  bool
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{runs: make(map[string]*runChannel)}
}

func (h *Hub) get(runID string) *runChannel {
	rc, ok := h.runs[runID]
	if !ok {
		rc = &runChannel{subs: make(map[chan Event]struct{})}
		h.runs[runID] = rc
	}
	return rc
}

// Publish records an event and delivers it to all current subscribers.
func (h *Hub) Publish(runID string, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rc := h.get(runID)
	rc.history = append(rc.history, ev)
	for ch := range rc.subs {
		select {
		case ch <- ev:
		default: // a slow consumer is dropped; the full trace remains in Postgres
		}
	}
}

// Subscribe returns the current history plus a channel of future events. If the
// run has already completed, done is true and the channel is already closed.
func (h *Hub) Subscribe(runID string) (history []Event, ch <-chan Event, done bool, cancel func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rc := h.get(runID)
	hist := append([]Event(nil), rc.history...)
	if rc.closed {
		closed := make(chan Event)
		close(closed)
		return hist, closed, true, func() {}
	}
	c := make(chan Event, 64)
	rc.subs[c] = struct{}{}
	return hist, c, false, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := rc.subs[c]; ok {
			delete(rc.subs, c)
			close(c)
		}
	}
}

// Close marks a run complete, closes all subscriber channels, and frees the
// run's entry (including its event history) so the hub does not grow without
// bound. This is safe because a run is finalized in the database BEFORE its hub
// entry is closed, and the SSE handler routes any later subscriber for a
// terminal run to the durable Postgres replay path rather than back here.
func (h *Hub) Close(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rc, ok := h.runs[runID]
	if !ok {
		return
	}
	rc.closed = true
	for ch := range rc.subs {
		delete(rc.subs, ch)
		close(ch)
	}
	delete(h.runs, runID)
}
