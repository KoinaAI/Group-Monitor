package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// otpState holds the single in-flight login OTP. The password gate lives in the
// Node frontend; this backend only mints, delivers (via master DMs) and verifies
// the code, so the code never leaves the trusted loopback backend.
type otpState struct {
	mu       sync.Mutex
	code     string
	expires  time.Time
	attempts int
}

const (
	otpTTL         = 5 * time.Minute
	otpMaxAttempts = 5
)

func genOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// handleOtpRequest mints a fresh 6-digit code and DMs it to every master. It is
// only meaningful when NapCat is online and at least one master exists; the
// frontend falls back to the fixed password otherwise. Returns how many masters
// were reached — never the code itself.
func (a *API) handleOtpRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	if !a.ob.Connected() {
		writeErr(w, 409, "NapCat 未在线，无法发送验证码")
		return
	}
	masters := a.store.Get().Masters
	if len(masters) == 0 {
		writeErr(w, 409, "尚未配置主人，无法发送验证码")
		return
	}

	code, err := genOTP()
	if err != nil {
		writeErr(w, 500, "生成验证码失败")
		return
	}
	text := fmt.Sprintf("【群哨】本次登录验证码：%s\n5 分钟内有效，仅可使用一次。若非本人操作，请忽略本条消息。", code)

	sent, failed := 0, []string{}
	for _, m := range masters {
		if err := a.ob.SendPrivateMsg(m.UserID, text); err != nil {
			failed = append(failed, fmt.Sprintf("%d", m.UserID))
			continue
		}
		sent++
	}
	if sent == 0 {
		writeErr(w, 502, "验证码发送失败，请检查主人 QQ 是否可私聊")
		return
	}

	a.otp.mu.Lock()
	a.otp.code = code
	a.otp.expires = time.Now().Add(otpTTL)
	a.otp.attempts = 0
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}

	a.otp.mu.Lock()
	defer a.otp.mu.Unlock()

	if a.otp.code == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "尚未获取验证码，请先点击获取"})
		return
	}
	if time.Now().After(a.otp.expires) {
		a.otp.code = ""
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "验证码已过期，请重新获取"})
		return
	}
	if a.otp.attempts >= otpMaxAttempts {
		a.otp.code = ""
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "尝试次数过多，请重新获取验证码"})
		return
	}

	if subtle.ConstantTimeCompare([]byte(body.Code), []byte(a.otp.code)) == 1 {
		a.otp.code = "" // consume: one-time use
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	a.otp.attempts++
	left := otpMaxAttempts - a.otp.attempts
	writeJSON(w, 200, map[string]any{"ok": false, "reason": fmt.Sprintf("验证码不正确，还可尝试 %d 次", left)})
}
