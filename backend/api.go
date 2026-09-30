package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type API struct {
	store     *Store
	ob        *OneBot
	hub       *Hub
	pipe      *Pipeline
	notices   *NoticeStore
	backup    *BackupManager
	otp       otpState
	masterOtp masterOtpState

	// Authentication (moved out of the removed frontend into this backend).
	sessions *sessionStore
	pw       pwState
	password string // break-glass password from NAP_PASSWORD; "" disables it
}

const (
	maxJSONBody        = 1 << 20
	maxAuthBody        = 64 << 10
	maxFileProxyBytes  = 64 << 20
	maxMediaProxyBytes = 32 << 20
	maxVoiceBytes      = 32 << 20
)

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	if limit <= 0 {
		limit = maxJSONBody
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func redactedConfig(c Config) Config {
	c = cloneConfig(c)
	c.OneBot.Token = ""
	c.LLM.APIKey = ""
	c.Jev.APIKey = ""
	c.Backup.AccessKey = ""
	c.Backup.SecretKey = ""
	c.Documents.APIKey = ""
	return c
}

func NewAPI(store *Store, ob *OneBot, hub *Hub, pipe *Pipeline) *API {
	ttl := defaultSessTTL
	if h := os.Getenv("NAP_SESSION_HOURS"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 && n <= int(maxSessTTL/time.Hour) {
			ttl = time.Duration(n) * time.Hour
		}
	}
	return &API{
		store:    store,
		ob:       ob,
		hub:      hub,
		pipe:     pipe,
		sessions: newSessionStore(ttl),
		password: os.Getenv("NAP_PASSWORD"),
	}
}

func (a *API) SetNoticeStore(store *NoticeStore) { a.notices = store }

func (a *API) SetBackupManager(backup *BackupManager) { a.backup = backup }

func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Public endpoints: the login gate itself. Everything else needs a session.
	mux.HandleFunc("/api/auth/status", a.handleAuthStatus)      // which methods are available + am I authed
	mux.HandleFunc("/api/auth/otp/request", a.handleOtpRequest) // POST: DM a login OTP to masters
	mux.HandleFunc("/api/auth/otp/verify", a.handleOtpVerify)   // POST: verify OTP → session
	mux.HandleFunc("/api/auth/password", a.handlePassword)      // POST: break-glass password → session
	mux.HandleFunc("/api/auth/logout", a.handleLogout)          // POST: revoke session

	// Protected endpoints: require a valid session cookie.
	protected := map[string]http.HandlerFunc{
		"/api/status":                 a.handleStatus,
		"/api/config":                 a.handleConfig,
		"/api/groups":                 a.handleGroups,              // live group list from NapCat
		"/api/groups/watch":           a.handleWatch,               // toggle/save watched groups
		"/api/groups/history":         a.handleGroupHistory,        // manual chat-log viewer
		"/api/groups/file-url":        a.handleGroupFileURL,        // resolve a group-file download link
		"/api/groups/file-download":   a.handleGroupFileDownload,   // same-origin download proxy (real filename)
		"/api/groups/media":           a.handleGroupMedia,          // same-origin inline image/voice/video proxy
		"/api/groups/voice":           a.handleGroupVoice,          // AMR/SILK voice → browser-playable mp3
		"/api/masters":                a.handleMasters,             // GET/POST masters
		"/api/masters/verify/request": a.handleMasterVerifyRequest, // POST: DM a 3-min bind code to a candidate
		"/api/masters/verify/confirm": a.handleMasterVerifyConfirm, // POST: verify code → bind master
		"/api/rules":                  a.handleRules,               // POST rules
		"/api/llm":                    a.handleLLM,                 // POST llm config
		"/api/llm/test":               a.handleLLMTest,             // POST test a sample batch
		"/api/jev":                    a.handleJev,                 // POST jev intent-gate config
		"/api/jev/test":               a.handleJevTest,             // POST test the gate on samples
		"/api/onebot":                 a.handleOneBotConfig,        // POST onebot connection
		"/api/enabled":                a.handleEnabled,             // POST global toggle
		"/api/test-notify":            a.handleTestNotify,          // POST send a test DM
		"/api/lookup":                 a.handleLookup,              // GET nickname for a QQ
		"/api/logs":                   a.handleLogs,                // GET recent logs
		"/api/escalations":            a.handleEscalations,         // GET recent escalations
		"/api/notices":                a.handleNotices,             // GET durable useful notices
		"/api/backup":                 a.handleBackup,
		"/api/backup/run":             a.handleBackupRun,
		"/api/backup/status":          a.handleBackupStatus,
		"/api/documents":              a.handleDocuments,
		"/api/events":                 a.handleSSE, // SSE stream
	}
	for path, h := range protected {
		mux.HandleFunc(path, a.requireAuth(h))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg := a.store.Get()
	watched := 0
	for _, g := range cfg.Groups {
		if g.Watch {
			watched++
		}
	}
	// Use the cached account so a slow/unreachable NapCat can't stall status
	// polls (get_login_info is a live round-trip with a 20s client timeout).
	li := a.ob.CachedLoginInfo()
	writeJSON(w, 200, map[string]any{
		"onebotConnected": a.ob.Connected(),
		"selfId":          a.ob.SelfID(),
		"account":         li,
		"enabled":         cfg.Enabled,
		"llmEnabled":      cfg.LLM.Enabled,
		"jevEnabled":      cfg.Jev.Enabled,
		"watchedGroups":   watched,
		"totalGroups":     len(cfg.Groups),
		"masters":         len(cfg.Masters),
		"quietWindowSec":  cfg.Rules.QuietWindowSec,
		"serverTime":      time.Now().UnixMilli(),
	})
}

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	// Secrets are write-only. Empty values sent by the UI preserve the existing
	// secret, so a redacted config can still round-trip through the forms.
	writeJSON(w, 200, redactedConfig(a.store.Get()))
}

