package main

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is a server-sent event pushed to connected frontends.
type Event struct {
	ID   uint64 `json:"id,omitempty"`
	Type string `json:"type"` // status | message | buffer | escalation | log
	Data any    `json:"data"`
	TS   int64  `json:"ts"`
}

// Hub fans out events to all connected SSE clients and keeps a bounded history
// of recent activity so a freshly-loaded page has context.
type Hub struct {
	mu        sync.Mutex
	clients   map[chan Event]struct{}
	events    []Event
	nextID    uint64
	maxEvents int

	logMu   sync.RWMutex
	logs    []LogEntry
	maxLogs int

	escMu   sync.RWMutex
	escs    []any // recent escalation payloads, oldest first
	maxEscs int
}

type LogEntry struct {
	TS      int64  `json:"ts"`
	Level   string `json:"level"` // info | escalate | suppress | urgent | error
	GroupID int64  `json:"groupId,omitempty"`
	Group   string `json:"group,omitempty"`
	Text    string `json:"text"`
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[chan Event]struct{}),
		maxEvents: 256,
		maxLogs:   200,
		maxEscs:   30,
	}
}

func (h *Hub) Subscribe() chan Event {
	ch, _ := h.SubscribeSince(0)
	return ch
}

func (h *Hub) SubscribeSince(lastID uint64) (chan Event, []Event) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	replay := make([]Event, 0)
	for _, ev := range h.events {
		if ev.ID > lastID {
			replay = append(replay, ev)
		}
	}
	h.mu.Unlock()
	return ch, replay
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Broadcast(typ string, data any) {
	h.mu.Lock()
	h.nextID++
	ev := Event{ID: h.nextID, Type: typ, Data: data, TS: time.Now().UnixMilli()}
	h.events = append(h.events, ev)
	if len(h.events) > h.maxEvents {
		h.events = h.events[len(h.events)-h.maxEvents:]
	}
	var slow []chan Event
	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
			slow = append(slow, ch)
		}
	}
	for _, ch := range slow {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Log records an activity entry and broadcasts it.
func (h *Hub) Log(level string, groupID int64, group, text string) {
	e := LogEntry{TS: time.Now().UnixMilli(), Level: level, GroupID: groupID, Group: group, Text: text}
	h.logMu.Lock()
	h.logs = append(h.logs, e)
	if len(h.logs) > h.maxLogs {
		h.logs = h.logs[len(h.logs)-h.maxLogs:]
	}
	h.logMu.Unlock()
	h.Broadcast("log", e)
}

// RecentLogs returns the recent log entries, newest first, to match the
// frontend's unshift ordering (and RecentEscalations).
func (h *Hub) RecentLogs() []LogEntry {
	h.logMu.RLock()
	defer h.logMu.RUnlock()
	out := make([]LogEntry, len(h.logs))
	for i, e := range h.logs {
		out[len(h.logs)-1-i] = e
	}
	return out
}

// Escalation records an escalation payload (bounded history) and broadcasts it,
// so a freshly-loaded page can backfill the "recent escalations" panel.
func (h *Hub) Escalation(data any) {
	h.escMu.Lock()
	h.escs = append(h.escs, data)
	if len(h.escs) > h.maxEscs {
		h.escs = h.escs[len(h.escs)-h.maxEscs:]
	}
	h.escMu.Unlock()
	h.Broadcast("escalation", data)
}

// RecentEscalations returns the recent escalation payloads, newest first, to
// match the frontend's unshift ordering.
func (h *Hub) RecentEscalations() []any {
	h.escMu.RLock()
	defer h.escMu.RUnlock()
	out := make([]any, len(h.escs))
	for i, e := range h.escs {
		out[len(h.escs)-1-i] = e
	}
	return out
}

func (e Event) Encode() []byte {
	b, _ := e.EncodeChecked()
	return b
}

func (e Event) EncodeChecked() ([]byte, error) {
	return json.Marshal(e)
}
