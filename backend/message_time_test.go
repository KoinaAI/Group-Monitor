package main

import (
	"strings"
	"testing"
	"time"
)

func TestTranscriptPreservesMessageDatesAcrossBeijingMidnight(t *testing.T) {
	before := time.Date(2026, 9, 29, 15, 59, 0, 0, time.UTC).Unix()
	transcript := buildTranscript("测试", []scored{
		{msg: GroupMessage{Time: before, Text: "明天开会"}},
		{msg: GroupMessage{Time: before + 120, Text: "今天推迟一小时"}},
		{msg: GroupMessage{Text: "时间不明"}},
	})
	for _, want := range []string{"当前北京时间：", "2026-09-29T23:59:00+08:00", "2026-09-30T00:01:00+08:00", "[时间未知]"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript missing %q: %s", want, transcript)
		}
	}
}