func (a *API) handleGroups(w http.ResponseWriter, r *http.Request) {
	gs, err := a.ob.GetGroupList()
	if err != nil {
		writeErr(w, 502, "获取群列表失败："+err.Error())
		return
	}
	cfg := a.store.Get()
	watchSet := map[int64]bool{}
	for _, g := range cfg.Groups {
		watchSet[g.GroupID] = g.Watch
	}
	type row struct {
		GroupID     int64  `json:"groupId"`
		GroupName   string `json:"groupName"`
		GroupRemark string `json:"groupRemark"`
		MemberCount int    `json:"memberCount"`
		Watch       bool   `json:"watch"`
	}
	out := make([]row, 0, len(gs))
	for _, g := range gs {
		out = append(out, row{g.GroupID, g.GroupName, g.GroupRemark, g.MemberCount, watchSet[g.GroupID]})
	}
	writeJSON(w, 200, out)
}

func (a *API) handleWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		Groups []GroupWatch `json:"groups"`
	}
	if err := decodeJSON(w, r, &body, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Groups = body.Groups })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.pipe.Reconcile(cfg)
	a.broadcastStatus()
	writeJSON(w, 200, cfg.Groups)
}

func (a *API) handleGroupHistory(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("groupId")), 10, 64)
	if err != nil || gid == 0 {
		writeErr(w, 400, "缺少或非法 groupId")
		return
	}
	count := 30
	if c := strings.TrimSpace(r.URL.Query().Get("count")); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			count = n
		}
	}
	var beforeSeq int64
	if b := strings.TrimSpace(r.URL.Query().Get("beforeSeq")); b != "" {
		beforeSeq, _ = strconv.ParseInt(b, 10, 64)
	}
	msgs, err := a.ob.GetGroupMsgHistoryContext(r.Context(), gid, count, beforeSeq)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		writeErr(w, 502, "获取聊天记录失败："+err.Error())
		return
	}
	writeJSON(w, 200, msgs)
}

// handleGroupFileURL resolves a downloadable link for a single group file on
// demand (the history payload carries only file ids; links expire, so we mint
// them per click).
func (a *API) handleGroupFileURL(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("groupId")), 10, 64)
	if err != nil || gid == 0 {
		writeErr(w, 400, "缺少或非法 groupId")
		return
	}
	fileID := strings.TrimSpace(r.URL.Query().Get("fileId"))
	if fileID == "" {
		writeErr(w, 400, "缺少 fileId")
		return
	}
	var busid int64
	if b := strings.TrimSpace(r.URL.Query().Get("busid")); b != "" {
		busid, _ = strconv.ParseInt(b, 10, 64)
	}
	url, err := a.ob.GetGroupFileURL(gid, fileID, busid)
	if err != nil {
		writeErr(w, 502, "获取下载链接失败："+err.Error())
		return
	}
	if !a.allowedNapCatDownloadURL(url) {
		writeErr(w, 502, "NapCat 返回了非法下载地址")
		return
	}
	writeJSON(w, 200, map[string]string{"url": url})
}

