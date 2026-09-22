package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestAPI(t *testing.T) *API {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	ob := NewOneBot()
	hub := NewHub()
	return NewAPI(store, ob, hub, NewPipeline(store, ob, hub))
}

func verifyOTP(t *testing.T, a *API, code string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"code": code})
	r := httptest.NewRequest("POST", "/api/auth/otp/verify", bytes.NewReader(b))
	w := httptest.NewRecorder()
	a.handleOtpVerify(w, r)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v (%s)", err, w.Body.String())
	}
	return out
}

func TestOTPVerify(t *testing.T) {
	a := newTestAPI(t)

	// No code minted yet.
	if got := verifyOTP(t, a, "123456"); got["ok"] != false {
		t.Fatalf("expected ok=false with no code, got %v", got)
	}

	// Correct code succeeds and is consumed (one-time use).
	a.otp.code = "654321"
	a.otp.expires = time.Now().Add(otpTTL)
	a.otp.attempts = 0
	if got := verifyOTP(t, a, "000000"); got["ok"] != false {
		t.Fatalf("wrong code should fail, got %v", got)
	}
	if a.otp.attempts != 1 {
		t.Fatalf("wrong code should bump attempts, got %d", a.otp.attempts)
	}
	if got := verifyOTP(t, a, "654321"); got["ok"] != true {
		t.Fatalf("correct code should succeed, got %v", got)
	}
	if got := verifyOTP(t, a, "654321"); got["ok"] != false {
		t.Fatalf("code should be consumed after success, got %v", got)
	}

	// Expired code is rejected.
	a.otp.code = "111111"
	a.otp.expires = time.Now().Add(-time.Second)
	a.otp.attempts = 0
	if got := verifyOTP(t, a, "111111"); got["ok"] != false {
		t.Fatalf("expired code should fail, got %v", got)
	}

	// Too many attempts invalidates the code.
	a.otp.code = "222222"
	a.otp.expires = time.Now().Add(otpTTL)
	a.otp.attempts = otpMaxAttempts
	if got := verifyOTP(t, a, "222222"); got["ok"] != false {
		t.Fatalf("exhausted attempts should fail even with right code, got %v", got)
	}
	if a.otp.code != "" {
		t.Fatalf("code should be cleared after exhausting attempts")
	}
}

func TestGenOTPFormat(t *testing.T) {
	for i := 0; i < 200; i++ {
		c, err := genOTP()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 6 {
			t.Fatalf("otp %q not 6 digits", c)
		}
		for _, r := range c {
			if r < '0' || r > '9' {
				t.Fatalf("otp %q has non-digit", c)
			}
		}
	}
}
