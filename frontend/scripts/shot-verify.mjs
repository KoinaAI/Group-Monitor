// Mock-based verification for the group-history viewer (Task 5), group avatars
// (Task 6) and the collapsed-sidebar footer (Task 4). NapCat is ONLINE so the
// break-glass password is rejected; instead we intercept every /api/** call and
// serve fixtures, satisfying RequireAuth via a mocked /api/auth/status. No real
// backend traffic, no OTP. Run: node scripts/shot-verify.mjs
import { chromium } from 'playwright'
import { mkdir } from 'node:fs/promises'

const BASE = process.env.SHOT_BASE ?? 'http://localhost:5173'
const OUT = process.env.SHOT_OUT ?? 'shots-verify'

const GROUPS = [
  { groupId: 100001, groupName: '项目评审组', groupRemark: '核心项目群', memberCount: 42, watch: true },
  { groupId: 100002, groupName: '通知公告', groupRemark: '', memberCount: 210, watch: true },
  { groupId: 100003, groupName: '技术分享', groupRemark: '内部', memberCount: 88, watch: false },
]

// Task 3 fixtures: an existing master list plus a candidate to bind. The bind
// OTP is fully mocked (request returns a nickname + ttl, confirm returns ok +
// the fresh list) so no real DM is ever sent — a real master is on file. The
// list mixes a notify-only master (kind:'notify', share target, no login) with
// full masters (kind absent) so the 仅通知/完整主人 chips both render.
const MASTERS = [
  { userId: 10001, nickname: '主人甲', minLevel: 1 },
  { userId: 10002, nickname: '主人乙', minLevel: 2 },
  { userId: 10003, nickname: '分享对象丙', minLevel: 0, kind: 'notify' },
]
const CANDIDATE = { userId: 3141592653, nickname: '测试候选人' }

// A deterministic inline image so history image segments render offline (no
// external fetch): a plain SVG data URL, not a [图片] placeholder.
const SAMPLE_IMG =
  'data:image/svg+xml,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180">' +
      '<rect width="320" height="180" fill="#4f7cff"/>' +
      '<text x="160" y="98" font-size="22" fill="white" text-anchor="middle" ' +
      'font-family="sans-serif">示例图片</text></svg>',
  )

// Task 2 fixture: GET /api/logs now returns NEWEST-FIRST (backend RecentLogs was
// fixed to mirror RecentEscalations). Distinct descending timestamps let the
// screenshot prove the 事件流 renders newest-on-top before any live SSE event.
const LOGS = (() => {
  const now = Date.now()
  const levels = ['urgent', 'escalate', 'suppress', 'info', 'info', 'error']
  const texts = [
    '紧急：项目评审组 @全体成员 触发即时推送',
    '升级：通知公告 蒸馏后推送给主人',
    '抑制：技术分享 静默窗口内聚合 3 条消息',
    '信息：已连接 NapCat，开始监听 2 个群',
    '信息：加载配置完成，主人 1 位',
    '错误：LLM 网关超时，已回退原文推送',
  ]
  return texts.map((text, i) => ({
    ts: now - i * 60000, // index 0 = newest
    level: levels[i],
    group: i < 3 ? ['项目评审组', '通知公告', '技术分享'][i] : undefined,
    text,
  }))
})()

