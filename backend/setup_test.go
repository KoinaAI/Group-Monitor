package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const testSetupToken = "local-setup-test-token"
const testSetupPassword = "initial-password-123"

func newSetupAPI(t *testing.T) *API {
	t.Helper()
	t.Setenv("NAP_SETUP_TOKEN", testSetupToken)
	t.Setenv("NAP_PASSWORD", "")
	return newTestAPI(t)
}

func submitSetup(a *API, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(b))
	r.RemoteAddr = "203.0.113.42:1234"
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	return w
}

func readSetupStatus(t *testing.T, a *API) map[string]bool {
	t.Helper()
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	var result map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
		t.Fatalf("setup status: %d %s", w.Code, w.Body.String())
	}
	return result
}

func TestSetupRequiresTokenAndValidPassword(t *testing.T) {
	a := newSetupAPI(t)
	if status := readSetupStatus(t, a); !status["required"] || !status["tokenRequired"] || len(status) != 2 {
		t.Fatalf("new instance status: %v", status)
	}
	for _, tc := range []struct {
		token, password string
		status          int
	}{
		{"", testSetupPassword, 403},
		{"wrong", testSetupPassword, 403},
		{testSetupToken, "short", 400},
		{testSetupToken, strings.Repeat("a", 73), 400},
		{testSetupToken, strings.Repeat("界", 25), 400},
	} {
		w := submitSetup(a, map[string]any{"setupToken": tc.token, "password": tc.password})
		if w.Code != tc.status || len(w.Result().Cookies()) != 0 {
			t.Errorf("invalid setup status = %d, want %d; body = %s", w.Code, tc.status, w.Body.String())
		}
		if a.store.Get().Security.Initialized || a.store.Get().Security.PasswordHash != "" {
			t.Fatal("rejected setup changed credentials")
		}
	}
}

