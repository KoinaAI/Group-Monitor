package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxDocumentsPerMessage   = 3
	maxDocumentXMLBytes      = 8 << 20
	maxDocumentResponseBytes = 1 << 20
)

// DocumentConfig enables bounded attachment extraction before classification.
// Every attachment is sent through MinerU. Mode "agent" uses the free,
// tokenless lightweight API; mode "api" uses the paid/token API.
type DocumentConfig struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	Timeout      int    `json:"timeoutSec"`
	MaxFileMB    int    `json:"maxFileMB"`
	MaxTextChars int    `json:"maxTextChars"`
}

func defaultDocumentConfig() DocumentConfig {
	return DocumentConfig{Mode: "agent", BaseURL: "https://mineru.net/api/v1/agent", Timeout: 120, MaxFileMB: 20, MaxTextChars: 12000}
}

func documentMode(c DocumentConfig) string {
	if strings.Contains(c.BaseURL, "/api/v4") && c.APIKey != "" {
		return "api"
	}
	if strings.EqualFold(strings.TrimSpace(c.Mode), "api") {
		return "api"
	}
	// Configurations written before mode was introduced used the v4 endpoint
	// and a key. Keep those installations working while new configs default to
	// the free Agent API.
	if strings.TrimSpace(c.Mode) == "" && c.APIKey != "" && strings.Contains(c.BaseURL, "/api/v4") {
		return "api"
	}
	return "agent"
}

// minerUBaseURL keeps old saved configurations usable when a user changes
// between the two official MinerU APIs. Custom test or self-hosted endpoints
// are left untouched; only the first-party mineru.net paths are normalized.
func minerUBaseURL(c DocumentConfig) string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	u, err := url.Parse(base)
	if err != nil || !strings.EqualFold(u.Hostname(), "mineru.net") {
		return base
	}
	mode := documentMode(c)
	if mode == "agent" && strings.HasSuffix(u.Path, "/api/v4") {
		u.Path = strings.TrimSuffix(u.Path, "/api/v4") + "/api/v1/agent"
	}
	if mode == "api" && strings.HasSuffix(u.Path, "/api/v1/agent") {
		u.Path = strings.TrimSuffix(u.Path, "/api/v1/agent") + "/api/v4"
	}
	return strings.TrimRight(u.String(), "/")
}

func validateDocumentConfig(c DocumentConfig) error {
	if !validURL(c.BaseURL, "https", "http") || len(c.APIKey) > maxTokenBytes || strings.ContainsAny(c.APIKey, "\r\n") || (c.Mode != "" && c.Mode != "agent" && c.Mode != "api") {
		return fmt.Errorf("invalid document endpoint or key")
	}
	if c.BaseURL != "" {
		u, _ := url.Parse(c.BaseURL)
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("document endpoint cannot contain credentials, query or fragment")
		}
	}
	if c.Timeout < 1 || c.Timeout > 600 || c.MaxFileMB < 1 || c.MaxFileMB > 100 || c.MaxTextChars < 256 || c.MaxTextChars > 32000 {
		return fmt.Errorf("document timeout, file size or text limit out of range")
	}
	return nil
}

type DocumentReader struct {
	ob           *OneBot
	client       *http.Client
	pollInterval time.Duration
}

func NewDocumentReader(ob *OneBot) *DocumentReader {
	return &DocumentReader{
		ob:           ob,
		client:       &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		pollInterval: 2 * time.Second,
	}
}

