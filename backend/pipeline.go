package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxJevConcurrent     = 8
	maxJevQueuedPerGroup = 256
	maxBatchMessages     = 256
	maxTranscriptBytes   = 64 << 10
	maxDigestBytes       = 16 << 10
)

// Sender levels (higher = more important).
const (
	LvlMuted  = -1
	LvlNormal = 0
	LvlAdmin  = 1
	LvlOwner  = 2
	LvlVIP    = 3
)

// scored is a message annotated with the classification results.
type scored struct {
	msg         GroupMessage
	senderLevel int
	senderLabel string
	urgent      bool
	urgentWhy   string
}

// groupBuffer accumulates non-urgent messages for one group until the quiet
// window elapses (or the max-hold cap fires).
type groupBuffer struct {
	groupID   int64
	groupName string
	msgs      []scored
	timer     *time.Timer
	timerGen  int64         // generation of the armed timer (guards stale fires)
	window    time.Duration // the delay of the current arm (for the UI ring)
	firstAt   time.Time     // when the batch started (for max-hold)
	flushAt   time.Time     // scheduled flush time (for the UI countdown)
}

type jevWork struct {
	cfg        Config
	sc         scored
	context    []GroupMessage
	generation uint64
}

type jevQueue struct {
	pending []jevWork
	current *jevWork
	cancel  context.CancelFunc
	running bool
}

type Pipeline struct {
	store *Store
	ob    *OneBot
	hub   *Hub

	mu         sync.Mutex
	buffers    map[int64]*groupBuffer
	recent     map[int64][]GroupMessage // rolling per-group context for the Jev gate
	jevQueues  map[int64]*jevQueue
	jevSlots   chan struct{}
	generation uint64
	timerSeq   int64 // monotonic source for timer generations
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stopped    bool
}

func NewPipeline(store *Store, ob *OneBot, hub *Hub) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pipeline{
		store:     store,
		ob:        ob,
		hub:       hub,
		buffers:   make(map[int64]*groupBuffer),
		recent:    make(map[int64][]GroupMessage),
		jevQueues: make(map[int64]*jevQueue),
		jevSlots:  make(chan struct{}, maxJevConcurrent),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (p *Pipeline) Shutdown() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.generation++
	if p.cancel != nil {
		p.cancel()
	}
	for _, b := range p.buffers {
		if b.timer != nil {
			b.timer.Stop()
		}
	}
	for _, q := range p.jevQueues {
		q.pending = nil
		if q.cancel != nil {
			q.cancel()
		}
	}
	p.buffers = make(map[int64]*groupBuffer)
	p.recent = make(map[int64][]GroupMessage)
	p.mu.Unlock()
	p.wg.Wait()
	p.broadcastBuffers()
}

// Ingest is the OneBot event hook. It always feeds the live feed, but only
// buffers/escalates messages from watched groups while enabled.
func (p *Pipeline) Ingest(gm GroupMessage) {
	cfg := p.store.Get()

	// Never react to our own messages (reportSelfMessage is on).
	if gm.UserID == p.ob.SelfID() && p.ob.SelfID() != 0 {
		return
	}

	gw, watched := cfg.IsWatched(gm.GroupID)
	if watched {
		gm.GroupName = gw.GroupName
	}

	// Push every message from watched groups to the live feed so the operator
	// can see raw traffic. Unwatched groups produce no reaction at all.
	if !watched {
		return
	}
	p.hub.Broadcast("message", gm)

	if !cfg.Enabled {
		return
	}

	sc := classify(&cfg, gm)
	if sc.senderLevel == LvlMuted {
		// Muted sender: acknowledged but never escalated.
		return
	}

	// Record recent context for the Jev gate before branching, so context stays
	// complete even for messages Jev later drops (snapshot excludes the current).
	ctxMsgs := p.pushRecent(gm, jevContextN(cfg))

	if sc.urgent {
		p.handleUrgent(cfg, gm.GroupID, gm.GroupName, sc)
		return
	}

	// Per-message intent gate: only messages Jev deems important enter the
	// packing queue. Runs in a goroutine so a slow API can't stall the WS read
	// loop. Fails open (buffers anyway) so a Jev outage never drops real notices.
	if cfg.Jev.Enabled && cfg.Jev.APIKey != "" {
		p.queueJev(cfg, sc, ctxMsgs)
		return
	}
	p.enqueue(cfg, gm.GroupID, gm.GroupName, sc)
}

