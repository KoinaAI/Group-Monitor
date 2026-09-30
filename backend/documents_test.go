package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func documentZIP(t *testing.T, name, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func documentOneBot(base string) *OneBot {
	o := NewOneBot()
	o.httpBase = base
	return o
}

func TestDocumentReaderAgentPreservesOriginalAndBoundsText(t *testing.T) {
	xml := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>请于周五提交表格</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>姓名</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:t>` + strings.Repeat("中文", 500) + `</w:t></w:r></w:p></w:body></w:document>`
	data := documentZIP(t, "word/document.xml", xml)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_group_file_url":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["group_id"] != float64(42) || body["file_id"] != "file-id" {
				t.Errorf("bad NapCat file request: %v", body)
			}
			fmt.Fprintf(w, `{"status":"ok","retcode":0,"data":{"url":%q}}`, srv.URL+"/file")
		case "/file":
			w.Write(data)
		case "/api/v1/agent/parse/file":
			if r.Header.Get("Authorization") != "" {
				t.Error("Agent API must not receive a token")
			}
			fmt.Fprint(w, `{"code":0,"data":{"task_id":"agent-task","file_url":"`+srv.URL+`/agent-upload"}}`)
		case "/agent-upload":
			if r.Method != http.MethodPut {
				t.Errorf("wrong Agent upload method: %s", r.Method)
			}
		case "/api/v1/agent/parse/agent-task":
			fmt.Fprint(w, `{"code":0,"data":{"task_id":"agent-task","state":"done","markdown_url":"`+srv.URL+`/result.md"}}`)
		case "/result.md":
			fmt.Fprint(w, "请于周五提交表格\n姓名")
		default:
			t.Errorf("unexpected external API path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := defaultDocumentConfig()
	cfg.Enabled = true
	cfg.BaseURL = srv.URL + "/api/v1/agent"
	cfg.MaxTextChars = 256
	gm := GroupMessage{Text: "请阅读通知", GroupID: 42, Files: []HistoryFile{{Name: "通知.docx", FileID: "file-id"}}}
	got := NewDocumentReader(documentOneBot(srv.URL)).Enrich(context.Background(), cfg, gm)
	if got.Text != gm.Text || len(got.DocumentErrors) != 0 {
		t.Fatalf("original changed or extraction failed: %+v", got)
	}
	if !strings.Contains(got.DocumentText, "请于周五提交表格\n姓名") {
		t.Fatalf("missing main/table text: %q", got.DocumentText)
	}
	if !utf8.ValidString(got.DocumentText) || utf8.RuneCountInString(got.DocumentText) > cfg.MaxTextChars {
		t.Fatalf("unbounded text: %q", got.DocumentText)
	}
	if !strings.Contains(groupMessageContent(got), gm.Text) || !strings.Contains(groupMessageContent(got), "周五") {
		t.Fatal("classification content omitted original or attachment")
	}
}

func TestDocumentReaderMinerUUploadPollingAndMarkdown(t *testing.T) {
	for _, ext := range []string{".pdf", ".doc"} {
		t.Run(ext, func(t *testing.T) {
			resultZIP := documentZIP(t, "result/full.md", "# 正式通知\n请于 10 月 1 日提交材料。")
			var polls atomic.Int32
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				isAPI := strings.HasPrefix(r.URL.Path, "/api/v4/")
				if isAPI && r.Header.Get("Authorization") != "Bearer parser-secret" {
					t.Error("missing parser authorization")
				}
				if !isAPI && r.Header.Get("Authorization") != "" {
					t.Error("API key leaked into secondary transfer")
				}
				switch r.URL.Path {
				case "/get_group_file_url":
					fmt.Fprintf(w, `{"status":"ok","retcode":0,"data":{"url":%q}}`, srv.URL+"/source")
				case "/source":
					w.Write([]byte("fixture-document"))
				case "/api/v4/file-urls/batch":
					if r.Method != http.MethodPost {
						t.Errorf("wrong batch method %s", r.Method)
					}
					var body struct {
						Files []struct {
							Name   string `json:"name"`
							DataID string `json:"data_id"`
						} `json:"files"`
						Model string `json:"model_version"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if len(body.Files) != 1 || body.Files[0].Name != "notice"+ext || body.Files[0].DataID != "attachment" || body.Model != "vlm" {
						t.Errorf("unsupported upload contract: %+v", body)
					}
					fmt.Fprintf(w, `{"code":0,"data":{"batch_id":"test-batch","file_urls":[%q]}}`, srv.URL+"/signed-upload")
				case "/signed-upload":
					if r.Method != http.MethodPut || r.Header.Get("Content-Type") != "" {
						t.Error("wrong presigned upload contract")
					}
					b, _ := io.ReadAll(r.Body)
					if string(b) != "fixture-document" {
						t.Errorf("wrong upload bytes: %q", b)
					}
				case "/api/v4/extract-results/batch/test-batch":
					if polls.Add(1) == 1 {
						io.WriteString(w, `{"code":0,"data":{"extract_result":[{"state":"running"}]}}`)
						return
					}
					fmt.Fprintf(w, `{"code":0,"data":{"extract_result":[{"state":"done","full_zip_url":%q}]}}`, srv.URL+"/result")
				case "/result":
					w.Write(resultZIP)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			cfg := defaultDocumentConfig()
			cfg.Enabled, cfg.APIKey, cfg.BaseURL = true, "parser-secret", srv.URL+"/api/v4"
			d := NewDocumentReader(documentOneBot(srv.URL))
			d.pollInterval = time.Millisecond
			gm := GroupMessage{Files: []HistoryFile{{Name: "notice" + ext, FileID: "notice"}}}
			got := d.Enrich(context.Background(), cfg, gm)
			if len(got.DocumentErrors) > 0 || !strings.Contains(got.DocumentText, "10 月 1 日") || polls.Load() != 2 {
				t.Fatalf("bad extraction: %+v, polls %d", got, polls.Load())
			}
		})
	}
}

