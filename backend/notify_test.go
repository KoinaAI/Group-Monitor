package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSendPrivateConcurrentBoundsAndReportsFailures(t *testing.T) {
	var active, maxActive atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		current := active.Add(1)
		for {
			old := maxActive.Load()
			if current <= old || maxActive.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		active.Add(-1)
		if int64(body["user_id"].(float64)) == 3 {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `failed`)
			return
		}
		fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{}}`)
	}))
	defer srv.Close()
	o := NewOneBot()
	o.httpBase = srv.URL
	masters := make([]Master, 10)
	for i := range masters {
		masters[i] = Master{UserID: int64(i + 1)}
	}
	sent, failed := sendPrivateConcurrent(o, masters, "test")
	if sent != 9 || len(failed) != 1 || failed[0] != 3 {
		t.Fatalf("sent=%d failed=%v", sent, failed)
	}
	if maxActive.Load() < 2 {
		t.Fatalf("notifications were serialized, max concurrency=%d", maxActive.Load())
	}
	if maxActive.Load() > maxNotifyWorkers {
		t.Fatalf("worker limit exceeded: %d", maxActive.Load())
	}
}

func TestFormatReminderIsCompactAndOmitsEmptyFields(t *testing.T) {
	got := formatReminder("软件工程一班", LLMResult{
		Level:   2,
		Title:   "体检安排",
		Time:    "周五 14:00",
		Place:   "校医院一楼",
		Event:   "完成体检并交表",
		Summary: "请携带学生证。",
	}, false)
	want := "⚠️ 软件工程一班｜体检安排\n时间 周五 14:00 · 地点 校医院一楼\n事项 完成体检并交表\n请携带学生证。"
	if got != want {
		t.Fatalf("compact reminder mismatch:\n%s", got)
	}
	if strings.Contains(got, "截止") || strings.Contains(got, "群哨提醒") || strings.Contains(got, "━━") {
		t.Fatalf("empty/ornamental fields leaked: %s", got)
	}
}
