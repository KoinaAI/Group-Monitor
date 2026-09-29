package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPipelineLLMVerdictAndMasterThreshold(t *testing.T) {
	pipe, store := testPipe(t, func(c *Config) {
		c.Masters = []Master{{UserID: 1, MinLevel: 1}, {UserID: 2, MinLevel: 2}, {UserID: 3, MinLevel: 3}}
		c.LLM.Enabled = true
	})
	notices, err := NewNoticeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pipe.SetNoticeStore(notices)
	jev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"answers":{"important":{"type":"noul","noul":0.9}}}`)
	}))
	defer jev.Close()
	if _, err := store.Update(func(c *Config) { c.Jev.Enabled = true; c.Jev.APIKey = "test"; c.Jev.BaseURL = jev.URL }); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(store, pipe.ob, pipe.hub, pipe)
	f := newOneBotFixture(t, a)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":1,\"title\":\"开会\",\"summary\":\"明日开会\"}"}}]}`)
	}))
	defer llm.Close()
	cfg := store.Get()
	cfg.LLM.BaseURL = llm.URL
	source := msg(100, 7, "owner", "明日开会")
	source.MessageID = 77
	sc := classify(&cfg, source)
	pipe.process(cfg, 100, "班群", []scored{sc}, false)
	sent := f.sent()
	recipients := map[int64]bool{}
	for _, item := range sent {
		recipients[item.userID] = strings.Contains(item.text, "开会")
	}
	if len(sent) != 2 || !recipients[1] || !recipients[2] || recipients[3] {
		t.Fatalf("owner notice recipients=%+v", sent)
	}
	escs := pipe.hub.RecentEscalations()
	if len(escs) != 1 || escs[0].(map[string]any)["level"] != 2 || escs[0].(map[string]any)["notified"] != 2 {
		t.Fatalf("escalation=%v", escs)
	}
	rows, err := notices.Query(NoticeQuery{GroupID: 100, Limit: 10})
	if err != nil || len(rows) != 1 || len(rows[0].MessageIDs) != 1 || rows[0].MessageIDs[0] != 77 {
		t.Fatalf("durable notice rows=%+v err=%v", rows, err)
	}
}

func TestJevOnlyArchiveDoesNotKeepAcknowledgements(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		score := 0.9
		if strings.Contains(string(body), "收到") {
			score = 0.1
		}
		fmt.Fprintf(w, `{"answers":{"important":{"type":"noul","noul":%f}}}`, score)
	}))
	defer srv.Close()
	pipe, store := testPipe(t, func(c *Config) { c.Jev.Enabled = true; c.Jev.APIKey = "key"; c.Jev.BaseURL = srv.URL })
	defer pipe.Shutdown()
	notices, _ := NewNoticeStore(t.TempDir())
	pipe.SetNoticeStore(notices)
	cfg := store.Get()
	pipe.process(cfg, 100, "班群", []scored{classify(&cfg, msg(100, 7, "admin", "周五提交材料")), classify(&cfg, msg(100, 8, "member", "收到"))}, false)
	rows, err := notices.Query(NoticeQuery{GroupID: 100})
	if err != nil || len(rows) != 1 || strings.Contains(rows[0].Result.Summary, "收到") {
		t.Fatalf("noise archived: %+v %v", rows, err)
	}
}

func TestPipelineSuppressAndUrgentFallback(t *testing.T) {
	pipe, store := testPipe(t, func(c *Config) { c.Masters = []Master{{UserID: 1, MinLevel: 3}} })
	a := NewAPI(store, pipe.ob, pipe.hub, pipe)
	f := newOneBotFixture(t, a)
	cfg := store.Get()
	sc := classify(&cfg, msg(100, 7, "member", "普通聊天"))
	pipe.process(cfg, 100, "班群", []scored{sc}, false)
	if len(f.sent()) != 0 || len(pipe.hub.RecentEscalations()) != 0 {
		t.Fatal("ordinary batch without LLM was pushed")
	}
	urgent := classify(&cfg, msg(100, 8, "member", "紧急安排"))
	pipe.process(cfg, 100, "班群", []scored{sc, urgent}, true)
	if len(f.sent()) != 1 || len(pipe.hub.RecentEscalations()) != 1 {
		t.Fatalf("urgent fallback: sent=%v esc=%v", f.sent(), pipe.hub.RecentEscalations())
	}
}

