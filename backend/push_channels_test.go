package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBroadcastPushProvidersScopeAndPartialFailure(t *testing.T) {
	var ntfy, bark atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Error("bad payload")
		}
		if r.URL.Path == "/push" {
			bark.Add(1)
			if p["device_key"] != "bark-secret" || p["body"] != "内容" {
				t.Error("bad Bark payload")
			}
			fmt.Fprint(w, `{"code":200}`)
			return
		}
		ntfy.Add(1)
		if r.Header.Get("Authorization") != "Bearer ntfy-secret" || p["topic"] != "notices" || p["priority"] != float64(4) {
			t.Error("bad ntfy payload")
		}
		fmt.Fprint(w, `{"id":"ok"}`)
	}))
	defer srv.Close()
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer fail.Close()
	cfg := Config{AccountID: "school", NotificationTargets: []NotificationTarget{
		{ID: "n", Kind: "ntfy", Enabled: true, URL: srv.URL, Topic: "notices", Token: "ntfy-secret", MinLevel: 1, AccountIDs: []string{"school"}},
		{ID: "b", Kind: "bark", Enabled: true, URL: srv.URL, DeviceKey: "bark-secret", MinLevel: 2},
		{ID: "failed", Kind: "ntfy", Enabled: true, URL: fail.URL, Topic: "notices"},
		{ID: "wrong-account", Kind: "ntfy", Enabled: true, URL: srv.URL, AccountIDs: []string{"work"}},
		{ID: "too-low", Kind: "bark", Enabled: true, URL: srv.URL, MinLevel: 3},
	}}
	sent, failed := broadcastPushContext(context.Background(), cfg, "标题", "内容", 2)
	if sent != 2 || len(failed) != 1 || failed[0] != "failed" || ntfy.Load() != 1 || bark.Load() != 1 {
		t.Fatalf("sent=%d failed=%v", sent, failed)
	}
	redacted, _ := json.Marshal(redactedNotificationTargets(cfg.NotificationTargets))
	if strings.Contains(string(redacted), "secret") {
		t.Fatal("secret leaked")
	}
}
func TestPushRejectsRedirectAndBarkApplicationError(t *testing.T) {
	var leaked atomic.Bool
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer dst.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dst.URL, 307) }))
	defer srv.Close()
	if sendPushContext(context.Background(), NotificationTarget{Kind: "ntfy", URL: srv.URL, Token: "secret"}, "title", "body", 1) == nil || leaked.Load() {
		t.Fatal("followed redirect")
	}
	bark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"code":400,"message":"secret"}`) }))
	defer bark.Close()
	err := sendPushContext(context.Background(), NotificationTarget{Kind: "bark", URL: bark.URL}, "title", "body", 1)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("Bark error mishandled")
	}
}

func TestNotificationAPISecretsAndAuthentication(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	if w := serveAPI(a, "GET", "/api/notifications", nil, nil); w.Code != 401 {
		t.Fatal("notification configuration is public")
	}
	body := []byte(`{"targets":[{"id":"phone","name":"手机","kind":"bark","enabled":true,"url":"https://api.day.app","deviceKey":"device-secret","minLevel":1}]}`)
	w := serveAPI(a, "POST", "/api/notifications", body, cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "device-secret") {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	body = []byte(`{"targets":[{"id":"phone","name":"手机","kind":"bark","enabled":true,"url":"https://api.day.app","deviceKey":"","minLevel":2}]}`)
	w = serveAPI(a, "POST", "/api/notifications", body, cookie)
	if w.Code != 200 || a.store.Get().NotificationTargets[0].DeviceKey != "device-secret" {
		t.Fatal("blank input erased device key")
	}
	if w = serveAPI(a, "GET", "/api/config", nil, cookie); strings.Contains(w.Body.String(), "device-secret") {
		t.Fatal("config leaked device key")
	}
}
