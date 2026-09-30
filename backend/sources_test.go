package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func twoAccountConfig() []SourceConfig {
	return []SourceConfig{{ID: "qq", Name: "QQ", Kind: "napcat", Accounts: []SourceAccount{
		{ID: "school", Name: "校园", Groups: []GroupWatch{{GroupID: 42, GroupName: "校园群", Watch: true}}, Masters: []Master{{UserID: 1}}},
		{ID: "work", Name: "工作", Groups: []GroupWatch{{GroupID: 42, GroupName: "工作群", Watch: false}}, Masters: []Master{{UserID: 2}}},
	}}}
}
func TestSourceDefaultMigrationAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Get().Sources) != 0 || s.Get().OneBot.HTTPBase != "" {
		t.Fatal("new install connects a default source")
	}
	_, err = s.Update(func(c *Config) { c.Sources = twoAccountConfig() })
	if err != nil {
		t.Fatal(err)
	}
	school, work := s.ForAccount("school"), s.ForAccount("work")
	sc := school.Get()
	if _, ok := sc.IsWatched(42); !ok {
		t.Fatal("missing school scope")
	}
	wc := work.Get()
	if _, ok := wc.IsWatched(42); ok {
		t.Fatal("cross-account watch leak")
	}
	_, err = school.Update(func(c *Config) {
		c.Groups[0].GroupName = "changed"
		c.Rules.SenderOverrides = []SenderOverride{{UserID: 1, Level: "muted"}}
	})
	if err != nil {
		t.Fatal(err)
	}
	if work.Get().Groups[0].GroupName != "工作群" || len(work.Get().Rules.SenderOverrides) != 0 {
		t.Fatal("cross-account mutation")
	}
	copy := s.Get()
	copy.Sources[0].Accounts[0].Groups[0].GroupName = "alias"
	if school.Get().Groups[0].GroupName != "changed" {
		t.Fatal("source clone aliases store")
	}
	legacy := defaultConfig()
	legacy.OneBot = OneBotConfig{HTTPBase: "http://localhost:3100", WSURL: "ws://localhost:3101"}
	legacy.Groups = []GroupWatch{{GroupID: 99, Watch: true}}
	b, _ := json.Marshal(legacy)
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	delete(fields, "sources")
	b, _ = json.Marshal(fields)
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	migrated, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	view := migrated.ForAccount(legacyAccountID).Get()
	if len(migrated.Get().Sources) != 1 || len(view.Groups) != 1 || view.Groups[0].GroupID != 99 || view.OneBot != legacy.OneBot {
		t.Fatal("migration lost legacy settings")
	}
}
func TestNoticeArchiveSeparatesSameQQGroupAcrossAccounts(t *testing.T) {
	store, err := NewNoticeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, b := store.ForAccount("qq", "school"), store.ForAccount("qq", "work")
	notice := NoticeRecord{GroupID: 42, MessageIDs: []int64{7}, Result: LLMResult{Summary: "同一群号同一消息号"}}
	for _, view := range []*NoticeStore{a, b} {
		if ok, err := view.Append(notice); !ok || err != nil {
			t.Fatalf("append=%v %v", ok, err)
		}
	}
	aa, _ := a.QueryContext(context.Background(), NoticeQuery{})
	bb, _ := b.Query(NoticeQuery{})
	all, _ := store.Query(NoticeQuery{})
	if len(aa) != 1 || len(bb) != 1 || len(all) != 2 || aa[0].ID == bb[0].ID || aa[0].AccountID != "school" || bb[0].AccountID != "work" {
		t.Fatalf("scope failure: %v %v", aa, bb)
	}
}
func TestSourceRuntimeDeletionAndDisabledDefault(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(func(c *Config) { c.Sources = twoAccountConfig() })
	if err != nil {
		t.Fatal(err)
	}
	bot, hub := NewOneBot(), NewHub()
	pipe := NewPipeline(s, bot, hub)
	defer pipe.Shutdown()
	api := NewAPI(s, bot, hub, pipe)
	api.sources = NewSourceManager(api)
	defer api.sources.Shutdown()
	api.sources.Reconcile()
	a, ok := api.sources.Account("school")
	b, bok := api.sources.Account("work")
	if !ok || !bok || a.Bot == b.Bot || a.Pipeline == b.Pipeline || a.Bot.Connected() {
		t.Fatal("accounts share runtime or disabled connection started")
	}
	_, err = s.Update(func(c *Config) { c.Sources[0].Accounts = c.Sources[0].Accounts[1:] })
	if err != nil {
		t.Fatal(err)
	}
	api.sources.Reconcile()
	if _, ok = api.sources.Account("school"); ok {
		t.Fatal("deleted runtime retained")
	}
	if !a.Pipeline.stopped || !a.Assistant.stopped {
		t.Fatal("deleted runtime not stopped")
	}
}

