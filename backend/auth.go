package main

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// otpState holds the single in-flight login OTP. Authentication lives entirely
// in this backend now: it mints, delivers (via master DMs) and verifies the
// code, then issues a session on success. The code never leaves the backend.
type otpState struct {
	mu               sync.Mutex
	code             string
	expires          time.Time
	attempts         int
	lastReq          time.Time // throttles the (pre-auth) request endpoint
	lastReqByClient  map[string]time.Time
	attemptsByClient map[string]int
	unavailableUntil time.Time
}

const (
	otpTTL         = 5 * time.Minute
	otpMaxAttempts = 5
	otpCooldown    = 30 * time.Second // min gap between OTP-request calls
)

// pwState throttles break-glass password attempts so the (pre-auth) password
// endpoint can't be brute-forced.
type pwState struct {
	mu          sync.Mutex
	attempts    int
	lockedUntil time.Time
	clients     map[string]pwClientState
}

type pwClientState struct {
	attempts    int
	lockedUntil time.Time
}

const (
	pwMaxAttempts  = 5
	pwLockout      = 15 * time.Minute
	maxAuthClients = 1024
)

func authClientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func pruneAuthClients[T any](m map[string]T) {
	for len(m) > maxAuthClients {
		for key := range m {
			delete(m, key)
			break
		}
	}
}

func genOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// handleOtpRequest mints a fresh 6-digit code and DMs it to every master. It is
// only meaningful when NapCat is online and at least one master exists; the
// break-glass password takes over otherwise. Returns how many masters were
// reached — never the code itself. Rate-limited: this endpoint is pre-auth and
// triggers outbound DMs, so it can't be spammed.
func (a *API) handleOtpRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	if !a.ob.Connected() {
		writeErr(w, 409, "NapCat 未在线，无法发送验证码")
		return
	}
	masters := a.store.Get().FullMasters()
	if len(masters) == 0 {
		writeErr(w, 409, "尚未配置主人，无法发送验证码")
		return
	}

	// Throttle before doing any work: reject if the last request was too recent.
	clientKey := authClientKey(r)
	a.otp.mu.Lock()
	if a.otp.lastReqByClient == nil {
		a.otp.lastReqByClient = make(map[string]time.Time)
	}
	lastReq := a.otp.lastReqByClient[clientKey]
	if since := time.Since(lastReq); !lastReq.IsZero() && since < otpCooldown {
		wait := int((otpCooldown - since).Seconds()) + 1
		a.otp.mu.Unlock()
		writeErr(w, 429, fmt.Sprintf("请求过于频繁，请 %d 秒后再试", wait))
		return
	}
	a.otp.lastReq = time.Now()
	a.otp.lastReqByClient[clientKey] = a.otp.lastReq
	pruneAuthClients(a.otp.lastReqByClient)
	a.otp.mu.Unlock()

	code, err := genOTP()
	if err != nil {
		writeErr(w, 500, "生成验证码失败")
		return
	}
	text := fmt.Sprintf("【讯枢】本次登录验证码：%s\n5 分钟内有效，仅可使用一次。若非本人操作，请忽略本条消息。", code)

	sent, failed := 0, []string{}
	for _, m := range masters {
		if err := a.ob.SendPrivateMsg(m.UserID, text); err != nil {
			failed = append(failed, fmt.Sprintf("%d", m.UserID))
			continue
		}
		sent++
	}
	if sent == 0 {
		a.otp.mu.Lock()
		a.otp.code = ""
		a.otp.expires = time.Time{}
		a.otp.attempts = 0
		a.otp.attemptsByClient = nil
		a.otp.unavailableUntil = time.Now().Add(otpTTL)
		a.otp.mu.Unlock()
		writeErr(w, 502, "验证码发送失败，请检查主人 QQ 是否可私聊")
		return
	}

	a.otp.mu.Lock()
	a.otp.code = code
	a.otp.expires = time.Now().Add(otpTTL)
	a.otp.attempts = 0
	a.otp.attemptsByClient = make(map[string]int)
	a.otp.unavailableUntil = time.Time{}
	a.otp.mu.Unlock()

	a.hub.Log("info", 0, "", fmt.Sprintf("已向 %d 位主人发送登录验证码", sent))
	writeJSON(w, 200, map[string]any{"sent": sent, "failed": failed, "ttlSec": int(otpTTL.Seconds())})
}

