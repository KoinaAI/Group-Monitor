package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	SourceID   string         `json:"sourceId,omitempty"`
	AccountID  string         `json:"accountId,omitempty"`
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
	AccountID     string
	IncludeLegacy bool
	GroupID       int64
	Query         string
	Limit         int
	Before        int64  // createdAt cursor; zero means newest
	BeforeID      string // tie-breaker when multiple notices share a millisecond
}

// NoticeStore is an append-only, date-sharded JSONL store. It is intentionally
// independent from config.json so a corrupt notice file cannot prevent startup.
type NoticeStore struct {
	parent    *NoticeStore
	sourceID  string
	accountID string
	dir       string
	mu        sync.RWMutex
	ids       map[string]string
	maxFile   int64
	maxBytes  int64
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
	s := &NoticeStore{dir: dir, ids: make(map[string]string), maxFile: defaultNoticeMaxFile, maxBytes: 256 << 20, retention: defaultNoticeDays * 24 * time.Hour}
	if err := s.pruneLocked(time.Now()); err != nil {
		return nil, err
	}
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
				s.ids[n.ID] = entry.Name()
			}
		}
		_ = f.Close()
	}
	return nil
}

func noticeID(groupID int64, ids []int64, sources []NoticeSource, result LLMResult, urgent bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|", groupID)
	if len(ids) == 0 && len(sources) == 0 {
		fmt.Fprintf(&b, "%t|%s|%s|%s|", urgent, result.Title, result.Summary, result.Deadline)
	}
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
	if s.parent != nil {
		n.SourceID, n.AccountID = s.sourceID, s.accountID
		if n.ID == "" {
			n.ID = noticeID(n.GroupID, n.MessageIDs, n.Sources, n.Result, n.Urgent)
		}
		sum := sha256.Sum256([]byte(n.SourceID + ":" + n.AccountID + ":" + n.ID))
		n.ID = hex.EncodeToString(sum[:16])
		return s.parent.Append(n)
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return false, err
	}
	// Recover an interrupted final append before adding another complete line.
	if info, statErr := f.Stat(); statErr == nil && info.Size() > 0 {
		size := info.Size()
		tailSize := int64(1 << 20)
		if size < tailSize {
			tailSize = size
		}
		tail := make([]byte, tailSize)
		if _, readErr := f.ReadAt(tail, size-tailSize); readErr != nil {
			f.Close()
			return false, readErr
		}
		if tail[len(tail)-1] != '\n' {
			last := strings.LastIndexByte(string(tail), '\n')
			if last < 0 && size > tailSize {
				f.Close()
				return false, fmt.Errorf("invalid notice tail")
			}
			if err := f.Truncate(size - tailSize + int64(last+1)); err != nil {
				f.Close()
				return false, err
			}
		}
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
	s.ids[n.ID] = name
	return true, s.pruneLocked(time.Now())
}

func (s *NoticeStore) pruneLocked(now time.Time) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-s.retention)
	type shard struct {
		name     string
		size     int64
		modified time.Time
	}
	var files []shard
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "notices-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, shard{entry.Name(), info.Size(), info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified.Before(files[j].modified) })
	for _, file := range files {
		if !file.modified.Before(cutoff) && (s.maxBytes <= 0 || total <= s.maxBytes) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, file.name)); err != nil {
			return err
		}
		total -= file.size
		for id, name := range s.ids {
			if name == file.name {
				delete(s.ids, id)
			}
		}
	}
	return nil
}

func (s *NoticeStore) Query(q NoticeQuery) ([]NoticeRecord, error) {
	return s.QueryContext(context.Background(), q)
}

func noticeNewer(a, b NoticeRecord) bool {
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt > b.CreatedAt
	}
	return a.ID > b.ID
}

// Keep only the best N matches in memory, regardless of archive size. Each
// file is read at a committed size snapshot so concurrent appends are excluded.
func (s *NoticeStore) QueryContext(ctx context.Context, q NoticeQuery) ([]NoticeRecord, error) {
	out := make([]NoticeRecord, 0)
	if s == nil {
		return out, nil
	}
	if s.parent != nil {
		q.AccountID = s.accountID
		q.IncludeLegacy = s.accountID == legacyAccountID
		return s.parent.QueryContext(ctx, q)
	}
	if q.Limit <= 0 || q.Limit > maxNoticeQuery {
		q.Limit = 30
	}
	needle := strings.ToLower(strings.TrimSpace(q.Query))
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "notices-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		s.mu.RLock()
		f, err := os.Open(filepath.Join(s.dir, entry.Name()))
		var size int64
		if err == nil {
			var info os.FileInfo
			info, err = f.Stat()
			if err == nil {
				size = info.Size()
			}
		}
		s.mu.RUnlock()
		if err != nil {
			if f != nil {
				f.Close()
			}
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sc := bufio.NewScanner(io.LimitReader(f, size))
		sc.Buffer(make([]byte, 1024), 1<<20)
		for sc.Scan() {
			if err := ctx.Err(); err != nil {
				f.Close()
				return nil, err
			}
			var n NoticeRecord
			if json.Unmarshal(sc.Bytes(), &n) != nil || n.ID == "" {
				continue
			}
			if q.GroupID != 0 && n.GroupID != q.GroupID {
				continue
			}
			if q.AccountID != "" && n.AccountID != q.AccountID && !(q.IncludeLegacy && n.AccountID == "") {
				continue
			}
			if q.Before != 0 && (n.CreatedAt > q.Before || n.CreatedAt == q.Before && (q.BeforeID == "" || n.ID >= q.BeforeID)) {
				continue
			}
			hay := strings.ToLower(n.Group + " " + n.Result.Title + " " + n.Result.Summary + " " + n.Result.Event + " " + n.Result.Deadline)
			if needle != "" && !strings.Contains(hay, needle) {
				continue
			}
			at := sort.Search(len(out), func(i int) bool { return noticeNewer(n, out[i]) })
			if at >= q.Limit {
				continue
			}
			out = append(out, NoticeRecord{})
			copy(out[at+1:], out[at:])
			out[at] = n
			if len(out) > q.Limit {
				out = out[:q.Limit]
			}
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *NoticeStore) Close() error { return nil }

// ForAccount shares the bounded archive while enforcing an account namespace.
func (s *NoticeStore) ForAccount(sourceID, accountID string) *NoticeStore {
	if s == nil {
		return nil
	}
	return &NoticeStore{parent: s, sourceID: sourceID, accountID: accountID}
}
