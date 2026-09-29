package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxOneBotResponseBytes = 4 << 20
	maxOneBotWSFrameBytes  = 4 << 20
	maxSeenMessageIDs      = 4096
	seenMessageTTL         = 15 * time.Minute
)

// OneBot is a thin client for a NapCat (OneBot v11) instance: it keeps a
// WebSocket connection open to receive events, and calls the HTTP API to send
// messages and query metadata.
type OneBot struct {
	mu        sync.RWMutex
	httpBase  string
	wsURL     string
	token     string
	client    *http.Client
	connected atomic.Bool
	selfID    atomic.Int64

	onEvent func(GroupMessage) // pipeline hook
	onRaw   func(map[string]any)

	// lastLogin caches the most recent successful get_login_info so /api/status
	// can report the account without a blocking round-trip on every poll.
	lastLogin atomic.Pointer[LoginInfo]

	cancel context.CancelFunc
	genMu  sync.Mutex // serialises cancel swaps across reconfigures
	gen    uint64

	seenMu         sync.Mutex
	seenMessageIDs map[int64]time.Time
}

// GroupMessage is the normalised form of an OneBot group message event.
type GroupMessage struct {
	Time      int64  `json:"time"`
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"` // card if present, else nickname
	Role      string `json:"role"`     // owner | admin | member
	Text      string `json:"text"`     // flattened plain text
	AtAll     bool   `json:"atAll"`
	AtSelf    bool   `json:"atSelf"`
	HasImage  bool   `json:"hasImage"`
	MessageID int64  `json:"messageId"`
	RawSender string `json:"-"`
}

func NewOneBot() *OneBot {
	client := &http.Client{Timeout: 20 * time.Second}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &OneBot{client: client, seenMessageIDs: make(map[int64]time.Time)}
}

func (o *OneBot) Connected() bool { return o.connected.Load() }
func (o *OneBot) SelfID() int64   { return o.selfID.Load() }

// Reconfigure points the client at (possibly new) endpoints and restarts the
// WebSocket loop.
func (o *OneBot) Reconfigure(c OneBotConfig) {
	o.genMu.Lock()
	o.mu.Lock()
	o.httpBase = strings.TrimRight(c.HTTPBase, "/")
	o.wsURL = c.WSURL
	o.token = c.Token
	o.mu.Unlock()
	o.selfID.Store(0)
	o.lastLogin.Store(nil)

	if o.cancel != nil {
		o.cancel()
	}
	o.gen++
	gen := o.gen
	o.connected.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel
	o.genMu.Unlock()

	go o.runWS(ctx, gen)
}

func (o *OneBot) setConnected(gen uint64, connected bool) {
	o.genMu.Lock()
	if o.gen == gen {
		o.connected.Store(connected)
	}
	o.genMu.Unlock()
}