// Reconcile is called after watch/enable changes. Old API work may finish, but
// its generation can no longer publish a reminder.
func (p *Pipeline) Reconcile(cfg Config) {
	p.mu.Lock()
	p.generation++
	for id, b := range p.buffers {
		if !cfg.Enabled || !isWatched(cfg, id) {
			b.timer.Stop()
			delete(p.buffers, id)
		}
	}
	for id, q := range p.jevQueues {
		if !cfg.Enabled || !isWatched(cfg, id) {
			q.pending = nil
			if q.cancel != nil {
				q.cancel()
			}
		}
	}
	if !cfg.Enabled {
		clear(p.recent)
	} else {
		for id := range p.recent {
			if !isWatched(cfg, id) {
				delete(p.recent, id)
			}
		}
	}
	p.mu.Unlock()
	p.broadcastBuffers()
}

func isWatched(cfg Config, groupID int64) bool {
	_, ok := cfg.IsWatched(groupID)
	return ok
}

// jevContextN returns the configured context window size (defaulting sanely).
func jevContextN(cfg Config) int {
	n := cfg.Jev.ContextN
	if n < 0 {
		n = 0
	}
	if n > 20 {
		n = 20
	}
	return n
}

// pushRecent appends gm to the group's rolling context and returns a snapshot
// of the prior messages (excluding gm), capped at n.
func (p *Pipeline) pushRecent(gm GroupMessage, n int) []GroupMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := append(p.recent[gm.GroupID], gm)
	if len(r) > n+1 {
		r = r[len(r)-(n+1):]
	}
	p.recent[gm.GroupID] = r
	ctx := make([]GroupMessage, len(r)-1)
	copy(ctx, r[:len(r)-1])
	return ctx
}

// jevGateAndEnqueue scores one message with Jev and enqueues it only if it
// clears the importance threshold. On any Jev error it fails open.
func (p *Pipeline) queueJev(cfg Config, sc scored, ctxMsgs []GroupMessage) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	q := p.jevQueues[sc.msg.GroupID]
	if q == nil {
		q = &jevQueue{}
		p.jevQueues[sc.msg.GroupID] = q
	}
	if len(q.pending) >= maxJevQueuedPerGroup {
		p.mu.Unlock()
		p.hub.Log("error", sc.msg.GroupID, sc.msg.GroupName, "Jev 队列已满，暂按重要处理")
		if p.active(p.currentGeneration(), sc.msg.GroupID) {
			p.enqueue(cfg, sc.msg.GroupID, sc.msg.GroupName, sc)
		}
		return
	}
	q.pending = append(q.pending, jevWork{cfg: cfg, sc: sc, context: ctxMsgs, generation: p.generation})
	if !q.running {
		q.running = true
		p.wg.Add(1)
		go p.runJevQueue(sc.msg.GroupID, q)
	}
	p.mu.Unlock()
}

func (p *Pipeline) runJevQueue(groupID int64, q *jevQueue) {
	defer p.wg.Done()
	for {
		p.mu.Lock()
		if len(q.pending) == 0 {
			q.running = false
			delete(p.jevQueues, groupID)
			p.mu.Unlock()
			return
		}
		work := q.pending[0]
		q.pending[0] = jevWork{}
		q.pending = q.pending[1:]
		q.current = &work
		ctx, cancel := context.WithCancel(p.ctx)
		q.cancel = cancel
		p.mu.Unlock()

		acquired := false
		select {
		case p.jevSlots <- struct{}{}:
			acquired = true
		case <-ctx.Done():
		}
		p.mu.Lock()
		valid := acquired && p.generation == work.generation && q.current == &work
		p.mu.Unlock()
		if valid {
			p.jevGateAndEnqueue(ctx, work)
		}
		if acquired {
			<-p.jevSlots
		}
		cancel()
		p.mu.Lock()
		q.current = nil
		q.cancel = nil
		p.mu.Unlock()
	}
}

