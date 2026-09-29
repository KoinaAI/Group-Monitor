package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAssistantDeliveryAndTimeoutReply(t *testing.T) {
	for _, mode := range []string{"answer", "timeout", "revoked", "unwatched"} {
		t.Run(mode, func(t *testing.T) {
			a := assistantTest(t)
			var sent atomic.Int32
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/chat/completions" {
					_, _ = io.Copy(io.Discard, r.Body)
					if mode == "timeout" {
						select {
						case <-r.Context().Done():
						case <-release:
						}
						return
					}
					if mode == "revoked" || mode == "unwatched" {
						a.store.mu.Lock()
						if mode == "revoked" {
							a.store.cfg.Masters[0].Kind = "notify"
						} else {
							a.store.cfg.Groups[0].Watch = false
						}
						a.store.mu.Unlock()
					}
					writeHistoryTestMessage(w, chatMsg{Content: "课程群：明日交材料 [CQ:image,file=example]"})
					return
				}
				if r.URL.Path != "/send_private_msg" {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
				}
				var params map[string]any
				_ = json.NewDecoder(r.Body).Decode(&params)
				if params["auto_escape"] != true || toInt64(params["user_id"]) != 1 {
					t.Errorf("unsafe recipient or message encoding: %v", params)
				}
				if mode == "timeout" && !strings.Contains(toStr(params["message"]), "暂时无法") {
					t.Error("timeout did not produce an explanatory reply")
				}
				sent.Add(1)
				fmt.Fprint(w, `{"status":"ok","data":{}}`)
			}))
			defer srv.Close()
			defer close(release)
			a.ob.httpBase = srv.URL
			a.store.cfg.LLM = LLMConfig{Enabled: true, BaseURL: srv.URL, Timeout: 1}
			a.handle(PrivateMessage{UserID: 1, Text: "最近的通知"})
			want := int32(1)
			if mode == "revoked" || mode == "unwatched" {
				want = 0
			}
			if sent.Load() != want {
				t.Fatalf("sent %d replies, want %d", sent.Load(), want)
			}
		})
	}
}

func TestAssistantSourceExpandsForwardAfterScopeCheck(t *testing.T) {
	a := assistantTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_msg":
			fmt.Fprint(w, `{"status":"ok","data":{"group_id":42,"message_id":91,"message":[{"type":"forward","data":{"id":"notice"}}]}}`)
		case "/get_forward_msg":
			fmt.Fprint(w, `{"status":"ok","data":{"messages":[{"message":"提交材料必须包含成绩单"}]}}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	a.ob.httpBase = srv.URL
	result := a.executeTool(context.Background(), 1, assistantCall("search_message_by_uuid", assistantJSON(map[string]string{"uuid": encodeMsgUUID(42, 91)})))
	if !strings.Contains(result, "必须包含成绩单") || !strings.Contains(result, "时间未知") {
		t.Fatalf("source omitted forwarded content or invented date: %s", result)
	}
}
