package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestForwardExpandedBeforeNoticeClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_forward_msg":
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"messages":[{"sender":{"nickname":"老师"},"message":"周五十点前提交奖学金材料"}]}}`)
		case "/chat/completions", "/jev":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "提交奖学金材料") {
				t.Error("model received only a forward placeholder")
			}
			if r.URL.Path == "/jev" {
				fmt.Fprint(w, `{"answers":{"important":{"type":"noul","noul":0.95}}}`)
			} else {
				fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":2,\"title\":\"奖学金\",\"summary\":\"周五十点前提交奖学金材料\"}"}}]}`)
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	pipe, _ := testPipe(t, func(c *Config) {
		c.Jev.Enabled, c.Jev.APIKey, c.Jev.BaseURL = true, "test", srv.URL+"/jev"
		c.LLM.Enabled, c.LLM.BaseURL = true, srv.URL
		c.Rules.QuietWindowSec = 60
	})
	defer pipe.Shutdown()
	pipe.ob.httpBase = srv.URL
	notices, err := NewNoticeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pipe.SetNoticeStore(notices)
	raw := map[string]any{"group_id": int64(100), "user_id": int64(7), "message_id": int64(23), "time": time.Now().Unix(), "message": []any{segment("forward", map[string]any{"id": "transient-forward-id"})}}
	gm := parseGroupMessage(raw, 0)
	encoded, _ := json.Marshal(gm)
	if gm.RawMessage == nil || strings.Contains(string(encoded), "transient-forward-id") {
		t.Fatalf("raw payload must be retained internally only: %s", encoded)
	}
	pipe.Ingest(gm)
	pipe.mu.Lock()
	buf := pipe.buffers[100]
	pipe.mu.Unlock()
	if buf == nil {
		t.Fatal("forward dropped before extraction")
	}
	pipe.flush(100, buf.timerGen)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := notices.Query(NoticeQuery{GroupID: 100})
		if err == nil && len(rows) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expanded notice was not archived")
}
