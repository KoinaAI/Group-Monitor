package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNoticeStoreAppendReopenQueryAndDedupe(t *testing.T) {
	dir := t.TempDir()
	s, err := NewNoticeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := NoticeRecord{GroupID: 42, Group: "班群", MessageIDs: []int64{9}, Sources: []NoticeSource{{MessageID: 9, Time: 100, UserID: 7, Nickname: "老师"}}, Result: LLMResult{Useful: true, Level: 2, Title: "体检", Summary: "周五前完成体检"}}
	if added, err := s.Append(n); err != nil || !added {
		t.Fatalf("append added=%v err=%v", added, err)
	}
	if added, err := s.Append(n); err != nil || added {
		t.Fatalf("duplicate added=%v err=%v", added, err)
	}
	rows, err := s.Query(NoticeQuery{GroupID: 42, Query: "体检", Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].Result.Title != "体检" {
		t.Fatalf("query rows=%+v err=%v", rows, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "notices-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected one shard, got %v", files)
	}
	if info, err := os.Stat(files[0]); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("notice permissions: info=%v err=%v", info, err)
	}

	s2, err := NewNoticeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if added, err := s2.Append(n); err != nil || added {
		t.Fatalf("reopened duplicate added=%v err=%v", added, err)
	}
	rows, err = s2.Query(NoticeQuery{GroupID: 42, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("reopened query rows=%+v err=%v", rows, err)
	}
}

func TestNoticeStoreRotationPagingAndInterruptedAppend(t *testing.T) {
	s, _ := NewNoticeStore(t.TempDir())
	s.maxFile = 600
	stamp := time.Now().UnixMilli()
	for i := 0; i < 8; i++ {
		_, err := s.Append(NoticeRecord{ID: fmt.Sprintf("id-%d", i), GroupID: 1, CreatedAt: stamp, Result: LLMResult{Title: "通知"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.Query(NoticeQuery{GroupID: 1, Limit: 3})
	if err != nil || len(first) != 3 || first[0].ID != "id-7" {
		t.Fatalf("page=%v err=%v", first, err)
	}
	second, err := s.Query(NoticeQuery{GroupID: 1, Limit: 3, Before: stamp, BeforeID: first[2].ID})
	if err != nil || len(second) != 3 || second[0].ID != "id-4" {
		t.Fatalf("page=%v err=%v", second, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.QueryContext(ctx, NoticeQuery{}); err == nil {
		t.Fatal("query ignored cancellation")
	}
	// The base shard's interrupted tail must not consume the next valid record.
	s.maxFile = 1 << 20
	file := filepath.Join(s.dir, "notices-"+time.UnixMilli(stamp).Format("20060102")+".jsonl")
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	fmt.Fprint(f, `{"interrupted":`)
	f.Close()
	if _, err := s.Append(NoticeRecord{ID: "recovered", GroupID: 1, CreatedAt: stamp + 1}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(NoticeQuery{Limit: 20})
	if err != nil || len(rows) != 9 || rows[0].ID != "recovered" {
		t.Fatalf("recovery rows=%v err=%v", rows, err)
	}
}

func TestNoticeIDStableAcrossDifferentModelSummaries(t *testing.T) {
	s, _ := NewNoticeStore(t.TempDir())
	n := NoticeRecord{GroupID: 1, MessageIDs: []int64{123}, Sources: []NoticeSource{{MessageID: 123, Time: 1, UserID: 2, TextHash: "same"}}, Result: LLMResult{Title: "First summary"}}
	if _, err := s.Append(n); err != nil {
		t.Fatal(err)
	}
	n.Result.Title = "Rephrased summary"
	if added, err := s.Append(n); err != nil || added {
		t.Fatalf("replayed source duplicated: added=%v err=%v", added, err)
	}
}

func TestNoticeStoreIgnoresTruncatedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notices-20260929.jsonl")
	data := []byte(`{"id":"ok","createdAt":2,"groupId":1,"group":"g","result":{"title":"公告"}}` + "\n" + `{"id":"broken"`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewNoticeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(NoticeQuery{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].ID != "ok" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestNoticeStoreQueryMultipleGroups(t *testing.T) {
	store, err := NewNoticeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, groupID := range []int64{42, 43, 44} {
		if _, err := store.Append(NoticeRecord{GroupID: groupID, Group: fmt.Sprintf("群 %d", groupID), Result: LLMResult{Title: fmt.Sprintf("通知 %d", groupID)}}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.Query(NoticeQuery{GroupIDs: []int64{42, 44}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].GroupID == 43 || rows[1].GroupID == 43 {
		t.Fatalf("multiple group query returned %#v", rows)
	}
}
