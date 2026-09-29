package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxForwardDepth  = 4
	maxUnpackedBytes = 32 << 10
	maxUnpackedItems = 512
	maxCardBytes     = 64 << 10
)

// unpackWriter bounds the entire rendered tree, including nested forwards.
type unpackWriter struct {
	strings.Builder
	items int
}

func (w *unpackWriter) add(s string) {
	left := maxUnpackedBytes - w.Len()
	if left <= 0 {
		return
	}
	w.WriteString(unpackTextLimit(s, left))
}

func unpackTextLimit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// flattenUnpackedMessage is pure: it never fetches chat URLs or OneBot data.
// Mentions in quoted/forwarded originals do not mark the new message @all/@self.
func flattenUnpackedMessage(msg any, selfID int64) (text string, atAll, atSelf, hasImage bool) {
	w := &unpackWriter{}
	atAll, atSelf, hasImage = renderUnpackedMessage(w, msg, selfID, 0)
	return strings.TrimSpace(w.String()), atAll, atSelf, hasImage
}

func renderUnpackedMessage(w *unpackWriter, msg any, selfID int64, depth int) (atAll, atSelf, hasImage bool) {
	if s, ok := msg.(string); ok {
		w.add(s)
		return
	}
	arr, _ := msg.([]any)
	for _, seg := range arr {
		if w.items >= maxUnpackedItems || w.Len() >= maxUnpackedBytes {
			break
		}
		w.items++
		sm, _ := seg.(map[string]any)
		typ, _ := sm["type"].(string)
		data, _ := sm["data"].(map[string]any)
		switch typ {
		case "text":
			w.add(toStr(data["text"]))
		case "at":
			if toStr(data["qq"]) == "all" {
				atAll = true
				w.add("@全体成员 ")
			} else {
				id := toInt64(data["qq"])
				if id == selfID && selfID != 0 {
					atSelf = true
				}
				w.add(fmt.Sprintf("@%d ", id))
			}
		case "image":
			hasImage = true
			w.add("[图片]")
		case "face", "mface":
			w.add("[表情]")
		case "reply":
			if q := inlineReplyQuote(data); q != nil {
				w.add("［引用 ")
				if q.Nickname != "" {
					w.add(q.Nickname + "：")
				}
				w.add(q.Text)
				w.add("］")
			} else {
				w.add("[引用]")
			}
		case "record":
			w.add("[语音]")
		case "video":
			w.add("[视频]")
		case "file":
			w.add("[文件:" + firstUnpackString(data["file"], data["name"]) + "]")
		case "json", "xml":
			w.add(flattenCard(typ, data))
		case "forward":
			nodes, _ := data["content"].([]any)
			if len(nodes) == 0 {
				w.add("[聊天记录]")
			} else {
				renderUnpackedNodes(w, nodes, selfID, depth)
			}
		case "markdown":
			w.add(toStr(data["content"]))
		case "":
		default:
			w.add("[" + unpackTextLimit(typ, 64) + "]")
		}
	}
	return
}

func flattenForwardInline(data map[string]any, selfID int64, depth int) string {
	nodes, _ := data["content"].([]any)
	if len(nodes) == 0 {
		return ""
	}
	w := &unpackWriter{}
	renderUnpackedNodes(w, nodes, selfID, depth)
	return w.String()
}

func renderUnpackedNodes(w *unpackWriter, nodes []any, selfID int64, depth int) {
	if depth >= maxForwardDepth {
		w.add("[聊天记录]")
		return
	}
	w.add("［聊天记录 ")
	first := true
	for _, node := range nodes {
		if w.items >= maxUnpackedItems || w.Len() >= maxUnpackedBytes {
			break
		}
		w.items++
		n, ok := node.(map[string]any)
		if !ok {
			continue
		}
		if !first {
			w.add(" ┃ ")
		}
		first = false
		nick, inner := unpackForwardNode(n)
		if nick != "" {
			w.add(unpackTextLimit(nick, 256) + "：")
		}
		renderUnpackedMessage(w, inner, selfID, depth+1)
	}
	w.add("］")
}

func unpackForwardNode(n map[string]any) (string, any) {
	d, _ := n["data"].(map[string]any)
	s, _ := n["sender"].(map[string]any)
	nick := firstUnpackString(d["nickname"], d["nick"], s["card"], s["nickname"])
	inner := d["content"]
	if inner == nil {
		inner = d["message"]
	}
	if inner == nil {
		inner = n["message"]
	}
	if inner == nil {
		inner = n["content"]
	}
	return nick, inner
}