func (p *Pipeline) jevGateAndEnqueue(ctx context.Context, work jevWork) {
	cfg, sc := work.cfg, work.sc
	gm := sc.msg
	current := p.store.Get()
	if !current.Enabled || !isWatched(current, gm.GroupID) {
		return
	}
	if !current.Jev.Enabled || current.Jev.APIKey == "" {
		p.enqueue(current, gm.GroupID, gm.GroupName, sc)
		return
	}
	cfg.Jev = current.Jev
	noul, err := jevImportanceContext(ctx, cfg.Jev, jevState(gm, work.context))
	if !p.active(work.generation, gm.GroupID) {
		return
	}
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		p.hub.Log("error", gm.GroupID, gm.GroupName, "Jev 预筛失败，暂按重要处理："+err.Error())
		p.enqueue(cfg, gm.GroupID, gm.GroupName, sc)
		return
	}
	thr := cfg.Jev.Threshold
	if thr <= 0 {
		thr = 0.6
	}
	if noul < thr {
		p.hub.Log("suppress", gm.GroupID, gm.GroupName,
			fmt.Sprintf("Jev 判定不重要（%.2f<%.2f）已跳过：%s：%s", noul, thr, gm.Nickname, truncate(gm.Text, 24)))
		return
	}
	p.enqueue(cfg, gm.GroupID, gm.GroupName, sc)
}

func (p *Pipeline) active(generation uint64, groupID int64) bool {
	p.mu.Lock()
	valid := p.generation == generation
	p.mu.Unlock()
	if !valid {
		return false
	}
	cfg := p.store.Get()
	return cfg.Enabled && isWatched(cfg, groupID)
}

// classify determines the sender level (role + overrides) and message urgency.
func classify(cfg *Config, gm GroupMessage) scored {
	sc := scored{msg: gm}

	// Sender level: override wins over role.
	if o, ok := cfg.overrideFor(gm.UserID); ok {
		switch o.Level {
		case "muted":
			sc.senderLevel = LvlMuted
			sc.senderLabel = "已屏蔽"
			return sc
		case "vip":
			sc.senderLevel = LvlVIP
			sc.senderLabel = "重点人物"
		default:
			sc.senderLevel = LvlNormal
			sc.senderLabel = "普通成员"
		}
	} else {
		switch gm.Role {
		case "owner":
			sc.senderLevel = LvlOwner
			sc.senderLabel = "群主"
		case "admin":
			sc.senderLevel = LvlAdmin
			sc.senderLabel = "管理员"
		default:
			sc.senderLevel = LvlNormal
			sc.senderLabel = "普通成员"
		}
	}

	// Urgency: keywords, @all, @self.
	text := gm.Text
	for _, kw := range cfg.Rules.UrgentKeywords {
		if kw == "" {
			continue
		}
		if strings.Contains(text, kw) {
			sc.urgent = true
			sc.urgentWhy = "关键词「" + kw + "」"
			break
		}
	}
	if !sc.urgent && cfg.Rules.AtAllUrgent && gm.AtAll {
		sc.urgent = true
		sc.urgentWhy = "@全体成员"
	}
	if !sc.urgent && gm.AtSelf {
		sc.urgent = true
		sc.urgentWhy = "@主人本人"
	}
	return sc
}

// handleUrgent flushes any pending batch for the group immediately, folding the
// urgent message in, and marks the batch urgent so it bypasses the LLM's
// noise filter threshold.
func (p *Pipeline) handleUrgent(cfg Config, groupID int64, groupName string, sc scored) {
	p.mu.Lock()
	generation := p.generation
	buf := p.buffers[groupID]
	if buf != nil && buf.timer != nil {
		buf.timer.Stop()
	}
	var pending []scored
	if buf != nil {
		pending = buf.msgs
	}
	if q := p.jevQueues[groupID]; q != nil {
		if q.current != nil {
			pending = append(pending, q.current.sc)
		}
		for _, work := range q.pending {
			pending = append(pending, work.sc)
		}
		q.pending = nil
		q.current = nil
		if q.cancel != nil {
			q.cancel()
		}
	}
	delete(p.buffers, groupID)
	p.mu.Unlock()

	batch := append(pending, sc)
	p.hub.Log("urgent", groupID, groupName, fmt.Sprintf("紧急消息（%s）来自 %s，立即处理 %d 条", sc.urgentWhy, sc.msg.Nickname, len(batch)))
	p.broadcastBuffers()
	p.startProcess(cfg, groupID, groupName, batch, true, generation)
}

