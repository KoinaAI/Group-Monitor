package main

import (
	"fmt"
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
func (s *Store) updateAccount(fn func(*Config)) (Config, error) {
	c, err := s.parent.Update(func(root *Config) {
		view := accountConfig(*root, s.accountID)
		fn(&view)
		for i := range root.Sources {
			for j := range root.Sources[i].Accounts {
				a := &root.Sources[i].Accounts[j]
				if a.ID != s.accountID {
					continue
				}
				a.OneBot, a.Groups, a.Masters = view.OneBot, view.Groups, view.Masters
				r := cloneRules(view.Rules)
				a.Rules = &r
			}
		}
	})
	return accountConfig(c, s.accountID), err
}
