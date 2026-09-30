package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func agentRequest(a *API, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, req)
	return w
}
func TestAgentKeyMCPAndDirectAPIIsolation(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	_, err := a.store.Update(func(c *Config) { c.Sources = twoAccountConfig() })
	if err != nil {
		t.Fatal(err)
	}
	a.notices, _ = NewNoticeStore(t.TempDir())
	a.sources = NewSourceManager(a)
	a.sources.Reconcile()
	defer a.sources.Shutdown()
	for _, id := range []string{"school", "work"} {
		_, err := a.notices.ForAccount("qq", id).Append(NoticeRecord{GroupID: 42, Result: LLMResult{Title: id, Summary: id + "-private-notice"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	created := serveAPI(a, "POST", "/api/agent-keys", []byte(`{"name":"test","accountIds":["school"]}`), cookie)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var record struct {
		APIKey string   `json:"apiKey"`
		Key    AgentKey `json:"key"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &record)
	if record.APIKey == "" || record.Key.Hash != "" {
		t.Fatal("key creation contract")
	}
	for _, path := range []string{"/api/mcp", "/api/agent/v1/query"} {
		if w := agentRequest(a, path, "", `{}`); w.Code != 401 {
			t.Fatal("agent API accepted no key")
		}
	}
	cfg := serveAPI(a, "GET", "/api/config", nil, cookie)
	if bytes.Contains(cfg.Body.Bytes(), []byte(record.APIKey)) || bytes.Contains(cfg.Body.Bytes(), []byte(agentKeyHash(record.APIKey))) {
		t.Fatal("config leaked API credentials")
	}
	init := agentRequest(a, "/api/mcp", record.APIKey, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if init.Code != 200 || !strings.Contains(init.Body.String(), "instructions") || !strings.Contains(init.Body.String(), "2025-11-25") {
		t.Fatal(init.Body.String())
	}
	notified := agentRequest(a, "/api/mcp", record.APIKey, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if notified.Code != 202 || notified.Body.Len() != 0 {
		t.Fatal("notification transport contract")
	}
	list := agentRequest(a, "/api/mcp", record.APIKey, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if list.Code != 200 || !strings.Contains(list.Body.String(), "inputSchema") || !strings.Contains(list.Body.String(), "readOnlyHint") {
		t.Fatal(list.Body.String())
	}
	w := agentRequest(a, "/api/mcp", record.APIKey, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"recent_notices","arguments":{"account_id":"school"}}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "school-private-notice") || strings.Contains(w.Body.String(), "work-private-notice") {
		t.Fatal(w.Body.String())
	}
	w = agentRequest(a, "/api/agent/v1/query", record.APIKey, `{"name":"list_sources","arguments":{}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "school") || strings.Contains(w.Body.String(), "work") {
		t.Fatal(w.Body.String())
	}
	w = agentRequest(a, "/api/agent/v1/query", record.APIKey, `{"name":"recent_notices","arguments":{"account_id":"work"}}`)
	if w.Code != 400 || strings.Contains(w.Body.String(), "private-notice") {
		t.Fatal(w.Body.String())
	}
	body, _ := json.Marshal(map[string]string{"id": record.Key.ID})
	if w := serveAPI(a, "POST", "/api/agent-keys/revoke", body, cookie); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := agentRequest(a, "/api/mcp", record.APIKey, `{"jsonrpc":"2.0","id":4,"method":"ping"}`); w.Code != 401 {
		t.Fatal("revoked key still works")
	}
}
func TestMCPTransportOriginAndProtocolErrors(t *testing.T) {
	a := newTestAPI(t)
	_, err := a.store.Update(func(c *Config) { c.AgentKeys = []AgentKey{{ID: "key", Name: "key", Hash: agentKeyHash("test")}} })
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, origin, version, body string
		status                        int
	}{
		{"GET", "", "", "", 405}, {"POST", "https://evil.example", "", `{}`, 403}, {"POST", "", "1900", `{}`, 400},
		{"POST", "", "", `{`, 400}, {"POST", "", "", `{"jsonrpc":"2.0","id":[],"method":"ping"}`, 400},
	} {
		req := httptest.NewRequest(tc.method, "http://app.test/api/mcp", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("MCP-Protocol-Version", tc.version)
		w := httptest.NewRecorder()
		a.Routes().ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Errorf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
}
