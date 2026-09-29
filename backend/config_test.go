package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStorePersistsAndLoadsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Rules.QuietWindowSec; got != 120 {
		t.Fatalf("default quiet window = %d, want 120", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	want, err := s.Update(func(c *Config) {
		c.Groups = []GroupWatch{{GroupID: 42, GroupName: "班群", Watch: true}}
		c.Masters = []Master{{UserID: 7, Kind: "notify"}}
		c.Rules.UrgentKeywords = []string{"考试"}
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Get(), want) {
		t.Fatalf("reload differs from saved config\ngot: %#v\nwant: %#v", reloaded.Get(), want)
	}
	if len(reloaded.Get().FullMasters()) != 0 {
		t.Fatal("notify-only master must not receive login OTP")
	}
}

func TestStoreLegacyConfigRetainsNewDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"enabled":false,"rules":{"quietWindowSec":45}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Get()
	if cfg.Enabled || cfg.Rules.QuietWindowSec != 45 || cfg.Rules.MaxHoldSec != 600 || cfg.Jev.Model != "jev-latest" {
		t.Fatalf("legacy overlay lost saved or default fields: %#v", cfg)
	}
	if err := os.WriteFile(path, []byte(`{"enabled":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(path); err == nil {
		t.Fatal("malformed config must fail at startup")
	}
}

func TestStoreCopiesSlicesAndRollsBackFailedSave(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(func(c *Config) {
		c.Masters = []Master{{UserID: 11}}
		c.Groups = []GroupWatch{{GroupID: 22, Watch: true}}
	})
	if err != nil {
		t.Fatal(err)
	}
	before := s.Get()
	copy := s.Get()
	copy.Masters[0].UserID = 99
	copy.Groups[0].Watch = false
	copy.Rules.UrgentKeywords[0] = "changed"
	copy.Rules.SenderOverrides = append(copy.Rules.SenderOverrides, SenderOverride{UserID: 9})
	if !reflect.DeepEqual(s.Get(), before) {
		t.Fatal("Get exposed mutable backing slices")
	}
	// A missing parent directory makes the temporary write fail on every OS,
	// without depending on root privileges or filesystem permissions.
	s.path = filepath.Join(t.TempDir(), "missing", "config.json")
	if _, err := s.Update(func(c *Config) { c.Enabled = false; c.Masters[0].UserID = 44 }); err == nil {
		t.Fatal("expected failed save")
	}
	if !reflect.DeepEqual(s.Get(), before) {
		t.Fatal("failed save changed the live config")
	}
}

func TestGroupWatchAndMasterPrivilege(t *testing.T) {
	c := Config{Groups: []GroupWatch{{GroupID: 1, Watch: true}, {GroupID: 2, Watch: false}},
		Masters: []Master{{UserID: 1}, {UserID: 2, Kind: "full"}, {UserID: 3, Kind: "notify"}}}
	if _, ok := c.IsWatched(1); !ok {
		t.Fatal("watched group missing")
	}
	if _, ok := c.IsWatched(2); ok {
		t.Fatal("disabled group marked watched")
	}
	if _, ok := c.IsWatched(3); ok {
		t.Fatal("unknown group marked watched")
	}
	if got := len(c.FullMasters()); got != 2 {
		t.Fatalf("full masters = %d, want 2", got)
	}
}