func TestSourceSavePreservesConcurrentScopedSettings(t *testing.T) {
	a := newTestAPI(t)
	defer a.pipe.Shutdown()
	if _, err := a.store.Update(func(c *Config) { c.Sources = twoAccountConfig() }); err != nil {
		t.Fatal(err)
	}
	cookie := sessionCookieFor(t, a)
	draft := redactedSources(a.store.Get().Sources)
	draft[0].Accounts[0].Name = "renamed"
	body, _ := json.Marshal(map[string]any{"sources": draft})
	start := make(chan struct{})
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			if _, err := a.store.ForAccount("school").Update(func(c *Config) {
				c.Groups[0].GroupName = fmt.Sprintf("group-%d", i)
				c.Masters = []Master{{UserID: int64(i + 100)}}
				c.OneBot.Token = fmt.Sprintf("token-%d", i)
				c.Rules.QuietWindowSec = i + 100
			}); err != nil {
				errors <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			w := serveAPI(a, http.MethodPost, "/api/sources", body, cookie)
			if w.Code != 200 {
				errors <- fmt.Errorf("source save returned %d", w.Code)
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	cfg := a.store.ForAccount("school").Get()
	if cfg.Groups[0].GroupName != "group-59" || cfg.Masters[0].UserID != 159 || cfg.OneBot.Token != "token-59" || cfg.Rules.QuietWindowSec != 159 {
		t.Fatal("source save discarded a completed scoped update")
	}
	if a.store.Get().Sources[0].Accounts[0].Name != "renamed" || a.store.ForAccount("work").Get().Groups[0].GroupName != "工作群" {
		t.Fatal("source edits were not applied within the intended account")
	}
	// An identity cannot move channels, even when the submitted form is stale.
	before := a.store.Get()
	draft[0].ID = "another-source"
	body, _ = json.Marshal(map[string]any{"sources": draft})
	if w := serveAPI(a, http.MethodPost, "/api/sources", body, cookie); w.Code != 400 || !reflect.DeepEqual(before, a.store.Get()) {
		t.Fatal("moving an account modified its existing configuration")
	}
}

func TestAccountUnrelatedEditsPreserveInheritedRules(t *testing.T) {
	a := newTestAPI(t)
	defer a.pipe.Shutdown()
	if _, err := a.store.Update(func(c *Config) {
		c.Sources = twoAccountConfig()
		c.Rules.UrgentKeywords = []string{"global"}
	}); err != nil {
		t.Fatal(err)
	}
	school := a.store.ForAccount("school")
	for _, edit := range []func(*Config){
		func(c *Config) { c.Groups[0].Watch = false },
		func(c *Config) { c.Masters = []Master{{UserID: 10}} },
		func(c *Config) { c.OneBot.Token = "updated-token" },
	} {
		if _, err := school.Update(edit); err != nil {
			t.Fatal(err)
		}
		if a.store.Get().Sources[0].Accounts[0].Rules != nil {
			t.Fatal("unrelated edit pinned inherited rules")
		}
	}
	if _, err := a.store.Update(func(c *Config) { c.Rules.QuietWindowSec = 137 }); err != nil {
		t.Fatal(err)
	}
	if school.Get().Rules.QuietWindowSec != 137 {
		t.Fatal("account stopped inheriting global rule changes")
	}
	// In-place slice edits must create an override without mutating the global
	// rules or another inheriting account through a shared backing array.
	if _, err := school.Update(func(c *Config) { c.Rules.UrgentKeywords[0] = "school" }); err != nil {
		t.Fatal(err)
	}
	if a.store.Get().Rules.UrgentKeywords[0] != "global" || a.store.ForAccount("work").Get().Rules.UrgentKeywords[0] != "global" {
		t.Fatal("account rule edit leaked into inherited global rules")
	}
	if _, err := a.store.Update(func(c *Config) { c.Rules.QuietWindowSec = 151 }); err != nil {
		t.Fatal(err)
	}
	if school.Get().Rules.QuietWindowSec != 137 || a.store.ForAccount("work").Get().Rules.QuietWindowSec != 151 {
		t.Fatal("explicit account override or other account inheritance changed")
	}
}

func TestGlobalModelEditsDiscardInFlightAccountNotifications(t *testing.T) {
	for _, path := range []string{"/api/llm", "/api/jev"} {
		t.Run(path, func(t *testing.T) {
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
				fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":2,\"title\":\"Notice\",\"summary\":\"Scheduled meeting\"}"}}]}`)
			}))
			defer llm.Close()
			defer unblock()
			var sent atomic.Int32
			push := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sent.Add(1); fmt.Fprint(w, `{}`) }))
			defer push.Close()
			onebot := httptest.NewServer(http.NotFoundHandler())
			defer onebot.Close()
			a := newTestAPI(t)
			defer a.pipe.Shutdown()
			if _, err := a.store.Update(func(c *Config) {
				c.Enabled = true
				c.Sources = twoAccountConfig()
				account := &c.Sources[0].Accounts[0]
				account.Enabled = true
				account.Masters = nil
				account.OneBot = OneBotConfig{HTTPBase: onebot.URL, WSURL: strings.Replace(onebot.URL, "http", "ws", 1)}
				c.LLM.Enabled, c.LLM.BaseURL, c.LLM.APIKey = true, llm.URL, "fixture"
				c.NotificationTargets = []NotificationTarget{{ID: "push", Name: "push", Kind: "ntfy", Enabled: true, URL: push.URL, Topic: "notice"}}
			}); err != nil {
				t.Fatal(err)
			}
			a.sources = NewSourceManager(a)
			a.sources.Reconcile()
			defer a.sources.Shutdown()
			runtime, _ := a.sources.Account("school")
			cfg := runtime.Store.Get()
			go func() {
				defer close(done)
				runtime.Pipeline.process(cfg, 42, "school", []scored{classify(&cfg, msg(42, 7, "member", "Meeting tomorrow"))}, false)
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("account model request did not start")
			}
			w := serveAPI(a, http.MethodPost, path, []byte(`{"enabled":false}`), sessionCookieFor(t, a))
			unblock()
			if w.Code != 200 {
				t.Fatalf("model config update returned %d", w.Code)
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("account model request did not finish")
			}
			if sent.Load() != 0 || len(runtime.Pipeline.hub.RecentEscalations()) != 0 {
				t.Fatal("model edit allowed the old account task to publish a notification")
			}
		})
	}
}
