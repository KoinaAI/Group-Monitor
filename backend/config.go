package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
)

// Config is the full persisted state of the notifier. It is written to disk as
// JSON whenever the frontend saves changes, and loaded once on startup.
type Config struct {
	AgentKeys           []AgentKey           `json:"agentKeys"`
	NotificationTargets []NotificationTarget `json:"notificationTargets"`
	Security            SecurityConfig       `json:"security"`
	Sources             []SourceConfig       `json:"sources"`
	SourceID            string               `json:"-"`
	AccountID           string               `json:"-"`
	// OneBot connection to the running NapCat instance.
	OneBot OneBotConfig `json:"onebot"`
	// LLM is the OpenAI-compatible endpoint used to format/filter messages.
	LLM LLMConfig `json:"llm"`

	// Jev is the TypeSafe System-One intent gate that pre-screens each message
	// (with recent context) before it enters the packing queue.
	Jev JevConfig `json:"jev"`
	// Backup uploads only finalized notice shards to an S3-compatible bucket.
	Backup    BackupConfig   `json:"backup"`
	Documents DocumentConfig `json:"documents"`

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

// BackupConfig controls scheduled uploads. Cloudflare R2 uses the S3 API with
// Provider="r2" and Region="auto".
type BackupConfig struct {
	Enabled   bool   `json:"enabled"`
	Provider  string `json:"provider"`
	Cron      string `json:"cron"`
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Timeout   int    `json:"timeoutSec"`
}

type Master struct {
	UserID   int64  `json:"userId"`
	Nickname string `json:"nickname"`
	// MinLevel: only escalations at or above this urgency reach this master.
	// 0 = everything useful, 1 = normal+, 2 = important+, 3 = urgent only.
	MinLevel int `json:"minLevel"`
	// Kind gates privilege. "" (default) / "full" = a full master: receives
	// escalations AND is a login-OTP recipient with bot authority. "notify" =
	// notify-only: still receives escalation/test pushes (a sharing target) but
	// has NO login path — excluded from the login-OTP fan-out and the master
	// count that unlocks OTP login.
	Kind string `json:"kind,omitempty"`
}

// IsFull reports whether m carries full privileges (login-OTP eligible). Empty
// Kind means full, so configs written before notify-only existed keep working.
func (m Master) IsFull() bool { return m.Kind != "notify" }

// FullMasters returns only the masters eligible to receive the login OTP.
func (c Config) FullMasters() []Master {
	out := make([]Master, 0, len(c.Masters))
	for _, m := range c.Masters {
		if m.IsFull() {
			out = append(out, m)
		}
	}
	return out
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
	parent    *Store
	accountID string
	path      string
	mu        sync.RWMutex
	cfg       Config
}

const (
	maxGroups          = 1000
	maxMasters         = 100
	maxSenderOverrides = 1000
	maxUrgentKeywords  = 100
	maxKeywordBytes    = 256
	maxTextBytes       = 4096
	maxModelBytes      = 256
	maxTokenBytes      = 4096
	maxURLBytes        = 2048
	maxLLMTimeoutSec   = 120
	maxJevContext      = 20
	maxQuietWindowSec  = 24 * 60 * 60
	maxHoldSec         = 7 * 24 * 60 * 60
)

func defaultConfig() Config {
	return Config{
		Sources: []SourceConfig{},
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
			Enabled: true,
			BaseURL: "https://api.typesafe.ai/v1/systemone",
			// APIKey is intentionally empty: it's a secret and must not be baked
			// into source. Supply it via config.json (gitignored) or the /api/jev
			// endpoint. With no key the gate fails open (buffers, doesn't drop).
			APIKey:    "",
			Model:     "jev-latest",
			Threshold: 0.6,
			ContextN:  6,
			Timeout:   10,
		},
		Backup: BackupConfig{
			Provider: "r2",
			Cron:     "0 3 * * *",
			Region:   "auto",
			Timeout:  60,
		},
		Documents: defaultDocumentConfig(),
		Masters:   []Master{},
		Groups:    []GroupWatch{},
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
			return s, s.save(s.cfg)
		}
		return nil, err
	}
	// Start from defaults so newly-added fields are populated even when the
	// on-disk file predates them, then overlay the saved values.
	s.cfg = defaultConfig()
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(b, &fields)
	_, hasSources := fields["sources"]
	if !hasSources && (s.cfg.OneBot.HTTPBase != "" || s.cfg.OneBot.WSURL != "") {
		rules := cloneRules(s.cfg.Rules)
		s.cfg.Sources = []SourceConfig{{ID: "legacy-napcat", Name: "NapCat", Kind: "napcat", Accounts: []SourceAccount{{ID: legacyAccountID, Name: "原有账号", Enabled: true, OneBot: s.cfg.OneBot, Groups: s.cfg.Groups, Masters: s.cfg.Masters, Rules: &rules}}}}
		s.cfg.OneBot, s.cfg.Groups, s.cfg.Masters = OneBotConfig{}, []GroupWatch{}, []Master{}
	}
	if err := validateConfig(s.cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if !hasSources {
		if err := s.save(s.cfg); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Get() Config {
	if s.parent != nil {
		return accountConfig(s.parent.Get(), s.accountID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

// Update persists a private copy before publishing it to readers. A failed
// write must leave the in-memory configuration consistent with the file.
func (s *Store) Update(fn func(*Config)) (Config, error) {
	if s.parent != nil {
		return s.updateAccount(fn)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneConfig(s.cfg)
	fn(&next)
	if err := validateConfig(next); err != nil {
		return cloneConfig(s.cfg), err
	}
	if err := s.save(next); err != nil {
		return cloneConfig(s.cfg), err
	}
	s.cfg = next
	return cloneConfig(next), nil
}

func cloneConfig(c Config) Config {
	c.AgentKeys = cloneAgentKeys(c.AgentKeys)
	c.NotificationTargets = cloneNotificationTargets(c.NotificationTargets)
	c.Sources = cloneSources(c.Sources)
	c.Masters = slices.Clone(c.Masters)
	c.Groups = slices.Clone(c.Groups)
	c.Rules.UrgentKeywords = slices.Clone(c.Rules.UrgentKeywords)
	c.Rules.SenderOverrides = slices.Clone(c.Rules.SenderOverrides)
	return c
}

func (s *Store) save(cfg Config) error {
	tmp := s.path + ".tmp"
	b, err := json.MarshalIndent(cfg, "", "  ")
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

func validURL(raw string, schemes ...string) bool {
	if raw == "" || len(raw) > maxURLBytes {
		return raw == ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	for _, scheme := range schemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return true
		}
	}
	return false
}

func validText(s string, max int) bool {
	return len(s) <= max && !strings.ContainsAny(s, "\x00\r\n")
}

// validateConfig protects every persistence path, including direct Store users.
// Zero values that historically meant "use the default" are normalized by the
// API handlers before Update; persisted values themselves must stay bounded.
func validateConfig(c Config) error {
	if err := validateAgentKeys(c.AgentKeys); err != nil {
		return err
	}
	if err := validateNotificationTargets(c.NotificationTargets); err != nil {
		return err
	}
	if err := validateSecurityConfig(c.Security); err != nil {
		return err
	}
	if err := validateSources(c); err != nil {
		return err
	}
	if !validURL(c.OneBot.HTTPBase, "http", "https") || !validURL(c.OneBot.WSURL, "ws", "wss") {
		return fmt.Errorf("invalid OneBot URL")
	}
	if len(c.OneBot.Token) > maxTokenBytes {
		return fmt.Errorf("OneBot token is too long")
	}
	if len(c.Masters) > maxMasters {
		return fmt.Errorf("too many masters")
	}
	seenMasters := make(map[int64]struct{}, len(c.Masters))
	for _, m := range c.Masters {
		if m.UserID <= 0 || m.MinLevel < 0 || m.MinLevel > 3 || !validText(m.Nickname, maxTextBytes) {
			return fmt.Errorf("invalid master")
		}
		if m.Kind != "" && m.Kind != "full" && m.Kind != "notify" {
			return fmt.Errorf("invalid master kind")
		}
		if _, ok := seenMasters[m.UserID]; ok {
			return fmt.Errorf("duplicate master %d", m.UserID)
		}
		seenMasters[m.UserID] = struct{}{}
	}
	if len(c.Groups) > maxGroups {
		return fmt.Errorf("too many groups")
	}
	seenGroups := make(map[int64]struct{}, len(c.Groups))
	for _, g := range c.Groups {
		if g.GroupID <= 0 || !validText(g.GroupName, maxTextBytes) {
			return fmt.Errorf("invalid group")
		}
		if _, ok := seenGroups[g.GroupID]; ok {
			return fmt.Errorf("duplicate group %d", g.GroupID)
		}
		seenGroups[g.GroupID] = struct{}{}
	}
	if len(c.Rules.UrgentKeywords) > maxUrgentKeywords {
		return fmt.Errorf("too many urgent keywords")
	}
	for _, kw := range c.Rules.UrgentKeywords {
		if !validText(kw, maxKeywordBytes) {
			return fmt.Errorf("invalid urgent keyword")
		}
	}
	if len(c.Rules.SenderOverrides) > maxSenderOverrides {
		return fmt.Errorf("too many sender overrides")
	}
	seenOverrides := make(map[int64]struct{}, len(c.Rules.SenderOverrides))
	for _, o := range c.Rules.SenderOverrides {
		if o.UserID <= 0 || !validText(o.Note, maxTextBytes) || (o.Level != "vip" && o.Level != "normal" && o.Level != "muted") {
			return fmt.Errorf("invalid sender override")
		}
		if _, ok := seenOverrides[o.UserID]; ok {
			return fmt.Errorf("duplicate sender override %d", o.UserID)
		}
		seenOverrides[o.UserID] = struct{}{}
	}
	if c.Rules.QuietWindowSec < 1 || c.Rules.QuietWindowSec > maxQuietWindowSec {
		return fmt.Errorf("quiet window out of range")
	}
	if c.Rules.MaxHoldSec < 0 || c.Rules.MaxHoldSec > maxHoldSec {
		return fmt.Errorf("max hold out of range")
	}
	if err := validateLLMConfig(c.LLM); err != nil {
		return fmt.Errorf("LLM: %w", err)
	}
	if err := validateJevConfig(c.Jev); err != nil {
		return fmt.Errorf("Jev: %w", err)
	}
	if err := validateBackupConfig(c.Backup); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := validateDocumentConfig(c.Documents); err != nil {
		return fmt.Errorf("documents: %w", err)
	}
	return nil
}

func validateLLMConfig(c LLMConfig) error {
	if c.Enabled && c.BaseURL == "" {
		return fmt.Errorf("enabled LLM requires a base URL")
	}
	if !validURL(c.BaseURL, "http", "https") || len(c.APIKey) > maxTokenBytes || !validText(c.Model, maxModelBytes) {
		return fmt.Errorf("invalid endpoint, key or model")
	}
	if c.Timeout < 1 || c.Timeout > maxLLMTimeoutSec || c.MaxTok < 64 || c.MaxTok > 8192 || c.Temp < 0 || c.Temp > 2 {
		return fmt.Errorf("invalid timeout, token or temperature")
	}
	return nil
}

func validateJevConfig(c JevConfig) error {
	if c.Enabled && c.BaseURL == "" {
		return fmt.Errorf("enabled Jev requires a base URL")
	}
	if !validURL(c.BaseURL, "http", "https") || len(c.APIKey) > maxTokenBytes || !validText(c.Model, maxModelBytes) {
		return fmt.Errorf("invalid endpoint, key or model")
	}
	if c.Threshold < 0 || c.Threshold > 1 || c.ContextN < 0 || c.ContextN > maxJevContext || c.Timeout < 1 || c.Timeout > maxLLMTimeoutSec {
		return fmt.Errorf("invalid threshold, context or timeout")
	}
	return nil
}

func validateBackupConfig(c BackupConfig) error {
	if c.Provider == "" {
		c.Provider = "r2"
	}
	if c.Provider != "r2" && c.Provider != "s3" {
		return fmt.Errorf("unsupported provider")
	}
	if _, err := parseCron(c.Cron); err != nil {
		return fmt.Errorf("invalid cron: %w", err)
	}
	if c.Endpoint != "" {
		u, err := url.Parse(c.Endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(c.Endpoint) > maxURLBytes {
			return fmt.Errorf("endpoint must be an HTTPS URL without credentials, query or fragment")
		}
	}
	if len(c.Bucket) > 256 || len(c.Prefix) > maxURLBytes || len(c.AccessKey) > maxTokenBytes || len(c.SecretKey) > maxTokenBytes {
		return fmt.Errorf("bucket, prefix or credentials too long")
	}
	if !validText(c.Bucket, 256) || strings.ContainsAny(c.Bucket, "/\\ ?#") || c.Bucket == "." || c.Bucket == ".." || !validText(c.Prefix, maxURLBytes) || strings.Contains(c.Prefix, "..") || strings.Contains(c.Prefix, "\\") || !validText(c.Region, 256) || !validText(c.AccessKey, maxTokenBytes) || !validText(c.SecretKey, maxTokenBytes) {
		return fmt.Errorf("invalid bucket, prefix, region or credentials")
	}
	if c.Enabled && (c.Endpoint == "" || c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "") {
		return fmt.Errorf("enabled backup requires endpoint, bucket and credentials")
	}
	if c.Timeout < 1 || c.Timeout > maxLLMTimeoutSec {
		return fmt.Errorf("invalid timeout")
	}
	return nil
}
