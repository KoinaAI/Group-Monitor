package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// SecurityConfig is persisted with the private configuration. PasswordHash is
// write-only; API configuration responses must remove it.
type SecurityConfig struct {
	Initialized  bool   `json:"initialized"`
	PasswordHash string `json:"passwordHash,omitempty"`
}

func validateSecurityConfig(c SecurityConfig) error {
	if c.PasswordHash == "" {
		if c.Initialized {
			return fmt.Errorf("initialized instance requires a password")
		}
		return nil
	}
	if _, err := bcrypt.Cost([]byte(c.PasswordHash)); err != nil {
		return fmt.Errorf("invalid password hash")
	}
	return nil
}

type setupState struct {
	mu    sync.Mutex
	token string
}

func (a *API) setupRequired() bool { return !a.store.Get().Security.Initialized }

// initializeSetup only writes the claim token to the process log, never to the
// Hub (which is visible in the dashboard) or persisted configuration.
func (a *API) initializeSetup() {
	a.setup = &setupState{}
	if !a.setupRequired() {
		return
	}
	token := strings.TrimSpace(os.Getenv("NAP_SETUP_TOKEN"))
	if token == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			log.Printf("初始化凭据生成失败；请重启服务后重试")
			return
		}
		token = base64.RawURLEncoding.EncodeToString(buf)
	}
	a.setup.token = token
	log.Printf("讯枢尚未初始化，请在初始化页面输入一次性设置令牌：%s", token)
}

func (a *API) registerSetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/setup/status", a.handleSetupStatus)
	mux.HandleFunc("/api/setup", a.handleSetup)
}

func (a *API) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	required := a.setupRequired()
	writeJSON(w, http.StatusOK, map[string]bool{"required": required, "tokenRequired": required})
}

type setupRequest struct {
	SetupToken string          `json:"setupToken"`
	Password   string          `json:"password"`
	LLM        *LLMConfig      `json:"llm,omitempty"`
	Sources    *[]SourceConfig `json:"sources,omitempty"`
}

var errSetupComplete = errors.New("instance is already initialized")

type setupValidationError struct{ err error }

func (e setupValidationError) Error() string { return e.err.Error() }

// completeSetup uses the Store lock so two simultaneous claims, including
// different API instances sharing this Store, cannot overwrite credentials.
// Omitted options preserve existing deployments during migration.
func (s *Store) completeSetup(body setupRequest, hash string) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Security.Initialized {
		return Config{}, errSetupComplete
	}
	next := cloneConfig(s.cfg)
	next.Security = SecurityConfig{Initialized: true, PasswordHash: hash}
	if body.LLM != nil {
		llm := *body.LLM
		if llm.Timeout == 0 {
			llm.Timeout = next.LLM.Timeout
		}
		if llm.MaxTok == 0 {
			llm.MaxTok = next.LLM.MaxTok
		}
		if llm.Model == "" {
			llm.Model = next.LLM.Model
		}
		next.LLM = llm
	}
	if body.Sources != nil {
		next.Sources = append(next.Sources, cloneSources(*body.Sources)...)
	}
	if err := validateConfig(next); err != nil {
		return Config{}, setupValidationError{err}
	}
	if err := s.save(next); err != nil {
		return Config{}, err
	}
	s.cfg = cloneConfig(next)
	return cloneConfig(next), nil
}

func (a *API) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.setupRequired() {
		writeErr(w, http.StatusConflict, "初始化已完成")
		return
	}
	var body setupRequest
	if err := decodeJSON(w, r, &body, maxJSONBody); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if a.setup == nil {
		writeErr(w, http.StatusServiceUnavailable, "初始化凭据不可用，请重启服务")
		return
	}
	a.setup.mu.Lock()
	defer a.setup.mu.Unlock()
	if !a.setupRequired() {
		writeErr(w, http.StatusConflict, "初始化已完成")
		return
	}
	if a.setup.token == "" {
		writeErr(w, http.StatusServiceUnavailable, "初始化凭据不可用，请重启服务")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.SetupToken), []byte(a.setup.token)) != 1 {
		writeErr(w, http.StatusForbidden, "设置令牌不正确，请查看服务启动日志")
		return
	}
	if len(body.Password) < 10 || len(body.Password) > 72 {
		writeErr(w, http.StatusBadRequest, "密码长度须为 10–72 字节")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "密码保存失败")
		return
	}
	cfg, err := a.store.completeSetup(body, string(hash))
	if err != nil {
		var validation setupValidationError
		switch {
		case errors.Is(err, errSetupComplete):
			writeErr(w, http.StatusConflict, "初始化已完成")
		case errors.As(err, &validation):
			writeErr(w, http.StatusBadRequest, validation.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "保存初始化配置失败")
		}
		return
	}
	a.setup.token = ""
	if a.sources != nil {
		a.sources.Reconcile()
	} else if a.pipe != nil {
		a.pipe.Reconcile(cfg)
	}
	a.hub.Log("info", 0, "", "初始化完成，已启用密码登录")
	a.issueSession(w, r)
}
