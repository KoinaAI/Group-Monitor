package main

import (
	"os"
	"path/filepath"
	"testing"
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
