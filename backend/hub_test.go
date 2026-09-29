package main

import (
	"encoding/json"
	"testing"
)

func TestHubBroadcastAndBoundedHistory(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe()
	h.Broadcast("status", map[string]bool{"ok": true})
	if ev := <-ch; ev.Type != "status" || ev.TS == 0 {
		t.Fatalf("event=%+v", ev)
	}
	for i := 0; i < 205; i++ {
		h.Log("info", 1, "group", string(rune('a'+i%26)))
	}
	logs := h.RecentLogs()
	if len(logs) != 200 || logs[0].Text != string(rune('a'+204%26)) || logs[199].Text != string(rune('a'+5%26)) {
		t.Fatalf("bounded recent logs: first=%+v last=%+v count=%d", logs[0], logs[len(logs)-1], len(logs))
	}
	for i := 0; i < 35; i++ {
		h.Escalation(i)
	}
	escs := h.RecentEscalations()
	if len(escs) != 30 || escs[0] != 34 || escs[29] != 5 {
		t.Fatalf("bounded escalations=%v", escs)
	}
	// A client that does not read is disconnected instead of silently losing an
	// unbounded stream of events.
	for range ch {
	}
	h.Unsubscribe(ch)
	h.Unsubscribe(ch) // repeated teardown is safe
	var decoded Event
	if err := json.Unmarshal((Event{Type: "hello", Data: 1, TS: 2}).Encode(), &decoded); err != nil || decoded.Type != "hello" || decoded.TS != 2 {
		t.Fatalf("event encoding: %+v %v", decoded, err)
	}
}

func TestHubReplaySinceID(t *testing.T) {
	h := NewHub()
	h.Broadcast("first", 1)
	h.Broadcast("second", 2)
	ch, replay := h.SubscribeSince(1)
	defer h.Unsubscribe(ch)
	if len(replay) != 1 || replay[0].ID != 2 || replay[0].Type != "second" {
		t.Fatalf("replay=%+v", replay)
	}
	h.Broadcast("third", 3)
	if ev := <-ch; ev.ID != 3 {
		t.Fatalf("live event=%+v", ev)
	}
}