// enqueue adds a normal message to the group buffer and (re)arms the quiet
// window timer. A busy group is force-flushed once it hits MaxHoldSec.
func (p *Pipeline) enqueue(cfg Config, groupID int64, groupName string, sc scored) {
	window := time.Duration(cfg.Rules.QuietWindowSec) * time.Second
	if window <= 0 {
		window = 120 * time.Second
	}
	maxHold := time.Duration(cfg.Rules.MaxHoldSec) * time.Second

	p.mu.Lock()
	buf := p.buffers[groupID]
	now := time.Now()
	if buf == nil {
		buf = &groupBuffer{groupID: groupID, groupName: groupName, firstAt: now}
		p.buffers[groupID] = buf
	}
	buf.groupName = groupName
	buf.msgs = append(buf.msgs, sc)
	if len(buf.msgs) >= maxBatchMessages {
		p.timerSeq++
		gen := p.timerSeq
		buf.timerGen = gen
		p.mu.Unlock()
		p.flush(groupID, gen)
		return
	}

	// Compute the delay: normally the full quiet window, but clamped so we
	// never exceed MaxHoldSec from the first message.
	delay := window
	if maxHold > 0 {
		remaining := maxHold - now.Sub(buf.firstAt)
		if remaining < delay {
			if remaining < 0 {
				remaining = 0
			}
			delay = remaining
		}
	}
	buf.flushAt = now.Add(delay)
	buf.window = delay
	if buf.timer != nil {
		buf.timer.Stop()
	}
	// Stopping an AfterFunc that has already fired does not un-fire it, so tag
	// each arm with a monotonic generation; a stale fire whose generation no
	// longer matches is ignored instead of flushing the batch early.
	p.timerSeq++
	gen := p.timerSeq
	buf.timerGen = gen
	buf.timer = time.AfterFunc(delay, func() { p.flush(groupID, gen) })
	count := len(buf.msgs)
	p.mu.Unlock()

	p.hub.Log("info", groupID, groupName, fmt.Sprintf("缓冲消息：%s（%s）· 当前 %d 条 · %.0fs 后打包", sc.msg.Nickname, sc.senderLabel, count, delay.Seconds()))
	p.broadcastBuffers()
}

func (p *Pipeline) flush(groupID int64, gen int64) {
	p.mu.Lock()
	buf := p.buffers[groupID]
	if buf == nil || buf.timerGen != gen {
		// A newer enqueue re-armed the timer (or an urgent flush already drained
		// and removed the buffer); this fire is stale — leave any live buffer be.
		p.mu.Unlock()
		return
	}
	if len(buf.msgs) == 0 {
		delete(p.buffers, groupID)
		p.mu.Unlock()
		return
	}
	msgs := buf.msgs
	name := buf.groupName
	delete(p.buffers, groupID)
	p.mu.Unlock()

	cfg := p.store.Get()
	p.broadcastBuffers()
	p.startProcess(cfg, groupID, name, msgs, false, p.currentGeneration())
}

func (p *Pipeline) startProcess(cfg Config, groupID int64, groupName string, batch []scored, urgent bool, generation uint64) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.wg.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.wg.Done()
		p.processGeneration(cfg, groupID, groupName, batch, urgent, generation)
	}()
}

func (p *Pipeline) currentGeneration() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation
}

// process runs the batch through the LLM (if enabled) and escalates when the
// verdict says the content is useful.
func (p *Pipeline) process(cfg Config, groupID int64, groupName string, batch []scored, urgent bool) {
	p.processGeneration(cfg, groupID, groupName, batch, urgent, p.currentGeneration())
}

