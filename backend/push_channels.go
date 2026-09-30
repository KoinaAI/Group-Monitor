package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

type NotificationTarget struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Enabled    bool     `json:"enabled"`
	URL        string   `json:"url"`
	Topic      string   `json:"topic,omitempty"`
	Token      string   `json:"token,omitempty"`
	DeviceKey  string   `json:"deviceKey,omitempty"`
	Group      string   `json:"group,omitempty"`
	MinLevel   int      `json:"minLevel"`
	AccountIDs []string `json:"accountIds,omitempty"`
}

func cloneNotificationTargets(src []NotificationTarget) []NotificationTarget {
	out := slices.Clone(src)
	for i := range out {
		out[i].AccountIDs = slices.Clone(out[i].AccountIDs)
	}
	return out
}
func redactedNotificationTargets(src []NotificationTarget) []NotificationTarget {
	out := cloneNotificationTargets(src)
	for i := range out {
		out[i].Token = ""
		out[i].DeviceKey = ""
	}
	return out
}
func validateNotificationTargets(targets []NotificationTarget) error {
	if len(targets) > 32 {
		return fmt.Errorf("too many notification targets")
	}
	ids := map[string]bool{}
	for _, t := range targets {
		if !identifierPattern.MatchString(t.ID) || ids[t.ID] || !validText(t.Name, 128) || t.Name == "" || t.MinLevel < 0 || t.MinLevel > 3 {
			return fmt.Errorf("invalid notification target")
		}
		ids[t.ID] = true
		if t.Kind != "ntfy" && t.Kind != "bark" {
			return fmt.Errorf("supported notification types are ntfy and bark")
		}
		u, err := url.Parse(t.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(t.URL) > maxURLBytes {
			return fmt.Errorf("notification server must be HTTP(S) without credentials, query or fragment")
		}
		if !validText(t.Topic, 256) || strings.ContainsAny(t.Topic, "/?# ") || !validText(t.Token, maxTokenBytes) || !validText(t.DeviceKey, maxTokenBytes) || !validText(t.Group, 128) {
			return fmt.Errorf("invalid notification credentials or topic")
		}
		if t.Enabled && (t.Kind == "ntfy" && t.Topic == "" || t.Kind == "bark" && t.DeviceKey == "") {
			return fmt.Errorf("enabled ntfy needs a topic; enabled Bark needs a device key")
		}
		if len(t.AccountIDs) > 32 {
			return fmt.Errorf("too many account filters")
		}
		for _, id := range t.AccountIDs {
			if !identifierPattern.MatchString(id) {
				return fmt.Errorf("invalid account filter")
			}
		}
	}
	return nil
}

var pushSlots = make(chan struct{}, 4)

func sendPushContext(parent context.Context, target NotificationTarget, title, body string, level int) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	select {
	case pushSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-pushSlots }()
	endpoint := strings.TrimRight(target.URL, "/")
	var payload any
	if target.Kind == "ntfy" {
		priority := 3
		if level >= 3 {
			priority = 5
		} else if level >= 2 {
			priority = 4
		}
		payload = map[string]any{"topic": target.Topic, "title": limitText(title, 120), "message": limitText(body, 6000), "priority": priority}
	} else if target.Kind == "bark" {
		endpoint += "/push"
		payload = map[string]any{"device_key": target.DeviceKey, "title": limitText(title, 120), "body": limitText(body, 6000), "group": target.Group}
	} else {
		return fmt.Errorf("unsupported notification target")
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid notification endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	if target.Token != "" {
		req.Header.Set("Authorization", "Bearer "+target.Token)
	}
	res, err := noRedirectClient(http.DefaultClient).Do(req)
	if err != nil {
		return fmt.Errorf("notification request failed")
	}
	defer res.Body.Close()
	response, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("notification response failed")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("notification server returned %d", res.StatusCode)
	}
	if target.Kind == "bark" {
		var result struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(response, &result) != nil || result.Code != 200 {
			return fmt.Errorf("Bark rejected notification")
		}
	}
	return nil
}
func broadcastPushContext(parent context.Context, cfg Config, title, body string, level int) (int, []string) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	type result struct {
		id  string
		err error
	}
	targets := []NotificationTarget{}
	for _, t := range cfg.NotificationTargets {
		if t.Enabled && level >= t.MinLevel && (len(t.AccountIDs) == 0 || slices.Contains(t.AccountIDs, cfg.AccountID)) {
			targets = append(targets, t)
		}
	}
	jobs := make(chan NotificationTarget, len(targets))
	results := make(chan result, len(targets))
	for _, t := range targets {
		jobs <- t
	}
	close(jobs)
	var wg sync.WaitGroup
	for i := 0; i < min(4, len(targets)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				results <- result{t.ID, sendPushContext(ctx, t, title, body, level)}
			}
		}()
	}
	wg.Wait()
	close(results)
	sent := 0
	failed := []string{}
	for r := range results {
		if r.err != nil {
			failed = append(failed, r.id)
		} else {
			sent++
		}
	}
	return sent, failed
}
func (a *API) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, redactedNotificationTargets(a.store.Get().NotificationTargets))
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		Targets []NotificationTarget `json:"targets"`
	}
	if decodeJSON(w, r, &body, maxJSONBody) != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) {
		for i := range body.Targets {
			for _, old := range c.NotificationTargets {
				if old.ID == body.Targets[i].ID && old.Kind == body.Targets[i].Kind {
					if body.Targets[i].Token == "" {
						body.Targets[i].Token = old.Token
					}
					if body.Targets[i].DeviceKey == "" {
						body.Targets[i].DeviceKey = old.DeviceKey
					}
				}
			}
		}
		c.NotificationTargets = body.Targets
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, redactedNotificationTargets(cfg.NotificationTargets))
}
func (a *API) handleNotificationTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if decodeJSON(w, r, &body, maxAuthBody) != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	for _, target := range a.store.Get().NotificationTargets {
		if target.ID == body.ID {
			if !target.Enabled {
				writeErr(w, 409, "请先启用并保存通知渠道")
				return
			}
			if err := sendPushContext(r.Context(), target, "讯枢测试通知", "通知渠道连接成功。", 1); err != nil {
				writeErr(w, 502, err.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	writeErr(w, 404, "notification target not found")
}
