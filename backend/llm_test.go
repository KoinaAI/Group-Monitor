package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallLLMStreamingAndRequest(t *testing.T) {
	const verdict = `{"useful":true,"level":2,"title":"考试通知","summary":"明日考试"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected LLM request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var req chatReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if !req.Stream || req.Model != "model" || len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Content != "group batch" {
			t.Errorf("request contract changed: %+v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private thoughts\",\"content\":\"{\\\"useful\\\":true,\\\"level\\\":2,\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"\\\"title\\\":\\\"考试通知\\\",\\\"summary\\\":\\\"明日考试\\\"}\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	got, raw, err := callLLM(LLMConfig{BaseURL: srv.URL + "/v1/", APIKey: "secret", Model: "model"}, "group batch")
	if err != nil {
		t.Fatal(err)
	}
	if raw != verdict || !got.Useful || got.Level != 2 || got.Title != "考试通知" || strings.Contains(raw, "private thoughts") {
		t.Fatalf("stream verdict = %+v raw=%q", got, raw)
	}
}

func TestCallLLMFallbackAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantTitle string
		wantError string
	}{
		{"json fallback", 200, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":1,\"title\":\"安排\"}"}}]}`, "安排", ""},
		{"upstream error", 429, `{"error":{"message":"quota exceeded"}}`, "", "quota exceeded"},
		{"empty choices", 200, `{"choices":[]}`, "", "no choices"},
		{"invalid response", 200, `not-json`, "", "bad LLM response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			got, _, err := callLLM(LLMConfig{BaseURL: srv.URL}, "batch")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || got.Title != tc.wantTitle {
				t.Fatalf("verdict = %+v, error = %v", got, err)
			}
		})
	}
}

func TestParseVerdictWrappedAndMultipleObjects(t *testing.T) {
	for _, input := range []string{
		"```json\n{\"useful\":true,\"title\":\"通知\"}\n```",
		"结果：{\"useful\":true,\"title\":\"通知\"} 补充 {\"debug\":1}",
	} {
		got, err := parseVerdict(input)
		if err != nil || !got.Useful || got.Title != "通知" {
			t.Fatalf("parse %q: %+v %v", input, got, err)
		}
	}
	if _, err := parseVerdict("no JSON"); err == nil {
		t.Fatal("non-JSON must fail")
	}
	if _, err := parseVerdict(`{"level":1,"title":"缺少 useful"}`); err == nil {
		t.Fatal("verdict without useful must fail")
	}
	if _, err := parseVerdict(`{"useful":true,"level":4}`); err == nil {
		t.Fatal("verdict with out-of-range level must fail")
	}
}

func TestCallLLMRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, strings.Repeat("x", maxLLMResponseBytes))
	}))
	defer srv.Close()
	if _, _, err := callLLM(LLMConfig{BaseURL: srv.URL}, "batch"); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized response error=%v", err)
	}
}