func (p *Pipeline) processGeneration(cfg Config, groupID int64, groupName string, batch []scored, urgent bool, generation uint64) {
	if len(batch) == 0 {
		return
	}
	if !p.active(generation, groupID) {
		return
	}
	// Highest sender level in the batch drives elevation.
	topLevel := LvlNormal
	for _, s := range batch {
		if s.senderLevel > topLevel {
			topLevel = s.senderLevel
		}
	}
	transcript := buildTranscript(groupName, batch)

	if !cfg.LLM.Enabled {
		// Without an LLM we only forward urgent batches, as a raw digest, to
		// avoid spamming masters with unfiltered chatter.
		if urgent {
			res := LLMResult{
				Useful:  true,
				Level:   3,
				Title:   "紧急消息（未经 LLM 处理）",
				Summary: rawDigest(batch),
			}
			p.escalateGeneration(groupID, groupName, res, urgent, generation)
		} else {
			p.hub.Log("suppress", groupID, groupName, fmt.Sprintf("已缓冲 %d 条，但 LLM 未启用，普通消息不推送", len(batch)))
		}
		return
	}

	res, _, err := callLLMContext(p.ctx, cfg.LLM, transcript)
	if err != nil {
		p.hub.Log("error", groupID, groupName, "LLM 处理失败："+err.Error())
		if urgent {
			res = LLMResult{Useful: true, Level: 3, Title: "紧急消息（LLM 处理失败）", Summary: rawDigest(batch)}
			p.escalateGeneration(groupID, groupName, res, urgent, generation)
		}
		return
	}

	// Elevate level if an owner/admin/VIP is involved.
	if cfg.Rules.ElevateOwnerAdmin && topLevel >= LvlAdmin && res.Level < 2 {
		res.Level = 2
	}
	if topLevel == LvlVIP && res.Level < 3 {
		res.Level = 3
	}
	if urgent {
		res.Level = 3
		res.Useful = true
		if strings.TrimSpace(res.Summary) == "" {
			res.Summary = rawDigest(batch)
		}
	}

	if !res.Useful {
		p.hub.Log("suppress", groupID, groupName, fmt.Sprintf("LLM 判定 %d 条为噪音已过滤：%s", len(batch), res.Reason))
		return
	}
	p.escalateGeneration(groupID, groupName, res, urgent, generation)
}

// escalate formats the reminder and DMs eligible masters.
func (p *Pipeline) escalate(cfg Config, groupID int64, groupName string, res LLMResult, urgent bool) {
	p.escalateGeneration(groupID, groupName, res, urgent, p.currentGeneration())
}

func (p *Pipeline) escalateGeneration(groupID int64, groupName string, res LLMResult, urgent bool, generation uint64) {
	if !p.active(generation, groupID) {
		return
	}
	cfg := p.store.Get()
	if res.Level <= 0 {
		res.Level = 1
	}
	text := formatReminder(groupName, res, urgent)

	targets := make([]Master, 0, len(cfg.Masters))
	for _, m := range cfg.Masters {
		if res.Level >= m.MinLevel {
			targets = append(targets, m)
		}
	}
	sent, failed := sendPrivateConcurrentContext(p.ctx, p.ob, targets, text)
	if !p.active(generation, groupID) {
		return
	}
	for _, uid := range failed {
		p.hub.Log("error", groupID, groupName, fmt.Sprintf("推送给主人 %d 失败", uid))
	}
	p.hub.Log("escalate", groupID, groupName, fmt.Sprintf("[%s] %s → 已通知 %d 位主人，失败 %d 位", levelLabel(res.Level), res.Title, sent, len(failed)))
	p.hub.Escalation(map[string]any{
		"groupId":  groupID,
		"group":    groupName,
		"level":    res.Level,
		"title":    res.Title,
		"summary":  res.Summary,
		"time":     res.Time,
		"place":    res.Place,
		"event":    res.Event,
		"deadline": res.Deadline,
		"notified": sent,
		"failed":   failed,
		"urgent":   urgent,
		"ts":       time.Now().UnixMilli(),
	})
}

