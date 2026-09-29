package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Standard five-field cron, with optional CRON_TZ=Area/City prefix.
func parseCron(expr string) (cron.Schedule, error) {
	return cron.ParseStandard(expr)
}

type BackupManager struct {
	store    *Store
	notices  *NoticeStore
	hub      *Hub
	client   *http.Client
	mu       sync.Mutex
	lastRun  time.Time
	runMu    sync.Mutex
	uploaded map[string]string
}

func NewBackupManager(store *Store, notices *NoticeStore, hub *Hub) *BackupManager {
	return &BackupManager{store: store, notices: notices, hub: hub, client: &http.Client{Timeout: 60 * time.Second}, uploaded: make(map[string]string)}
}

func (m *BackupManager) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				m.tick(ctx, now)
			}
		}
	}()
}

func (m *BackupManager) tick(ctx context.Context, now time.Time) {
	cfg := m.store.Get().Backup
	if !cfg.Enabled {
		return
	}
	schedule, err := parseCron(cfg.Cron)
	if err != nil || !schedule.Next(now.Truncate(time.Minute).Add(-time.Minute)).Equal(now.Truncate(time.Minute)) {
		return
	}
	slot := now.Truncate(time.Minute)
	m.mu.Lock()
	if !m.lastRun.IsZero() && m.lastRun.Equal(slot) {
		m.mu.Unlock()
		return
	}
	m.lastRun = slot
	m.mu.Unlock()
	func() {
		if err := m.RunOnce(ctx, cfg); err != nil {
			m.log("error", "云端备份失败："+err.Error())
			return
		}
		m.log("info", "云端备份完成")
	}()
}

func (m *BackupManager) log(level, text string) {
	if m.hub != nil {
		m.hub.Log(level, 0, "", text)
	}
}

func (m *BackupManager) RunOnce(ctx context.Context, cfg BackupConfig) error {
	if !m.runMu.TryLock() {
		return fmt.Errorf("backup already running")
	}
	defer m.runMu.Unlock()
	check := cfg
	check.Enabled = true
	if err := validateBackupConfig(check); err != nil {
		return err
	}
	if m.notices == nil {
		return fmt.Errorf("notice store unavailable")
	}
	if _, err := parseCron(cfg.Cron); err != nil {
		return err
	}
	entries, err := os.ReadDir(m.notices.Directory())
	if err != nil {
		return err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "notices-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil
	}
	to := cfg.Timeout
	if to <= 0 {
		to = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(to)*time.Second)
	defer cancel()
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.notices.mu.RLock()
		body, err := os.ReadFile(filepath.Join(m.notices.Directory(), name))
		m.notices.mu.RUnlock()
		if err != nil {
			return err
		}
		// Never upload a partial final line left by an interrupted append.
		if at := bytes.LastIndexByte(body, '\n'); at >= 0 {
			body = body[:at+1]
		} else {
			continue
		}
		key := strings.Trim(cfg.Prefix, "/")
		if key != "" {
			key += "/"
		}
		key += name
		digest := sha256.Sum256(body)
		fingerprint := hex.EncodeToString(digest[:])
		cacheKey := cfg.Endpoint + "/" + cfg.Bucket + "/" + key + "/" + cfg.AccessKey
		if m.uploaded[cacheKey] == fingerprint {
			continue
		}
		if err := putS3Object(ctx, m.client, cfg, key, body); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		m.uploaded[cacheKey] = fingerprint
	}
	return nil
}

func putS3Object(ctx context.Context, client *http.Client, cfg BackupConfig, key string, body []byte) error {
	base, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil {
		return fmt.Errorf("invalid S3 endpoint")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path.Join(cfg.Bucket, key), "/")
	contentHash := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(contentHash[:])
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, base.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Host", base.Host)
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	canonicalHeaders := "host:" + base.Host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalURI := base.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalRequest := strings.Join([]string{http.MethodPut, canonicalURI, "", canonicalHeaders, signedHeaders, payloadHash}, "\n")
	service := "s3"
	scope := date + "/" + region + "/" + service + "/aws4_request"
	crHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(crHash[:])
	kDate := hmacSHA256([]byte("AWS4"+cfg.SecretKey), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+cfg.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	resp, err := noRedirectClient(client).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("S3 %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}