// Enrich keeps original message text untouched, and adds extracted document
// text with a shared per-message character budget. Binary files are held only
// in bounded memory. The pipeline's bounded queue supplies concurrency control.
func (d *DocumentReader) Enrich(ctx context.Context, cfg DocumentConfig, gm GroupMessage) GroupMessage {
	if !cfg.Enabled || len(gm.Files) == 0 {
		return gm
	}
	if err := validateDocumentConfig(cfg); err != nil {
		gm.DocumentErrors = []string{"文件读取配置无效"}
		return gm
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Second)
	defer cancel()
	var out strings.Builder
	remaining := cfg.MaxTextChars
	for i, file := range gm.Files {
		if i >= maxDocumentsPerMessage {
			gm.DocumentErrors = append(gm.DocumentErrors, "附件过多：每条消息最多读取 3 个文件")
			break
		}
		if remaining <= 0 {
			break
		}
		name := documentName(file.Name)
		text, err := d.readFile(ctx, cfg, gm.GroupID, file, name)
		if err != nil {
			// Do not expose signed URLs, API response bodies or keys via events.
			gm.DocumentErrors = append(gm.DocumentErrors, name+": "+err.Error())
			continue
		}
		text = strings.TrimSpace(strings.ToValidUTF8(text, ""))
		if text == "" {
			gm.DocumentErrors = append(gm.DocumentErrors, name+": 未提取到可读文字")
			continue
		}
		part := "\n[附件正文: " + name + "]\n" + text
		part = truncateDocumentChars(part, remaining)
		out.WriteString(part)
		remaining -= utf8.RuneCountInString(part)
	}
	gm.DocumentText = strings.TrimSpace(out.String())
	return gm
}

// groupMessageContent is the classification/transcript view. Original Text
// remains available for accurate display and archival provenance.
func groupMessageContent(gm GroupMessage) string {
	if gm.DocumentText == "" {
		return gm.Text
	}
	return gm.Text + "\n" + gm.DocumentText
}

func documentName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	return truncateDocumentChars(name, 160)
}

func truncateDocumentChars(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

func (d *DocumentReader) readFile(ctx context.Context, cfg DocumentConfig, groupID int64, file HistoryFile, name string) (string, error) {
	if documentMode(cfg) == "api" && cfg.APIKey == "" {
		return "", fmt.Errorf("需要配置 MinerU API Key")
	}
	limit := int64(cfg.MaxFileMB) << 20
	if file.Size > limit {
		return "", fmt.Errorf("文件超过 %d MB 限制", cfg.MaxFileMB)
	}
	link := ""
	if file.FileID != "" && d.ob != nil {
		var err error
		link, err = d.ob.GetGroupFileURLContext(ctx, groupID, file.FileID, file.Busid)
		if err != nil {
			return "", fmt.Errorf("无法获取群文件下载地址")
		}
	} else {
		// Event payloads are untrusted. Only known QQ CDN links can bypass
		// NapCat's file-id resolution; internal or arbitrary URLs are rejected.
		u, err := url.Parse(file.URL)
		if err != nil || u.User != nil || u.Scheme != "https" || !documentCDNHost(u.Hostname()) {
			return "", fmt.Errorf("缺少可用的群文件 ID")
		}
		link = file.URL
	}
	if !documentHTTPURL(link) {
		return "", fmt.Errorf("群文件下载地址无效")
	}
	data, err := d.download(ctx, link, limit)
	if err != nil {
		return "", fmt.Errorf("文件下载失败或超过大小限制")
	}
	return d.extractMinerU(ctx, cfg, name, data)
}

func documentCDNHost(host string) bool {
	host = strings.ToLower(host)
	for _, suffix := range []string{".qq.com", ".qq.com.cn", ".qpic.cn", ".gtimg.cn"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func documentHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.User == nil && (u.Scheme == "http" || u.Scheme == "https")
}

func (d *DocumentReader) download(ctx context.Context, link string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return nil, fmt.Errorf("download rejected")
	}
	return readDocumentBytes(resp.Body, limit)
}

func readDocumentBytes(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("document exceeds limit")
	}
	return b, nil
}

func extractDOCX(data []byte, maxChars int) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	if len(zr.File) > 4096 {
		return "", fmt.Errorf("too many ZIP members")
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			if doc != nil {
				return "", fmt.Errorf("duplicate document XML")
			}
			doc = f
		}
	}
	if doc == nil || doc.UncompressedSize64 > maxDocumentXMLBytes {
		return "", fmt.Errorf("missing or oversized document XML")
	}
	r, err := doc.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	b, err := readDocumentBytes(r, maxDocumentXMLBytes)
	if err != nil {
		return "", err
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	var out strings.Builder
	inText := false
	count := 0
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		s := ""
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
			if t.Name.Local == "tab" {
				s = "\t"
			}
			if t.Name.Local == "br" {
				s = "\n"
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
			if t.Name.Local == "p" || t.Name.Local == "tr" {
				s = "\n"
			}
			if t.Name.Local == "tc" {
				s = "\t"
			}
		case xml.CharData:
			if inText {
				s = string(t)
			}
		}
		s = truncateDocumentChars(s, maxChars-count)
		out.WriteString(s)
		count += utf8.RuneCountInString(s)
		if count >= maxChars {
			break
		}
	}
	return out.String(), nil
}

type minerUEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

func (d *DocumentReader) minerUJSON(ctx context.Context, cfg DocumentConfig, method, suffix string, input any, output any) error {
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	base := minerUBaseURL(cfg)
	req, err := http.NewRequestWithContext(ctx, method, base+suffix, body)
	if err != nil {
		return fmt.Errorf("MinerU 请求地址无效")
	}
	if documentMode(cfg) == "api" && cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("MinerU 请求失败或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("MinerU HTTP %d", resp.StatusCode)
	}
	b, err := readDocumentBytes(resp.Body, maxDocumentResponseBytes)
	if err != nil {
		return fmt.Errorf("MinerU 响应过大")
	}
	var envelope minerUEnvelope
	if json.Unmarshal(b, &envelope) != nil || len(envelope.Data) == 0 {
		return fmt.Errorf("MinerU 响应格式无效")
	}
	if envelope.Code != 0 {
		return fmt.Errorf("MinerU 错误码 %d", envelope.Code)
	}
	if json.Unmarshal(envelope.Data, output) != nil {
		return fmt.Errorf("MinerU 数据格式无效")
	}
	return nil
}

// Signed upload and result URLs come from the configured parser. Keep secrets
// on the API origin, and restrict secondary transfers to that origin or the
// documented MinerU storage domains. Never follow redirects.
func minerUTransferURL(raw, base string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	if u.Scheme == b.Scheme && strings.EqualFold(u.Host, b.Host) {
		return u.Scheme == "http" || u.Scheme == "https"
	}
	if u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".aliyuncs.com", ".openxlab.org.cn", ".mineru.net"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func (d *DocumentReader) extractMinerU(ctx context.Context, cfg DocumentConfig, name string, data []byte) (string, error) {
	base := minerUBaseURL(cfg)
	if documentMode(cfg) == "agent" {
		return d.extractMinerUAgent(ctx, cfg, name, data)
	}
	var upload struct {
		BatchID  string   `json:"batch_id"`
		FileURLs []string `json:"file_urls"`
	}
	request := map[string]any{"files": []map[string]any{{"name": name, "data_id": "attachment"}}, "model_version": "vlm"}
	if err := d.minerUJSON(ctx, cfg, http.MethodPost, "/file-urls/batch", request, &upload); err != nil {
		return "", err
	}
	if upload.BatchID == "" || len(upload.BatchID) > 128 || len(upload.FileURLs) != 1 || !minerUTransferURL(upload.FileURLs[0], base) {
		return "", fmt.Errorf("MinerU 上传地址或批次无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, upload.FileURLs[0], bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("MinerU 上传地址无效")
	}
	// MinerU explicitly requires no Content-Type on the presigned PUT.
	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MinerU 文件上传失败或超时")
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("MinerU 文件上传 HTTP %d", resp.StatusCode)
	}
	for {
		var result struct {
			Results []struct {
				State string `json:"state"`
				URL   string `json:"full_zip_url"`
			} `json:"extract_result"`
		}
		if err := d.minerUJSON(ctx, cfg, http.MethodGet, "/extract-results/batch/"+url.PathEscape(upload.BatchID), nil, &result); err != nil {
			return "", err
		}
		if len(result.Results) > 1 {
			return "", fmt.Errorf("MinerU 返回意外数量的结果")
		}
		if len(result.Results) == 1 {
			r := result.Results[0]
			switch r.State {
			case "done":
				if !minerUTransferURL(r.URL, base) {
					return "", fmt.Errorf("MinerU 结果地址无效")
				}
				b, err := d.download(ctx, r.URL, int64(cfg.MaxFileMB)<<20)
				if err != nil {
					return "", fmt.Errorf("MinerU 结果下载失败或超过大小限制")
				}
				return extractMinerUMarkdown(b, cfg.MaxTextChars)
			case "failed":
				return "", fmt.Errorf("MinerU 文件解析失败")
			case "waiting-file", "pending", "running", "converting":
			default:
				return "", fmt.Errorf("MinerU 返回未知任务状态")
			}
		}
		timer := time.NewTimer(d.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("MinerU 解析超时或取消")
		case <-timer.C:
		}
	}
}

// extractMinerUAgent follows the documented lightweight Agent API. It is
// deliberately separate from the paid batch flow because Agent requests have
// no Authorization header and return a markdown URL instead of a ZIP archive.
func (d *DocumentReader) extractMinerUAgent(ctx context.Context, cfg DocumentConfig, name string, data []byte) (string, error) {
	base := minerUBaseURL(cfg)
	var created struct {
		TaskID  string `json:"task_id"`
		FileURL string `json:"file_url"`
	}
	request := map[string]any{
		"file_name":      name,
		"language":       "ch",
		"enable_table":   true,
		"is_ocr":         false,
		"enable_formula": true,
	}
	if err := d.minerUJSON(ctx, cfg, http.MethodPost, "/parse/file", request, &created); err != nil {
		return "", err
	}
	if created.TaskID == "" || len(created.TaskID) > 160 || !minerUTransferURL(created.FileURL, base) {
		return "", fmt.Errorf("MinerU Agent 上传地址或任务无效")
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, created.FileURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("MinerU Agent 上传地址无效")
	}
	putResp, err := d.client.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("MinerU Agent 文件上传失败或超时")
	}
	putResp.Body.Close()
	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		return "", fmt.Errorf("MinerU Agent 文件上传 HTTP %d", putResp.StatusCode)
	}
	for {
		var result struct {
			State       string `json:"state"`
			MarkdownURL string `json:"markdown_url"`
			Error       string `json:"err_msg"`
		}
		if err := d.minerUJSON(ctx, cfg, http.MethodGet, "/parse/"+url.PathEscape(created.TaskID), nil, &result); err != nil {
			return "", err
		}
		switch result.State {
		case "done":
			if !minerUTransferURL(result.MarkdownURL, base) {
				return "", fmt.Errorf("MinerU Agent 结果地址无效")
			}
			markdown, err := d.download(ctx, result.MarkdownURL, int64(cfg.MaxFileMB)<<20)
			if err != nil {
				return "", fmt.Errorf("MinerU Agent 结果下载失败或超过大小限制")
			}
			return truncateDocumentChars(strings.ToValidUTF8(string(markdown), ""), cfg.MaxTextChars), nil
		case "failed":
			if result.Error != "" {
				return "", fmt.Errorf("MinerU Agent 文件解析失败: %s", limitText(result.Error, 160))
			}
			return "", fmt.Errorf("MinerU Agent 文件解析失败")
		case "waiting-file", "uploading", "pending", "running":
		default:
			return "", fmt.Errorf("MinerU Agent 返回未知任务状态")
		}
		timer := time.NewTimer(d.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("MinerU Agent 解析超时或取消")
		case <-timer.C:
		}
	}
}

func extractMinerUMarkdown(data []byte, maxChars int) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) > 4096 {
		return "", fmt.Errorf("MinerU 结果压缩包无效")
	}
	for _, f := range zr.File {
		if path.Base(f.Name) != "full.md" {
			continue
		}
		if f.UncompressedSize64 > maxDocumentXMLBytes {
			return "", fmt.Errorf("MinerU 文本解压后过大")
		}
		r, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("MinerU 文本打开失败")
		}
		b, err := readDocumentBytes(r, maxDocumentXMLBytes)
		r.Close()
		if err != nil {
			return "", fmt.Errorf("MinerU 文本解压失败")
		}
		return truncateDocumentChars(strings.ToValidUTF8(string(b), ""), maxChars), nil
	}
	return "", fmt.Errorf("MinerU 结果缺少 full.md")
}
