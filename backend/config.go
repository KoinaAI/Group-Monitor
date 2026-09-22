package main

import (
	"encoding/json"
	"os"
	"sync"
)

// Config is the full persisted state of the notifier. It is written to disk as
// JSON whenever the frontend saves changes, and loaded once on startup.
type Config struct {
	// OneBot connection to the running NapCat instance.
	OneBot OneBotConfig `json:"onebot"`
	// LLM is the OpenAI-compatible endpoint used to format/filter messages.
	LLM LLMConfig `json:"llm"`

	// Jev is the TypeSafe System-One intent gate that pre-screens each message
	// (with recent context) before it enters the packing queue.
	Jev JevConfig `json:"jev"`

	// Masters receive the distilled reminders (their QQ user IDs).
	Masters []Master `json:"masters"`

	// Groups the user has chosen to watch. Only these are processed; any
	// message from a group not listed here (or listed with watch=false) is
	// ignored entirely.
	Groups []GroupWatch `json:"groups"`

	// Rules controls debounce timing, urgency detection and sender levels.
	Rules Rules `json:"rules"`

	// Enabled is the global on/off switch for processing. When false, events
	// are still received (for the live feed) but never buffered or escalated.
	Enabled bool `json:"enabled"`
}

type OneBotConfig struct {
	HTTPBase string `json:"httpBase"` // e.g. http://172.17.0.2:3100
	WSURL    string `json:"wsUrl"`    // e.g. ws://172.17.0.2:3101
	Token    string `json:"token"`    // Bearer / access_token
}

type LLMConfig struct {
	Enabled bool    `json:"enabled"`
	BaseURL string  `json:"baseUrl"` // e.g. http://127.0.0.1:3000/v1
	APIKey  string  `json:"apiKey"`
	Model   string  `json:"model"`
	Timeout int     `json:"timeoutSec"`
	MaxTok  int     `json:"maxTokens"`
	Temp    float64 `json:"temperature"`
	// Prompt/format is system-managed (see systemPrompt in llm.go); the user no
	// longer edits it, so it is intentionally absent here.
}

// JevConfig configures the per-message importance gate (TypeSafe Jev). Each
// non-urgent message is scored with recent context before buffering; only
// messages scoring at/above Threshold enter the packing queue.
type JevConfig struct {
	Enabled   bool    `json:"enabled"`
	BaseURL   string  `json:"baseUrl"` // https://api.typesafe.ai/v1/systemone
	APIKey    string  `json:"apiKey"`
	Model     string  `json:"model"`     // jev-latest
	Threshold float64 `json:"threshold"` // noul >= threshold ⇒ important
	ContextN  int     `json:"contextN"`  // recent messages passed as context
	Timeout   int     `json:"timeoutSec"`
}

type Master struct {
	UserID   int64  `json:"userId"`
	Nickname string `json:"nickname"`
	// MinLevel: only escalations at or above this urgency reach this master.
	// 0 = everything useful, 1 = normal+, 2 = important+, 3 = urgent only.
	MinLevel int `json:"minLevel"`
}

type GroupWatch struct {
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	Watch     bool   `json:"watch"`
}

// SenderOverride pins a specific QQ user to a level regardless of their group
// role, or mutes them entirely.
type SenderOverride struct {
	UserID int64  `json:"userId"`
	Note   string `json:"note"`
	Level  string `json:"level"` // "vip" | "normal" | "muted"
}

type Rules struct {
	// QuietWindowSec: batch normal messages, flushing only after this many
	// seconds of silence in a group. Requirement default: 120 (two minutes).
	QuietWindowSec int `json:"quietWindowSec"`
	// MaxHoldSec caps how long a busy group can defer a flush, so a group that
	// never goes quiet still gets processed. 0 disables the cap.
	MaxHoldSec int `json:"maxHoldSec"`

	// UrgentKeywords bypass the quiet window and trigger an immediate flush.
	UrgentKeywords []string `json:"urgentKeywords"`
	// AtAllUrgent treats an @全体成员 (@everyone) as urgent.
	AtAllUrgent bool `json:"atAllUrgent"`
	// OwnerAdminUrgent treats any message from owner/admin as at least
	// "important", raising its base level.
	ElevateOwnerAdmin bool `json:"elevateOwnerAdmin"`

	// SenderOverrides pin/mute specific users.
	SenderOverrides []SenderOverride `json:"senderOverrides"`
}

// ---- persistence ----

type Store struct {
	path string
	mu   sync.RWMutex
	cfg  Config
}

func defaultConfig() Config {
	return Config{
		OneBot: OneBotConfig{
			HTTPBase: "http://172.17.0.2:3100",
			WSURL:    "ws://172.17.0.2:3101",
			Token:    "agn-onebot-token-2026",
		},
		LLM: LLMConfig{
			Enabled: false,
			BaseURL: "http://127.0.0.1:3000/v1",
			APIKey:  "",
			Model:   "gemini-3.5-flash",
			Timeout: 60,
			// Reasoning models spend tokens on reasoning_content before the
			// answer; a tight cap truncates the JSON verdict. Give it headroom.
			MaxTok: 4096,
			Temp:   0.2,
		},
		Jev: JevConfig{
			Enabled:   true,
			BaseURL:   "https://api.typesafe.ai/v1/systemone",
			APIKey:    "apikey_21093d0a5d87a5134f5b91178edec3171bfd_49a3782ae0a14476a576acccc2e6c123472f465668ba0d4b99576ea8d4260187",
			Model:     "jev-latest",
			Threshold: 0.6,
			ContextN:  6,
			Timeout:   10,
		},
		Masters: []Master{},
		Groups:  []GroupWatch{},
		Rules: Rules{
			QuietWindowSec:    120,
			MaxHoldSec:        600,
			UrgentKeywords:    []string{"紧急", "急", "尽快", "马上", "立刻", "报警", "求助", "重要通知", "@所有人"},
			AtAllUrgent:       true,
			ElevateOwnerAdmin: true,
			SenderOverrides:   []SenderOverride{},
		},
		Enabled: true,
	}
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = defaultConfig()
			return s, s.save()
		}
		return nil, err
	}
	// Start from defaults so newly-added fields are populated even when the
	// on-disk file predates them, then overlay the saved values.
	s.cfg = defaultConfig()
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Update applies fn to a copy under lock, persists, and returns the new config.
func (s *Store) Update(fn func(*Config)) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	if err := s.save(); err != nil {
		return s.cfg, err
	}
	return s.cfg, nil
}

func (s *Store) save() error {
	tmp := s.path + ".tmp"
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// IsWatched reports whether a group is in the watch list with watch enabled.
func (c *Config) IsWatched(groupID int64) (GroupWatch, bool) {
	for _, g := range c.Groups {
		if g.GroupID == groupID {
			return g, g.Watch
		}
	}
	return GroupWatch{}, false
}

func (c *Config) overrideFor(userID int64) (SenderOverride, bool) {
	for _, o := range c.Rules.SenderOverrides {
		if o.UserID == userID {
			return o, true
		}
	}
	return SenderOverride{}, false
}
