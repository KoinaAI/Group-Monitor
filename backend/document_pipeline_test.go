package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFileOnlyNotificationIsReadBeforeClassification(t *testing.T) {
	data := documentZIP(t, "word/document.xml", `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>周五十点前提交奖学金材料</w:t></w:r></w:p></w:body></w:document>`)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_group_file_url":
			fmt.Fprintf(w, `{"status":"ok","retcode":0,"data":{"url":%q}}`, srv.URL+"/source")
		case "/source":
			w.Write(data)
		case "/chat/completions":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "提交奖学金材料") {
				t.Error("LLM missed attachment text")
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"useful\":true,\"level\":2,\"title\":\"奖学金\",\"summary\":\"周五十点前提交奖学金材料\"}"}}]}`)
		case "/jev":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "提交奖学金材料") {
				t.Error("Jev judged only the filename")
			}
			fmt.Fprint(w, `{"answers":{"important":{"type":"noul","noul":0.95}}}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	pipe, _ := testPipe(t, func(c *Config) {
		c.Documents.Enabled = true
		c.LLM.Enabled = true
		c.LLM.BaseURL = srv.URL
		c.Jev.Enabled = true
		c.Jev.BaseURL = srv.URL + "/jev"
		c.Jev.APIKey = "key"
		c.Rules.QuietWindowSec = 60
	})
	defer pipe.Shutdown()
	pipe.ob.httpBase = srv.URL
	notices, _ := NewNoticeStore(t.TempDir())
	pipe.SetNoticeStore(notices)
	pipe.Ingest(GroupMessage{GroupID: 100, UserID: 7, MessageID: 123, Time: time.Now().Unix(), Text: "[文件:通知.docx]", Files: []HistoryFile{{Name: "通知.docx", FileID: "file"}}})
	pipe.mu.Lock()
	buf := pipe.buffers[100]
	pipe.mu.Unlock()
	if buf == nil {
		t.Fatal("file-only notice dropped before reading")
	}
	pipe.flush(100, buf.timerGen)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, _ := notices.Query(NoticeQuery{GroupID: 100})
		if len(rows) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("file-only notice was not archived")
}
