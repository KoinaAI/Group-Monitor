package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestUnpackStructuredCards(t *testing.T) {
	announcement, _ := json.Marshal(map[string]any{"app": "com.tencent.mannounce", "prompt": "群公告", "meta": map[string]any{"mannounce": map[string]any{"title": base64.StdEncoding.EncodeToString([]byte("开会")), "text": base64.StdEncoding.EncodeToString([]byte("明天九点开会"))}}})
	tests := []struct{ typ, raw, want string }{
		{"json", string(announcement), "［卡片 群公告 明天九点开会 开会］"},
		{"json", `{"meta":{"detail":{"title":"Release","desc":"SGVsbG8gd29ybGQ="}}}`, "［卡片 SGVsbG8gd29ybGQ= Release］"},
		{"xml", `<msg brief='会议 &amp; 日程'><item><title><![CDATA[明天 <九点>]]></title><summary>请携带材料</summary></item></msg>`, "［卡片 会议 & 日程 明天 <九点> 请携带材料］"},
		{"xml", `<!DOCTYPE x [<!ENTITY external SYSTEM "file:///etc/passwd">]><msg><title>&external;</title></msg>`, "[卡片]"},
		{"json", `{invalid`, "[卡片]"},
		{"json", strings.Repeat("a", maxCardBytes+1), "[卡片]"},
	}
	for _, tt := range tests {
		for i := 0; i < 10; i++ {
			if got := flattenCard(tt.typ, map[string]any{"data": tt.raw}); got != tt.want {
				t.Fatalf("%s card got %q want %q", tt.typ, got, tt.want)
			}
		}
	}
}

func TestUnpackInlineForwardsAndLimits(t *testing.T) {
	msg := []any{segment("text", map[string]any{"text": "转发："}), segment("forward", map[string]any{"content": []any{
		map[string]any{"type": "node", "data": map[string]any{"nickname": "老师", "content": []any{segment("at", map[string]any{"qq": "all"}), segment("text", map[string]any{"text": "明天开会"})}}},
		map[string]any{"sender": map[string]any{"card": "班长"}, "message": []any{segment("forward", map[string]any{"content": []any{map[string]any{"sender": map[string]any{"nickname": "同学"}, "message": "收到"}}})}},
	}})}
	text, all, self, _ := flattenUnpackedMessage(msg, 99)
	if all || self || !strings.Contains(text, "老师：@全体成员 明天开会") || !strings.Contains(text, "同学：收到") {
		t.Fatalf("unexpected unpack: %q all=%v self=%v", text, all, self)
	}
	var deep any = "DEPTH_LIMIT_LEAK"
	for i := 0; i < maxForwardDepth+1; i++ {
		deep = []any{segment("forward", map[string]any{"content": []any{map[string]any{"message": deep}}})}
	}
	if text, _, _, _ := flattenUnpackedMessage(deep, 0); strings.Contains(text, "DEPTH_LIMIT_LEAK") {
		t.Fatalf("depth not bounded: %s", text)
	}
	long := strings.Repeat("界", maxUnpackedBytes)
	text, _, _, _ = flattenUnpackedMessage([]any{segment("text", map[string]any{"text": long})}, 0)
	if len(text) > maxUnpackedBytes || !utf8.ValidString(text) {
		t.Fatalf("bad bounded unicode length=%d", len(text))
	}
	many := make([]any, maxUnpackedItems+1)
	for i := range many {
		many[i] = segment("text", map[string]any{"text": "x"})
	}
	text, _, _, _ = flattenUnpackedMessage(many, 0)
	if len(text) != maxUnpackedItems {
		t.Fatalf("item budget not applied: %d", len(text))
	}
}

