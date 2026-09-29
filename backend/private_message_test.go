package main

import "testing"

func TestOneBotPrivateDispatchDedupAndGeneration(t *testing.T) {
	o := NewOneBot()
	o.gen = 3
	var private []PrivateMessage
	groups := 0
	o.onPrivate = func(pm PrivateMessage) { private = append(private, pm) }
	o.onEvent = func(GroupMessage) { groups++ }
	frame := []byte(`{"post_type":"message","message_type":"private","message_id":-17,"self_id":99,"user_id":7,"time":123,"sender":{"nickname":"主人"},"message":[{"type":"text","data":{"text":"最近有哪些通知"}}]}`)
	o.handleFrameForGeneration(frame, 2)
	if len(private) != 0 {
		t.Fatal("stale connection delivered private message")
	}
	o.handleFrameForGeneration(frame, 3)
	o.handleFrameForGeneration(frame, 3)
	if len(private) != 1 || groups != 0 || private[0].Text != "最近有哪些通知" || private[0].Nickname != "主人" || private[0].UserID != 7 {
		t.Fatalf("private=%+v groups=%d", private, groups)
	}
	pm := parsePrivateMessage(map[string]any{"raw_message": "通知原文"}, 99)
	if pm.Text != "通知原文" {
		t.Fatalf("string fallback = %q", pm.Text)
	}
}