func TestDocumentReaderAgentForwardsAnyFilenameToMinerU(t *testing.T) {
	var names []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/parse/file":
			var body struct {
				Name string `json:"file_name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			names = append(names, body.Name)
			fmt.Fprintf(w, `{"code":0,"data":{"task_id":%q,"file_url":%q}}`, body.Name, srv.URL+"/upload")
		case "/upload":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/agent/parse/notice.txt", "/api/v1/agent/parse/notice.md", "/api/v1/agent/parse/notice.xyz":
			fmt.Fprintf(w, `{"code":0,"data":{"state":"done","markdown_url":%q}}`, srv.URL+"/result.md")
		case "/result.md":
			fmt.Fprint(w, "正文")
		default:
			t.Errorf("unexpected MinerU path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := defaultDocumentConfig()
	cfg.Enabled, cfg.BaseURL = true, srv.URL+"/api/v1/agent"
	d := NewDocumentReader(nil)
	for _, name := range []string{"notice.txt", "notice.md", "notice.xyz"} {
		if text, err := d.extractMinerU(context.Background(), cfg, name, []byte("bytes")); err != nil || text != "正文" {
			t.Fatalf("%s extraction text=%q err=%v", name, text, err)
		}
	}
	if strings.Join(names, ",") != "notice.txt,notice.md,notice.xyz" {
		t.Fatalf("MinerU did not receive all filenames: %v", names)
	}
}

func TestDocumentReaderRejectsUnsafeURLsAndMissingKeys(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); io.WriteString(w, "private") }))
	defer srv.Close()
	d := NewDocumentReader(nil)
	cfg := defaultDocumentConfig()
	cfg.Enabled = true
	for _, file := range []HistoryFile{{Name: "private.docx", URL: srv.URL}, {Name: "notice.pdf", URL: srv.URL}, {Name: "oversize.docx", Size: 100 << 20, URL: srv.URL}} {
		got := d.Enrich(context.Background(), cfg, GroupMessage{Files: []HistoryFile{file}})
		if len(got.DocumentErrors) != 1 || got.DocumentText != "" {
			t.Fatalf("unreported error: %+v", got)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unsafe URL or unconfigured service contacted")
	}
	for _, target := range []string{"http://169.254.169.254/latest", "https://example.com/file", "https://user:pass@cdn-mineru.openxlab.org.cn/file", "https://cdn-mineru.openxlab.org.cn.evil.test/file"} {
		if minerUTransferURL(target, "https://mineru.net/api/v4") {
			t.Errorf("unsafe parser transfer accepted: %s", target)
		}
	}
}

func TestDocumentReaderCancellationAndNoRedirects(t *testing.T) {
	var unexpected atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, srv.URL+"/private", http.StatusFound)
		case "/private":
			unexpected.Add(1)
		default:
			<-r.Context().Done()
		}
	}))
	defer srv.Close()
	d := NewDocumentReader(nil)
	if _, err := d.download(context.Background(), srv.URL+"/redirect", 1024); err == nil {
		t.Fatal("followed download redirect")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := d.download(ctx, srv.URL+"/slow", 1024); err == nil {
		t.Fatal("ignored context cancellation")
	}
	if unexpected.Load() != 0 {
		t.Fatal("redirect reached internal target")
	}
}

func TestDocumentArchivesRejectMalformedAndOversizedMembers(t *testing.T) {
	for _, data := range [][]byte{[]byte("bad zip"), documentZIP(t, "missing.md", "text"), documentZIP(t, "full.md", strings.Repeat("x", maxDocumentXMLBytes+1))} {
		if _, err := extractMinerUMarkdown(data, 100); err == nil {
			t.Fatal("accepted invalid MinerU ZIP")
		}
	}
}

func TestGroupMessageExtractsFileMetadata(t *testing.T) {
	gm := parseGroupMessage(map[string]any{"message": []any{map[string]any{"type": "file", "data": map[string]any{"name": "notice.pdf", "file_id": "id", "size": float64(512), "busid": float64(3)}}}}, 0)
	if len(gm.Files) != 1 || gm.Files[0].Name != "notice.pdf" || gm.Files[0].FileID != "id" || gm.Files[0].Size != 512 || gm.Files[0].Busid != 3 {
		t.Fatalf("file metadata missing: %+v", gm)
	}
}

func TestOneBotGroupUploadEntersDocumentPipeline(t *testing.T) {
	o := NewOneBot()
	var got []GroupMessage
	o.onEvent = func(gm GroupMessage) { got = append(got, gm) }
	o.handleFrame([]byte(`{"post_type":"notice","notice_type":"group_upload","group_id":42,"user_id":7,"time":1234,"file":{"id":"uploaded-id","name":"群通知.doc","size":128,"busid":4}}`))
	if len(got) != 1 || got[0].GroupID != 42 || got[0].UserID != 7 || got[0].Time != 1234 || len(got[0].Files) != 1 || got[0].Files[0].FileID != "uploaded-id" || got[0].Files[0].Name != "群通知.doc" {
		t.Fatalf("group upload ignored: %+v", got)
	}
}