func TestPipelineUrgentOverridesLLMUsefulFalse(t *testing.T) {
	pipe, store := testPipe(t, func(c *Config) {
		c.Masters = []Master{{UserID: 1, MinLevel: 3}}
		c.LLM.Enabled = true
	})
	a := NewAPI(store, pipe.ob, pipe.hub, pipe)
	f := newOneBotFixture(t, a)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"useful\":false,\"level\":0,\"title\":\"噪音\",\"reason\":\"无用\"}"}}]}`)
	}))
	defer llm.Close()
	cfg := store.Get()
	cfg.LLM.BaseURL = llm.URL
	sc := classify(&cfg, msg(100, 7, "member", "紧急故障"))
	pipe.process(cfg, 100, "班群", []scored{sc}, true)
	if len(f.sent()) != 1 || len(pipe.hub.RecentEscalations()) != 1 {
		t.Fatalf("urgent useful=false was suppressed: sent=%v escalations=%v", f.sent(), pipe.hub.RecentEscalations())
	}
}

func TestPipelineDropsInFlightResultAfterPause(t *testing.T) {
	pipe, store := testPipe(t, func(c *Config) {
		c.Masters = []Master{{UserID: 1, MinLevel: 1}}
		c.LLM.Enabled = true
	})
	a := NewAPI(store, pipe.ob, pipe.hub, pipe)
	f := newOneBotFixture(t, a)
	started := make(chan struct{})
	release := make(chan struct{})
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":2,\"title\":\"通知\"}"}}]}`)
	}))
	defer llm.Close()
	cfg := store.Get()
	cfg.LLM.BaseURL = llm.URL
	go pipe.process(cfg, 100, "班群", []scored{classify(&cfg, msg(100, 7, "member", "通知"))}, false)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("LLM request did not start")
	}
	next, err := store.Update(func(c *Config) { c.Enabled = false })
	if err != nil {
		t.Fatal(err)
	}
	pipe.Reconcile(next)
	close(release)
	time.Sleep(100 * time.Millisecond)
	if len(f.sent()) != 0 || len(pipe.hub.RecentEscalations()) != 0 {
		t.Fatalf("paused pipeline delivered in-flight result: sent=%v escalations=%v", f.sent(), pipe.hub.RecentEscalations())
	}
}

func TestPipelineJevContextAndSelfMessage(t *testing.T) {
	pipe, _ := testPipe(t, func(c *Config) { c.Jev.ContextN = 2; c.Rules.QuietWindowSec = 60 })
	pipe.ob.selfID.Store(99)
	pipe.Ingest(msg(100, 99, "member", "self"))
	if pipe.pending(100) != 0 {
		t.Fatal("self message entered buffer")
	}
	for i := 0; i < 4; i++ {
		pipe.Ingest(msg(100, int64(i+1), "member", "ordinary"))
	}
	pipe.mu.Lock()
	recent := append([]GroupMessage(nil), pipe.recent[100]...)
	pipe.mu.Unlock()
	if len(recent) != 3 || recent[0].UserID != 2 || recent[2].UserID != 4 {
		t.Fatalf("rolling Jev context=%+v", recent)
	}
	if got := jevContextN(Config{Jev: JevConfig{ContextN: 99}}); got != 20 {
		t.Fatalf("context cap=%d", got)
	}
}

func TestArchiveRequiresSuccessfulJevNoticeVerdict(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		score  float64
		key    string
		want   int
	}{
		{"chatter", 200, 0.1, "key", 0},
		{"outage", 502, 0, "key", 0},
		{"unconfigured", 200, 0.9, "", 0},
		{"notice", 200, 0.9, "key", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"answers":{"important":{"type":"noul","noul":%f}}}`, tc.score)
			}))
			defer srv.Close()
			pipe, store := testPipe(t, func(c *Config) { c.Jev.Enabled = true; c.Jev.BaseURL = srv.URL; c.Jev.APIKey = tc.key })
			defer pipe.Shutdown()
			notices, err := NewNoticeStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			pipe.SetNoticeStore(notices)
			cfg := store.Get()
			// A keyword-triggered urgent fallback is not by itself an archive verdict.
			pipe.process(cfg, 100, "班群", []scored{classify(&cfg, msg(100, 7, "member", "紧急：内容待判定"))}, true)
			rows, err := notices.Query(NoticeQuery{GroupID: 100})
			if err != nil || len(rows) != tc.want {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
		})
	}
}
