package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---- session store ----

func TestSessionStoreLifecycle(t *testing.T) {
	s := newSessionStore(time.Hour)
	tok, err := s.create()
	if err != nil {
		t.Fatal(err)
	}
	if !s.valid(tok) {
		t.Fatal("fresh token should be valid")
	}
	if s.valid("") || s.valid("nope") {
		t.Fatal("empty/unknown token must be invalid")
	}
	s.revoke(tok)
	if s.valid(tok) {
		t.Fatal("revoked token must be invalid")
	}
}

func TestSessionExpiryPurges(t *testing.T) {
	s := newSessionStore(time.Hour)
	tok, _ := s.create()
	s.mu.Lock()
	s.sessions[tok] = time.Now().Add(-time.Second) // force expiry
	s.mu.Unlock()
	if s.valid(tok) {
		t.Fatal("expired token must be invalid")
	}
	s.mu.Lock()
	_, ok := s.sessions[tok]
	s.mu.Unlock()
	if ok {
		t.Fatal("expired token should be purged on validate")
	}
}

func TestSessionZeroTTLDefaults(t *testing.T) {
	if s := newSessionStore(0); s.ttl != defaultSessTTL {
		t.Fatalf("zero ttl should default to %s, got %s", defaultSessTTL, s.ttl)
	}
}

// ---- requireAuth gating ----

func sessionCookieFor(t *testing.T, a *API) *http.Cookie {
	t.Helper()
	tok, err := a.sessions.create()
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookie, Value: tok}
}

func TestRequireAuthGate(t *testing.T) {
	a := newTestAPI(t)
	called := false
	h := a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// No cookie → 401, handler never runs.
	r := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without cookie, got %d", w.Code)
	}
	if called {
		t.Fatal("handler must not run without auth")
	}

	// Valid cookie → handler runs.
	r = httptest.NewRequest("GET", "/api/status", nil)
	r.AddCookie(sessionCookieFor(t, a))
	w = httptest.NewRecorder()
	h(w, r)
	if w.Code != 200 || !called {
		t.Fatalf("want handler to run with valid cookie, code=%d called=%v", w.Code, called)
	}
}

// ---- OTP verify issues a session ----

func TestOTPVerifyIssuesSession(t *testing.T) {
	a := newTestAPI(t)
	a.otp.code = "424242"
	a.otp.expires = time.Now().Add(otpTTL)

	b, _ := json.Marshal(map[string]string{"code": "424242"})
	r := httptest.NewRequest("POST", "/api/auth/otp/verify", bytes.NewReader(b))
	w := httptest.NewRecorder()
	a.handleOtpVerify(w, r)

	if w.Code != 200 {
		t.Fatalf("verify returned %d", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("successful OTP verify must set a session cookie")
	}
	if !a.sessions.valid(cookie.Value) {
		t.Fatal("issued session cookie must name a live session")
	}
	if !cookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Error("session cookie should be SameSite=Lax")
	}
}

// ---- break-glass password ----

func postPassword(t *testing.T, a *API, pw string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"password": pw})
	r := httptest.NewRequest("POST", "/api/auth/password", bytes.NewReader(b))
	w := httptest.NewRecorder()
	a.handlePassword(w, r)
	return w
}

func TestPasswordLoginWhenOTPUnavailable(t *testing.T) {
	a := newTestAPI(t)
	a.password = "hunter2" // ob offline in tests ⇒ break-glass allowed

	w := postPassword(t, a, "hunter2")
	if w.Code != 200 {
		t.Fatalf("password login returned %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["ok"] != true {
		t.Fatalf("correct password should succeed, got %v", out)
	}
	ok := false
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && a.sessions.valid(c.Value) {
			ok = true
		}
	}
	if !ok {
		t.Fatal("password login must issue a valid session cookie")
	}
}

func TestPasswordNotConfigured(t *testing.T) {
	a := newTestAPI(t) // a.password == ""
	if w := postPassword(t, a, "whatever"); w.Code != http.StatusForbidden {
		t.Fatalf("want 403 when no password configured, got %d", w.Code)
	}
}

func TestPasswordRefusedWhenOTPAvailable(t *testing.T) {
	a := newTestAPI(t)
	a.password = "hunter2"
	// Simulate OTP being deliverable: NapCat online + a master configured.
	a.ob.connected.Store(true)
	a.store.Update(func(c *Config) { c.Masters = []Master{{UserID: 1}} })

	if w := postPassword(t, a, "hunter2"); w.Code != http.StatusForbidden {
		t.Fatalf("password must be refused when OTP available, got %d", w.Code)
	}
}

func TestPasswordLockout(t *testing.T) {
	a := newTestAPI(t)
	a.password = "correct"
	for i := 0; i < pwMaxAttempts; i++ {
		if w := postPassword(t, a, "wrong"); w.Code != 200 {
			t.Fatalf("attempt %d: unexpected code %d", i, w.Code)
		}
	}
	// Locked out now: even the correct password is turned away with 429.
	if w := postPassword(t, a, "correct"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 after lockout, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- public auth status ----

func TestAuthStatusReflectsAvailability(t *testing.T) {
	a := newTestAPI(t)
	a.password = "pw"

	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	a.handleAuthStatus(w, r)

	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if out["authed"] != false {
		t.Error("unauthed request should report authed=false")
	}
	if out["passwordConfigured"] != true {
		t.Error("passwordConfigured should be true")
	}
	// Offline in tests ⇒ OTP unavailable, break-glass password available.
	if out["passwordAvailable"] != true {
		t.Error("password should be available when OTP can't be delivered")
	}
	if out["otpAvailable"] != false {
		t.Error("otp should be unavailable when offline")
	}
}
