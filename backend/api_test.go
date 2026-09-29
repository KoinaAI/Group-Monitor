package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func serveAPI(a *API, method, path string, body []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	return w
}

func TestAPIRouteAuthentication(t *testing.T) {
	a := newTestAPI(t)
	for _, path := range []string{"/api/status", "/api/config", "/api/groups", "/api/groups/history", "/api/masters", "/api/rules", "/api/events"} {
		w := serveAPI(a, "GET", path, nil, nil)
		if w.Code != 401 {
			t.Errorf("%s without cookie = %d", path, w.Code)
		}
	}
	if w := serveAPI(a, "GET", "/api/auth/status", nil, nil); w.Code != 200 {
		t.Fatalf("public auth status = %d", w.Code)
	}
	cookie := sessionCookieFor(t, a)
	if w := serveAPI(a, "GET", "/api/status", nil, cookie); w.Code != 200 {
		t.Fatalf("protected status = %d", w.Code)
	}
	if w := serveAPI(a, "GET", "/api/config", nil, cookie); w.Code != 200 {
		t.Fatalf("protected config = %d", w.Code)
	}
	if w := serveAPI(a, "POST", "/api/auth/logout", nil, cookie); w.Code != 200 {
		t.Fatalf("logout = %d", w.Code)
	}
	if w := serveAPI(a, "GET", "/api/status", nil, cookie); w.Code != 401 {
		t.Fatalf("revoked session = %d", w.Code)
	}
}

