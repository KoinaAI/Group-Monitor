package main

import (
	"context"
	"net/http"
	"reflect"
	"sync"
)

type AccountRuntime struct {
	SourceID  string
	AccountID string
	Config    SourceAccount
	Store     *Store
	Bot       *OneBot
	Pipeline  *Pipeline
	Assistant *Assistant
	API       *API
	cancel    context.CancelFunc
}

func (r *AccountRuntime) shutdown() {
	r.cancel()
	r.Assistant.Shutdown()
	r.Pipeline.Shutdown()
	r.Bot.Shutdown()
}

type SourceManager struct {
	root        *API
	mu          sync.RWMutex
	reconcileMu sync.Mutex
	accounts    map[string]*AccountRuntime
	stopped     bool
}

func NewSourceManager(root *API) *SourceManager {
	return &SourceManager{root: root, accounts: map[string]*AccountRuntime{}}
}
func (m *SourceManager) Account(id string) (*AccountRuntime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.accounts[id]
	return a, ok
}
func (m *SourceManager) Reconcile() {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	if m.stopped {
		return
	}
	cfg := m.root.store.Get()
	wanted := map[string]bool{}
	for _, source := range cfg.Sources {
		for _, account := range source.Accounts {
			wanted[account.ID] = true
			old, exists := m.Account(account.ID)
			if exists && old.SourceID == source.ID && old.Config.OneBot == account.OneBot && old.Config.Enabled == account.Enabled {
				old.Pipeline.Reconcile(old.Store.Get())
				continue
			}
			if exists {
				m.mu.Lock()
				delete(m.accounts, account.ID)
				m.mu.Unlock()
				old.shutdown()
			}
			view := m.root.store.ForAccount(account.ID)
			bot, hub := NewOneBot(), NewHub()
			notices := m.root.notices.ForAccount(source.ID, account.ID)
			pipe := NewPipeline(view, bot, hub)
			pipe.SetNoticeStore(notices)
			assistant := NewAssistant(view, bot, hub, notices)
			child := NewAPI(view, bot, hub, pipe)
			child.sessions = m.root.sessions
			child.notices = notices
			ctx, cancel := context.WithCancel(context.Background())
			runtime := &AccountRuntime{SourceID: source.ID, AccountID: account.ID, Config: account, Store: view, Bot: bot, Pipeline: pipe, Assistant: assistant, API: child, cancel: cancel}
			bot.onEvent = func(gm GroupMessage) { gm.SourceID = source.ID; gm.AccountID = account.ID; pipe.Ingest(gm) }
			bot.onPrivate = func(pm PrivateMessage) { assistant.Submit(pm) }
			m.mu.Lock()
			m.accounts[account.ID] = runtime
			m.mu.Unlock()
			if account.Enabled {
				bot.Reconfigure(account.OneBot)
				go watchConnection(ctx, bot, hub)
			}
		}
	}
	m.mu.Lock()
	removed := []*AccountRuntime{}
	for id, a := range m.accounts {
		if !wanted[id] {
			delete(m.accounts, id)
			removed = append(removed, a)
		}
	}
	m.mu.Unlock()
	for _, a := range removed {
		a.shutdown()
	}
}
func (m *SourceManager) Shutdown() {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	m.stopped = true
	m.mu.Lock()
	old := m.accounts
	m.accounts = map[string]*AccountRuntime{}
	m.mu.Unlock()
	for _, a := range old {
		a.shutdown()
	}
}

