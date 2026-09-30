package main

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
)

// SourceConfig groups independent accounts under an extensible channel kind.
type SourceConfig struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Accounts []SourceAccount `json:"accounts"`
}
type SourceAccount struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Enabled bool         `json:"enabled"`
	OneBot  OneBotConfig `json:"onebot"`
	Groups  []GroupWatch `json:"groups"`
	Masters []Master     `json:"masters"`
	Rules   *Rules       `json:"rules,omitempty"`
}

const legacyAccountID = "legacy-default"

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func cloneSources(src []SourceConfig) []SourceConfig {
	out := slices.Clone(src)
	for i := range out {
		out[i].Accounts = slices.Clone(out[i].Accounts)
		for j := range out[i].Accounts {
			a := &out[i].Accounts[j]
			a.Groups = slices.Clone(a.Groups)
			a.Masters = slices.Clone(a.Masters)
			if a.Rules != nil {
				r := cloneRules(*a.Rules)
				a.Rules = &r
			}
		}
	}
	return out
}
func cloneRules(r Rules) Rules {
	r.UrgentKeywords = slices.Clone(r.UrgentKeywords)
	r.SenderOverrides = slices.Clone(r.SenderOverrides)
	return r
}
func validateSources(c Config) error {
	if len(c.Sources) > 16 {
		return fmt.Errorf("too many sources")
	}
	sources, accounts := map[string]bool{}, map[string]bool{}
	for _, s := range c.Sources {
		if !identifierPattern.MatchString(s.ID) || sources[s.ID] || !validText(s.Name, 128) || s.Name == "" || s.Kind != "napcat" {
			return fmt.Errorf("invalid or duplicate source; supported kind is napcat")
		}
		sources[s.ID] = true
		if len(s.Accounts) > 16 {
			return fmt.Errorf("too many accounts in source")
		}
		for _, a := range s.Accounts {
			if !identifierPattern.MatchString(a.ID) || accounts[a.ID] || !validText(a.Name, 128) || a.Name == "" {
				return fmt.Errorf("invalid or duplicate account")
			}
			accounts[a.ID] = true
			if len(accounts) > 32 {
				return fmt.Errorf("too many accounts")
			}
			if a.Enabled && (a.OneBot.HTTPBase == "" || a.OneBot.WSURL == "") {
				return fmt.Errorf("enabled account requires HTTP and WebSocket URLs")
			}
			scoped := c
			scoped.Sources = nil
			scoped.OneBot, scoped.Groups, scoped.Masters = a.OneBot, a.Groups, a.Masters
			if a.Rules != nil {
				scoped.Rules = *a.Rules
			}
			if err := validateConfig(scoped); err != nil {
				return fmt.Errorf("account %s: %w", a.ID, err)
			}
		}
	}
	return nil
}
func accountConfig(c Config, id string) Config {
	for _, s := range c.Sources {
		for _, a := range s.Accounts {
			if a.ID == id {
				c.SourceID, c.AccountID = s.ID, a.ID
				c.OneBot, c.Groups, c.Masters = a.OneBot, a.Groups, a.Masters
				c.Enabled = c.Enabled && a.Enabled
				if a.Rules != nil {
					c.Rules = *a.Rules
				}
				return c
			}
		}
	}
	c.SourceID, c.AccountID = "", id
	c.OneBot, c.Groups, c.Masters = OneBotConfig{}, nil, nil
	c.Enabled = false
	return c
}
func (s *Store) ForAccount(id string) *Store { return &Store{parent: s, accountID: id} }

// Source management owns labels and connections. Preserve the latest scoped
// settings and blank credentials under the same lock as the config write.
func (s *Store) updateSources(sources []SourceConfig) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneConfig(s.cfg)
	next.Sources = cloneSources(sources)
	for i := range next.Sources {
		for j := range next.Sources[i].Accounts {
			account := &next.Sources[i].Accounts[j]
			account.Groups, account.Masters, account.Rules = []GroupWatch{}, []Master{}, nil
			for _, source := range s.cfg.Sources {
				for _, old := range source.Accounts {
					if old.ID != account.ID {
						continue
					}
					if source.ID != next.Sources[i].ID {
						return Config{}, fmt.Errorf("account cannot move between sources")
					}
					if account.OneBot.Token == "" {
						account.OneBot.Token = old.OneBot.Token
					}
					account.Groups, account.Masters = slices.Clone(old.Groups), slices.Clone(old.Masters)
					if old.Rules != nil {
						rules := cloneRules(*old.Rules)
						account.Rules = &rules
					}
				}
			}
		}
	}
	if err := validateConfig(next); err != nil {
		return Config{}, err
	}
	if err := s.save(next); err != nil {
		return Config{}, err
	}
	s.cfg = next
	return cloneConfig(next), nil
}

func (s *Store) updateAccount(fn func(*Config)) (Config, error) {
	c, err := s.parent.Update(func(root *Config) {
		view := accountConfig(*root, s.accountID)
		beforeRules := cloneRules(view.Rules)
		view.Rules = cloneRules(view.Rules)
		fn(&view)
		for i := range root.Sources {
			for j := range root.Sources[i].Accounts {
				a := &root.Sources[i].Accounts[j]
				if a.ID != s.accountID {
					continue
				}
				a.OneBot, a.Groups, a.Masters = view.OneBot, view.Groups, view.Masters
				if a.Rules != nil || !reflect.DeepEqual(beforeRules, view.Rules) {
					r := cloneRules(view.Rules)
					a.Rules = &r
				}
			}
		}
	})
	return accountConfig(c, s.accountID), err
}