func TestExpandMessagesScopeCyclesAndCache(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		var result any
		switch r.URL.Path {
		case "/get_forward_msg":
			id := toStr(p["message_id"])
			switch id {
			case "outer":
				result = map[string]any{"messages": []any{map[string]any{"sender": map[string]any{"nickname": "老师"}, "message": []any{segment("text", map[string]any{"text": "考试安排"}), segment("forward", map[string]any{"id": "inner"})}}}}
			case "inner":
				result = map[string]any{"content": []any{map[string]any{"message": []any{segment("text", map[string]any{"text": "周五九点"}), segment("forward", map[string]any{"id": "outer"})}}}}
			}
		case "/get_msg":
			id := toInt64(p["message_id"])
			group := int64(42)
			if id == 2 {
				group = 999
			}
			result = map[string]any{"group_id": group, "user_id": 7, "sender": map[string]any{"nickname": "班长"}, "message": []any{segment("text", map[string]any{"text": fmt.Sprintf("引用原文%d", id)}), segment("reply", map[string]any{"id": id})}}
		default:
			t.Errorf("unexpected action %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "retcode": 0, "data": result})
	}))
	defer srv.Close()
	o := NewOneBot()
	o.httpBase = srv.URL
	msg := []any{segment("forward", map[string]any{"id": "outer"}), segment("forward", map[string]any{"id": "outer"}), segment("reply", map[string]any{"id": 1}), segment("reply", map[string]any{"id": 2}), segment("text", map[string]any{"text": " http://127.0.0.1/never-fetch"})}
	before, _ := json.Marshal(msg)
	expanded := o.ExpandMessagesContext(context.Background(), 42, []any{msg, msg})
	for _, v := range expanded {
		text, _, _, _ := flattenUnpackedMessage(v, 0)
		if !strings.Contains(text, "考试安排") || !strings.Contains(text, "周五九点") || !strings.Contains(text, "班长：引用原文1") || strings.Contains(text, "引用原文2") {
			t.Fatalf("expanded text=%q", text)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("expected 4 cached calls, got %d", calls.Load())
	}
	after, _ := json.Marshal(msg)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("expansion mutated the input")
	}
}

func TestExpandMessageRequestBudgetAndCancellation(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"messages":[{"message":"通知"}]}}`)
	}))
	defer srv.Close()
	o := NewOneBot()
	o.httpBase = srv.URL
	msg := make([]any, 100)
	for i := range msg {
		msg[i] = segment("forward", map[string]any{"id": fmt.Sprint(i)})
	}
	text := o.ExpandGroupMessageContext(context.Background(), 42, msg)
	if calls.Load() != maxMessageExpansionCalls || !strings.Contains(text, "[聊天记录]") {
		t.Fatalf("calls=%d text=%q", calls.Load(), text)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	text = o.ExpandGroupMessageContext(ctx, 42, msg)
	if calls.Load() != maxMessageExpansionCalls || strings.Contains(text, "通知") {
		t.Fatal("canceled expansion performed work")
	}
}

func TestExpansionConcurrentCallsAreBounded(t *testing.T) {
	var active, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		time.Sleep(15 * time.Millisecond)
		fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"messages":[{"message":"通知"}]}}`)
	}))
	defer srv.Close()
	o := NewOneBot()
	o.httpBase = srv.URL
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.ExpandGroupMessageContext(context.Background(), 42, []any{segment("forward", map[string]any{"id": "f"})})
		}()
	}
	wg.Wait()
	if peak.Load() > 4 || peak.Load() < 2 {
		t.Fatalf("unexpected API parallelism %d", peak.Load())
	}
}

func TestExpansionCancelsInFlightAPI(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	o := NewOneBot()
	o.httpBase = srv.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan string, 1)
	go func() {
		done <- o.ExpandGroupMessageContext(ctx, 42, []any{segment("forward", map[string]any{"id": "f"})})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("API request did not start")
	}
	cancel()
	select {
	case text := <-done:
		if text != "[聊天记录]" {
			t.Fatalf("canceled fallback=%q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight API ignored cancellation")
	}
}
