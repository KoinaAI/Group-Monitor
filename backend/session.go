package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Authentication now lives entirely in this backend: the (removed) Node frontend
// no longer gates access. A successful login — OTP or the break-glass password —
// mints an in-memory session whose opaque token is stored in an httpOnly cookie.
// Every /api route except the login/status endpoints requires a valid session.
//
// Sessions are intentionally in-memory: a restart logs everyone out, which is an
// acceptable trade for a single-binary local tool and avoids persisting a
// bearer-equivalent secret to disk.

const (
	sessionCookie  = "nap_session"
	defaultSessTTL = 12 * time.Hour
	maxSessTTL     = 30 * 24 * time.Hour
)

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
	ttl      time.Duration
}

func newSessionStore(ttl time.Duration) *sessionStore {
	if ttl <= 0 || ttl > maxSessTTL {
		ttl = defaultSessTTL
	}
	return &sessionStore{sessions: make(map[string]time.Time), ttl: ttl}
}

// create mints a new session token and records its expiry. It also opportunistically
// sweeps expired entries, so the map can't grow without bound across many logins.
func (s *sessionStore) create() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for t, exp := range s.sessions {
		if !now.Before(exp) {
			delete(s.sessions, t)
		}
	}
	s.sessions[token] = now.Add(s.ttl)
	return token, nil
}

// valid reports whether token names a live session, deleting it if expired.
func (s *sessionStore) valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[token]
	if !ok {
		return false
	}
	if !time.Now().Before(exp) {
		delete(s.sessions, token)
		return false
	}
	return true
}

func (s *sessionStore) revoke(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// ---- cookie helpers ----

// isHTTPS reports whether the request reached us over TLS, directly or via a
// terminating reverse proxy. Used to set the Secure attribute only when it won't
// break a plain-HTTP loopback session.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *API) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.sessions.ttl.Seconds()),
	})
}

func (a *API) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func requestToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

func (a *API) authed(r *http.Request) bool {
	return a.sessions.valid(requestToken(r))
}

// issueSession mints a session, sets the cookie and writes the success response.
// Shared by every login path (OTP and break-glass password).
func (a *API) issueSession(w http.ResponseWriter, r *http.Request) {
	token, err := a.sessions.create()
	if err != nil {
		writeErr(w, 500, "创建会话失败")
		return
	}
	a.setSessionCookie(w, r, token)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// requireAuth wraps a handler so it only runs for requests carrying a valid
// session cookie; otherwise it returns 401 without invoking the handler.
func (a *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authed(r) {
			writeErr(w, http.StatusUnauthorized, "未登录或会话已过期")
			return
		}
		next(w, r)
	}
}

// ---- auth status / logout ----

// handleAuthStatus is public: it lets a login UI decide which methods to offer
// and whether the current session is already valid. It never reveals the OTP or
// password, only their availability.
func (a *API) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	// Count only full masters: they alone receive the login OTP, so they alone
	// decide whether OTP login is offered and are what the login page tallies.
	masters := len(a.store.Get().FullMasters())
	online := a.ob.Connected()
	writeJSON(w, 200, map[string]any{
		"authed":             a.authed(r),
		"otpAvailable":       online && masters > 0,
		"passwordConfigured": a.password != "",
		"passwordAvailable":  a.passwordAvailable(),
		"onebotConnected":    online,
		"masters":            masters,
	})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	a.sessions.revoke(requestToken(r))
	a.clearSessionCookie(w, r)
	writeJSON(w, 200, map[string]any{"ok": true})
}
