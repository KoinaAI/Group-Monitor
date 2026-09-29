package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStorageAPIRedactsAndPreservesCredentials(t *testing.T) {
	a := newTestAPI(t)
	cookie := sessionCookieFor(t, a)
	for _, p := range []string{"/api/notices", "/api/backup", "/api/backup/status", "/api/backup/run", "/api/documents"} {
		if w := serveAPI(a, "GET", p, nil, nil); w.Code != 401 {
			t.Errorf("unprotected %s: %d", p, w.Code)
		}
	}
	_, err := a.store.Update(func(c *Config) {
		c.Backup.AccessKey = "access-private"
		c.Backup.SecretKey = "backup-private"
		c.Documents.APIKey = "miner-private"
	})
	if err != nil {
		t.Fatal(err)
	}
	b := a.store.Get().Backup
	b.AccessKey = ""
	b.SecretKey = ""
	body, _ := json.Marshal(b)
	w := serveAPI(a, "POST", "/api/backup", body, cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("backup response=%d %s", w.Code, w.Body.String())
	}
	d := a.store.Get().Documents
	d.APIKey = ""
	body, _ = json.Marshal(d)
	w = serveAPI(a, "POST", "/api/documents", body, cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("document response=%d %s", w.Code, w.Body.String())
	}
	c := a.store.Get()
	if c.Backup.SecretKey != "backup-private" || c.Documents.APIKey != "miner-private" {
		t.Fatal("blank form erased stored credentials")
	}
	w = serveAPI(a, "GET", "/api/config", nil, cookie)
	if strings.Contains(w.Body.String(), "private") {
		t.Fatal("config exposed credentials")
	}
	b.Cron = "invalid"
	body, _ = json.Marshal(b)
	if w := serveAPI(a, "POST", "/api/backup", body, cookie); w.Code != 400 {
		t.Fatalf("invalid cron=%d", w.Code)
	}
}