func TestSetupPersistsCredentialsAndLoginSurvivesRestart(t *testing.T) {
	a := newSetupAPI(t)
	w := submitSetup(a, map[string]any{"setupToken": testSetupToken, "password": testSetupPassword})
	if w.Code != 200 {
		t.Fatalf("setup failed: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !a.sessions.valid(cookies[0].Value) {
		t.Fatal("setup must issue a valid HttpOnly session cookie")
	}
	security := a.store.Get().Security
	if !security.Initialized || bcrypt.CompareHashAndPassword([]byte(security.PasswordHash), []byte(testSetupPassword)) != nil {
		t.Fatal("setup must persist a password hash")
	}
	disk, err := os.ReadFile(a.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(disk, []byte(testSetupPassword)) || bytes.Contains(disk, []byte(testSetupToken)) {
		t.Fatal("plaintext password or setup token persisted")
	}
	if a.setup.token != "" {
		t.Fatal("setup token must be consumed")
	}
	if status := readSetupStatus(t, a); status["required"] || status["tokenRequired"] {
		t.Fatalf("completed setup status: %v", status)
	}

	reloaded, err := NewStore(a.store.path)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewAPI(reloaded, a.ob, NewHub(), nil)
	if restarted.setupRequired() || restarted.setup.token != "" {
		t.Fatal("restart must not reopen setup")
	}
	if login := postPassword(t, restarted, testSetupPassword); login.Code != 200 || !strings.Contains(login.Body.String(), `"ok":true`) {
		t.Fatalf("persisted password login failed: %d %s", login.Code, login.Body.String())
	}
	if got := submitSetup(restarted, map[string]any{"setupToken": testSetupToken, "password": "replacement-password"}); got.Code != 409 {
		t.Fatalf("initialized instance was reclaimable: %d", got.Code)
	}
	if restarted.store.Get().Security.PasswordHash != security.PasswordHash {
		t.Fatal("setup overwrite changed saved password")
	}
}

func TestSavedPasswordRemainsAvailableWithOnlineSource(t *testing.T) {
	a := newSetupAPI(t)
	if got := submitSetup(a, map[string]any{"setupToken": testSetupToken, "password": testSetupPassword}); got.Code != 200 {
		t.Fatalf("setup: %s", got.Body.String())
	}
	a.ob.connected.Store(true)
	if _, err := a.store.Update(func(c *Config) { c.Masters = []Master{{UserID: 1}} }); err != nil {
		t.Fatal(err)
	}
	a.password = "legacy-password"
	if !a.passwordAvailable() {
		t.Fatal("configured password must be available with OTP online")
	}
	if got := postPassword(t, a, "legacy-password"); strings.Contains(got.Body.String(), `"ok":true`) {
		t.Fatal("legacy environment password must not override saved credentials")
	}
	if got := postPassword(t, a, testSetupPassword); !strings.Contains(got.Body.String(), `"ok":true`) {
		t.Fatalf("saved password unavailable: %s", got.Body.String())
	}
	w := httptest.NewRecorder()
	a.handleAuthStatus(w, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	var status map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["passwordAvailable"] != true || status["passwordConfigured"] != true || status["setupRequired"] != false {
		t.Fatalf("auth status: %v", status)
	}
}

func TestSetupSavesOptionsWithoutLeakingSecrets(t *testing.T) {
	a := newSetupAPI(t)
	options := map[string]any{
		"setupToken": testSetupToken, "password": testSetupPassword,
		"llm": map[string]any{"enabled": true, "baseUrl": "https://llm.example/v1", "apiKey": "llm-test-secret", "model": "test-model"},
		"sources": []SourceConfig{{ID: "qq", Name: "QQ", Kind: "napcat", Accounts: []SourceAccount{{
			ID: "work", Name: "工作账号", Enabled: false, OneBot: OneBotConfig{HTTPBase: "http://127.0.0.1:3000", WSURL: "ws://127.0.0.1:3001", Token: "source-test-secret"},
		}}}},
	}
	w := submitSetup(a, options)
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	cfg := a.store.Get()
	if cfg.LLM.APIKey != "llm-test-secret" || cfg.LLM.Timeout == 0 || len(cfg.Sources) != 1 || cfg.Sources[0].Accounts[0].OneBot.Token != "source-test-secret" {
		t.Fatal("setup options were not saved")
	}
	configResponse := httptest.NewRecorder()
	a.handleConfig(configResponse, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	logs, _ := json.Marshal(a.hub.RecentLogs())
	for _, secret := range []string{testSetupToken, testSetupPassword, cfg.Security.PasswordHash, "llm-test-secret", "source-test-secret"} {
		if strings.Contains(configResponse.Body.String(), secret) || strings.Contains(w.Body.String(), secret) || bytes.Contains(logs, []byte(secret)) {
			t.Fatalf("API or dashboard log exposed credentials")
		}
	}
	if strings.Contains(configResponse.Body.String(), "passwordHash") {
		t.Fatal("configuration response must omit passwordHash")
	}
}

func TestConcurrentSetupOnlyOneClaimSucceeds(t *testing.T) {
	a := newSetupAPI(t)
	const clients = 8
	responses := make(chan int, clients)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- submitSetup(a, map[string]any{"setupToken": testSetupToken, "password": testSetupPassword}).Code
		}()
	}
	wg.Wait()
	close(responses)
	ok := 0
	for status := range responses {
		if status == 200 {
			ok++
		} else if status != 409 {
			t.Fatalf("unexpected concurrent setup status %d", status)
		}
	}
	if ok != 1 {
		t.Fatalf("successful setup claims = %d, want 1", ok)
	}
}

func TestSetupFailureLeavesInstanceUnclaimed(t *testing.T) {
	a := newSetupAPI(t)
	w := submitSetup(a, map[string]any{
		"setupToken": testSetupToken, "password": testSetupPassword,
		"llm": map[string]any{"enabled": true, "baseUrl": "file:///tmp/llm"},
	})
	if w.Code != 400 || !a.setupRequired() || a.setup.token != testSetupToken {
		t.Fatalf("validation failure claimed instance: %d %s", w.Code, w.Body.String())
	}
	oldPath := a.store.path
	a.store.path = filepath.Join(t.TempDir(), "missing", "config.json")
	w = submitSetup(a, map[string]any{"setupToken": testSetupToken, "password": testSetupPassword})
	if w.Code != 500 || !a.setupRequired() || a.setup.token != testSetupToken {
		t.Fatalf("write failure claimed instance: %d %s", w.Code, w.Body.String())
	}
	a.store.path = oldPath
	if got := submitSetup(a, map[string]any{"setupToken": testSetupToken, "password": testSetupPassword}); got.Code != 200 {
		t.Fatalf("setup could not recover from failure: %s", got.Body.String())
	}
}

func TestSetupMigrationPreservesExistingOptions(t *testing.T) {
	a := newSetupAPI(t)
	if _, err := a.store.Update(func(c *Config) {
		c.LLM.APIKey = "existing-llm-secret"
		c.Groups = []GroupWatch{{GroupID: 456, Watch: true}}
		c.Masters = []Master{{UserID: 123}}
		c.Sources = []SourceConfig{{ID: "qq", Name: "existing", Kind: "napcat", Accounts: []SourceAccount{{ID: "existing", Name: "existing", OneBot: OneBotConfig{Token: "existing-source-secret"}}}}}
	}); err != nil {
		t.Fatal(err)
	}
	if got := submitSetup(a, map[string]any{"setupToken": testSetupToken, "password": testSetupPassword}); got.Code != 200 {
		t.Fatalf("migration setup: %s", got.Body.String())
	}
	cfg := a.store.Get()
	if cfg.LLM.APIKey != "existing-llm-secret" || len(cfg.Groups) != 1 || len(cfg.Masters) != 1 || len(cfg.Sources) != 1 || cfg.Sources[0].Accounts[0].OneBot.Token != "existing-source-secret" {
		t.Fatal("migration setup replaced existing options")
	}
}

func TestSetupGeneratesLocalTokenWhenEnvironmentUnset(t *testing.T) {
	t.Setenv("NAP_SETUP_TOKEN", "")
	a := newTestAPI(t)
	if len(a.setup.token) < 32 {
		t.Fatal("setup token must contain sufficient random entropy")
	}
	status, _ := json.Marshal(readSetupStatus(t, a))
	if bytes.Contains(status, []byte(a.setup.token)) {
		t.Fatal("public status exposed setup token")
	}
}

func TestSetupAddingSourcePreservesMigratedAccounts(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(func(c *Config) {
		c.Sources = []SourceConfig{{ID: "old", Name: "已有渠道", Kind: "napcat", Accounts: []SourceAccount{{ID: "old-account", Name: "原账号"}}}}
	})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	added := []SourceConfig{{ID: "new", Name: "新增渠道", Kind: "napcat", Accounts: []SourceAccount{}}}
	cfg, err := store.completeSetup(setupRequest{Sources: &added}, string(hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 2 || cfg.Sources[0].Accounts[0].ID != "old-account" {
		t.Fatal("setup replaced existing sources")
	}
}