func buildTranscript(groupName string, batch []scored) string {
	var b strings.Builder
	fmt.Fprintf(&b, "群聊：%s\n消息条数：%d\n\n", limitText(groupName, 256), len(batch))
	for _, s := range batch {
		if b.Len() >= maxTranscriptBytes-5000 {
			break
		}
		t := time.Unix(s.msg.Time, 0).Format("15:04:05")
		tag := ""
		if s.senderLevel >= LvlAdmin {
			tag = "★"
		}
		if s.urgent {
			tag += "‼"
		}
		fmt.Fprintf(&b, "[%s] %s%s(%s,QQ%d): %s\n", t, tag, limitText(s.msg.Nickname, 128), s.senderLabel, s.msg.UserID, limitText(s.msg.Text, 4096))
	}
	return b.String()
}

func rawDigest(batch []scored) string {
	var b strings.Builder
	for _, s := range batch {
		if b.Len() >= maxDigestBytes-5000 {
			break
		}
		fmt.Fprintf(&b, "· %s：%s\n", limitText(s.msg.Nickname, 128), limitText(s.msg.Text, 4096))
	}
	return strings.TrimSpace(b.String())
}

// formatReminder builds the private message sent to masters.
func formatReminder(groupName string, res LLMResult, urgent bool) string {
	groupName = limitText(groupName, 256)
	res.Title = limitText(res.Title, 256)
	res.Summary = limitText(res.Summary, 8192)
	res.Time = limitText(res.Time, 256)
	res.Place = limitText(res.Place, 256)
	res.Event = limitText(res.Event, 512)
	res.Deadline = limitText(res.Deadline, 256)
	head := "🔔"
	switch res.Level {
	case 3:
		head = "🚨"
	case 2:
		head = "⚠️"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s 群哨提醒 · %s\n", head, levelLabel(res.Level))
	fmt.Fprintf(&b, "━━━━━━━━━━\n")
	if res.Title != "" {
		fmt.Fprintf(&b, "【%s】\n", res.Title)
	}
	fmt.Fprintf(&b, "群聊：%s\n", groupName)
	if res.Time != "" {
		fmt.Fprintf(&b, "时间：%s\n", res.Time)
	}
	if res.Place != "" {
		fmt.Fprintf(&b, "地点：%s\n", res.Place)
	}
	if res.Deadline != "" {
		fmt.Fprintf(&b, "截止：%s\n", res.Deadline)
	}
	if res.Event != "" {
		fmt.Fprintf(&b, "事件：%s\n", res.Event)
	}
	if res.Summary != "" {
		fmt.Fprintf(&b, "──────────\n%s\n", res.Summary)
	}
	fmt.Fprintf(&b, "━━━━━━━━━━\n⏱ %s", time.Now().Format("2006-01-02 15:04"))
	return b.String()
}

func limitText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}

// broadcastBuffers pushes the current pending-buffer state for the dashboard
// countdown rings.
func (p *Pipeline) broadcastBuffers() {
	type bufView struct {
		GroupID   int64  `json:"groupId"`
		GroupName string `json:"groupName"`
		Count     int    `json:"count"`
		FlushAt   int64  `json:"flushAt"`
		WindowMs  int64  `json:"windowMs"` // delay of the current arm (for the ring)
		TopLabel  string `json:"topLabel"`
	}
	p.mu.Lock()
	views := make([]bufView, 0, len(p.buffers))
	for _, b := range p.buffers {
		top := "普通成员"
		lvl := LvlNormal
		for _, s := range b.msgs {
			if s.senderLevel > lvl {
				lvl = s.senderLevel
				top = s.senderLabel
			}
		}
		views = append(views, bufView{b.groupID, b.groupName, len(b.msgs), b.flushAt.UnixMilli(), b.window.Milliseconds(), top})
	}
	p.mu.Unlock()
	p.hub.Broadcast("buffer", views)
}

func levelLabel(l int) string {
	switch l {
	case 3:
		return "紧急"
	case 2:
		return "重要"
	default:
		return "一般"
	}
}