func firstUnpackString(values ...any) string {
	for _, v := range values {
		if s := strings.TrimSpace(toStr(v)); s != "" {
			return s
		}
	}
	return ""
}

func inlineReplyQuote(data map[string]any) *ReplyQuote {
	s, _ := data["_expanded_text"].(string)
	if s == "" {
		return nil
	}
	return &ReplyQuote{UserID: toInt64(data["_expanded_user_id"]), Nickname: toStr(data["_expanded_nickname"]), Text: s}
}

var cardTextKeys = map[string]bool{
	"title": true, "desc": true, "text": true, "tag": true, "summary": true,
	"brief": true, "label": true, "content": true, "nativetext": true,
}

func flattenCard(typ string, data map[string]any) string {
	raw, _ := data["data"].(string)
	if len(raw) == 0 || len(raw) > maxCardBytes {
		return "[卡片]"
	}
	var parts []string
	if typ == "xml" {
		parts = cardXMLText(raw)
	} else {
		var card map[string]any
		if json.Unmarshal([]byte(raw), &card) != nil {
			return "[卡片]"
		}
		if p, _ := card["prompt"].(string); p != "" {
			parts = append(parts, p)
		}
		announcement := toStr(card["app"]) == "com.tencent.mannounce"
		count := 0
		collectUnpackedCardText(card["meta"], announcement, 0, &count, &parts)
	}
	seen := map[string]bool{}
	w := &unpackWriter{}
	for _, p := range parts {
		p = strings.Join(strings.Fields(p), " ")
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if w.Len() > 0 {
			w.add(" ")
		}
		w.add(p)
	}
	if w.Len() == 0 {
		return "[卡片]"
	}
	return "［卡片 " + unpackTextLimit(w.String(), maxUnpackedBytes-16) + "］"
}

func collectUnpackedCardText(v any, decode bool, depth int, count *int, out *[]string) {
	if depth > 12 || *count >= maxUnpackedItems {
		return
	}
	*count++
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s, ok := t[k].(string); ok {
				if cardTextKeys[strings.ToLower(k)] {
					if decode {
						s = maybeDecodeAnnouncement(s)
					}
					*out = append(*out, s)
				}
			} else {
				collectUnpackedCardText(t[k], decode, depth+1, count, out)
			}
			if len(*out) >= maxUnpackedItems {
				return
			}
		}
	case []any:
		for _, e := range t {
			collectUnpackedCardText(e, decode, depth+1, count, out)
		}
	}
}

func maybeDecodeAnnouncement(s string) string {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || !utf8.Valid(b) || len(b) == 0 {
		return s
	}
	for _, r := range string(b) {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return s
		}
	}
	return string(b)
}

func cardXMLText(raw string) []string {
	d := xml.NewDecoder(strings.NewReader(raw))
	var parts []string
	var stack []bool
	for n := 0; n < maxUnpackedItems*4; n++ {
		token, err := d.Token()
		if err == io.EOF {
			return parts
		}
		if err != nil {
			return nil
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) >= 32 {
				return parts
			}
			selected := cardTextKeys[strings.ToLower(t.Name.Local)]
			if len(stack) > 0 {
				selected = selected || stack[len(stack)-1]
			}
			stack = append(stack, selected)
			for _, a := range t.Attr {
				if cardTextKeys[strings.ToLower(a.Name.Local)] {
					parts = append(parts, a.Value)
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] {
				parts = append(parts, string(t))
			}
		}
	}
	return parts
}

// Expansion only calls the configured OneBot API. URLs in cards/messages are
// display text and are never fetched. A shared semaphore bounds concurrent API
// work across live messages and history requests, without holding a slot while
// walking nested messages.
var messageExpansionSlots = make(chan struct{}, 4)

const maxMessageExpansionCalls = 16

type messageExpansion struct {
	bot     *OneBot
	ctx     context.Context
	groupID int64
	calls   int
	items   int
	cache   map[string]any
	active  map[string]bool
}

// ExpandGroupMessageContext renders same-group reply originals and forwarded
// records for the classification worker. Call outside the WS reader. On API
// failure, timeout, or a resource limit it retains the normal placeholders.
func (o *OneBot) ExpandGroupMessageContext(ctx context.Context, groupID int64, msg any) string {
	expanded := o.ExpandMessagesContext(ctx, groupID, []any{msg})
	text, _, _, _ := flattenUnpackedMessage(expanded[0], o.SelfID())
	return text
}