// handleOtpVerify checks a submitted code in constant time, consuming it on
// success and invalidating it after too many wrong attempts.
func (a *API) handleOtpVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &body, maxAuthBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}

	a.otp.mu.Lock()
	defer a.otp.mu.Unlock()
	clientKey := authClientKey(r)

	if a.otp.code == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "尚未获取验证码，请先点击获取"})
		return
	}
	if time.Now().After(a.otp.expires) {
		a.otp.code = ""
		delete(a.otp.attemptsByClient, clientKey)
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "验证码已过期，请重新获取"})
		return
	}
	attempts := a.otp.attemptsByClient[clientKey]
	if len(a.otp.attemptsByClient) == 0 {
		attempts = a.otp.attempts
	}
	if attempts >= otpMaxAttempts {
		a.otp.code = ""
		delete(a.otp.attemptsByClient, clientKey)
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "尝试次数过多，请重新获取验证码"})
		return
	}

	if subtle.ConstantTimeCompare([]byte(body.Code), []byte(a.otp.code)) == 1 {
		a.otp.code = "" // consume: one-time use
		a.otp.attempts = 0
		delete(a.otp.attemptsByClient, clientKey)
		a.hub.Log("info", 0, "", "已通过验证码登录")
		a.issueSession(w, r)
		return
	}
	attempts++
	if a.otp.attemptsByClient == nil {
		a.otp.attemptsByClient = make(map[string]int)
	}
	a.otp.attemptsByClient[clientKey] = attempts
	a.otp.attempts = attempts
	pruneAuthClients(a.otp.attemptsByClient)
	left := otpMaxAttempts - attempts
	writeJSON(w, 200, map[string]any{"ok": false, "reason": fmt.Sprintf("验证码不正确，还可尝试 %d 次", left)})
}

// A persisted password is always usable, including when an information source
// is online. NAP_PASSWORD retains its legacy emergency-only behavior for older
// deployments which have not completed initialization yet.
func (a *API) passwordAvailable() bool {
	if a.store.Get().Security.PasswordHash != "" {
		return true
	}
	if a.password == "" {
		return false
	}
	if !a.ob.Connected() || len(a.store.Get().FullMasters()) == 0 {
		return true
	}
	a.otp.mu.Lock()
	defer a.otp.mu.Unlock()
	return time.Now().Before(a.otp.unavailableUntil)
}

func (a *API) passwordConfigured() bool {
	return a.store.Get().Security.PasswordHash != "" || a.password != ""
}

// handlePassword verifies the saved password (or legacy emergency password)
// and issues a session, with a per-client lockout after repeated failures.
func (a *API) handlePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	hash := a.store.Get().Security.PasswordHash
	if hash == "" && a.password == "" {
		writeErr(w, 403, "未配置登录密码")
		return
	}
	if !a.passwordAvailable() {
		writeErr(w, 403, "OTP 可用时请使用验证码登录")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body, maxAuthBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}

	clientKey := authClientKey(r)
	a.pw.mu.Lock()
	if a.pw.clients == nil {
		a.pw.clients = make(map[string]pwClientState)
	}
	state := a.pw.clients[clientKey]
	if now := time.Now(); now.Before(state.lockedUntil) {
		wait := int(time.Until(state.lockedUntil).Seconds()) + 1
		a.pw.mu.Unlock()
		writeErr(w, 429, fmt.Sprintf("尝试次数过多，请 %d 秒后再试", wait))
		return
	}
	valid := false
	if hash != "" {
		valid = bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) == nil
	} else {
		valid = subtle.ConstantTimeCompare([]byte(body.Password), []byte(a.password)) == 1
	}
	if !valid {
		state.attempts++
		left := pwMaxAttempts - state.attempts
		if state.attempts >= pwMaxAttempts {
			state.lockedUntil = time.Now().Add(pwLockout)
			state.attempts = 0
			a.pw.clients[clientKey] = state
			pruneAuthClients(a.pw.clients)
			a.pw.mu.Unlock()
			writeJSON(w, 200, map[string]any{"ok": false, "reason": "密码错误次数过多，已临时锁定 15 分钟"})
			return
		}
		a.pw.clients[clientKey] = state
		pruneAuthClients(a.pw.clients)
		a.pw.attempts = state.attempts
		a.pw.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": false, "reason": fmt.Sprintf("密码不正确，还可尝试 %d 次", left)})
		return
	}
	state.attempts = 0
	state.lockedUntil = time.Time{}
	a.pw.clients[clientKey] = state
	a.pw.attempts = 0
	a.pw.mu.Unlock()

	a.hub.Log("info", 0, "", "已通过密码登录")
	a.issueSession(w, r)
}

// ---- master bind OTP ----
//
// Adding a master is a privileged change: the target QQ must prove control of
// the account by reading a one-time code we DM them. This is SEPARATE from the
// login OTP above (shorter lifetime, single named candidate) so an in-flight
// login code and an in-flight bind code never clobber each other. Flow: the
// operator confirms the looked-up avatar/nickname, requests a 3-minute code
// (DM'd to the candidate), then submits it to bind. One bind in flight at a time.
type masterOtpState struct {
	mu       sync.Mutex
	userID   int64  // candidate under verification (0 = none in flight)
	nickname string // resolved at request time, bound verbatim on success
	code     string
	expires  time.Time
	attempts int
	lastReq  time.Time
}

const masterOtpTTL = 3 * time.Minute

// reset clears the in-flight bind state. Caller must hold mu.
func (s *masterOtpState) reset() {
	s.userID = 0
	s.nickname = ""
	s.code = ""
	s.attempts = 0
}

