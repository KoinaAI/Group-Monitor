package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// NoticeSource keeps only the provenance needed to explain a notification.
// Raw chatter, media bytes and short-lived CDN URLs are deliberately omitted.
type NoticeSource struct {
	MessageID int64   `json:"messageId,omitempty"`
	Time      int64   `json:"time"`
	UserID    int64   `json:"userId"`
	Nickname  string  `json:"nickname,omitempty"`
	TextHash  string  `json:"textHash,omitempty"`
	JevNoul   float64 `json:"jevNoul,omitempty"`
}

// NoticeRecord is the durable representation of a useful notification.
type NoticeRecord struct {
	ID         string         `json:"id"`
	CreatedAt  int64          `json:"createdAt"`
	GroupID    int64          `json:"groupId"`
	Group      string         `json:"group"`
	MessageIDs []int64        `json:"messageIds,omitempty"`
	Sources    []NoticeSource `json:"sources,omitempty"`
	Result     LLMResult      `json:"result"`
	Urgent     bool           `json:"urgent"`
}

type NoticeQuery struct {
	GroupID int64
	Query   string
	Limit   int
	Before  int64 // createdAt cursor, exclusive; zero means newest
}

// NoticeStore is an append-only, date-sharded JSONL store. It is intentionally
// independent from config.json so a corrupt notice file cannot prevent startup.
type NoticeStore struct {
	dir       string
	mu        sync.RWMutex
	ids       map[string]struct{}
	maxFile   int64
	retention time.Duration
}

const (
	defaultNoticeMaxFile = 16 << 20
	defaultNoticeDays    = 90
	maxNoticeQuery       = 100
	maxNoticeText        = 4096
)

func NewNoticeStore(dir string) (*NoticeStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("notice directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &NoticeStore{dir: dir, ids: make(map[string]struct{}), maxFile: defaultNoticeMaxFile, retention: defaultNoticeDays * 24 * time.Hour}
	if err := s.loadIDs(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *NoticeStore) Directory() string { return s.dir }

func (s *NoticeStore) loadIDs() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "notices-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024), 1<<20)
		for sc.Scan() {
			var n NoticeRecord
			if json.Unmarshal(sc.Bytes(), &n) == nil && n.ID != "" {
				s.ids[n.ID] = struct{}{}
			}
		}
		_ = f.Close()
	}
	return nil
}

func noticeID(groupID int64, ids []int64, sources []NoticeSource, result LLMResult, urgent bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%t|%s|%s|%s|", groupID, urgent, result.Title, result.Summary, result.Deadline)
	for _, id := range ids {
		fmt.Fprintf(&b, "%d,", id)
	}
	for _, source := range sources {
		fmt.Fprintf(&b, "%d:%d:%s:%s:%d,", source.Time, source.UserID, source.Nickname, source.TextHash, source.MessageID)
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

func (s *NoticeStore) Append(n NoticeRecord) (bool, error) {
	if s == nil {
		return false, nil
	}
	if len(n.Result.Summary) > maxNoticeText {
		n.Result.Summary = limitText(n.Result.Summary, maxNoticeText)
	}
	if len(n.Result.Title) > 256 {
		n.Result.Title = limitText(n.Result.Title, 256)
	}
	if n.CreatedAt == 0 {
		n.CreatedAt = time.Now().UnixMilli()
	}
	if n.ID == "" {
		n.ID = noticeID(n.GroupID, n.MessageIDs, n.Sources, n.Result, n.Urgent)
	}
	b, err := json.Marshal(n)
	if err != nil {
		return false, err
	}
	if len(b) > 1<<20 {
		return false, fmt.Errorf("notice too large")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.ids[n.ID]; exists {
		return false, nil
	}
	name := "notices-" + time.UnixMilli(n.CreatedAt).In(time.Local).Format("20060102") + ".jsonl"
	path := filepath.Join(s.dir, name)
	if info, statErr := os.Stat(path); statErr == nil && info.Size()+int64(len(b)+1) > s.maxFile {
		stamp := time.UnixMilli(n.CreatedAt).In(time.Local).Format("20060102-150405")
		for part := 1; ; part++ {
			name = fmt.Sprintf("notices-%s-%03d.jsonl", stamp, part)
			path = filepath.Join(s.dir, name)
			if info, statErr := os.Stat(path); statErr != nil || info.Size()+int64(len(b)+1) <= s.maxFile {
				break
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return false, err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	s.ids[n.ID] = struct{}{}
	return true, s.pruneLocked(time.Now())
}

func (s *NoticeStore) pruneLocked(now time.Time) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-s.retention)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "notices-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.dir, entry.Name()))
		}
	}
	return nil
}

func (s *NoticeStore) Query(q NoticeQuery) ([]NoticeRecord, error) {
	if s == nil {
		return nil, nil
	}
	if q.Limit <= 0 || q.Limit > maxNoticeQuery {
		q.Limit = 30
	}
	needle := strings.ToLower(strings.TrimSpace(q.Query))
	s.mu.RLock()
	entries, err := os.ReadDir(s.dir)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	all := make([]NoticeRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "notices-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		var rows []NoticeRecord
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024), 1<<20)
		for sc.Scan() {
			var n NoticeRecord
			if json.Unmarshal(sc.Bytes(), &n) == nil {
				rows = append(rows, n)
			}
		}
		_ = f.Close()
		all = append(all, rows...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt != all[j].CreatedAt {
			return all[i].CreatedAt > all[j].CreatedAt
		}
		return all[i].ID > all[j].ID
	})
	out := make([]NoticeRecord, 0, q.Limit)
	for _, n := range all {
		if q.GroupID != 0 && n.GroupID != q.GroupID || q.Before != 0 && n.CreatedAt >= q.Before {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(n.Group + " " + n.Result.Title + " " + n.Result.Summary + " " + n.Result.Event)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		out = append(out, n)
		if len(out) >= q.Limit {
			return out, nil
		}
	}
	return out, nil
}

func (s *NoticeStore) Close() error { return nil }