func redactedSources(src []SourceConfig) []SourceConfig {
	out := cloneSources(src)
	for i := range out {
		for j := range out[i].Accounts {
			out[i].Accounts[j].OneBot.Token = ""
		}
	}
	return out
}
func (a *API) handleSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, redactedSources(a.store.Get().Sources))
	case http.MethodPost:
		var body struct {
			Sources []SourceConfig `json:"sources"`
		}
		if decodeJSON(w, r, &body, maxJSONBody) != nil {
			writeErr(w, 400, "invalid sources")
			return
		}
		cfg, err := a.store.updateSources(body.Sources)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if a.sources != nil {
			a.sources.Reconcile()
		}
		writeJSON(w, 200, redactedSources(cfg.Sources))
	default:
		writeErr(w, 405, "method not allowed")
	}
}
func (a *API) handleSourceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "method not allowed")
		return
	}
	out := []map[string]any{}
	for _, s := range a.store.Get().Sources {
		for _, ac := range s.Accounts {
			row := map[string]any{"sourceId": s.ID, "accountId": ac.ID, "connected": false, "selfId": int64(0)}
			if a.sources != nil {
				if runtime, ok := a.sources.Account(ac.ID); ok {
					row["connected"] = runtime.Bot.Connected()
					row["selfId"] = runtime.Bot.SelfID()
				}
			}
			out = append(out, row)
		}
	}
	writeJSON(w, 200, out)
}

func (a *API) accountHandler(path string, fallback http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.sources == nil {
			fallback(w, r)
			return
		}
		id := r.URL.Query().Get("accountId")
		scoped := map[string]func(*API) http.HandlerFunc{
			"/api/status":                 func(c *API) http.HandlerFunc { return c.handleStatus },
			"/api/config":                 func(c *API) http.HandlerFunc { return c.handleConfig },
			"/api/groups":                 func(c *API) http.HandlerFunc { return c.handleGroups },
			"/api/groups/watch":           func(c *API) http.HandlerFunc { return c.handleWatch },
			"/api/groups/history":         func(c *API) http.HandlerFunc { return c.handleGroupHistory },
			"/api/groups/file-url":        func(c *API) http.HandlerFunc { return c.handleGroupFileURL },
			"/api/groups/file-download":   func(c *API) http.HandlerFunc { return c.handleGroupFileDownload },
			"/api/groups/media":           func(c *API) http.HandlerFunc { return c.handleGroupMedia },
			"/api/groups/voice":           func(c *API) http.HandlerFunc { return c.handleGroupVoice },
			"/api/masters":                func(c *API) http.HandlerFunc { return c.handleMasters },
			"/api/masters/verify/request": func(c *API) http.HandlerFunc { return c.handleMasterVerifyRequest },
			"/api/masters/verify/confirm": func(c *API) http.HandlerFunc { return c.handleMasterVerifyConfirm },
			"/api/rules":                  func(c *API) http.HandlerFunc { return c.handleRules },
			"/api/onebot":                 func(c *API) http.HandlerFunc { return c.handleOneBotConfig },
			"/api/test-notify":            func(c *API) http.HandlerFunc { return c.handleTestNotify },
			"/api/lookup":                 func(c *API) http.HandlerFunc { return c.handleLookup },
			"/api/logs":                   func(c *API) http.HandlerFunc { return c.handleLogs },
			"/api/escalations":            func(c *API) http.HandlerFunc { return c.handleEscalations },
			"/api/notices":                func(c *API) http.HandlerFunc { return c.handleNotices },
			"/api/events":                 func(c *API) http.HandlerFunc { return c.handleSSE },
			"/api/auth/status":            func(c *API) http.HandlerFunc { return c.handleAuthStatus },
			"/api/auth/otp/request":       func(c *API) http.HandlerFunc { return c.handleOtpRequest },
			"/api/auth/otp/verify":        func(c *API) http.HandlerFunc { return c.handleOtpVerify },
		}
		selector, usesAccount := scoped[path]
		if !usesAccount {
			fallback(w, r)
			return
		}
		if id == "" {
			switch path {
			case "/api/config", "/api/status", "/api/logs", "/api/escalations", "/api/notices", "/api/events", "/api/rules", "/api/auth/status":
				fallback(w, r)
			default:
				writeErr(w, 400, "请先选择信息源账号")
			}
			return
		}
		runtime, ok := a.sources.Account(id)
		if !ok {
			writeErr(w, 404, "信息源账号不存在")
			return
		}
		selector(runtime.API)(w, r)
		if r.Method == http.MethodPost && (path == "/api/onebot") {
			a.sources.Reconcile()
		}
	}
}

// sameSourceSettings helps callers avoid restarting independent accounts on
// unrelated global settings changes.
func sameSourceSettings(a, b []SourceConfig) bool { return reflect.DeepEqual(a, b) }