// handleMasterVerifyRequest mints a 3-minute bind code and DMs it to the
// candidate QQ. It refuses if the candidate is already a master, if NapCat is
// offline, or if called too frequently. The resolved nickname is returned so
// the UI can show who the code was sent to; the code itself never leaves here.
func (a *API) handleMasterVerifyRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		UserID int64 `json:"userId"`
	}
	if err := decodeJSON(w, r, &body, maxAuthBody); err != nil || body.UserID <= 0 {
		writeErr(w, 400, "invalid userId")
		return
	}
	if !a.ob.Connected() {
		writeErr(w, 409, "NapCat 未在线，无法发送验证码")
		return
	}
	for _, m := range a.store.Get().Masters {
		if m.UserID == body.UserID {
			writeErr(w, 409, "该 QQ 已是主人")
			return
		}
	}

	a.masterOtp.mu.Lock()
	if since := time.Since(a.masterOtp.lastReq); !a.masterOtp.lastReq.IsZero() && since < otpCooldown {
		wait := int((otpCooldown - since).Seconds()) + 1
		a.masterOtp.mu.Unlock()
		writeErr(w, 429, fmt.Sprintf("请求过于频繁，请 %d 秒后再试", wait))
		return
	}
	a.masterOtp.lastReq = time.Now()
	a.masterOtp.mu.Unlock()
	// Resolve the nickname so the operator sees who was contacted. A lookup
	// failure is non-fatal (the profile may be hidden); binding still works.
	si, _ := a.ob.GetStrangerInfo(body.UserID)

	code, err := genOTP()
	if err != nil {
		writeErr(w, 500, "生成验证码失败")
		return
	}
	text := fmt.Sprintf("【讯枢】有人正将你的 QQ 绑定为主人，验证码：%s\n3 分钟内有效，仅可使用一次。若非本人操作，请忽略本条消息。", code)
	if err := a.ob.SendPrivateMsg(body.UserID, text); err != nil {
		writeErr(w, 502, "验证码发送失败，请确认该 QQ 可私聊")
		return
	}

	a.masterOtp.mu.Lock()
	a.masterOtp.userID = body.UserID
	a.masterOtp.nickname = si.Nickname
	a.masterOtp.code = code
	a.masterOtp.expires = time.Now().Add(masterOtpTTL)
	a.masterOtp.attempts = 0
	a.masterOtp.mu.Unlock()

	a.hub.Log("info", 0, "", fmt.Sprintf("已向候选主人 %d 发送绑定验证码", body.UserID))
	writeJSON(w, 200, map[string]any{"nickname": si.Nickname, "ttlSec": int(masterOtpTTL.Seconds())})
}

// handleMasterVerifyConfirm checks the bind code (constant time) and, on
// success, appends the candidate as a master at the chosen level. Returns the
// updated master list so the UI can refresh without a second round-trip.
func (a *API) handleMasterVerifyConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		UserID   int64  `json:"userId"`
		Code     string `json:"code"`
		MinLevel int    `json:"minLevel"`
		Kind     string `json:"kind"`
	}
	if err := decodeJSON(w, r, &body, maxAuthBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if body.MinLevel < 0 || body.MinLevel > 3 {
		body.MinLevel = 1
	}
	// Only "notify" is a recognised non-default kind; anything else is a full
	// master. Normalise so we never persist a junk privilege string.
	if body.Kind != "notify" {
		body.Kind = ""
	}

	a.masterOtp.mu.Lock()
	defer a.masterOtp.mu.Unlock()

	if a.masterOtp.code == "" || a.masterOtp.userID == 0 {
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "尚未发送验证码，请先发送"})
		return
	}
	if body.UserID != a.masterOtp.userID {
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "验证对象与当前候选不一致，请重新发送"})
		return
	}
	if time.Now().After(a.masterOtp.expires) {
		a.masterOtp.reset()
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "验证码已过期，请重新发送"})
		return
	}
	if a.masterOtp.attempts >= otpMaxAttempts {
		a.masterOtp.reset()
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "尝试次数过多，请重新发送验证码"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Code), []byte(a.masterOtp.code)) != 1 {
		a.masterOtp.attempts++
		left := otpMaxAttempts - a.masterOtp.attempts
		writeJSON(w, 200, map[string]any{"ok": false, "reason": fmt.Sprintf("验证码不正确，还可尝试 %d 次", left)})
		return
	}

	// Verified. Bind the master (guarding against a duplicate that slipped in
	// between request and confirm), persist, and clear the one-time state.
	uid, nick, level, kind := a.masterOtp.userID, a.masterOtp.nickname, body.MinLevel, body.Kind
	a.masterOtp.reset()

	cfg, err := a.store.Update(func(c *Config) {
		for _, m := range c.Masters {
			if m.UserID == uid {
				return // already present; no-op
			}
		}
		c.Masters = append(c.Masters, Master{UserID: uid, Nickname: nick, MinLevel: level, Kind: kind})
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.broadcastStatus()
	a.hub.Log("info", 0, "", fmt.Sprintf("已通过验证码绑定主人 %d", uid))
	writeJSON(w, 200, map[string]any{"ok": true, "masters": cfg.Masters})
}