// ExpandMessagesContext returns copies for a history batch. The batch shares
// one timeout, request budget and cache, so repeated IDs are fetched only once.
// The returned segments can be passed to flattenMessage/buildSegments. In the
// latter, inlineReplyQuote(data) supplies already resolved reply previews.
func (o *OneBot) ExpandMessagesContext(ctx context.Context, groupID int64, messages []any) []any {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	e := &messageExpansion{bot: o, ctx: ctx, groupID: groupID, cache: map[string]any{}, active: map[string]bool{}}
	out := make([]any, len(messages))
	for i, msg := range messages {
		out[i] = e.expand(msg, 0)
	}
	return out
}

func copyUnpackMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (e *messageExpansion) expand(msg any, depth int) any {
	arr, ok := msg.([]any)
	if !ok || depth >= maxForwardDepth || e.ctx.Err() != nil {
		return msg
	}
	out := append([]any(nil), arr...)
	for i, seg := range arr {
		if e.items >= maxUnpackedItems || e.ctx.Err() != nil {
			break
		}
		e.items++
		sm, ok := seg.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := sm["type"].(string)
		if typ != "reply" && typ != "forward" {
			continue
		}
		data, _ := sm["data"].(map[string]any)
		data = copyUnpackMap(data)
		outSeg := copyUnpackMap(sm)
		outSeg["data"] = data
		out[i] = outSeg
		if typ == "reply" {
			delete(data, "_expanded_text")
			id := toInt64(data["id"])
			if id == 0 || e.groupID <= 0 {
				continue
			}
			key := fmt.Sprintf("reply:%d", id)
			if e.active[key] {
				continue
			}
			raw, ok := e.fetch(key, "get_msg", map[string]any{"message_id": id}).(map[string]any)
			// get_msg is global, so never expose a quoted DM/other group's text.
			if !ok || toInt64(raw["group_id"]) != e.groupID || raw["message_type"] == "private" {
				continue
			}
			e.active[key] = true
			inner := e.expand(raw["message"], depth+1)
			delete(e.active, key)
			text, _, _, _ := flattenUnpackedMessage(inner, e.bot.SelfID())
			if text == "" {
				continue
			}
			sender, _ := raw["sender"].(map[string]any)
			data["_expanded_text"] = unpackTextLimit(text, 4096)
			data["_expanded_nickname"] = unpackTextLimit(firstUnpackString(sender["card"], sender["nickname"]), 256)
			data["_expanded_user_id"] = toInt64(raw["user_id"])
			continue
		}
		nodes, _ := data["content"].([]any)
		key := ""
		if len(nodes) == 0 {
			id := firstUnpackString(data["id"], data["res_id"], data["resid"])
			if id == "" || len(id) > 256 {
				continue
			}
			key = "forward:" + id
			if e.active[key] {
				continue
			}
			value := e.fetch(key, "get_forward_msg", map[string]any{"message_id": id})
			if raw, ok := value.(map[string]any); ok {
				for _, field := range []string{"messages", "message", "content"} {
					if candidate, ok := raw[field].([]any); ok && len(candidate) > 0 {
						nodes = candidate
						break
					}
				}
			} else {
				nodes, _ = value.([]any)
			}
			if len(nodes) == 0 {
				continue
			}
			e.active[key] = true
		}
		expanded := make([]any, 0, min(len(nodes), maxUnpackedItems))
		for _, node := range nodes {
			if e.items >= maxUnpackedItems || e.ctx.Err() != nil {
				break
			}
			e.items++
			n, ok := node.(map[string]any)
			if !ok {
				continue
			}
			nick, inner := unpackForwardNode(n)
			expanded = append(expanded, map[string]any{"sender": map[string]any{"nickname": nick}, "message": e.expand(inner, depth+1)})
		}
		if key != "" {
			delete(e.active, key)
		}
		if len(expanded) > 0 {
			data["content"] = expanded
		}
	}
	return out
}

func (e *messageExpansion) fetch(key, action string, params map[string]any) any {
	if v, ok := e.cache[key]; ok {
		return v
	}
	if e.calls >= maxMessageExpansionCalls || e.ctx.Err() != nil {
		return nil
	}
	e.calls++
	e.cache[key] = nil
	select {
	case messageExpansionSlots <- struct{}{}:
	case <-e.ctx.Done():
		return nil
	}
	data, err := e.bot.callContext(e.ctx, action, params)
	<-messageExpansionSlots
	if err != nil || len(data) > 256<<10 {
		return nil
	}
	var v any
	if json.Unmarshal(data, &v) != nil {
		return nil
	}
	e.cache[key] = v
	return v
}
