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
	"time"
)

func assistantTest(t *testing.T) *Assistant {
	t.Helper()
	cfg := Config{Masters: []Master{{UserID: 1}, {UserID: 2, Kind: "notify"}}, Groups: []GroupWatch{{GroupID: 42, GroupName: "课程群", Watch: true}, {GroupID: 43, GroupName: "未监控群"}}}
	a := NewAssistant(&Store{cfg: cfg}, NewOneBot(), NewHub(), historyTestStore(t))
	t.Cleanup(a.Shutdown)
	return a
}

func assistantCall(name, args string) historyToolCall {
	call := historyToolCall{ID: "call-1", Type: "function"}
	call.Function.Name, call.Function.Arguments = name, args
	return call
}

func TestAssistantArchiveToolCycleAndScope(t *testing.T) {
	a := assistantTest(t)
	for _, n := range []NoticeRecord{
		{ID: "allowed", GroupID: 42, MessageIDs: []int64{91}, Result: LLMResult{Useful: true, Title: "体检", Summary: "周五到校医院", Event: "带学生证"}},
		{ID: "secret", GroupID: 43, Result: LLMResult{Useful: true, Title: "体检", Summary: "秘密内容"}},
	} {
		if _, err := a.notices.Append(n); err != nil {
			t.Fatal(err)
		}
	}
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Tools) != 5 || body.MaxTokens > 2048 || !strings.Contains(body.Messages[0].Content, "北京时间") {
			t.Errorf("bad assistant request: %+v", body)
		}
		if requests.Add(1) == 1 {
			writeHistoryTestMessage(w, chatMsg{ToolCalls: []historyToolCall{assistantCall("search_notices", `{"query":"体检"}`)}})
			return
		}
		output := body.Messages[len(body.Messages)-1]
		if output.Role != "tool" || !strings.Contains(output.Content, "周五到校医院") || !strings.Contains(output.Content, "带学生证") || !strings.Contains(output.Content, encodeMsgUUID(42, 91)) || strings.Contains(output.Content, "秘密内容") {
			t.Errorf("wrong tool result: %+v", output)
		}
		writeHistoryTestMessage(w, chatMsg{Content: "课程群：周五到校医院，带学生证。"})
	}))
	defer srv.Close()
	answer, err := a.answer(context.Background(), LLMConfig{BaseURL: srv.URL}, 1, "体检要求是什么")
	if err != nil || requests.Load() != 2 || !strings.Contains(answer, "带学生证") {
		t.Fatalf("answer=%q err=%v requests=%d", answer, err, requests.Load())
	}
	for _, user := range []int64{2, 999} {
		if result := a.executeTool(context.Background(), user, assistantCall("recent_notices", `{}`)); !strings.Contains(result, "denied") {
			t.Errorf("unauthorized output=%s", result)
		}
	}
	if result := a.executeTool(context.Background(), 1, assistantCall("recent_notices", `{"group_id":43}`)); !strings.Contains(result, "not watched") {
		t.Fatal(result)
	}
}

func TestAssistantSourceValidatesActualGroupAndLightweightContext(t *testing.T) {
	for _, actual := range []int64{42, 43, 0} {
		t.Run(fmt.Sprint(actual), func(t *testing.T) {
			a := assistantTest(t)
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var data any
				switch r.URL.Path {
				case "/get_msg":
					data = map[string]any{"group_id": actual, "message_id": 91, "message_seq": 11, "time": time.Now().Unix(), "message": "原通知"}
				case "/get_group_msg_history":
					var params map[string]any
					_ = json.NewDecoder(r.Body).Decode(&params)
					if toInt64(params["group_id"]) != 42 || toInt64(params["count"]) != 6 {
						t.Errorf("bad params: %v", params)
					}
					seq := int64(12)
					if params["reverseOrder"] == true {
						seq = 10
					}
					data = map[string]any{"messages": []any{
						map[string]any{"group_id": 42, "message_id": seq, "message_seq": seq, "time": time.Now().Unix(), "message": "前后文"},
						map[string]any{"group_id": 43, "message_id": 99, "message_seq": 9, "time": time.Now().Unix(), "message": "跨群秘密"},
					}}
				default:
					t.Errorf("unexpected rich history call: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": data})
			}))
			defer srv.Close()
			a.ob.httpBase = srv.URL
			result := a.executeTool(context.Background(), 1, assistantCall("search_message_by_uuid", assistantJSON(map[string]string{"uuid": encodeMsgUUID(42, 91)})))
			if strings.Contains(result, "跨群秘密") {
				t.Fatal(result)
			}
			if actual != 42 {
				if strings.Contains(result, "原通知") || calls.Load() != 1 || !strings.Contains(result, "mismatch") {
					t.Fatalf("unsafe lookup: %s; calls=%d", result, calls.Load())
				}
			} else if calls.Load() != 3 || !strings.Contains(result, "原通知") || !strings.Contains(result, "前后文") {
				t.Fatalf("incomplete source: %s; calls=%d", result, calls.Load())
			}
		})
	}
}

