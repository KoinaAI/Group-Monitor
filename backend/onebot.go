package main

import (
	"bytes"
	"context"
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
	return &OneBot{client: &http.Client{Timeout: 20 * time.Second}}
}

func (o *OneBot) Connected() bool { return o.connected.Load() }
func (o *OneBot) SelfID() int64   { return o.selfID.Load() }

// Reconfigure points the client at (possibly new) endpoints and restarts the
// WebSocket loop.
func (o *OneBot) Reconfigure(c OneBotConfig) {
	o.mu.Lock()
	o.httpBase = strings.TrimRight(c.HTTPBase, "/")
	o.wsURL = c.WSURL
	o.token = c.Token
	o.mu.Unlock()

	o.genMu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel
	o.genMu.Unlock()

	go o.runWS(ctx)
}

func (o *OneBot) runWS(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := o.dialAndRead(ctx); err != nil {
			o.connected.Store(false)
			log.Printf("[onebot] ws disconnected: %v (retry in %s)", err, backoff)
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

func (o *OneBot) dialAndRead(ctx context.Context) error {
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
	o.connected.Store(true)
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
		o.handleFrame(data)
	}
}

func (o *OneBot) handleFrame(data []byte) {
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
	if o.onEvent != nil {
		o.onEvent(gm)
	}
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
	o.mu.RLock()
	base, token := o.httpBase, o.token
	o.mu.RUnlock()
	if base == "" {
		return nil, fmt.Errorf("no http base configured")
	}
	body, _ := json.Marshal(params)
	req, err := http.NewRequest(http.MethodPost, base+"/"+action, bytes.NewReader(body))
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
	raw, _ := io.ReadAll(resp.Body)
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

// HistoryMsg is one flattened entry from a group's message history, shaped for
// the chat-log view (no classification, just who-said-what-when).
type HistoryMsg struct {
	MessageID int64  `json:"messageId"`
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"` // card if present, else nickname
	Role      string `json:"role"`     // owner | admin | member
	Time      int64  `json:"time"`     // unix seconds
	Text      string `json:"text"`     // flattened plain text
	HasImage  bool   `json:"hasImage"`
	IsSelf    bool   `json:"isSelf"`
}

// GetGroupMsgHistory fetches up to count recent messages for a group and
// flattens them for display. message_seq:0 asks NapCat for the latest window.
func (o *OneBot) GetGroupMsgHistory(groupID int64, count int) ([]HistoryMsg, error) {
	if count <= 0 || count > 60 {
		count = 30
	}
	data, err := o.call("get_group_msg_history", map[string]any{
		"group_id":    groupID,
		"message_seq": 0,
		"count":       count,
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
	out := make([]HistoryMsg, 0, len(env.Messages))
	for _, raw := range env.Messages {
		hm := HistoryMsg{
			MessageID: toInt64(raw["message_id"]),
			UserID:    toInt64(raw["user_id"]),
			Time:      toInt64(raw["time"]),
		}
		if s, ok := raw["sender"].(map[string]any); ok {
			hm.Role, _ = s["role"].(string)
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
		hm.IsSelf = hm.UserID == self && self != 0
		out = append(out, hm)
	}
	return out, nil
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
