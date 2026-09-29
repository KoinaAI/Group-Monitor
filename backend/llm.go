package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// systemPrompt is the fixed, system-managed instruction for the model. The
// output contract (the JSON verdict below) is preset here and never exposed to
// the user — every reminder is formatted the same way downstream.
const systemPrompt = `你是「群哨」的消息分拣助手。输入是某个 QQ 群一段时间内的群聊记录（含发送者身份、时间），你要判断这批消息里是否存在对「主人」（群里的一名学生）真正有价值的信息，并把它提炼成一条简明、可直接行动的提醒。宁可漏报琐碎信息，也不要把日常噪音当成通知打扰主人。

【判定「有用」】——面向全体同学、需要主人知晓或行动的正式信息，满足任一：
- 含明确时间 / 地点 / 截止点的安排：会议、大会、上课调整、集合、体检、考试与报名（如四六级）、答辩、活动、参观、纳新宣讲等。
- 需要提交或办理的事项：交材料 / 表格 / 费用、填写在线表格、办卡、申报奖学金、图像采集等，尤其带「X 点前 / 本周 X 前 / 今天内」等截止。
- 群主 / 管理 / 老师 / 辅导员 / 学办发布的正式通知、要求、催办。
- 点名到主人，或 @主人本人 / @全体成员 的通知。
- 重要安全提醒（如防诈骗、账号安全）。

【判定「无用」】——直接过滤，useful=false，这类占绝大多数：
- 回执与附和刷屏：「收到」「好的」「OK」「+1」「我也是」「谢谢」「棒」「🫡」以及各种表情 / 贴纸。哪怕几十条也一律视为噪音。
- 纯媒体且无文字说明：单独的 [图片] [表情] [卡片] [视频] [语音] [非文本]。
- 班长 / 学委 / 课代表 / 各类委员 / 负责人之间的事务性协调与闲聊（如「问学导」「交给谁」「这个先撤掉吧」「这是系统还有延迟嘛」）——除非其中确实夹带了面向全体同学的正式通知或安排。
- 广告、拉票、红包，以及纳新 / 社团 / 公众号的纯宣传口播和中二文案——除非明确给出时间、地点、报名方式。
- 与主人无关的私下对话、玩笑、日常闲聊。

【提炼要求】
- 透过「收到 / +1」这类回执噪音，抓住其背后真正的那条通知；把多条描述同一件事的消息合并成一条，不要逐条罗列。
- 输入末尾可能附有「历史正式通知参考」。它们只用于确认简称、时间线和上下文；只有当前群聊明确延续了历史事项时才引用，不要把历史内容误当成当前新通知。
- summary 用书面、凝练的语言复述关键信息，去掉口语和冗余；保留人名 / 部门 / 具体数字 / 文件名等硬信息。
- 若同一批里有多件有用的事，聚焦最重要的一件写进 title/time/place/event/deadline，其余要点并入 summary。

【分级 level】
- 1 一般：有一定价值但不紧迫的信息或通知。
- 2 重要：明确的安排 / 任务 / 需较快响应；群主 / 管理 / 老师的正式通知。
- 3 紧急：有明确且临近的截止（今天 / 数小时内 / 明日）、点名要求、或需立即处理的事项。

【示例】
- 「@全体成员 今日内到校医院一楼完成体检，班长收齐体检表交收费处」→ useful=true, level=2, event=体检并收表, place=校医院一楼。
- 连续多条「收到」「+1」「🫡」→ useful=false, reason=仅为回执附和。
- 「[引用] 这个先撤掉吧」「好的」→ useful=false, reason=班委之间的事务协调，无面向全体的通知。

只输出一个 JSON 对象，不要输出任何解释、前后缀或代码块标记，结构如下：
{
  "useful": true/false,
  "level": 1,
  "title": "一句话主题，不超过20字",
  "summary": "凝练正文，突出关键信息，去除口语与冗余，可含要点",
  "time": "涉及的时间，没有填空字符串",
  "place": "涉及的地点，没有填空字符串",
  "event": "核心事件或待办，没有填空字符串",
  "deadline": "明确的截止时间，如「今天18:00前」「本周五前」，没有填空字符串",
  "reason": "判定有用/无用的简短理由"
}`

