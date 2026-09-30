// All API traffic is mocked: no real credentials, messages or configuration writes.
import assert from 'node:assert/strict'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright'
const base = process.env.SHOT_BASE ?? 'http://127.0.0.1:15178'
const out = process.env.SHOT_OUT ?? '/tmp/xunshu-workspace'
await mkdir(out, { recursive: true })
const now = Date.now()
const rules = { quietWindowSec: 120, maxHoldSec: 600, urgentKeywords: ['截止', '紧急'], atAllUrgent: true, elevateOwnerAdmin: true, senderOverrides: [] }
const groups = [{ groupId: 42, groupName: '产品与项目协作', groupRemark: '工作', memberCount: 42, watch: true }, { groupId: 43, groupName: '校园通知公告', groupRemark: '校园', memberCount: 218, watch: true }, { groupId: 44, groupName: '技术交流', memberCount: 86, watch: false }]
const masters = [{ userId: 10001, nickname: '小林', minLevel: 1, kind: 'full' }, { userId: 10002, nickname: '工作提醒', minLevel: 2, kind: 'notify' }]
const onebot = { httpBase: 'http://localhost:3000', wsUrl: 'ws://localhost:3001', token: '' }
const sources = [{ id: 'qq', name: 'NapCat', kind: 'napcat', accounts: [{ id: 'school', name: '校园账号', enabled: true, onebot, rules, groups, masters }] }]
const status = { onebotConnected: true, selfId: 10086, account: { nickname: '校园账号' }, enabled: true, llmEnabled: true, jevEnabled: true, watchedGroups: 2, totalGroups: 3, masters: 2, quietWindowSec: 120 }
const config = { sources, onebot, rules, masters, groups, enabled: true, llm: { enabled: true, baseUrl: 'https://api.example.com/v1', apiKey: '', model: 'gpt-4.1-mini', timeoutSec: 60, maxTokens: 4096, temperature: 0.2 }, jev: { enabled: true, baseUrl: 'https://api.example.com', apiKey: '', model: 'intent', threshold: 0.6, contextN: 5, timeoutSec: 30 }, documents: { enabled: true, baseUrl: 'https://mineru.net/api/v4', apiKey: '', timeoutSec: 120, maxFileMB: 20, maxTextChars: 12000 }, backup: { enabled: true, provider: 'r2', cron: '0 3 * * *', endpoint: 'https://example.r2.cloudflarestorage.com', bucket: 'notices', prefix: 'xunshu', region: 'auto', accessKey: '', secretKey: '', timeoutSec: 60 } }
const targets = [{ id: 'ntfy', name: '日常广播', kind: 'ntfy', url: 'https://ntfy.sh', topic: 'school-notices', minLevel: 1, accountIds: [], enabled: true }, { id: 'bark', name: '手机紧急提醒', kind: 'bark', url: 'https://api.day.app', group: '讯枢', minLevel: 3, accountIds: ['school'], enabled: true }]
const notices = Array.from({ length: 5 }, (_, i) => ({ id: `notice-${i}`, createdAt: now - i * 600000, groupId: 42, group: '产品与项目协作', sourceId: 'qq', accountId: 'school', sources: [{ userId: 10001, nickname: '小林', time: now / 1000 }], result: { useful: true, level: i === 0 ? 3 : 2, title: ['项目评审会安排', '本周进度提交提醒', '下周会议室调整', '材料归档要求', '周会纪要'][i], summary: '请相关同事及时确认安排，并在截止时间前准备好评审材料。', time: '周五 14:00', place: '第二会议室', event: '提交评审材料', deadline: '周四 18:00' } }))
const logs = ['已连接 NapCat，开始监听 2 个群', '项目评审会安排已推送到通知目标', '窗口内已聚合 3 条消息', '模型请求超时，稍后重试'].map((text, i) => ({ ts: now - i * 30000, text, level: ['info', 'escalate', 'suppress', 'error'][i], group: '产品与项目协作' }))
const browser = await chromium.launch({ headless: true })
const errors = [], failed = []
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
  await context.addInitScript(() => { localStorage.setItem('xunshu-account', 'school'); if (!localStorage.getItem('nap-theme')) localStorage.setItem('nap-theme', 'light') })
  await context.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const fixtures = {
      '/api/auth/status': { authed: true }, '/api/sources': sources,
      '/api/sources/status': [{ sourceId: 'qq', accountId: 'school', connected: true, selfId: 10086 }],
      '/api/status': status, '/api/config': config, '/api/groups': groups, '/api/masters': masters,
      '/api/notifications': targets, '/api/agent-keys': [{ id: 'agent', name: '个人助理', accountIds: ['school'], createdAt: now }],
      '/api/notices': notices, '/api/logs': logs,
      '/api/escalations': [{ ...notices[0].result, ts: now, groupId: 42, group: '产品与项目协作', notified: true, urgent: true }],
      '/api/backup/status': { running: false, lastRun: now, lastSuccess: now, lastError: '', filesUploaded: 12 },
      '/api/groups/history': [{ messageId: 1, messageSeq: 1, userId: 10001, nickname: '小林', role: 'admin', time: now / 1000, text: '请大家提前准备项目评审材料，谢谢。', isSelf: false }],
    }
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: [
      { type: 'hello', data: status },
      { type: 'message', data: { groupId: 42, groupName: '产品与项目协作', userId: 10001, nickname: '小林', role: 'admin', text: '请大家提前准备项目评审材料，谢谢。', messageId: 1, time: now / 1000 } },
      { type: 'buffer', data: [{ groupId: 42, groupName: '产品与项目协作', count: 3, flushAt: now + 120000, windowMs: 120000, topLabel: '管理员' }] },
    ].map((event) => `data: ${JSON.stringify(event)}\n\n`).join('') })
    assert(path in fixtures, `Unexpected endpoint ${path}`)
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(fixtures[path]) })
  })
  const page = await context.newPage()
  page.on('pageerror', (e) => errors.push(e.message))
  const routes = ['/', '/groups', '/notices', '/logs', '/notifications', '/masters', '/rules', '/intelligence', '/sources', '/agents', '/storage', '/connection', '/groups/42/history']
  for (const width of [1440, 768, 390]) {
    await page.setViewportSize({ width, height: 960 })
    for (const route of routes) {
      await page.goto(base + route)
      await page.locator('h1').first().waitFor()
      await page.waitForTimeout(200)
      const overflow = await page.evaluate(() => {
        const root = document.querySelector('.app-layout__main')
        return document.documentElement.scrollWidth > innerWidth + 1 || (root && root.scrollWidth > root.clientWidth + 1)
      })
      if (overflow) failed.push(`${width}px ${route} overflows horizontally`)
      if (width !== 768) await page.screenshot({ path: `${out}/${width}-${route === '/' ? 'overview' : route.slice(1).replaceAll('/', '-')}.png` })
      if (width === 1440 && route === '/notifications') {
        const boxes = await page.locator('[data-slot="card"]').evaluateAll((nodes) => nodes.map((node) => { const b = node.getBoundingClientRect(); return { top: b.top, height: b.height } }))
        assert.equal(boxes.length, 2)
        assert(Math.abs(boxes[0].top - boxes[1].top) < 3, 'Notification targets should share one desktop row')
        assert(boxes.every((b) => b.height < 550), 'Notification cards should stay compact')
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 960 })
  await page.goto(base + '/')
  await page.getByRole('button', { name: '搜索页面', exact: true }).click()
  await page.getByRole('searchbox').fill('Bark')
  await page.getByRole('menuitem', { name: /广播通知/ }).click()
  await page.waitForURL(base + '/notifications')
  await page.getByRole('navigation', { name: '配置页面' }).getByRole('link', { name: '主人', exact: true }).click()
  await page.waitForURL(base + '/masters')
  await page.getByRole('button', { name: '收起或展开侧栏' }).click()
  await page.waitForTimeout(350)
  await page.screenshot({ path: `${out}/sidebar-collapsed.png` })
  assert((await page.locator('aside[data-slot=sidebar]').boundingBox()).width < 100, 'Collapsed sidebar should form an icon rail')
  await page.getByRole('button', { name: '收起或展开侧栏' }).click()
  await page.goto(base + '/notifications')
  await page.getByLabel('暗色', { exact: true }).click()
  await page.getByRole('heading', { name: '日常广播', exact: true }).waitFor()
  await page.waitForTimeout(300)
  await page.screenshot({ path: `${out}/desktop-dark.png` })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByLabel('打开导航', { exact: true }).click()
  await page.getByRole('dialog').locator('[data-slot=sidebar-menu-item]').filter({ hasText: '处理策略' }).click()
  await page.waitForURL(base + '/rules')
  await page.getByRole('heading', { name: '规则', exact: true }).waitFor()
  await page.getByRole('dialog').waitFor({ state: 'hidden' })
  assert.deepEqual(errors, [], 'Browser runtime errors')
  assert.deepEqual(failed, [], 'Responsive layout failures')
  console.log('PASS 13 routes at desktop/tablet/mobile, compact cards, command search, contextual tabs, mobile navigation, theme and sidebar')
  console.log(`Screenshots: ${out}`)
} finally { await browser.close() }