// handleGroupFileDownload proxies a group file through this origin so the
// browser saves it under its real name. A raw NapCat link is cross-origin and
// often has no Content-Disposition, so a direct <a download> is ignored and the
// file lands as "下载" with no extension. We resolve the short-lived link
// server-side, stream it back, and set Content-Disposition ourselves. The name
// comes from the query (the client already has it from the history payload);
// resolving is by fileId+busid only, so this can't be turned into an open proxy.
func (a *API) handleGroupFileDownload(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("groupId")), 10, 64)
	if err != nil || gid == 0 {
		writeErr(w, 400, "缺少或非法 groupId")
		return
	}
	fileID := strings.TrimSpace(r.URL.Query().Get("fileId"))
	if fileID == "" {
		writeErr(w, 400, "缺少 fileId")
		return
	}
	var busid int64
	if b := strings.TrimSpace(r.URL.Query().Get("busid")); b != "" {
		busid, _ = strconv.ParseInt(b, 10, 64)
	}
	name := sanitizeFilename(r.URL.Query().Get("name"))

	link, err := a.ob.GetGroupFileURL(gid, fileID, busid)
	if err != nil {
		writeErr(w, 502, "获取下载链接失败："+err.Error())
		return
	}
	if !a.allowedNapCatDownloadURL(link) {
		writeErr(w, 502, "NapCat 返回了非法下载地址")
		return
	}
	client := noRedirectClient(a.ob.client)
	resp, err := client.Get(link)
	if err != nil {
		writeErr(w, 502, "下载失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, 502, fmt.Sprintf("下载失败：上游返回 %d", resp.StatusCode))
		return
	}
	if tooLarge, err := responseTooLarge(resp, maxFileProxyBytes); err != nil {
		writeErr(w, 502, "下载大小无效")
		return
	} else if tooLarge {
		writeErr(w, http.StatusRequestEntityTooLarge, "文件过大")
		return
	}

	// RFC 6266 / 5987: an ASCII fallback plus a UTF-8 form for the real name.
	ascii := toASCIIFallback(name)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", ascii, url.PathEscape(name)))
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	} else {
		w.Header().Del("Content-Length")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := copyLimited(w, resp.Body, maxFileProxyBytes); err != nil {
		return
	}
}

func (a *API) allowedNapCatDownloadURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	cfg := a.store.Get()
	if strings.EqualFold(u.Host, httpHost(cfg.OneBot.HTTPBase)) {
		return true
	}
	// NapCat may return a local container address or a signed CDN URL. The
	// endpoint itself is trusted; redirects are disabled below so it cannot
	// pivot to a second target after validation.
	return true
}

func responseTooLarge(resp *http.Response, max int64) (bool, error) {
	if resp.ContentLength < 0 {
		return false, nil
	}
	return resp.ContentLength > max, nil
}

func copyLimited(dst io.Writer, src io.Reader, max int64) (int64, error) {
	n, err := io.Copy(dst, io.LimitReader(src, max+1))
	if n > max {
		return n, fmt.Errorf("response exceeds limit")
	}
	return n, err
}