// LLMResult is the structured verdict we ask the model to return for a batch.
type LLMResult struct {
	Useful   bool   `json:"useful"`
	Level    int    `json:"level"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Time     string `json:"time"`
	Place    string `json:"place"`
	Event    string `json:"event"`
	Deadline string `json:"deadline"`
	Reason   string `json:"reason"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

const (
	maxLLMResponseBytes = 1 << 20
	maxLLMRequestBytes  = 128 << 10
)

// streamChunk is one SSE delta frame. We only ever consume `content`; any
// `reasoning_content` (the model's chain-of-thought scratchpad) is read but
// discarded so it never leaks into the verdict.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callLLM sends the batch to the configured OpenAI-compatible endpoint using a
// preset system prompt and streaming, then parses the JSON verdict out of the
// reply. Only the assistant's main content is parsed; reasoning_content is
// dropped. Endpoints that ignore stream:true and return a normal JSON body are
// handled by a fallback.
func callLLM(cfg LLMConfig, userContent string) (LLMResult, string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return callLLMContext(ctx, cfg, userContent)
}

func callLLMContext(parent context.Context, cfg LLMConfig, userContent string) (LLMResult, string, error) {
	var res LLMResult
	if cfg.BaseURL == "" {
		return res, "", fmt.Errorf("no LLM base url")
	}
	if err := validateHTTPURL(cfg.BaseURL); err != nil {
		return res, "", err
	}
	to := cfg.Timeout
	if to <= 0 {
		to = 45
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(to)*time.Second)
	defer cancel()

	body := chatReq{
		Model:       cfg.Model,
		Temperature: cfg.Temp,
		MaxTokens:   cfg.MaxTok,
		Stream:      true,
		Messages: []chatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
	}
	b, _ := json.Marshal(body)
	if len(b) > maxLLMRequestBytes {
		return res, "", fmt.Errorf("LLM request too large")
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return res, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := noRedirectClient(http.DefaultClient).Do(req)
	if err != nil {
		return res, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var upstream chatResp
		if json.Unmarshal(raw, &upstream) == nil && upstream.Error != nil {
			return res, "", fmt.Errorf("LLM %d: %s", resp.StatusCode, upstream.Error.Message)
		}
		return res, "", fmt.Errorf("LLM %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var content string
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		content, err = readSSEContent(resp.Body)
		if err != nil {
			return res, content, err
		}
	} else {
		// Fallback: the endpoint returned a single JSON body despite stream:true.
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxLLMResponseBytes+1))
		if err != nil {
			return res, "", fmt.Errorf("read LLM response: %w", err)
		}
		if len(raw) > maxLLMResponseBytes {
			return res, "", fmt.Errorf("LLM response too large")
		}
		var cr chatResp
		if err := json.Unmarshal(raw, &cr); err != nil {
			return res, "", fmt.Errorf("bad LLM response: %s", truncate(string(raw), 300))
		}
		if cr.Error != nil {
			return res, "", fmt.Errorf("LLM error: %s", cr.Error.Message)
		}
		if len(cr.Choices) == 0 {
			return res, "", fmt.Errorf("LLM returned no choices: %s", truncate(string(raw), 300))
		}
		content = cr.Choices[0].Message.Content
	}

	if strings.TrimSpace(content) == "" {
		return res, "", fmt.Errorf("LLM returned empty content")
	}
	parsed, err := parseVerdict(content)
	if err != nil {
		return res, content, err
	}
	return parsed, content, nil
}

// readSSEContent consumes an OpenAI-style streaming response, concatenating the
// delta.content pieces. delta.reasoning_content is intentionally ignored.
func readSSEContent(r io.Reader) (string, error) {
	var sb strings.Builder
	sc := bufio.NewScanner(r)
	// Allow long data lines (a full chunk can exceed the default 64KB token).
	sc.Buffer(make([]byte, 0, 64*1024), maxLLMResponseBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // skip keep-alives / non-JSON frames
		}
		if chunk.Error != nil {
			return sb.String(), fmt.Errorf("LLM error: %s", chunk.Error.Message)
		}
		for _, c := range chunk.Choices {
			// Only the answer body; reasoning_content is deliberately dropped.
			if sb.Len()+len(c.Delta.Content) > maxLLMResponseBytes {
				return sb.String(), fmt.Errorf("LLM response too large")
			}
			sb.WriteString(c.Delta.Content)
		}
	}
	if err := sc.Err(); err != nil {
		return sb.String(), fmt.Errorf("stream read error: %v", err)
	}
	return sb.String(), nil
}

// parseVerdict extracts the JSON object from a model reply that may be wrapped
// in prose or ```json fences.
func parseVerdict(content string) (LLMResult, error) {
	var res LLMResult
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if candidate, ok := decodeVerdict(strings.TrimSpace(s)); ok {
		return candidate, nil
	}
	// Decode from each opening brace. Decoder stops after the first complete
	// object, so explanatory text or a second object cannot make the first
	// verdict invalid.
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		candidate, ok := decodeVerdictFromReader(strings.NewReader(content[i:]))
		if ok {
			return candidate, nil
		}
	}
	return res, fmt.Errorf("could not parse JSON verdict from: %s", truncate(content, 200))
}

func decodeVerdict(s string) (LLMResult, bool) {
	return decodeVerdictFromReader(strings.NewReader(s))
}

func decodeVerdictFromReader(r io.Reader) (LLMResult, bool) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(io.LimitReader(r, maxLLMResponseBytes))
	if err := dec.Decode(&raw); err != nil || raw == nil {
		return LLMResult{}, false
	}
	usefulRaw, ok := raw["useful"]
	if !ok {
		return LLMResult{}, false
	}
	var useful bool
	if err := json.Unmarshal(usefulRaw, &useful); err != nil {
		return LLMResult{}, false
	}
	levelRaw, ok := raw["level"]
	var level int
	if ok {
		if err := json.Unmarshal(levelRaw, &level); err != nil || level < 0 || level > 3 {
			return LLMResult{}, false
		}
	}
	var result LLMResult
	if err := json.Unmarshal(mustMarshal(raw), &result); err != nil {
		return LLMResult{}, false
	}
	result.Useful = useful
	result.Level = level
	return result, true
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