func TestAPIConfigRoundTripAndValidation(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	cases := []struct{ path, body string }{
		{"/api/groups/watch", `{"groups":[{"groupId":42,"groupName":"班群","watch":true}]}`},
		{"/api/rules", `{"quietWindowSec":15,"maxHoldSec":90,"urgentKeywords":["考试"],"atAllUrgent":true,"elevateOwnerAdmin":true,"senderOverrides":[]}`},
		{"/api/enabled", `{"enabled":false}`},
	}
	for _, tc := range cases {
		w := serveAPI(a, "POST", tc.path, []byte(tc.body), cookie)
		if w.Code != 200 {
			t.Fatalf("POST %s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	cfg := a.store.Get()
	if cfg.Enabled || cfg.Rules.QuietWindowSec != 15 || len(cfg.Groups) != 1 || cfg.Groups[0].GroupID != 42 {
		t.Fatalf("config save did not take effect: %+v", cfg)
	}
	for _, path := range []string{"/api/groups/watch", "/api/rules", "/api/llm", "/api/jev", "/api/onebot", "/api/enabled"} {
		if w := serveAPI(a, "POST", path, []byte(`{`), cookie); w.Code != 400 {
			t.Errorf("%s malformed JSON = %d", path, w.Code)
		}
	}
	if !reflect.DeepEqual(a.store.Get(), cfg) {
		t.Fatal("invalid JSON changed config")
	}
	if w := serveAPI(a, "GET", "/api/rules", nil, cookie); w.Code != 405 {
		t.Fatalf("rules GET = %d", w.Code)
	}
	for _, path := range []string{"/api/groups/history", "/api/groups/file-url", "/api/lookup"} {
		if w := serveAPI(a, "GET", path, nil, cookie); w.Code != 400 {
			t.Errorf("%s missing query = %d", path, w.Code)
		}
	}
}

func TestAPIStatusLogsAndEscalations(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	_, err := a.store.Update(func(c *Config) {
		c.Groups = []GroupWatch{{GroupID: 1, Watch: true}, {GroupID: 2, Watch: false}}
		c.Masters = []Master{{UserID: 3}}
	})
	if err != nil {
		t.Fatal(err)
	}
	a.hub.Log("info", 1, "a", "first")
	a.hub.Log("error", 1, "a", "last")
	a.hub.Escalation(map[string]any{"title": "notice"})
	w := serveAPI(a, "GET", "/api/status", nil, cookie)
	var status map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["watchedGroups"] != float64(1) || status["totalGroups"] != float64(2) || status["masters"] != float64(1) {
		t.Fatalf("status=%v", status)
	}
	w = serveAPI(a, "GET", "/api/logs", nil, cookie)
	var logs []LogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &logs); err != nil || len(logs) != 2 || logs[0].Text != "last" {
		t.Fatalf("logs=%v err=%v", logs, err)
	}
	w = serveAPI(a, "GET", "/api/escalations", nil, cookie)
	var escs []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &escs); err != nil || len(escs) != 1 || escs[0]["title"] != "notice" {
		t.Fatalf("escalations=%v err=%v", escs, err)
	}
}

func TestAPISSEHelloAndLiveEvent(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	srv := httptest.NewServer(a.Routes())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE status=%d content-type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(resp.Body)
	read := func() Event {
		t.Helper()
		for scanner.Scan() {
			if !strings.HasPrefix(scanner.Text(), "data: ") {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &ev); err != nil {
				t.Fatal(err)
			}
			return ev
		}
		t.Fatalf("SSE closed before event: %v", scanner.Err())
		return Event{}
	}
	if ev := read(); ev.Type != "hello" {
		t.Fatalf("first event=%+v", ev)
	}
	a.hub.Log("info", 0, "", "streamed")
	if ev := read(); ev.Type != "log" {
		t.Fatalf("live event=%+v", ev)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAPIMediaRejectsUnsafeURLAndRedirect(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	for _, raw := range []string{"http://a.qpic.cn/p", "https://127.0.0.1/p", "https://evilqpic.cn/p", "https://a.qpic.cn.evil.test/p"} {
		w := serveAPI(a, "GET", "/api/groups/media?u="+raw, nil, cookie)
		if w.Code != 400 {
			t.Errorf("unsafe media URL %q returned %d", raw, w.Code)
		}
	}
	requests := 0
	a.ob.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"http://127.0.0.1/private"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	w := serveAPI(a, "GET", "/api/groups/media?u=https://a.qpic.cn/photo", nil, cookie)
	if requests != 1 || w.Code != 502 {
		t.Fatalf("redirect followed: requests=%d status=%d body=%s", requests, w.Code, w.Body.String())
	}
	if !mediaHostAllowed("a.qpic.cn") || mediaHostAllowed("qpic.cn.evil.test") {
		t.Fatal("media host allowlist boundary failed")
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("../a\r\nb\".pdf"); got != "_ab.pdf" {
		t.Fatalf("sanitized=%q", got)
	}
	if got := toASCIIFallback("讲义.pdf"); got != ".pdf" {
		t.Fatalf("fallback=%q", got)
	}
	if got := sanitizeFilename("..."); got != "download" {
		t.Fatalf("empty=%q", got)
	}
}

func TestAPIOneBotBackedGroupsHistoryAndMasterActions(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	var sent int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_group_list":
			io.WriteString(w, `{"status":"ok","retcode":0,"data":[{"group_id":42,"group_name":"班群","member_count":30}]}`)
		case "/get_group_msg_history":
			io.WriteString(w, `{"status":"ok","retcode":0,"data":{"messages":[{"message_id":8,"message_seq":7,"user_id":3,"sender":{"nickname":"甲"},"message":"公告"}]}}`)
		case "/get_group_member_list":
			io.WriteString(w, `{"status":"ok","retcode":0,"data":[]}`)
		case "/get_group_file_url":
			io.WriteString(w, `{"status":"ok","retcode":0,"data":{"url":"https://example.test/file"}}`)
		case "/get_stranger_info":
			io.WriteString(w, `{"status":"ok","retcode":0,"data":{"user_id":3,"nickname":"甲"}}`)
		case "/send_private_msg":
			sent++
			io.WriteString(w, `{"status":"ok","retcode":0,"data":{}}`)
		default:
			t.Errorf("unexpected OneBot action %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	a.ob.httpBase = srv.URL
	_, err := a.store.Update(func(c *Config) {
		c.Groups = []GroupWatch{{GroupID: 42, GroupName: "班群", Watch: true}}
		c.Masters = []Master{{UserID: 9}}
	})
	if err != nil {
		t.Fatal(err)
	}
	w := serveAPI(a, "GET", "/api/groups", nil, cookie)
	var groups []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &groups); err != nil || len(groups) != 1 || groups[0]["watch"] != true {
		t.Fatalf("groups=%v err=%v", groups, err)
	}
	w = serveAPI(a, "GET", "/api/groups/history?groupId=42&count=1", nil, cookie)
	var history []HistoryMsg
	if err := json.Unmarshal(w.Body.Bytes(), &history); err != nil || len(history) != 1 || history[0].Text != "公告" {
		t.Fatalf("history=%v err=%v", history, err)
	}
	w = serveAPI(a, "GET", "/api/groups/file-url?groupId=42&fileId=f", nil, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://example.test/file") {
		t.Fatalf("file URL=%d %s", w.Code, w.Body.String())
	}
	w = serveAPI(a, "GET", "/api/lookup?userId=3", nil, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "甲") {
		t.Fatalf("lookup=%d %s", w.Code, w.Body.String())
	}
	w = serveAPI(a, "GET", "/api/masters", nil, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"userId":9`) {
		t.Fatalf("masters=%d %s", w.Code, w.Body.String())
	}
	w = serveAPI(a, "POST", "/api/test-notify", nil, cookie)
	if w.Code != 200 || sent != 1 {
		t.Fatalf("test notify=%d sent=%d", w.Code, sent)
	}
	w = serveAPI(a, "POST", "/api/masters", []byte(`{"masters":[{"userId":3,"minLevel":2}]}`), cookie)
	if w.Code != 200 || len(a.store.Get().Masters) != 1 || a.store.Get().Masters[0].UserID != 3 {
		t.Fatalf("masters save=%d %s", w.Code, w.Body.String())
	}
}

func TestAPIConfigSectionsSaveAndTest(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":2,\"title\":\"通知\"}"}}]}`)
	}))
	defer llm.Close()
	jev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"answers":{"important":{"type":"noul","noul":0.8}}}`)
	}))
	defer jev.Close()
	w := serveAPI(a, "POST", "/api/llm", []byte(`{"enabled":true,"baseUrl":"`+llm.URL+`","model":"test"}`), cookie)
	if w.Code != 200 || !a.store.Get().LLM.Enabled {
		t.Fatalf("LLM save=%d %s", w.Code, w.Body.String())
	}
	w = serveAPI(a, "POST", "/api/llm/test", []byte(`{"baseUrl":"`+llm.URL+`","model":"test"}`), cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("LLM test=%d %s", w.Code, w.Body.String())
	}
	w = serveAPI(a, "POST", "/api/jev", []byte(`{"enabled":true,"baseUrl":"`+jev.URL+`","threshold":0.6}`), cookie)
	if w.Code != 200 || a.store.Get().Jev.BaseURL != jev.URL {
		t.Fatalf("Jev save=%d %s", w.Code, w.Body.String())
	}
	w = serveAPI(a, "POST", "/api/jev/test", []byte(`{"baseUrl":"`+jev.URL+`","threshold":0.6}`), cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) || !strings.Contains(w.Body.String(), `"important":true`) {
		t.Fatalf("Jev test=%d %s", w.Code, w.Body.String())
	}
}

func TestAPIMediaRangeAndVoice(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	a.ob.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "img.qpic.cn" || r.Header.Get("Range") != "bytes=2-4" {
			t.Errorf("media request host=%q range=%q", r.URL.Host, r.Header.Get("Range"))
		}
		return &http.Response{StatusCode: 206, Header: http.Header{
			"Content-Type": []string{"image/png"}, "Content-Range": []string{"bytes 2-4/5"},
		}, Body: io.NopCloser(strings.NewReader("PNG")), Request: r}, nil
	})
	r := httptest.NewRequest("GET", "/api/groups/media?u=https://img.qpic.cn/photo", nil)
	r.AddCookie(cookie)
	r.Header.Set("Range", "bytes=2-4")
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "PNG" || w.Header().Get("Content-Range") != "bytes 2-4/5" || w.Header().Get("Cache-Control") == "" {
		t.Fatalf("media proxy=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
	// Voice uses OneBot's get_record action and returns decoded MP3 data.
	a.ob.client = &http.Client{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get_record" {
			t.Errorf("action=%s", r.URL.Path)
		}
		io.WriteString(w, `{"status":"ok","retcode":0,"data":{"base64":"TVAz"}}`)
	}))
	defer srv.Close()
	a.ob.httpBase = srv.URL
	w = serveAPI(a, "GET", "/api/groups/voice?file=voice.amr", nil, cookie)
	if w.Code != 200 || w.Body.String() != "MP3" || w.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("voice=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
}

func TestAPIFileDownloadUsesSafeName(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	file := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		io.WriteString(w, "PDF bytes")
	}))
	defer file.Close()
	napcat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get_group_file_url" {
			t.Errorf("action=%s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"status":"ok","retcode":0,"data":{"url":%q}}`, file.URL)
	}))
	defer napcat.Close()
	a.ob.httpBase = napcat.URL
	w := serveAPI(a, "GET", "/api/groups/file-download?groupId=42&fileId=f&name=..%2Fnotes.pdf", nil, cookie)
	if w.Code != 200 || w.Body.String() != "PDF bytes" || !strings.Contains(w.Header().Get("Content-Disposition"), "_notes.pdf") || w.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("download=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
}
