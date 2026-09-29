package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type oneBotFixture struct {
	mu      sync.Mutex
	private []struct {
		userID int64
		text   string
	}
	server *httptest.Server
}

func newOneBotFixture(t *testing.T, a *API) *oneBotFixture {
	t.Helper()
	f := &oneBotFixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/send_private_msg":
			f.mu.Lock()
			f.private = append(f.private, struct {
				userID int64
				text   string
			}{int64(params["user_id"].(float64)), params["message"].(string)})
			f.mu.Unlock()
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{}}`)
		case "/get_stranger_info":
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"user_id":77,"nickname":"新主人"}}`)
		default:
			t.Errorf("unexpected OneBot action %s", r.URL.Path)
		}
	}))
	a.ob.httpBase = f.server.URL
	a.ob.connected.Store(true)
	t.Cleanup(f.server.Close)
	return f
}

func (f *oneBotFixture) sent() []struct {
	userID int64
	text   string
} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]struct {
		userID int64
		text   string
	}(nil), f.private...)
}

func authJSON(t *testing.T, a *API, path, body string, cookie *http.Cookie) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := serveAPI(a, "POST", path, []byte(body), cookie)
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("%s: %v: %s", path, err, w.Body.String())
	}
	return w, result
}

func TestLoginOTPRequestDeliveryCooldownAndVerify(t *testing.T) {
	a := newTestAPI(t)
	f := newOneBotFixture(t, a)
	_, err := a.store.Update(func(c *Config) {
		c.Masters = []Master{{UserID: 1}, {UserID: 2, Kind: "notify"}, {UserID: 3, Kind: "full"}}
	})
	if err != nil {
		t.Fatal(err)
	}
	w, out := authJSON(t, a, "/api/auth/otp/request", "", nil)
	if w.Code != 200 || out["sent"] != float64(2) || out["ttlSec"] != float64(300) {
		t.Fatalf("OTP request: %d %v", w.Code, out)
	}
	sent := f.sent()
	if len(sent) != 2 || sent[0].userID != 1 || sent[1].userID != 3 || strings.Contains(sent[0].text, "password") {
		t.Fatalf("OTP delivered to wrong recipients: %+v", sent)
	}
	if w, _ := authJSON(t, a, "/api/auth/otp/request", "", nil); w.Code != 429 {
		t.Fatalf("cooldown status=%d", w.Code)
	}
	code := regexp.MustCompile(`[0-9]{6}`).FindString(sent[0].text)
	if code == "" {
		t.Fatalf("no OTP in DM %q", sent[0].text)
	}
	w, out = authJSON(t, a, "/api/auth/otp/verify", fmt.Sprintf(`{"code":%q}`, code), nil)
	if w.Code != 200 || out["ok"] != true || len(w.Result().Cookies()) == 0 {
		t.Fatalf("verify: %d %v", w.Code, out)
	}
	if w, out := authJSON(t, a, "/api/auth/otp/verify", fmt.Sprintf(`{"code":%q}`, code), nil); w.Code != 200 || out["ok"] != false {
		t.Fatalf("OTP reused: %d %v", w.Code, out)
	}
}

func TestLoginOTPRequestRequiresConnectionAndFullMaster(t *testing.T) {
	a := newTestAPI(t)
	if w, _ := authJSON(t, a, "/api/auth/otp/request", "", nil); w.Code != 409 {
		t.Fatalf("offline OTP=%d", w.Code)
	}
	a.ob.connected.Store(true)
	_, err := a.store.Update(func(c *Config) { c.Masters = []Master{{UserID: 2, Kind: "notify"}} })
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := authJSON(t, a, "/api/auth/otp/request", "", nil); w.Code != 409 {
		t.Fatalf("notify-only OTP=%d", w.Code)
	}
}

func TestMasterBindRequestAndConfirm(t *testing.T) {
	a := newTestAPI(t)
	f := newOneBotFixture(t, a)
	cookie := sessionCookieFor(t, a)
	w, out := authJSON(t, a, "/api/masters/verify/request", `{"userId":77}`, cookie)
	if w.Code != 200 || out["nickname"] != "新主人" || out["ttlSec"] != float64(180) {
		t.Fatalf("bind request: %d %v", w.Code, out)
	}
	if w, _ := authJSON(t, a, "/api/masters/verify/request", `{"userId":77}`, cookie); w.Code != 429 {
		t.Fatalf("bind cooldown=%d", w.Code)
	}
	if w, out := authJSON(t, a, "/api/masters/verify/confirm", `{"userId":78,"code":"000000"}`, cookie); w.Code != 200 || out["ok"] != false {
		t.Fatalf("wrong candidate: %d %v", w.Code, out)
	}
	code := regexp.MustCompile(`[0-9]{6}`).FindString(f.sent()[0].text)
	if w, out := authJSON(t, a, "/api/masters/verify/confirm", fmt.Sprintf(`{"userId":77,"code":%q,"minLevel":3,"kind":"notify"}`, code), cookie); w.Code != 200 || out["ok"] != true {
		t.Fatalf("bind confirm: %d %v", w.Code, out)
	}
	masters := a.store.Get().Masters
	if len(masters) != 1 || masters[0].UserID != 77 || masters[0].Nickname != "新主人" || masters[0].MinLevel != 3 || masters[0].Kind != "notify" {
		t.Fatalf("saved master=%+v", masters)
	}
	if len(a.store.Get().FullMasters()) != 0 {
		t.Fatal("notify master gained OTP login")
	}
	if w, out := authJSON(t, a, "/api/masters/verify/confirm", fmt.Sprintf(`{"userId":77,"code":%q}`, code), cookie); w.Code != 200 || out["ok"] != false {
		t.Fatalf("bind OTP reused: %d %v", w.Code, out)
	}
}

func TestMasterBindExpiredAndAttemptLimit(t *testing.T) {
	a := newTestAPI(t)
	a.masterOtp.userID, a.masterOtp.code = 77, "123456"
	a.masterOtp.expires = time.Now().Add(-time.Second)
	if w, out := authJSON(t, a, "/api/masters/verify/confirm", `{"userId":77,"code":"123456"}`, sessionCookieFor(t, a)); w.Code != 200 || out["ok"] != false || a.masterOtp.code != "" {
		t.Fatalf("expired bind: %d %v", w.Code, out)
	}
	a.masterOtp.userID, a.masterOtp.code = 77, "123456"
	a.masterOtp.expires = time.Now().Add(time.Minute)
	a.masterOtp.attempts = otpMaxAttempts
	if w, out := authJSON(t, a, "/api/masters/verify/confirm", `{"userId":77,"code":"123456"}`, sessionCookieFor(t, a)); w.Code != 200 || out["ok"] != false || a.masterOtp.code != "" {
		t.Fatalf("attempt-limited bind: %d %v", w.Code, out)
	}
}