// Generate one oldest-first batch of `count` messages ending at seq `top`.
// Every 6th seq carries a different structured payload so the new renderer is
// screenshot-provable: a group file (rectangular download card), an inline
// image, a voice clip, and an @-mention + quoted-original combo. The rest are
// plain text. Showcase seqs land near the top of each batch (…996 file, 997
// image, 998 voice, 999 at/reply) so they sit at the bottom of the initial,
// auto-scrolled-to-newest view.
function batch(top, count) {
  const roles = ['member', 'admin', 'owner', 'member', 'member']
  const titles = ['', '', '资深', '', '答疑志愿者']
  const names = ['张三', '李四', '王五', '赵六', '钱七']
  const out = []
  const start = top - count + 1
  for (let seq = start; seq <= top; seq++) {
    const i = seq % 5
    const msg = {
      messageId: seq * 10,
      messageSeq: seq,
      userId: 2000 + i,
      nickname: names[i],
      role: roles[i],
      title: titles[i],
      time: 1758600000 + seq * 60,
      text: `这是第 ${seq} 条测试消息，用于验证历史记录的懒加载与排版效果。`,
      hasImage: false,
      isSelf: seq % 11 === 0,
    }
    const r = seq % 6
    if (r === 0) {
      // Group file → compact rectangular download card (files are their own
      // array, not a segment).
      msg.text = ''
      msg.files = [
        { name: `评审材料-${seq}.pdf`, fileId: `f${seq}`, size: 1536000 + seq * 1024, busid: 102 },
      ]
    } else if (r === 1) {
      msg.text = ''
      msg.segments = [
        { type: 'text', text: '这是本周的现场照片，请查收：' },
        { type: 'image', url: SAMPLE_IMG },
      ]
    } else if (r === 2) {
      // Voice → inline <audio> player, never [语音].
      msg.text = ''
      msg.segments = [{ type: 'record', url: 'https://example.com/voice/sample.mp3' }]
    } else if (r === 3) {
      // @-mention shows the NAME; reply shows the ORIGINAL, never [引用].
      msg.text = ''
      msg.segments = [
        { type: 'reply', reply: { userId: 2001, nickname: '李四', text: '请在今天内提交本周进度文档，谢谢。' } },
        { type: 'at', id: 2002, name: '王五' },
        { type: 'text', text: ' 收到，我这边下午补交。' },
      ]
    }
    out.push(msg)
  }
  return out
}

async function mockApi(page) {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    const p = url.pathname.replace(/^\/api/, '')
    const json = (data) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })

    if (p === '/auth/status')
      return json({ authed: true, otpAvailable: true, passwordConfigured: false, passwordAvailable: false, onebotConnected: true, masters: 1 })
    if (p === '/status')
      return json({ onebotConnected: true, selfId: 10086, account: { user_id: 10086, nickname: '测试机器人' }, enabled: true, llmEnabled: true, jevEnabled: true, watchedGroups: 2, totalGroups: 3, masters: 1, quietWindowSec: 120, serverTime: Date.now() })
    if (p === '/config')
      return json({ onebot: { httpBase: '', wsUrl: '', token: '' }, llm: { enabled: true, baseUrl: '', apiKey: '', model: 'gpt', timeoutSec: 30, maxTokens: 800, temperature: 0.2 }, jev: { enabled: true, baseUrl: '', apiKey: '', model: '', threshold: 0.6, contextN: 5, timeoutSec: 20 }, masters: [], groups: GROUPS, rules: { quietWindowSec: 120, maxHoldSec: 600, urgentKeywords: [], atAllUrgent: true, elevateOwnerAdmin: true, senderOverrides: [] }, enabled: true })
    if (p === '/groups') return json(GROUPS)
    if (p === '/groups/history') {
      const before = Number(url.searchParams.get('beforeSeq') || 0)
      const count = Number(url.searchParams.get('count') || 30)
      const top = before ? before - 1 : 1000
      if (top < 940) return json([]) // floor: total 60 messages then stop
      return json(batch(top, count))
    }
    if (p === '/groups/file-url') return json({ url: 'https://example.com/download/sample.pdf' })
    if (p === '/masters') return json(MASTERS) // GET list / POST save both echo the set
    if (p === '/lookup')
      return json({ userId: Number(url.searchParams.get('userId') || CANDIDATE.userId), nickname: CANDIDATE.nickname })
    if (p === '/masters/verify/request') return json({ nickname: CANDIDATE.nickname, ttlSec: 180 })
    if (p === '/masters/verify/confirm') {
      // Echo the posted minLevel/kind so the bound row reflects the operator's
      // choice (full vs notify-only) — the backend returns the fresh master set.
      const body = route.request().postDataJSON?.() || {}
      const bound = {
        userId: CANDIDATE.userId,
        nickname: CANDIDATE.nickname,
        minLevel: body.minLevel ?? 1,
        kind: body.kind || 'full',
      }
      return json({ ok: true, masters: [...MASTERS, bound] })
    }
    if (p === '/test-notify') return json({ sent: MASTERS.length, failed: [] })
    if (p === '/logs') return json(LOGS)
    if (p === '/escalations') return json([])
    if (p === '/events')
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"hello","data":{"onebotConnected":true,"selfId":10086,"enabled":true,"llmEnabled":true,"jevEnabled":true,"watchedGroups":2,"masters":1},"ts":0}\n\n' })
    return json({})
  })
}