// mediaHostAllowed reports whether host is a QQ media/avatar CDN we are willing
// to proxy. Matching the parsed hostname by suffix keeps this from becoming an
// open proxy (an attacker-supplied internal host or IP never matches), so the
// ?u= param can't be pointed at localhost, link-local, or arbitrary origins.
func mediaHostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, suffix := range []string{".qq.com.cn", ".qpic.cn", ".qlogo.cn", ".gtimg.cn"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// handleGroupMedia streams an inline history image/voice/video through this
// origin. Chrome blocks a cross-origin QQ CDN response handed to <img>/<audio>
// (net::ERR_BLOCKED_BY_ORB) even though the bytes are a valid image, so a direct
// src=<cdn url> renders broken. We re-fetch server-side (where the rkey link is
// reachable) and stream the bytes back same-origin with the upstream media type,
// passing Range through so <audio>/<video> can seek. The url is host-allowlisted,
// so this can't be turned into an open proxy.
func (a *API) handleGroupMedia(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("u"))
	if raw == "" {
		writeErr(w, 400, "缺少 u")
		return
	}
	target, err := url.Parse(raw)
	if err != nil || target.User != nil || target.Scheme != "https" || !mediaHostAllowed(target.Hostname()) {
		writeErr(w, 400, "非法媒体地址")
		return
	}
	// History URLs from NapCat often embed an already-expired rkey; the qpic CDN
	// then answers 400 (X-ErrNo -1317) and the <img> renders broken. Swap in a
	// fresh, cached group rkey whenever the link carries one. The token is
	// base64url (no reserved chars), so re-encoding the query is a safe no-op.
	if q := target.Query(); q.Get("rkey") != "" {
		if tok := a.ob.GroupRKey(r.Context()); tok != "" {
			q.Set("rkey", tok)
			target.RawQuery = q.Encode()
		}
	}
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		writeErr(w, 400, "非法媒体地址")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	// Do not follow redirects from a URL supplied by a history segment. A QQ
	// host could otherwise redirect this server to a private network address.
	client := *a.ob.client
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		writeErr(w, 502, "媒体获取失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		writeErr(w, 502, fmt.Sprintf("媒体获取失败：上游返回 %d", resp.StatusCode))
		return
	}
	if tooLarge, err := responseTooLarge(resp, maxMediaProxyBytes); err != nil {
		writeErr(w, 502, "媒体大小无效")
		return
	} else if tooLarge {
		writeErr(w, http.StatusRequestEntityTooLarge, "媒体过大")
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Accept-Ranges", "Content-Range"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	// rkey links are immutable while valid; let the browser cache within a tab.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(resp.StatusCode)
	if _, err := copyLimited(w, resp.Body, maxMediaProxyBytes); err != nil {
		return
	}
}

// handleGroupVoice serves a group voice message as browser-playable MP3. QQ voice
// is AMR/SILK — no browser decodes it natively, and the CDN download mislabels the
// AMR as audio/mp3, so an inline <audio> pointed at it just fails to load. We hand
// the record's file id to NapCat's get_record (ffmpeg) and stream back the decoded
// mp3. ServeContent adds Range/caching so the <audio> element can seek. The file
// id is a bare NapCat cache name (e.g. "<hash>.amr") from the history segment.
func (a *API) handleGroupVoice(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimSpace(r.URL.Query().Get("file"))
	if file == "" || len(file) > maxURLBytes || strings.ContainsAny(file, "\x00\r\n") {
		writeErr(w, 400, "缺少 file")
		return
	}
	mp3, err := a.ob.GetRecordMP3(file)
	if err != nil {
		writeErr(w, 502, "语音转码失败："+err.Error())
		return
	}
	if len(mp3) > maxVoiceBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "语音过大")
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "voice.mp3", time.Time{}, bytes.NewReader(mp3))
}

// sanitizeFilename strips path separators and control characters so a
// history-supplied name can't traverse or inject into the response header.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.NewReplacer("/", "_", "\\", "_", "\r", "", "\n", "", "\"", "").Replace(name)
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "download"
	}
	return name
}

// toASCIIFallback keeps only printable ASCII for the legacy filename= token;
// browsers that understand filename*= ignore it, older ones get something safe.
func toASCIIFallback(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' {
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		return s
	}
	return "download"
}

func (a *API) handleMasters(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, a.store.Get().Masters)
		return
	}
	var body struct {
		Masters []Master `json:"masters"`
	}
	if err := decodeJSON(w, r, &body, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Masters = body.Masters })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.broadcastStatus()
	writeJSON(w, 200, cfg.Masters)
}

func (a *API) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var rules Rules
	if err := decodeJSON(w, r, &rules, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Rules = rules })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.broadcastStatus()
	writeJSON(w, 200, cfg.Rules)
}

