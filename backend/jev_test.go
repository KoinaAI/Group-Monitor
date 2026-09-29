package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJevImportanceRequestAndResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad Jev headers/method: %s %v", r.Method, r.Header)
		}
		var req jevReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		q := req.Questions["important"]
		if req.Model != "jev-latest" || q.Type != "noul" || q.Instructions == nil || q.Criteria == nil {
			t.Errorf("Jev request contract changed: %+v", req)
		}
		fmt.Fprint(w, `{"answers":{"important":{"type":"noul","noul":0.85}}}`)
	}))
	defer srv.Close()
	state := jevState(GroupMessage{GroupName: "班群", Nickname: "李四", Role: "member", Text: "收到"},
		[]GroupMessage{{Nickname: "老师", Role: "admin", Text: "明天开会"}})
	if state["group"] != "班群" || !strings.Contains(state["message"].(string), "李四(成员)") || !strings.Contains(state["recent"].([]string)[0], "老师(管理员)") {
		t.Fatalf("unexpected Jev state: %#v", state)
	}
	got, err := jevImportance(JevConfig{BaseURL: srv.URL, APIKey: "key"}, state)
	if err != nil || got != 0.85 {
		t.Fatalf("noul=%v err=%v", got, err)
	}
}

func TestJevImportanceRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   string
	}{
		{`{"error":"offline"}`, 503, "jev 503"},
		{`broken`, 200, "bad jev response"},
		{`{"answers":{}}`, 200, "no 'important' answer"},
		{`{"answers":{"important":{"type":"noul","noul":1.5}}}`, 200, "out of range"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		_, err := jevImportance(JevConfig{BaseURL: srv.URL}, nil)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("response %q: err=%v, want %q", tc.body, err, tc.want)
		}
	}
}
