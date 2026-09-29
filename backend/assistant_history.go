package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Lite history is an ephemeral text view for the assistant. It does not fetch
// member lists, media, files or reply chains, and never persists raw messages.
func (o *OneBot) GetGroupMsgHistoryLiteContext(ctx context.Context, groupID int64, count int, anchorSeq int64, reverse bool) ([]HistoryMsg, error) {
	if groupID <= 0 || anchorSeq < 0 {
		return nil, fmt.Errorf("invalid history scope")
	}
	if count <= 0 || count > 60 {
		count = 60
	}
	data, err := o.callContext(ctx, "get_group_msg_history", map[string]any{
		"group_id": groupID, "count": count, "message_seq": anchorSeq, "reverseOrder": reverse,
	})
	if err != nil {
		return nil, err
	}
	var env struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		if err := json.Unmarshal(data, &env.Messages); err != nil {
			return nil, err
		}
	}
	if len(env.Messages) > count {
		env.Messages = env.Messages[:count]
	}
	out := make([]HistoryMsg, 0, len(env.Messages))
	for _, raw := range env.Messages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Group history responses may omit group_id; an explicit mismatch is
		// never safe to expose, even if the request selected an allowed group.
		if id := toInt64(raw["group_id"]); id != 0 && id != groupID {
			continue
		}
		if kind := toStr(raw["message_type"]); kind != "" && kind != "group" {
			continue
		}
		out = append(out, parseAssistantHistory(raw, o.SelfID()))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Time == out[j].Time {
			return out[i].MessageSeq < out[j].MessageSeq
		}
		return out[i].Time < out[j].Time
	})
	return out, nil
}

// Source lookup validates the actual response group before exposing any text.
func (o *OneBot) GetMsgInGroupContext(ctx context.Context, groupID, messageID int64) (HistoryMsg, error) {
	data, err := o.callContext(ctx, "get_msg", map[string]any{"message_id": messageID})
	if err != nil {
		return HistoryMsg{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return HistoryMsg{}, err
	}
	if toInt64(raw["group_id"]) != groupID || toInt64(raw["message_id"]) != messageID {
		return HistoryMsg{}, fmt.Errorf("source does not belong to the requested group")
	}
	if kind := toStr(raw["message_type"]); kind != "" && kind != "group" {
		return HistoryMsg{}, fmt.Errorf("source is not a group message")
	}
	if needsMessageExpansion(raw["message"]) {
		raw["message"] = o.ExpandMessagesContext(ctx, groupID, []any{raw["message"]})[0]
	}
	return parseAssistantHistory(raw, o.SelfID()), ctx.Err()
}

func parseAssistantHistory(raw map[string]any, self int64) HistoryMsg {
	m := HistoryMsg{MessageID: toInt64(raw["message_id"]), MessageSeq: toInt64(raw["message_seq"]), UserID: toInt64(raw["user_id"]), Time: toInt64(raw["time"])}
	if m.MessageSeq == 0 {
		m.MessageSeq = toInt64(raw["real_seq"])
	}
	if m.MessageSeq == 0 {
		m.MessageSeq = toInt64(raw["real_id"])
	}
	if sender, ok := raw["sender"].(map[string]any); ok {
		m.Nickname = strings.TrimSpace(toStr(sender["card"]))
		if m.Nickname == "" {
			m.Nickname = toStr(sender["nickname"])
		}
		if m.UserID == 0 {
			m.UserID = toInt64(sender["user_id"])
		}
	}
	m.Text, _, _, m.HasImage = flattenMessage(raw["message"], self)
	m.Text = limitText(m.Text, 1600)
	m.Nickname = limitText(m.Nickname, 80)
	return m
}

// These are opaque source references, not authorization tokens. Every lookup
// independently validates current privileges, watched scope and response IDs.
func encodeMsgUUID(groupID, messageID int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%d", groupID, messageID)))
}

func decodeMsgUUID(ref string) (int64, int64, error) {
	if len(ref) > 80 {
		return 0, 0, fmt.Errorf("invalid source reference")
	}
	b, err := base64.RawURLEncoding.DecodeString(ref)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid source reference")
	}
	parts := strings.Split(string(b), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid source reference")
	}
	groupID, e1 := strconv.ParseInt(parts[0], 10, 64)
	messageID, e2 := strconv.ParseInt(parts[1], 10, 64)
	if e1 != nil || e2 != nil || groupID <= 0 || messageID == 0 {
		return 0, 0, fmt.Errorf("invalid source reference")
	}
	return groupID, messageID, nil
}