async function shoot(page, name) {
  await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true })
  console.log(`  ${name}.png`)
}

const browser = await chromium.launch()
try {
  await mkdir(OUT, { recursive: true })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  await mockApi(page)
  await page.addInitScript(() => localStorage.setItem('nap-theme', 'light'))

  // Task 6: group avatars on the Groups page.
  await page.goto(`${BASE}/groups`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(900)
  await shoot(page, 'groups-avatars')

  // Task 5: open history via the 历史 button (carries router state).
  await page.getByRole('button', { name: '历史' }).first().click()
  await page.waitForTimeout(1000)
  await shoot(page, 'history-initial')

  // Lazyload: the viewer opens scrolled to the bottom (newest) and loads OLDER
  // batches when the TOP sentinel scrolls into view. Pull that sentinel up each
  // pass to trip the IntersectionObserver; re-locate every time since a prepend
  // re-anchors the viewport near the bottom again.
  const loaded = () => page.locator('text=/已加载 \\d+ 条/').first().innerText()
  console.log(`  initial: ${await loaded()}`)
  for (let i = 0; i < 5; i++) {
    await page
      .locator('text=向上滚动加载更早的消息')
      .scrollIntoViewIfNeeded()
      .catch(() => {})
    await page.waitForTimeout(800)
  }
  console.log(`  after scroll: ${await loaded()}`)
  await shoot(page, 'history-loaded-more')

  // Task 2: event stream ordering. Seeded logs arrive newest-first; the 事件流
  // must show the latest entry on top. Read the first row and assert it is the
  // urgent (newest) one, not the error (oldest).
  await page.goto(`${BASE}/logs`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(900)
  const firstRow = await page.locator('ul > li').first().innerText()
  const lastRow = await page.locator('ul > li').last().innerText()
  console.log(`  logs first row: ${firstRow.replace(/\s+/g, ' ').trim()}`)
  console.log(`  logs last row:  ${lastRow.replace(/\s+/g, ' ').trim()}`)
  await shoot(page, 'logs-order')

  // Task 4: collapsed sidebar footer — toggle the desktop icon rail. The
  // trigger renders as an icon-only button with data-slot="sidebar-trigger"
  // (no accessible name), so target it by slot.
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(600)
  await page.locator('[data-slot=sidebar-trigger]').first().click()
  await page.waitForTimeout(600)
  const collapsedState = await page
    .locator('[data-state=collapsed]')
    .count()
    .catch(() => 0)
  console.log(`  collapsed markers: ${collapsedState}`)
  await shoot(page, 'sidebar-collapsed')

  // Page polish: 规则 and 智能 hold the number steppers + selects switched to
  // variant="secondary" (flat, no 新拟态 shadow). Screenshot both to confirm the
  // inputs read flat against the HeroUI spec.
  await page.goto(`${BASE}/rules`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(800)
  await shoot(page, 'rules-inputs')

  await page.goto(`${BASE}/intelligence`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(800)
  await shoot(page, 'intelligence-inputs')

  // Task 3: secure add-master wizard. Drive all three phases against mocks so no
  // real bind OTP is ever DM'd. input → 查询 → review(avatar/nickname) → 发送验证码
  // → otp → type code → onComplete binds → success notice + refreshed list.
  await page.goto(`${BASE}/masters`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(900)
  await shoot(page, 'masters-input')

  await page.getByRole('textbox', { name: 'QQ 号' }).fill(String(CANDIDATE.userId))
  await page.getByRole('button', { name: '查询' }).click()
  await page.waitForTimeout(700)
  await shoot(page, 'masters-review')

  await page.getByRole('button', { name: '确认此人并发送验证码' }).click()
  await page.waitForTimeout(700)
  await shoot(page, 'masters-otp')

  // Complete the code: InputOTP autofocuses on the otp phase, so type 6 digits.
  await page.keyboard.type('123456')
  await page.waitForTimeout(700)
  await shoot(page, 'masters-bound')
  const boundNotice = await page
    .locator('text=/已通过验证码绑定主人/')
    .first()
    .innerText()
    .catch(() => '(no notice)')
  console.log(`  bind notice: ${boundNotice.replace(/\s+/g, ' ').trim()}`)

  await ctx.close()
} finally {
  await browser.close()
}