func TestAssistantAdmissionAndShutdownCancelInflight(t *testing.T) {
	a := assistantTest(t)
	entered := make(chan struct{}, assistantMaxConcurrent)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	a.store.cfg.LLM = LLMConfig{Enabled: true, BaseURL: srv.URL}
	for i := int64(3); i <= 6; i++ {
		a.store.cfg.Masters = append(a.store.cfg.Masters, Master{UserID: i})
	}
	for _, user := range []int64{0, 2, 999} {
		if a.Submit(PrivateMessage{UserID: user, Text: "查通知"}) {
			t.Fatalf("admitted user %d", user)
		}
	}
	if a.Submit(PrivateMessage{UserID: 1, Text: strings.Repeat("问", assistantQuestionChars+1)}) {
		t.Fatal("admitted oversized question")
	}
	for _, user := range []int64{1, 3, 4, 5} {
		if !a.Submit(PrivateMessage{UserID: user, Text: "查通知"}) {
			t.Fatalf("rejected user %d", user)
		}
	}
	if a.Submit(PrivateMessage{UserID: 1, Text: "再查"}) || a.Submit(PrivateMessage{UserID: 6, Text: "再查"}) {
		t.Fatal("concurrency cap exceeded")
	}
	for i := 0; i < assistantMaxConcurrent; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("request did not start")
		}
	}
	done := make(chan struct{})
	go func() { a.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel requests")
	}
	if a.Submit(PrivateMessage{UserID: 1, Text: "再查"}) {
		t.Fatal("admitted after shutdown")
	}
}

func TestAssistantBudgetsAndArgumentValidation(t *testing.T) {
	a := assistantTest(t)
	for _, args := range []string{`null`, `{ } { }`, `{"limit":100}`, `{"group_id":43}`, `{"unknown":"value"}`, `{"query":"` + strings.Repeat("x", 121) + `"}`} {
		if result := a.executeTool(context.Background(), 1, assistantCall("recent_notices", args)); !strings.Contains(result, "error") {
			t.Fatalf("accepted %s: %s", args, result)
		}
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		call := assistantCall("list_watched_groups", `{}`)
		call.ID = fmt.Sprint(n)
		writeHistoryTestMessage(w, chatMsg{ToolCalls: []historyToolCall{call}})
	}))
	defer srv.Close()
	if _, err := a.answer(context.Background(), LLMConfig{BaseURL: srv.URL}, 1, "查通知"); err == nil || calls.Load() != assistantMaxRounds+1 {
		t.Fatalf("unbounded loop: calls=%d err=%v", calls.Load(), err)
	}
	_, err := a.notices.Append(NoticeRecord{ID: "huge", GroupID: 42, Result: LLMResult{Summary: strings.Repeat("\x00", 4000), Title: strings.Repeat("\x01", 200)}})
	if err != nil {
		t.Fatal(err)
	}
	output := a.executeTool(context.Background(), 1, assistantCall("recent_notices", `{}`))
	if len(output) > assistantToolBytes || !json.Valid([]byte(output)) {
		t.Fatalf("invalid/oversized tool output: %d", len(output))
	}
}

func TestAssistantCancellationAndPrivilegeRevocation(t *testing.T) {
	a := assistantTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.answer(ctx, LLMConfig{BaseURL: "http://example.test"}, 1, "查通知"); err == nil {
		t.Fatal("ignored cancellation")
	}
	if result := a.executeTool(ctx, 1, assistantCall("list_watched_groups", `{}`)); !strings.Contains(result, "cancelled") {
		t.Fatal(result)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.store.mu.Lock()
		a.store.cfg.Masters[0].Kind = "notify"
		a.store.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": map[string]any{"messages": []any{map[string]any{"group_id": 42, "message_id": 91, "time": time.Now().Unix(), "message": "secret"}}}})
	}))
	defer srv.Close()
	a.ob.httpBase = srv.URL
	output := a.executeTool(context.Background(), 1, assistantCall("search_group_history", `{"group_id":42}`))
	if strings.Contains(output, "secret") || !strings.Contains(output, "access changed") {
		t.Fatalf("revoked access leaked output: %s", output)
	}
}

func TestAssistantLiveSearchHasBoundedPaging(t *testing.T) {
	a := assistantTest(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.URL.Path != "/get_group_msg_history" {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		if toInt64(params["count"]) != 60 || params["reverseOrder"] != true {
			t.Errorf("bad history params: %v", params)
		}
		messages := []any{}
		for i := int64(0); i < 60; i++ {
			seq := 200 - int64(n)*60 + i
			messages = append(messages, map[string]any{"message_id": seq, "message_seq": seq, "time": time.Now().Unix(), "message": "无关聊天"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": messages})
	}))
	defer srv.Close()
	a.ob.httpBase = srv.URL
	result := a.executeTool(context.Background(), 1, assistantCall("search_group_history", `{"group_id":42,"query":"体检"}`))
	if calls.Load() != 2 || !strings.Contains(result, `"limited_window":true`) || !strings.Contains(result, `"messages":[]`) {
		t.Fatalf("calls=%d result=%s", calls.Load(), result)
	}
}
