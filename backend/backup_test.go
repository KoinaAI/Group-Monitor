package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupCronTimezoneAndValidation(t *testing.T) {
	schedule, err := parseCron("CRON_TZ=Asia/Shanghai 0 3 * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 18, 59, 0, 0, time.UTC)
	if got := schedule.Next(now); !got.Equal(time.Date(2026, 9, 29, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("next=%v", got)
	}
	for _, expr := range []string{"", "* * * *", "*/0 * * * *", "60 * * * *"} {
		if _, err := parseCron(expr); err == nil {
			t.Errorf("accepted %q", expr)
		}
	}
}

func TestBackupUploadsOnlyCompleteNoticeShardsAndSkipsUnchanged(t *testing.T) {
	n, err := NewNoticeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("{\"id\":\"notice\"}\n")
	os.WriteFile(filepath.Join(n.dir, "notices-20260929.jsonl"), append(body, []byte(`{"partial"`)...), 0600)
	os.WriteFile(filepath.Join(n.dir, "config.json"), []byte("private credential"), 0600)
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.Path != "/notices/prefix/notices-20260929.jsonl" || string(got) != string(body) {
			t.Errorf("upload %s %s %q", r.Method, r.URL.Path, got)
		}
		sum := sha256.Sum256(got)
		if r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) {
			t.Error("payload digest mismatch")
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test/") || !strings.Contains(r.Header.Get("Authorization"), "/auto/s3/aws4_request") {
			t.Error("missing S3 signature")
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	m := NewBackupManager(nil, n, nil)
	m.client = srv.Client()
	cfg := defaultConfig().Backup
	cfg.Endpoint = srv.URL
	cfg.Bucket = "notices"
	cfg.Prefix = "prefix"
	cfg.AccessKey = "test"
	cfg.SecretKey = "private"
	if err := m.RunOnce(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.RunOnce(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestBackupFailureRetriesAndRejectsRedirect(t *testing.T) {
	n, _ := NewNoticeStore(t.TempDir())
	os.WriteFile(filepath.Join(n.dir, "notices-test.jsonl"), []byte("{}\n"), 0600)
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "https://other.invalid", 307)
	}))
	defer srv.Close()
	m := NewBackupManager(nil, n, nil)
	m.client = srv.Client()
	cfg := defaultConfig().Backup
	cfg.Endpoint = srv.URL
	cfg.Bucket = "bucket"
	cfg.AccessKey = "key"
	cfg.SecretKey = "secret"
	for i := 0; i < 2; i++ {
		if err := m.RunOnce(context.Background(), cfg); err == nil {
			t.Fatal("accepted redirect")
		}
	}
	if calls != 2 {
		t.Fatalf("failed upload not retried: %d", calls)
	}
}