func (o *OneBot) runWS(ctx context.Context, gen uint64) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := o.dialAndRead(ctx, gen); err != nil {
			wasConnected := o.Connected()
			o.setConnected(gen, false)
			if ctx.Err() == nil {
				if wasConnected {
					backoff = time.Second
				}
				log.Printf("[onebot] ws disconnected: %v (retry in %s)", err, backoff)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func (o *OneBot) dialAndRead(ctx context.Context, gen uint64) error {
	o.mu.RLock()
	url, token := o.wsURL, o.token
	o.mu.RUnlock()
	if url == "" {
		return fmt.Errorf("no ws url configured")
	}

	h := http.Header{}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := d.DialContext(ctx, url, h)
	if err != nil {
		return err
	}
	defer conn.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	o.setConnected(gen, true)
	log.Printf("[onebot] ws connected to %s", url)

	// Close the socket when the context is cancelled (reconfigure/shutdown).
	// The closer's lifetime is tied to this connection via `done` so it exits
	// as soon as the read loop returns — otherwise every reconnect would leak a
	// goroutine blocked on ctx.Done() until the next reconfigure.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	// Heartbeat watchdog: NapCat emits a heartbeat meta-event (~30s) plus pongs.
	// Without a read deadline, a silently half-open socket would block
	// ReadMessage forever, leaving `connected` true and never reconnecting.
	const readWait = 90 * time.Second
	conn.SetReadLimit(maxOneBotWSFrameBytes)
	conn.SetReadDeadline(time.Now().Add(readWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readWait))
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		conn.SetReadDeadline(time.Now().Add(readWait))
		if ctx.Err() == nil {
			o.handleFrameForGeneration(data, gen)
		}
	}
}

func (o *OneBot) handleFrame(data []byte) {
	o.handleFrameForGeneration(data, 0)
}

func (o *OneBot) handleFrameForGeneration(data []byte, gen uint64) {
	if gen != 0 && !o.isCurrentGeneration(gen) {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if o.onRaw != nil {
		o.onRaw(raw)
	}
	// Capture self id from lifecycle/meta events.
	if sid, ok := raw["self_id"].(float64); ok {
		o.selfID.Store(int64(sid))
	}
	if raw["post_type"] != "message" {
		return
	}
	if raw["message_type"] != "group" {
		return
	}
	gm := parseGroupMessage(raw, o.SelfID())
	if gm.MessageID > 0 && o.seenMessage(gm.MessageID) {
		return
	}
	if o.onEvent != nil {
		o.onEvent(gm)
	}
}

func (o *OneBot) isCurrentGeneration(gen uint64) bool {
	o.genMu.Lock()
	defer o.genMu.Unlock()
	return o.gen == gen
}

func (o *OneBot) seenMessage(id int64) bool {
	now := time.Now()
	o.seenMu.Lock()
	defer o.seenMu.Unlock()
	if o.seenMessageIDs == nil {
		o.seenMessageIDs = make(map[int64]time.Time)
	}
	for oldID, at := range o.seenMessageIDs {
		if now.Sub(at) > seenMessageTTL {
			delete(o.seenMessageIDs, oldID)
		}
	}
	if _, ok := o.seenMessageIDs[id]; ok {
		return true
	}
	if len(o.seenMessageIDs) >= maxSeenMessageIDs {
		var oldestID int64
		var oldest time.Time
		for oldID, at := range o.seenMessageIDs {
			if oldest.IsZero() || at.Before(oldest) {
				oldestID, oldest = oldID, at
			}
		}
		delete(o.seenMessageIDs, oldestID)
	}
	o.seenMessageIDs[id] = now
	return false
}

func parseGroupMessage(raw map[string]any, selfID int64) GroupMessage {
	gm := GroupMessage{}
	gm.Time = toInt64(raw["time"])
	gm.GroupID = toInt64(raw["group_id"])
	gm.UserID = toInt64(raw["user_id"])
	gm.MessageID = toInt64(raw["message_id"])

	if s, ok := raw["sender"].(map[string]any); ok {
		gm.Role, _ = s["role"].(string)
		card, _ := s["card"].(string)
		nick, _ := s["nickname"].(string)
		if strings.TrimSpace(card) != "" {
			gm.Nickname = card
		} else {
			gm.Nickname = nick
		}
	}
	if gm.Role == "" {
		gm.Role = "member"
	}

	gm.Text, gm.AtAll, gm.AtSelf, gm.HasImage = flattenMessage(raw["message"], selfID)
	return gm
}

// flattenMessage renders the OneBot "array" message format (and the string
// fallback) into plain text, and reports @all / @self / image presence.
func flattenMessage(msg any, selfID int64) (text string, atAll, atSelf, hasImage bool) {
	var b strings.Builder
	switch m := msg.(type) {
	case string:
		text = m
	case []any:
		for _, seg := range m {
			sm, ok := seg.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := sm["type"].(string)
			data, _ := sm["data"].(map[string]any)
			switch typ {
			case "text":
				if t, ok := data["text"].(string); ok {
					b.WriteString(t)
				}
			case "at":
				if fmt.Sprint(data["qq"]) == "all" {
					atAll = true
					b.WriteString("@全体成员 ")
				} else {
					id := toInt64(data["qq"])
					if id == selfID && selfID != 0 {
						atSelf = true
					}
					// Format the integer id directly. A numeric qq arrives as a
					// JSON float64, and fmt.Sprint on a large float renders it in
					// scientific notation (e.g. "@3.8069e+09").
					b.WriteString(fmt.Sprintf("@%d ", id))
				}
			case "image":
				hasImage = true
				b.WriteString("[图片]")
			case "face":
				b.WriteString("[表情]")
			case "reply":
				b.WriteString("[引用]")
			case "record":
				b.WriteString("[语音]")
			case "video":
				b.WriteString("[视频]")
			case "file":
				name, _ := data["file"].(string)
				b.WriteString("[文件:" + name + "]")
			case "json", "xml":
				b.WriteString("[卡片]")
			case "forward":
				b.WriteString("[聊天记录]")
			case "markdown":
				if c, ok := data["content"].(string); ok {
					b.WriteString(c)
				}
			}
		}
		text = b.String()
	}
	return strings.TrimSpace(text), atAll, atSelf, hasImage
}

// ---- HTTP API ----

func (o *OneBot) call(action string, params map[string]any) (json.RawMessage, error) {
	return o.callContext(context.Background(), action, params)
}

func (o *OneBot) callContext(ctx context.Context, action string, params map[string]any) (json.RawMessage, error) {
	o.mu.RLock()
	base, token := o.httpBase, o.token
	o.mu.RUnlock()
	if base == "" {
		return nil, fmt.Errorf("no http base configured")
	}
	body, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+action, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("onebot %s http %d: %s", action, resp.StatusCode, truncate(string(raw), 200))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxOneBotResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read onebot %s response: %w", action, err)
	}
	if len(raw) > maxOneBotResponseBytes {
		return nil, fmt.Errorf("onebot %s response too large", action)
	}
	var env struct {
		Status  string          `json:"status"`
		Retcode int             `json:"retcode"`
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("bad response: %s", truncate(string(raw), 200))
	}
	// A call succeeds only when retcode is 0 and the status is not "failed".
	// The old `retcode != 0 && status != "ok"` let a {retcode:0,status:"failed"}
	// envelope slip through as success, handing callers empty/garbage data.
	if env.Retcode != 0 || env.Status == "failed" {
		return nil, fmt.Errorf("onebot %s failed: %s", action, env.Message)
	}
	return env.Data, nil
}

type GroupInfo struct {
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`
	GroupRemark    string `json:"group_remark"` // the user's own note for the group, set in QQ
	MemberCount    int    `json:"member_count"`
	MaxMemberCount int    `json:"max_member_count"`
}

func (o *OneBot) GetGroupList() ([]GroupInfo, error) {
	data, err := o.call("get_group_list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var gs []GroupInfo
	if err := json.Unmarshal(data, &gs); err != nil {
		return nil, err
	}
	return gs, nil
}

type LoginInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
}

func (o *OneBot) GetLoginInfo() (LoginInfo, error) {
	var li LoginInfo
	data, err := o.call("get_login_info", map[string]any{})
	if err != nil {
		return li, err
	}
	err = json.Unmarshal(data, &li)
	if err == nil && li.UserID != 0 {
		o.selfID.Store(li.UserID)
		o.lastLogin.Store(&li)
	}
	return li, err
}

// CachedLoginInfo returns the last successfully fetched login info without a
// network round-trip. Zero value until the first successful GetLoginInfo.
func (o *OneBot) CachedLoginInfo() LoginInfo {
	if li := o.lastLogin.Load(); li != nil {
		return *li
	}
	return LoginInfo{}
}

type StrangerInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
}

func (o *OneBot) GetStrangerInfo(userID int64) (StrangerInfo, error) {
	var si StrangerInfo
	data, err := o.call("get_stranger_info", map[string]any{"user_id": userID})
	if err != nil {
		return si, err
	}
	return si, json.Unmarshal(data, &si)
}

func (o *OneBot) SendPrivateMsg(userID int64, text string) error {
	_, err := o.call("send_private_msg", map[string]any{
		"user_id": userID,
		"message": text,
	})
	return err
}

// HistoryFile is one downloadable file segment inside a history message. Its
// download URL is resolved on demand via /api/groups/file-url (get_group_file_url),
// since group-file links are short-lived and signed.
type HistoryFile struct {
	Name   string `json:"name"`
	FileID string `json:"fileId"`
	Size   int64  `json:"size"`
	Busid  int64  `json:"busid"`
	URL    string `json:"url,omitempty"` // set only if the segment already carried one
}

// ReplyQuote is a compact preview of the message a reply segment quotes, so the
// UI can show the original text instead of a bare "[引用]" placeholder.
type ReplyQuote struct {
	UserID   int64  `json:"userId"`
	Nickname string `json:"nickname"`
	Text     string `json:"text"`
}

// MsgSegment is one structured piece of a history message. It lets the frontend
// render media, @-mentions and quotes natively (inline image/voice/video, @name,
// quoted original) rather than the bracket placeholders flattenMessage emits for
// the live feed. Only the fields relevant to Type are populated.
type MsgSegment struct {
	Type    string      `json:"type"`              // text|at|image|face|record|video|reply|card|forward
	Text    string      `json:"text,omitempty"`    // text body; face/card/forward label
	Name    string      `json:"name,omitempty"`    // at: resolved display name
	ID      int64       `json:"id,omitempty"`      // at: target QQ (0 = @全体成员)
	URL     string      `json:"url,omitempty"`     // image/record/video source
	File    string      `json:"file,omitempty"`    // record: NapCat voice file id, for server-side mp3 transcode
	Sticker bool        `json:"sticker,omitempty"` // image: a sticker/大表情 (sub_type=1 or mface) — render small
	Reply   *ReplyQuote `json:"reply,omitempty"`   // reply: the quoted original
}

// HistoryMsg is one flattened entry from a group's message history, shaped for
// the chat-log view (no classification, just who-said-what-when). Text/HasImage
// stay for backward-compat; Segments carries the structured render (preferred).
type HistoryMsg struct {
	MessageID  int64         `json:"messageId"`
	MessageSeq int64         `json:"messageSeq"` // pagination cursor for older batches
	UserID     int64         `json:"userId"`
	Nickname   string        `json:"nickname"` // card if present, else nickname
	Role       string        `json:"role"`     // owner | admin | member
	Title      string        `json:"title"`    // custom group title (头衔), if any
	Time       int64         `json:"time"`     // unix seconds
	Text       string        `json:"text"`     // flattened plain text (fallback)
	HasImage   bool          `json:"hasImage"`
	Files      []HistoryFile `json:"files,omitempty"`
	Segments   []MsgSegment  `json:"segments,omitempty"`
	IsSelf     bool          `json:"isSelf"`
}

// GetGroupMsgHistory fetches up to count messages for a group and flattens them
// for display. beforeSeq is a pagination cursor: 0 asks NapCat for the latest
// window; a non-zero value anchors on that message_seq so the frontend can
// lazy-load older batches. Messages come back oldest-first.
func (o *OneBot) GetGroupMsgHistory(groupID int64, count int, beforeSeq int64) ([]HistoryMsg, error) {
	return o.GetGroupMsgHistoryContext(context.Background(), groupID, count, beforeSeq)
}

func (o *OneBot) GetGroupMsgHistoryContext(ctx context.Context, groupID int64, count int, beforeSeq int64) ([]HistoryMsg, error) {
	if count <= 0 || count > 60 {
		count = 30
	}
	if beforeSeq < 0 {
		beforeSeq = 0
	}
	// reverseOrder:true is essential for scroll-up paging. NapCat defaults to
	// forward-inclusive (anchor + NEWER), so anchoring on the oldest message we
	// already show returns only messages already on screen — the "到头就说没有更早
	// 的消息" bug. With reverseOrder:true the same anchor returns the anchor + OLDER
	// messages (still oldest-first), so the cursor walks backward through history.
	// message_seq=0 stays the "from latest" sentinel and returns the newest window.
	data, err := o.callContext(ctx, "get_group_msg_history", map[string]any{
		"group_id":     groupID,
		"message_seq":  beforeSeq,
		"count":        count,
		"reverseOrder": true,
	})
	if err != nil {
		return nil, err
	}
	var env struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		// Some builds return a bare array rather than {messages:[...]}.
		var arr []map[string]any
		if err2 := json.Unmarshal(data, &arr); err2 != nil {
			return nil, err
		}
		env.Messages = arr
	}
	self := o.SelfID()
	// Resolve @-mention names from a single member-list call, and quoted
	// originals from get_msg (cached per batch). Both are best-effort: on any
	// failure the UI falls back to the raw QQ id / a "[消息]" stub.
	members := o.GetGroupMemberListContext(ctx, groupID)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nameOf := func(id int64) string { return members[id] }
	quoteCache := o.prefetchQuotes(ctx, env.Messages)
	quoteOf := func(id int64) *ReplyQuote {
		if id == 0 {
			return nil
		}
		return quoteCache[id]
	}
	out := make([]HistoryMsg, 0, len(env.Messages))
	for _, raw := range env.Messages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hm := HistoryMsg{
			MessageID:  toInt64(raw["message_id"]),
			MessageSeq: toInt64(raw["message_seq"]),
			UserID:     toInt64(raw["user_id"]),
			Time:       toInt64(raw["time"]),
		}
		if hm.MessageSeq == 0 {
			hm.MessageSeq = toInt64(raw["real_seq"]) // some builds name it real_seq
		}
		if s, ok := raw["sender"].(map[string]any); ok {
			hm.Role, _ = s["role"].(string)
			hm.Title, _ = s["title"].(string)
			card, _ := s["card"].(string)
			nick, _ := s["nickname"].(string)
			if strings.TrimSpace(card) != "" {
				hm.Nickname = card
			} else {
				hm.Nickname = nick
			}
		}
		if hm.Role == "" {
			hm.Role = "member"
		}
		hm.Text, _, _, hm.HasImage = flattenMessage(raw["message"], self)
		hm.Files = extractFiles(raw["message"])
		hm.Segments = buildSegments(raw["message"], self, nameOf, quoteOf)
		hm.IsSelf = hm.UserID == self && self != 0
		out = append(out, hm)
	}
	return out, nil
}

// extractFiles pulls structured file segments out of an OneBot array message so
// the UI can render download cards. String (non-array) messages have none.
func extractFiles(msg any) []HistoryFile {
	arr, ok := msg.([]any)
	if !ok {
		return nil
	}
	var files []HistoryFile
	for _, seg := range arr {
		sm, ok := seg.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := sm["type"].(string); t != "file" {
			continue
		}
		data, _ := sm["data"].(map[string]any)
		if data == nil {
			continue
		}
		f := HistoryFile{Busid: toInt64(data["busid"]), Size: toInt64(data["file_size"])}
		f.Name, _ = data["file"].(string)
		f.FileID = toStr(data["file_id"])
		if f.FileID == "" {
			f.FileID = toStr(data["file_unique"])
		}
		f.URL, _ = data["url"].(string)
		files = append(files, f)
	}
	return files
}

// buildSegments renders an OneBot array message into structured segments for the
// history view. names resolves a QQ id to a group display name; quote resolves a
// quoted message id to its original. Both are best-effort (may return a zero
// value / nil). File segments are skipped here — extractFiles renders them as
// download cards. A string (non-array) message yields a single text segment.
func buildSegments(msg any, selfID int64, names func(int64) string, quote func(int64) *ReplyQuote) []MsgSegment {
	pick := func(d map[string]any) string {
		if u := toStr(d["url"]); u != "" {
			return u
		}
		if f := toStr(d["file"]); strings.HasPrefix(f, "http") {
			return f
		}
		return ""
	}
	arr, ok := msg.([]any)
	if !ok {
		if s, _ := msg.(string); strings.TrimSpace(s) != "" {
			return []MsgSegment{{Type: "text", Text: s}}
		}
		return nil
	}
	out := make([]MsgSegment, 0, len(arr))
	for _, seg := range arr {
		sm, ok := seg.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := sm["type"].(string)
		data, _ := sm["data"].(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		switch typ {
		case "text":
			if t, _ := data["text"].(string); t != "" {
				out = append(out, MsgSegment{Type: "text", Text: t})
			}
		case "at":
			if fmt.Sprint(data["qq"]) == "all" {
				out = append(out, MsgSegment{Type: "at", Name: "全体成员"})
				break
			}
			id := toInt64(data["qq"])
			name, _ := data["name"].(string)
			if strings.TrimSpace(name) == "" && names != nil {
				name = names(id)
			}
			if strings.TrimSpace(name) == "" {
				name = fmt.Sprintf("%d", id)
			}
			out = append(out, MsgSegment{Type: "at", ID: id, Name: name})
		case "image":
			// sub_type 1 marks a sticker/大表情 (vs a real photo); QQ商城 stickers
			// also arrive as their own "mface" segment. Both render small.
			out = append(out, MsgSegment{Type: "image", URL: pick(data), Sticker: toStr(data["sub_type"]) == "1"})
		case "mface":
			out = append(out, MsgSegment{Type: "image", URL: pick(data), Sticker: true})
		case "record":
			out = append(out, MsgSegment{Type: "record", URL: pick(data), File: toStr(data["file"])})
		case "video":
			out = append(out, MsgSegment{Type: "video", URL: pick(data)})
		case "face":
			out = append(out, MsgSegment{Type: "face", Text: "[表情]"})
		case "reply":
			out = append(out, MsgSegment{Type: "reply", Reply: quote(toInt64(data["id"]))})
		case "json", "xml":
			out = append(out, MsgSegment{Type: "card", Text: "[卡片]"})
		case "forward":
			out = append(out, MsgSegment{Type: "forward", Text: "[聊天记录]"})
		case "markdown":
			if c, _ := data["content"].(string); c != "" {
				out = append(out, MsgSegment{Type: "text", Text: c})
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetGroupMemberList fetches a group's members and returns an id→display-name
// map (card if set, else nickname). Best-effort: on any failure it returns an
// empty map so callers fall back to the raw QQ id. Used to resolve @-mention
// names in history without a per-mention round-trip.
func (o *OneBot) GetGroupMemberList(groupID int64) map[int64]string {
	return o.GetGroupMemberListContext(context.Background(), groupID)
}

func (o *OneBot) GetGroupMemberListContext(ctx context.Context, groupID int64) map[int64]string {
	out := map[int64]string{}
	data, err := o.callContext(ctx, "get_group_member_list", map[string]any{"group_id": groupID})
	if err != nil {
		return out
	}
	var members []struct {
		UserID   int64  `json:"user_id"`
		Card     string `json:"card"`
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(data, &members); err != nil {
		return out
	}
	for _, m := range members {
		if strings.TrimSpace(m.Card) != "" {
			out[m.UserID] = m.Card
		} else {
			out[m.UserID] = m.Nickname
		}
	}
	return out
}

// GetMsg resolves one message id to a compact quote preview (sender name +
// flattened text) for rendering reply segments. Best-effort: returns a "[消息]"
// stub on any failure so the UI still shows something for the quoted original.
func (o *OneBot) GetMsg(messageID int64) *ReplyQuote {
	return o.GetMsgContext(context.Background(), messageID)
}

func (o *OneBot) GetMsgContext(ctx context.Context, messageID int64) *ReplyQuote {
	data, err := o.callContext(ctx, "get_msg", map[string]any{"message_id": messageID})
	if err != nil {
		return &ReplyQuote{Text: "[消息]"}
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return &ReplyQuote{Text: "[消息]"}
	}
	q := &ReplyQuote{UserID: toInt64(raw["user_id"])}
	if s, ok := raw["sender"].(map[string]any); ok {
		card, _ := s["card"].(string)
		nick, _ := s["nickname"].(string)
		if strings.TrimSpace(card) != "" {
			q.Nickname = card
		} else {
			q.Nickname = nick
		}
	}
	q.Text, _, _, _ = flattenMessage(raw["message"], o.SelfID())
	if strings.TrimSpace(q.Text) == "" {
		q.Text = "[消息]"
	}
	return q
}

func (o *OneBot) prefetchQuotes(ctx context.Context, messages []map[string]any) map[int64]*ReplyQuote {
	ids := make(map[int64]struct{})
	for _, raw := range messages {
		collectReplyIDs(raw["message"], ids)
	}
	quotes := make(map[int64]*ReplyQuote, len(ids))
	if len(ids) == 0 {
		return quotes
	}
	jobs := make(chan int64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := 8
	if len(ids) < workers {
		workers = len(ids)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case id, ok := <-jobs:
					if !ok {
						return
					}
					q := o.GetMsgContext(ctx, id)
					mu.Lock()
					quotes[id] = q
					mu.Unlock()
				}
			}
		}()
	}
	for id := range ids {
		select {
		case jobs <- id:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	return quotes
}

func collectReplyIDs(msg any, ids map[int64]struct{}) {
	arr, ok := msg.([]any)
	if !ok {
		return
	}
	for _, seg := range arr {
		sm, ok := seg.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := sm["type"].(string)
		if typ != "reply" {
			continue
		}
		if data, _ := sm["data"].(map[string]any); data != nil {
			if id := toInt64(data["id"]); id > 0 {
				ids[id] = struct{}{}
			}
		}
	}
}

// GetGroupFileURL resolves a short-lived download URL for a group file. busid is
// optional (0 omits it) for newer NapCat builds that key only on file_id.
func (o *OneBot) GetGroupFileURL(groupID int64, fileID string, busid int64) (string, error) {
	params := map[string]any{"group_id": groupID, "file_id": fileID}
	if busid != 0 {
		params["busid"] = busid
	}
	data, err := o.call("get_group_file_url", params)
	if err != nil {
		return "", err
	}
	var r struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", err
	}
	return r.URL, nil
}

// GetRecordMP3 transcodes a group voice message to MP3 via NapCat's get_record
// (ffmpeg under the hood) and returns the decoded bytes. QQ voice is AMR/SILK,
// which no browser plays natively — and the CDN download even mislabels the AMR
// as audio/mp3, so a direct <audio src=cdn> just fails to load. get_record hands
// back the transcoded audio inline as base64 (the file/url it also returns are
// paths inside the NapCat container, unreachable from here), so we decode that.
func (o *OneBot) GetRecordMP3(file string) ([]byte, error) {
	data, err := o.call("get_record", map[string]any{"file": file, "out_format": "mp3"})
	if err != nil {
		return nil, err
	}
	var r struct {
		Base64 string `json:"base64"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Base64 == "" {
		return nil, fmt.Errorf("get_record 未返回音频数据")
	}
	// Some builds prefix a data: URI scheme on the base64; tolerate both forms.
	b64 := r.Base64
	if strings.HasPrefix(b64, "data:") {
		if i := strings.Index(b64, ","); i >= 0 {
			b64 = b64[i+1:]
		}
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("解码音频失败：%w", err)
	}
	return raw, nil
}

// ---- helpers ----

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		var i int64
		fmt.Sscan(n, &i)
		return i
	}
	return 0
}

func toStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return fmt.Sprintf("%d", int64(s))
	case int64:
		return fmt.Sprintf("%d", s)
	case json.Number:
		return s.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
