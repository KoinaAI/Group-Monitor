package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type API struct {
	store *Store
	ob    *OneBot
	hub   *Hub
	pipe  *Pipeline
	otp   otpState
}

func NewAPI(store *Store, ob *OneBot, hub *Hub, pipe *Pipeline) *API {
	return &API{store: store, ob: ob, hub: hub, pipe: pipe}
}

func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/groups", a.handleGroups)               // live group list from NapCat
	mux.HandleFunc("/api/groups/watch", a.handleWatch)          // toggle/save watched groups
	mux.HandleFunc("/api/groups/history", a.handleGroupHistory) // manual chat-log viewer
	mux.HandleFunc("/api/masters", a.handleMasters)             // GET/POST masters
	mux.HandleFunc("/api/rules", a.handleRules)                 // POST rules
	mux.HandleFunc("/api/llm", a.handleLLM)                     // POST llm config
	mux.HandleFunc("/api/llm/test", a.handleLLMTest)            // POST test a sample batch
	mux.HandleFunc("/api/jev", a.handleJev)                     // POST jev intent-gate config
	mux.HandleFunc("/api/jev/test", a.handleJevTest)            // POST test the gate on samples
	mux.HandleFunc("/api/onebot", a.handleOneBotConfig)         // POST onebot connection
	mux.HandleFunc("/api/enabled", a.handleEnabled)             // POST global toggle
	mux.HandleFunc("/api/test-notify", a.handleTestNotify)      // POST send a test DM
	mux.HandleFunc("/api/lookup", a.handleLookup)               // GET nickname for a QQ
	mux.HandleFunc("/api/logs", a.handleLogs)                   // GET recent logs
	mux.HandleFunc("/api/escalations", a.handleEscalations)     // GET recent escalations
	mux.HandleFunc("/api/auth/otp/request", a.handleOtpRequest) // POST: DM a login OTP to masters
	mux.HandleFunc("/api/auth/otp/verify", a.handleOtpVerify)   // POST: verify a login OTP
	mux.HandleFunc("/api/events", a.handleSSE)                  // SSE stream
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
	// Full config (used by the UI to populate all forms). API keys are returned
	// so the settings form round-trips; this API is intended to bind locally.
	writeJSON(w, 200, a.store.Get())
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Groups = body.Groups })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.broadcastStatus()
	writeJSON(w, 200, cfg.Groups)
}

func (a *API) handleGroupHistory(w http.ResponseWriter, r *http.Request) {
	var gid int64
	fmt.Sscan(r.URL.Query().Get("groupId"), &gid)
	if gid == 0 {
		writeErr(w, 400, "缺少 groupId")
		return
	}
	count := 30
	if c := r.URL.Query().Get("count"); c != "" {
		fmt.Sscan(c, &count)
	}
	msgs, err := a.ob.GetGroupMsgHistory(gid, count)
	if err != nil {
		writeErr(w, 502, "获取聊天记录失败："+err.Error())
		return
	}
	writeJSON(w, 200, msgs)
}

func (a *API) handleMasters(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, a.store.Get().Masters)
		return
	}
	var body struct {
		Masters []Master `json:"masters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&llm); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.LLM = llm })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.broadcastStatus()
	writeJSON(w, 200, cfg.LLM)
}

func (a *API) handleLLMTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var llm LLMConfig
	if err := json.NewDecoder(r.Body).Decode(&llm); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&jev); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Jev = jev })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.broadcastStatus()
	writeJSON(w, 200, cfg.Jev)
}

func (a *API) handleJevTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var jev JevConfig
	if err := json.NewDecoder(r.Body).Decode(&jev); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&obc); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.OneBot = obc })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.ob.Reconfigure(cfg.OneBot)
	writeJSON(w, 200, cfg.OneBot)
}

func (a *API) handleEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method not allowed")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	cfg, err := a.store.Update(func(c *Config) { c.Enabled = body.Enabled })
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
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
	text := "✅ 测试提醒\n──────────\n这是一条来自 QQ 群哨的测试消息，说明推送通道已打通。"
	sent, fails := 0, []string{}
	for _, m := range cfg.Masters {
		if err := a.ob.SendPrivateMsg(m.UserID, text); err != nil {
			fails = append(fails, fmt.Sprintf("%d: %v", m.UserID, err))
			continue
		}
		sent++
	}
	writeJSON(w, 200, map[string]any{"sent": sent, "failed": fails})
}

func (a *API) handleLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	var id int64
	fmt.Sscan(q, &id)
	if id == 0 {
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

func (a *API) handleSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := a.hub.Subscribe()
	defer a.hub.Unsubscribe(ch)

	// Prime with a hello + current status so the page renders immediately.
	fmt.Fprintf(w, "data: %s\n\n", Event{Type: "hello", Data: a.statusPayload(), TS: time.Now().UnixMilli()}.Encode())
	fl.Flush()

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
			fmt.Fprintf(w, "data: %s\n\n", ev.Encode())
			fl.Flush()
		case <-ping.C:
			fmt.Fprintf(w, ": ping\n\n")
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
