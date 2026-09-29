package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func historyTestStore(t *testing.T) *NoticeStore {
	t.Helper()
	store, err := NewNoticeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func historyTestCall(id, arguments string) historyToolCall {
	call := historyToolCall{ID: id, Type: "function"}
	call.Function.Name = "search_notices"
	call.Function.Arguments = arguments
	return call
}

func writeHistoryTestMessage(w http.ResponseWriter, message chatMsg) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message}}})
}

func TestLLMHistoryRetrievesOlderNoticeWithinCurrentGroup(t *testing.T) {
	store := historyTestStore(t)
	now := time.Now().UnixMilli()
	for i := 0; i < 9; i++ {
		_, err := store.Append(NoticeRecord{ID: fmt.Sprintf("recent-%d", i), CreatedAt: now + int64(i), GroupID: 42, Result: LLMResult{Useful: true, Title: "其他通知"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []NoticeRecord{
		{ID: "target", CreatedAt: now - 1000, GroupID: 42, Result: LLMResult{Useful: true, Title: "体检安排", Summary: "校医院一楼", Deadline: "周五"}, Sources: []NoticeSource{{UserID: 999999, TextHash: "private-provenance"}}},
		{ID: "other-group", CreatedAt: now, GroupID: 43, Result: LLMResult{Useful: true, Title: "体检安排", Summary: "other-group-private"}},
	} {
		if _, err := store.Append(n); err != nil {
			t.Fatal(err)
		}
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body chatReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Stream || len(body.Tools) != 1 || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("bad request contract: %+v", body)
		}
		if requests == 1 {
			if len(body.Messages) != 2 || strings.Contains(body.Messages[1].Content, "校医院") {
				t.Error("history should only be sent after a tool request")
			}
			writeHistoryTestMessage(w, chatMsg{ToolCalls: []historyToolCall{historyTestCall("search-1", `{"query":"体检","limit":5}`)}})
			return
		}
		if len(body.Messages) != 4 {
			t.Errorf("expected assistant call and tool response, got %d messages", len(body.Messages))
			return
		}
		result := body.Messages[3]
		if result.Role != "tool" || result.ToolCallID != "search-1" || !strings.Contains(result.Content, "校医院一楼") || !strings.Contains(result.Content, "recordedAt") {
			t.Errorf("missing matched historical notice: %+v", result)
		}
		for _, forbidden := range []string{"other-group-private", "其他通知", "private-provenance", "999999"} {
			if strings.Contains(result.Content, forbidden) {
				t.Errorf("tool output leaked %q: %s", forbidden, result.Content)
			}
		}
		writeHistoryTestMessage(w, chatMsg{Content: `{"useful":true,"level":2,"title":"体检延期","summary":"校医院一楼体检改为下周一"}`})
	}))
	defer srv.Close()
	res, _, err := callLLMWithHistoryContext(context.Background(), LLMConfig{BaseURL: srv.URL, APIKey: "secret"}, "上次的体检延到下周一，地点不变", store, 42)
	if err != nil || requests != 2 || !res.Useful || res.Title != "体检延期" {
		t.Fatalf("history interaction: result=%+v requests=%d err=%v", res, requests, err)
	}
}

func TestLLMHistoryDirectVerdictAndUnsupportedFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback=%t", fallback), func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body chatReq
				_ = json.NewDecoder(r.Body).Decode(&body)
				if fallback && requests == 1 {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"error":{"message":"tools are not supported by this model"}}`)
					return
				}
				if fallback && (!body.Stream || len(body.Tools) != 0) {
					t.Error("fallback must preserve original non-tool API")
				}
				writeHistoryTestMessage(w, chatMsg{Content: `{"useful":false,"level":1,"reason":"闲聊"}`})
			}))
			defer srv.Close()
			res, _, err := callLLMWithHistoryContext(context.Background(), LLMConfig{BaseURL: srv.URL}, "哈哈", historyTestStore(t), 42)
			wantRequests := 1
			if fallback {
				wantRequests++
			}
			if err != nil || res.Useful || requests != wantRequests {
				t.Fatalf("result=%+v requests=%d err=%v", res, requests, err)
			}
		})
	}
}

func TestLLMHistoryLimitsRoundsAndToolOutput(t *testing.T) {
	store := historyTestStore(t)
	for i := 0; i < 5; i++ {
		_, err := store.Append(NoticeRecord{ID: fmt.Sprint(i), GroupID: 42, Result: LLMResult{Useful: true, Title: "体检", Summary: strings.Repeat("历史内容", 300), Event: strings.Repeat("任务", 300)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body chatReq
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, message := range body.Messages {
			if message.Role == "tool" && (len(message.Content) > maxHistoryToolBytes || !json.Valid([]byte(message.Content))) {
				t.Error("tool content must remain bounded valid JSON")
			}
		}
		if requests == 3 && body.ToolChoice != "none" {
			t.Error("final request must disable further tools")
		}
		writeHistoryTestMessage(w, chatMsg{ToolCalls: []historyToolCall{historyTestCall(fmt.Sprint(requests), `{"query":"体检","limit":100}`)}})
	}))
	defer srv.Close()
	_, _, err := callLLMWithHistoryContext(context.Background(), LLMConfig{BaseURL: srv.URL}, "上次体检改期", store, 42)
	if err == nil || !strings.Contains(err.Error(), "limit exceeded") || requests != 3 {
		t.Fatalf("unbounded tool requests=%d err=%v", requests, err)
	}
}

func TestLLMHistoryRejectsUnknownArgumentsAndHandlesEmptyResults(t *testing.T) {
	store := historyTestStore(t)
	for _, arguments := range []string{`{"query":"通知","groupId":43}`, `{"query":"通知"} {}`, `{"query":null}`, `{"query":"通知","before":-1}`} {
		got := executeNoticeSearch(context.Background(), store, 42, historyTestCall("a", arguments))
		if !strings.Contains(got, "error") {
			t.Errorf("unexpected accepted args %s: %s", arguments, got)
		}
	}
	got := executeNoticeSearch(context.Background(), store, 42, historyTestCall("a", `{"query":"不存在"}`))
	if !strings.Contains(got, `"notices":[]`) || strings.Contains(got, "error") {
		t.Errorf("empty result must be explicit: %s", got)
	}
}

func TestLLMHistoryCancellationStopsFurtherRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		cancel()
		writeHistoryTestMessage(w, chatMsg{ToolCalls: []historyToolCall{historyTestCall("a", `{"query":"体检"}`)}})
	}))
	defer srv.Close()
	_, _, err := callLLMWithHistoryContext(ctx, LLMConfig{BaseURL: srv.URL}, "上次体检", historyTestStore(t), 42)
	if err == nil || requests.Load() != 1 {
		t.Fatalf("cancelled request count=%d err=%v", requests.Load(), err)
	}
}
