package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