func (a *API) handleLLM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var llm LLMConfig
	if err := decodeJSON(w, r, &llm, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) {
		if llm.APIKey == "" {
			llm.APIKey = c.LLM.APIKey
		}
		if llm.Timeout <= 0 {
			llm.Timeout = c.LLM.Timeout
		}
		if llm.MaxTok <= 0 {
			llm.MaxTok = c.LLM.MaxTok
		}
		if llm.Model == "" {
			llm.Model = c.LLM.Model
		}
		c.LLM = llm
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.pipe.Reconcile(cfg)
	a.broadcastStatus()
	writeJSON(w, 200, redactedConfig(cfg).LLM)
}

func (a *API) handleLLMTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var llm LLMConfig
	if err := decodeJSON(w, r, &llm, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	sample := "群聊：测试群\n消息条数：3\n\n" +
		"[12:00:01] 张三(成员,QQ1001): 明天下午三点在公司三楼会议室开项目评审会\n" +
		"[12:00:05] 李四(管理员,QQ1002): 收到，请大家带上各自的进度文档\n" +
		"[12:00:20] 王五(成员,QQ1003): 哈哈哈哈好的[表情]\n"
	res, raw, err := callLLM(llm, sample)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "raw": raw})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "result": res, "raw": raw, "preview": formatReminder("测试群", res, false)})
}

func (a *API) handleJev(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var jev JevConfig
	if err := decodeJSON(w, r, &jev, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) {
		if jev.APIKey == "" {
			jev.APIKey = c.Jev.APIKey
		}
		if jev.Timeout <= 0 {
			jev.Timeout = c.Jev.Timeout
		}
		if jev.Model == "" {
			jev.Model = c.Jev.Model
		}
		c.Jev = jev
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.pipe.Reconcile(cfg)
	a.broadcastStatus()
	writeJSON(w, 200, redactedConfig(cfg).Jev)
}

func (a *API) handleJevTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var jev JevConfig
	if err := decodeJSON(w, r, &jev, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	// Two illustrative samples: a genuine notice (should pass) and a bare
	// acknowledgement in that notice's context (should be filtered out).
	notice := GroupMessage{GroupName: "测试群", Nickname: "赵老师", Role: "admin",
		Text: "@全体成员 今日内到校医院一楼完成体检，班长收齐体检表交收费处"}
	ack := GroupMessage{GroupName: "测试群", Nickname: "李四", Role: "member", Text: "收到，谢谢老师"}

	type sample struct {
		Label     string  `json:"label"`
		Text      string  `json:"text"`
		Noul      float64 `json:"noul"`
		Important bool    `json:"important"`
	}
	thr := jev.Threshold
	if thr <= 0 {
		thr = 0.6
	}
	run := func(label string, msg GroupMessage, ctx []GroupMessage) (sample, error) {
		n, err := jevImportance(jev, jevState(msg, ctx))
		return sample{Label: label, Text: msg.Text, Noul: n, Important: n >= thr}, err
	}

	s1, err := run("正式通知", notice, nil)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s2, err := run("回执附和", ack, []GroupMessage{notice})
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "threshold": thr, "samples": []sample{s1, s2}})
}

func (a *API) handleOneBotConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var obc OneBotConfig
	if err := decodeJSON(w, r, &obc, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) {
		if obc.Token == "" {
			obc.Token = c.OneBot.Token
		}
		c.OneBot = obc
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.ob.Reconfigure(cfg.OneBot)
	writeJSON(w, 200, redactedConfig(cfg).OneBot)
}

func (a *API) handleEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &body, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Enabled = body.Enabled })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.pipe.Reconcile(cfg)
	a.hub.Log("info", 0, "", fmt.Sprintf("全局监听已%s", ternary(cfg.Enabled, "开启", "暂停")))
	a.broadcastStatus()
	writeJSON(w, 200, map[string]any{"enabled": cfg.Enabled})
}

func (a *API) handleTestNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	cfg := a.store.Get()
	if len(cfg.Masters) == 0 {
		writeErr(w, 400, "尚未配置主人")
		return
	}
	text := "✅ 测试提醒\n──────────\n这是一条来自讯枢的测试消息，说明通知推送通道已打通。"
	sent, failed := sendPrivateConcurrent(a.ob, cfg.Masters, text)
	fails := make([]string, 0, len(failed))
	for _, id := range failed {
		fails = append(fails, fmt.Sprintf("%d", id))
	}
	writeJSON(w, 200, map[string]any{"sent": sent, "failed": fails})
}

