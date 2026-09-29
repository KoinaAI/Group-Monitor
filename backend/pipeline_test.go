package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testPipe(t *testing.T, mutate func(*Config)) (*Pipeline, *Store) {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "cfg.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.Update(func(c *Config) {
		c.LLM.Enabled = false // keep tests offline
		c.Jev.Enabled = false // gate off: exercise buffering directly, no network
		c.Enabled = true
		c.Groups = []GroupWatch{{GroupID: 100, GroupName: "测试群", Watch: true}}
		if mutate != nil {
			mutate(c)
		}
	})
	ob := NewOneBot()
	return NewPipeline(store, ob, NewHub()), store
}

func (p *Pipeline) pending(groupID int64) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if b := p.buffers[groupID]; b != nil {
		return len(b.msgs)
	}
	return 0
}

func msg(group, user int64, role, text string) GroupMessage {
	return GroupMessage{GroupID: group, UserID: user, Role: role, Text: text, Nickname: "u", Time: time.Now().Unix()}
}

// ---- classification ----

func TestClassifySenderLevels(t *testing.T) {
	cfg := defaultConfig()
	cfg.Rules.SenderOverrides = []SenderOverride{
		{UserID: 7, Level: "vip"}, {UserID: 8, Level: "muted"},
	}
	cases := []struct {
		user int64
		role string
		want int
	}{
		{1, "owner", LvlOwner},
		{2, "admin", LvlAdmin},
		{3, "member", LvlNormal},
		{7, "member", LvlVIP},  // override beats role
		{8, "owner", LvlMuted}, // mute beats owner role
	}
	for _, c := range cases {
		got := classify(&cfg, msg(100, c.user, c.role, "hi"))
		if got.senderLevel != c.want {
			t.Errorf("user %d role %s: got level %d want %d", c.user, c.role, got.senderLevel, c.want)
		}
	}
}

func TestClassifyUrgency(t *testing.T) {
	cfg := defaultConfig()
	cfg.Rules.UrgentKeywords = []string{"紧急", "报警"}

	if sc := classify(&cfg, msg(100, 1, "member", "这条很普通")); sc.urgent {
		t.Error("normal message should not be urgent")
	}
	if sc := classify(&cfg, msg(100, 1, "member", "紧急：马上处理")); !sc.urgent {
		t.Error("keyword message should be urgent")
	}
	atAll := msg(100, 1, "member", "通知")
	atAll.AtAll = true
	if sc := classify(&cfg, atAll); !sc.urgent {
		t.Error("@all should be urgent when AtAllUrgent on")
	}
	atSelf := msg(100, 1, "member", "找你")
	atSelf.AtSelf = true
	if sc := classify(&cfg, atSelf); !sc.urgent {
		t.Error("@self should always be urgent")
	}
}

// ---- filtering: unwatched groups produce no reaction ----

func TestUnwatchedGroupIgnored(t *testing.T) {
	pipe, _ := testPipe(t, nil)
	pipe.Ingest(msg(999, 1, "member", "unwatched group message")) // 999 not in watch list
	time.Sleep(50 * time.Millisecond)
	if n := pipe.pending(999); n != 0 {
		t.Fatalf("unwatched group should not buffer, got %d pending", n)
	}
}

// ---- debounce: quiet-window batching ----

func TestDebounceBuffersUntilQuiet(t *testing.T) {
	pipe, _ := testPipe(t, func(c *Config) {
		c.Rules.QuietWindowSec = 1 // shrink window for the test
		c.Rules.MaxHoldSec = 0
	})

	pipe.Ingest(msg(100, 1, "member", "first"))
	pipe.Ingest(msg(100, 2, "member", "second"))
	if n := pipe.pending(100); n != 2 {
		t.Fatalf("expected 2 buffered before window elapses, got %d", n)
	}

	// Before the window elapses, still buffered.
	time.Sleep(600 * time.Millisecond)
	if n := pipe.pending(100); n != 2 {
		t.Fatalf("expected still 2 buffered mid-window, got %d", n)
	}

	// After the full quiet window with no new messages, flushed.
	time.Sleep(700 * time.Millisecond)
	if n := pipe.pending(100); n != 0 {
		t.Fatalf("expected flush after quiet window, got %d pending", n)
	}
}

func TestDebounceTimerResetsOnNewMessage(t *testing.T) {
	pipe, _ := testPipe(t, func(c *Config) {
		c.Rules.QuietWindowSec = 1
		c.Rules.MaxHoldSec = 0
	})
	pipe.Ingest(msg(100, 1, "member", "a"))
	time.Sleep(700 * time.Millisecond)      // 0.7s into a 1s window
	pipe.Ingest(msg(100, 2, "member", "b")) // resets the window
	time.Sleep(700 * time.Millisecond)      // 1.4s total, but only 0.7s since reset
	if n := pipe.pending(100); n != 2 {
		t.Fatalf("timer should have reset; expected 2 still buffered, got %d", n)
	}
	time.Sleep(500 * time.Millisecond) // now past the window since last msg
	if n := pipe.pending(100); n != 0 {
		t.Fatalf("expected flush after quiet since last message, got %d", n)
	}
}

