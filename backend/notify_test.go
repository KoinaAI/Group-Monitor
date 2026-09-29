package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