func (a *API) handleLookup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("userId")), 10, 64)
	if err != nil || id == 0 {
		writeErr(w, 400, "invalid userId")
		return
	}
	si, err := a.ob.GetStrangerInfo(id)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, si)
}

func (a *API) handleLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.hub.RecentLogs())
}

// handleEscalations backfills the "recent escalations" panel on page load;
// escalation cards otherwise exist only as transient SSE events and vanish on
// refresh.
func (a *API) handleEscalations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.hub.RecentEscalations())
}

func (a *API) handleNotices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "method not allowed")
		return
	}
	if a.notices == nil {
		writeJSON(w, 200, []NoticeRecord{})
		return
	}
	var q NoticeQuery
	q.GroupID, _ = strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("groupId")), 10, 64)
	q.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	q.Limit, _ = strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	q.Before, _ = strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("before")), 10, 64)
	q.BeforeID = strings.TrimSpace(r.URL.Query().Get("beforeId"))
	rows, err := a.notices.QueryContext(r.Context(), q)
	if err != nil {
		writeErr(w, 500, "读取正式通知失败："+err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (a *API) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var backup BackupConfig
	if err := decodeJSON(w, r, &backup, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) {
		if backup.AccessKey == "" {
			backup.AccessKey = c.Backup.AccessKey
		}
		if backup.SecretKey == "" {
			backup.SecretKey = c.Backup.SecretKey
		}
		c.Backup = backup
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, redactedConfig(cfg).Backup)
}

func (a *API) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	if a.backup == nil {
		writeErr(w, 503, "备份服务未启用")
		return
	}
	if err := a.backup.RunOnce(r.Context(), a.store.Get().Backup); err != nil {
		writeErr(w, 502, "备份失败："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *API) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "method not allowed")
		return
	}
	if a.backup == nil {
		writeJSON(w, 200, BackupStatus{})
		return
	}
	writeJSON(w, 200, a.backup.Status())
}

func (a *API) handleDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var documents DocumentConfig
	if err := decodeJSON(w, r, &documents, maxJSONBody); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) {
		if documents.APIKey == "" {
			documents.APIKey = c.Documents.APIKey
		}
		c.Documents = documents
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.pipe.Reconcile(cfg)
	writeJSON(w, 200, redactedConfig(cfg).Documents)
}

func (a *API) handleSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var lastID uint64
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		lastID, _ = strconv.ParseUint(raw, 10, 64)
	}
	ch, replay := a.hub.SubscribeSince(lastID)
	defer a.hub.Unsubscribe(ch)

	write := func(ev Event) error {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			// Some test writers do not expose a deadline; writing is still safe.
			if err != http.ErrNotSupported {
				return err
			}
		}
		payload, err := ev.EncodeChecked()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.ID, payload); err != nil {
			return err
		}
		fl.Flush()
		return nil
	}

	// Replay missed events before the live stream, then prime with current status.
	for _, ev := range replay {
		if err := write(ev); err != nil {
			return
		}
	}
	if err := write(Event{Type: "hello", Data: a.statusPayload(), TS: time.Now().UnixMilli()}); err != nil {
		return
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := write(ev); err != nil {
				return
			}
		case <-ping.C:
			if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && err != http.ErrNotSupported {
				return
			}
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (a *API) statusPayload() map[string]any {
	cfg := a.store.Get()
	watched := 0
	for _, g := range cfg.Groups {
		if g.Watch {
			watched++
		}
	}
	return map[string]any{
		"onebotConnected": a.ob.Connected(),
		"selfId":          a.ob.SelfID(),
		"enabled":         cfg.Enabled,
		"llmEnabled":      cfg.LLM.Enabled,
		"jevEnabled":      cfg.Jev.Enabled,
		"watchedGroups":   watched,
		"masters":         len(cfg.Masters),
	}
}

func (a *API) broadcastStatus() {
	a.hub.Broadcast("status", a.statusPayload())
}

func ternary(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}