// ---- urgent bypasses the window and flushes immediately ----

func TestUrgentFlushesImmediately(t *testing.T) {
	pipe, _ := testPipe(t, func(c *Config) {
		c.Rules.QuietWindowSec = 60 // long window; urgent must not wait for it
		c.Rules.UrgentKeywords = []string{"紧急"}
	})
	pipe.Ingest(msg(100, 1, "member", "normal one"))
	if n := pipe.pending(100); n != 1 {
		t.Fatalf("expected 1 buffered, got %d", n)
	}
	pipe.Ingest(msg(100, 2, "member", "紧急！出事了")) // urgent
	time.Sleep(50 * time.Millisecond)
	// Urgent handling drains the whole buffer immediately (folds pending in).
	if n := pipe.pending(100); n != 0 {
		t.Fatalf("expected buffer drained by urgent message, got %d pending", n)
	}
}

// ---- Jev intent gate: only important messages reach the packing queue ----

func waitPending(p *Pipeline, gid int64, want int) bool {
	for i := 0; i < 60; i++ {
		if p.pending(gid) == want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return p.pending(gid) == want
}

func TestJevGate(t *testing.T) {
	var mu sync.Mutex
	noul := 0.9
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n, st := noul, status
		mu.Unlock()
		if st != 200 {
			w.WriteHeader(st)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"important": map[string]any{"type": "noul", "noul": n}},
		})
	}))
	defer srv.Close()

	pipe, _ := testPipe(t, func(c *Config) {
		c.Jev.Enabled = true
		c.Jev.BaseURL = srv.URL
		c.Jev.APIKey = "test"
		c.Jev.Threshold = 0.6
		c.Jev.Timeout = 5
		c.Jev.ContextN = 3
		c.Rules.QuietWindowSec = 60 // long: nothing flushes during the test
		c.Rules.MaxHoldSec = 0
	})

	// Important message clears the threshold and is buffered.
	pipe.Ingest(msg(100, 1, "member", "明天下午三点开评审会"))
	if !waitPending(pipe, 100, 1) {
		t.Fatalf("important message should be buffered, got %d", pipe.pending(100))
	}

	// Below-threshold message is dropped, not buffered (count stays 1).
	mu.Lock()
	noul = 0.1
	mu.Unlock()
	pipe.Ingest(msg(100, 2, "member", "收到"))
	time.Sleep(400 * time.Millisecond)
	if n := pipe.pending(100); n != 1 {
		t.Fatalf("unimportant message should be dropped, expected 1 buffered, got %d", n)
	}

	// On a Jev error the gate fails open and buffers the message anyway.
	mu.Lock()
	status = 500
	mu.Unlock()
	pipe.Ingest(msg(100, 3, "member", "又一条"))
	if !waitPending(pipe, 100, 2) {
		t.Fatalf("Jev error should fail open and buffer, got %d", pipe.pending(100))
	}
}

func TestJevWithoutAPIKeyBypassesExternalCall(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	pipe, _ := testPipe(t, func(c *Config) {
		c.Jev.Enabled = true
		c.Jev.APIKey = ""
		c.Jev.BaseURL = srv.URL
		c.Rules.QuietWindowSec = 60
	})
	pipe.Ingest(msg(100, 1, "member", "没有密钥时应直接进入缓冲"))
	if !waitPending(pipe, 100, 1) {
		t.Fatalf("missing-key Jev should bypass gate, pending=%d", pipe.pending(100))
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("missing-key Jev made %d external calls", got)
	}
}

// ---- max-hold cap forces a flush even on a busy group ----

func TestMaxHoldCap(t *testing.T) {
	pipe, _ := testPipe(t, func(c *Config) {
		c.Rules.QuietWindowSec = 10 // never reached within the test
		c.Rules.MaxHoldSec = 1      // but capped at 1s from first message
	})
	// Keep the group "busy" so the quiet window keeps resetting. Because the
	// quiet window (10s) is never reached, only the max-hold cap can trigger a
	// flush; each non-urgent flush emits one "suppress" activity log (LLM off).
	done := make(chan struct{})
	go func() {
		for i := 0; i < 8; i++ {
			pipe.Ingest(msg(100, int64(i), "member", "spam"))
			time.Sleep(150 * time.Millisecond)
		}
		close(done)
	}()
	<-done
	time.Sleep(200 * time.Millisecond)

	// ~1.4s elapsed with a 10s quiet window: without the max-hold cap nothing
	// would ever have flushed. Prove at least one forced flush happened.
	flushes := 0
	for _, l := range pipe.hub.RecentLogs() {
		if l.Level == "suppress" {
			flushes++
		}
	}
	if flushes == 0 {
		t.Fatalf("max-hold cap did not force any flush despite a busy stream")
	}
}
