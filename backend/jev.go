package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// Jev (TypeSafe System One) is used as a per-message intent gate: given a
// message and its recent group context, it returns the probability (noul, 0..1)
// that the message carries information worth notifying about. We keep the
// question/criteria system-managed so every message is judged the same way.
const (
	jevQuestion         = "结合 `recent` 提供的上下文，判断 `message` 这条群消息本身是否包含对群里学生本人有用、值得单独提醒的信息（如通知、安排、时间/地点/截止、点名、需提交或办理的事项）。若仅为回执附和（收到/好的/+1）、表情、纯图片、班委/学委之间的琐碎协调、广告或日常闲聊，则不重要。"
	jevTrueDesc         = "包含面向学生的有用通知，或需要主人知晓/行动的信息"
	jevFalseDesc        = "回执/附和/表情/纯媒体/班委琐碎协调/广告/闲聊等噪音"
	maxJevRequestBytes  = 64 << 10
	maxJevResponseBytes = 1 << 20
)

type jevQuestionBody struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type jevReq struct {
	State     any                        `json:"state"`
	Model     string                     `json:"model"`
	Questions map[string]jevQuestionBody `json:"questions"`
}

type jevResp struct {
	Model   string `json:"model"`
	Answers map[string]struct {
		Type string  `json:"type"`
		Noul float64 `json:"noul"`
	} `json:"answers"`
}

// jevImportance asks Jev for the importance probability of a single message.
// state should carry the message plus its recent context.
func jevImportance(cfg JevConfig, state any) (float64, error) {
	to := cfg.Timeout
	if to <= 0 {
		to = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(to)*time.Second)
	defer cancel()
	return jevImportanceContext(ctx, cfg, state)
}

func jevImportanceContext(ctx context.Context, cfg JevConfig, state any) (float64, error) {
	return jevScoreContext(ctx, cfg, state, jevQuestion)
}

// Archiving has stricter semantics than urgent delivery: keywords and outage
// fallbacks must never turn unclassified chatter into permanent history.
func jevNoticeContext(ctx context.Context, cfg JevConfig, state any) (float64, error) {
	return jevScoreContext(ctx, cfg, state, "判断当前消息块是否包含正式通知或对学生有用的具体安排、待办、截止、变更。忽略附和、吹水、广告、仅因紧急词或@触发的闲聊。只评判当前原始消息及文件内容，不把历史参考或自动生成的摘要当作新通知。")
}

func jevScoreContext(parent context.Context, cfg JevConfig, state any, question string) (float64, error) {
	to := cfg.Timeout
	if to <= 0 {
		to = 10
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(to)*time.Second)
	defer cancel()
	if cfg.BaseURL == "" {
		return 0, fmt.Errorf("no jev endpoint configured")
	}
	if err := validateHTTPURL(cfg.BaseURL); err != nil {
		return 0, err
	}

	model := cfg.Model
	if model == "" {
		model = "jev-latest"
	}
	body := jevReq{
		State: state,
		Model: model,
		Questions: map[string]jevQuestionBody{
			"important": {
				Type:         "noul",
				Instructions: map[string]any{"question": question},
				Criteria:     map[string]any{"true": jevTrueDesc, "false": jevFalseDesc},
			},
		},
	}
	b, _ := json.Marshal(body)
	if len(b) > maxJevRequestBytes {
		return 0, fmt.Errorf("jev request too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := noRedirectClient(http.DefaultClient).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJevResponseBytes+1))
	if err != nil {
		return 0, fmt.Errorf("read jev response: %w", err)
	}
	if len(raw) > maxJevResponseBytes {
		return 0, fmt.Errorf("jev response too large")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("jev %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var jr jevResp
	if err := json.Unmarshal(raw, &jr); err != nil {
		return 0, fmt.Errorf("bad jev response: %s", truncate(string(raw), 200))
	}
	a, ok := jr.Answers["important"]
	if !ok {
		return 0, fmt.Errorf("jev returned no 'important' answer: %s", truncate(string(raw), 200))
	}
	if math.IsNaN(a.Noul) || a.Noul < 0 || a.Noul > 1 {
		return 0, fmt.Errorf("jev noul out of range: %v", a.Noul)
	}
	return a.Noul, nil
}

// jevState packs a message and its recent context into the shape Jev sees.
func jevState(gm GroupMessage, ctxMsgs []GroupMessage) map[string]any {
	recent := make([]string, 0, len(ctxMsgs))
	for _, m := range ctxMsgs {
		recent = append(recent, jevLine(m))
	}
	return map[string]any{
		"group":   gm.GroupName,
		"recent":  recent,
		"message": jevLine(gm),
	}
}

func jevLine(m GroupMessage) string {
	txt := limitText(groupMessageContent(m), 12000)
	if txt == "" {
		if m.HasImage {
			txt = "[图片]"
		} else {
			txt = "[非文本]"
		}
	}
	return fmt.Sprintf("%s(%s): %s", m.Nickname, roleLabel(m.Role), txt)
}

func roleLabel(role string) string {
	switch role {
	case "owner":
		return "群主"
	case "admin":
		return "管理员"
	}
	return "成员"
}
