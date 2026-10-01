// All API traffic is mocked: no real credentials, messages or configuration writes.
import assert from 'node:assert/strict'
import { mkdir, writeFile } from 'node:fs/promises'
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
const screenshots = []
let authScene = 'protected'
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
  await context.addInitScript(() => { localStorage.setItem('xunshu-account', 'school'); if (!localStorage.getItem('nap-theme')) localStorage.setItem('nap-theme', 'light') })
  await context.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const fixtures = {
      '/api/auth/status': authScene === 'protected' ? { authed: true } : authScene === 'login'
        ? { authed: false, setupRequired: false, otpAvailable: true, passwordAvailable: true, passwordConfigured: true, onebotConnected: true, masters: 2 }
        : { authed: false, setupRequired: true },
      '/api/setup/status': { required: authScene === 'setup', tokenRequired: authScene === 'setup' },
      '/api/sources': sources,
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
  const capture = async (name, width, theme, file) => {
    // AppLayout scrolls its main pane internally, so fullPage alone only sees
    // the viewport. Expand that pane briefly to capture every section.
    const expand = await page.addStyleTag({ content: `
      html, body, #root { height: auto !important; }
      [data-app-layout][data-scroll-mode="content"] { height: auto !important; min-height: 100vh !important; overflow: visible !important; }
      [data-app-layout][data-scroll-mode="content"] .app-layout__main { height: auto !important; overflow: visible !important; }
    ` })
    await page.screenshot({ path: `${out}/${file}`, fullPage: true })
    await expand.evaluate((node) => node.remove())
    screenshots.push({ name, width, theme, file })
  }
  const assertGlass = async (theme) => {
    const result = await page.evaluate(() => {
      const root = document.documentElement
      return {
        classes: root.className,
        blur: getComputedStyle(root).getPropertyValue('--glass-blur').trim(),
        surface: getComputedStyle(root).getPropertyValue('--surface').trim(),
        widgetBlur: getComputedStyle(document.querySelector('.widget')).backdropFilter,
      }
    })
    assert(result.classes.includes(`glass-${theme}`), `${theme} Glass class should be active`)
    assert.equal(result.blur, theme === 'dark' ? '36px' : '20px')
    assert.match(result.surface, /oklch/)
    assert(result.widgetBlur.includes('blur('), 'Glass should blur Pro widgets')
  }
  const routes = ['/', '/groups', '/notices', '/logs', '/notifications', '/masters', '/rules', '/intelligence', '/sources', '/agents', '/storage', '/connection', '/groups/42/history']
  for (const width of [1440, 768, 390]) {
    await page.setViewportSize({ width, height: 960 })
    for (const route of routes) {
      await page.goto(base + route)
      await page.locator('h1').first().waitFor()
      await page.waitForTimeout(200)
      if (width === 1440 && route === '/') await assertGlass('light')
      const overflow = await page.evaluate(() => {
        const root = document.querySelector('.app-layout__main')
        return document.documentElement.scrollWidth > innerWidth + 1 || (root && root.scrollWidth > root.clientWidth + 1)
      })
      if (overflow) failed.push(`${width}px ${route} overflows horizontally`)
      if (width !== 768) {
        const name = route === '/' ? 'overview' : route.slice(1).replaceAll('/', '-')
        await capture(name, width, 'light', `${width}-${name}.png`)
      }
      if (width === 1440 && route === '/notifications') {
        const boxes = await page.locator('[data-slot="card"]').evaluateAll((nodes) => nodes.map((node) => { const b = node.getBoundingClientRect(); return { top: b.top, height: b.height } }))
        assert.equal(boxes.length, 2)
        assert(Math.abs(boxes[0].top - boxes[1].top) < 3, 'Notification targets should share one desktop row')
        assert(boxes.every((b) => b.height < 550), 'Notification cards should stay compact')
      }
    }
  }
  // Capture the same route matrix with the persisted dark theme so surface
  // contrast and compact spacing are checked in both color schemes.
  await page.evaluate(() => localStorage.setItem('nap-theme', 'dark'))
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 960 })
    for (const route of routes) {
      await page.goto(base + route)
      await page.locator('h1').first().waitFor()
      await page.waitForTimeout(120)
      if (width === 1440 && route === '/') await assertGlass('dark')
      const name = route === '/' ? 'overview' : route.slice(1).replaceAll('/', '-')
      await capture(name, width, 'dark', `${width}-dark-${name}.png`)
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
  await capture('sidebar-collapsed', 1440, 'dark', 'sidebar-collapsed.png')
  assert((await page.locator('aside[data-slot=sidebar]').boundingBox()).width < 100, 'Collapsed sidebar should form an icon rail')
  await page.getByRole('button', { name: '收起或展开侧栏' }).click()
  await page.goto(base + '/notifications')
  await page.getByLabel('暗色', { exact: true }).click()
  await page.getByRole('heading', { name: '日常广播', exact: true }).waitFor()
  await page.waitForTimeout(300)
  await capture('desktop-dark', 1440, 'dark', 'desktop-dark.png')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByLabel('打开导航', { exact: true }).click()
  await page.getByRole('dialog').locator('[data-slot=sidebar-menu-item]').filter({ hasText: '处理策略' }).click()
  await page.waitForURL(base + '/rules')
  await page.getByRole('heading', { name: '规则', exact: true }).waitFor()
  await page.getByRole('dialog').waitFor({ state: 'hidden' })

  for (const scene of ['login', 'setup']) {
    authScene = scene
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => localStorage.setItem('nap-theme', value), theme)
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 960 })
        await page.goto(`${base}/${scene}`)
        await page.getByRole('heading', { name: scene === 'login' ? '登录讯枢' : '设置讯枢' }).waitFor()
        await page.waitForTimeout(120)
        assert.equal(new URL(page.url()).pathname, `/${scene}`, `${scene} should remain on its public route`)
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)
        if (overflow) failed.push(`${width}px ${theme} /${scene} overflows horizontally`)
        await capture(scene, width, theme, `${width}-${theme}-${scene}.png`)
      }
    }
  }

  assert.deepEqual(errors, [], 'Browser runtime errors')
  assert.deepEqual(failed, [], 'Responsive layout failures')
  const labels = {
    overview: '总览', groups: '群组', notices: '通知记录', logs: '运行日志', notifications: '广播通知',
    masters: '主人', rules: '规则', intelligence: '智能', sources: '信息源', agents: 'Agent 接入',
    storage: '存储', connection: '连接', 'groups-42-history': '群历史', login: '登录', setup: '初始化',
    'sidebar-collapsed': '收起侧栏', 'desktop-dark': '暗色交互',
  }
  const sections = [...new Set(screenshots.map((shot) => shot.name))].map((name) => {
    const tiles = screenshots.filter((shot) => shot.name === name).map(({ width, theme, file }) => `
      <a class="shot" href="${file}" target="_blank" rel="noopener">
        <span>${width}px · ${theme === 'light' ? '亮色' : '暗色'}</span>
        <img src="${file}" alt="${labels[name]} ${width}px ${theme === 'light' ? '亮色' : '暗色'}" loading="lazy">
      </a>`).join('')
    return `<section><h2>${labels[name]}</h2><div class="shots">${tiles}</div></section>`
  }).join('')
  await writeFile(`${out}/index.html`, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>讯枢页面截图</title>
<style>body{margin:0;background:#f5f6f7;color:#1f252c;font:14px/1.5 system-ui,-apple-system,sans-serif}main{max-width:1440px;margin:auto;padding:24px}h1{font-size:24px;margin:0 0 4px}p{color:#5b6570;margin:0 0 28px}section{margin:0 0 32px}h2{font-size:17px;margin:0 0 12px}.shots{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,280px),1fr));gap:12px}.shot{display:block;min-width:0;overflow:hidden;border:1px solid #dce1e5;border-radius:6px;background:white;color:inherit;text-decoration:none}.shot span{display:block;padding:8px 12px;border-bottom:1px solid #e8ecef;font-weight:600}.shot img{display:block;width:100%;height:300px;object-fit:contain;object-position:top;background:#eef0f2}@media(max-width:640px){main{padding:16px}.shot img{height:240px}}</style>
</head><body><main><h1>讯枢页面截图</h1><p>${screenshots.length} 张整页截图。点击缩略图查看原图。</p>${sections}</main></body></html>`)
  console.log('PASS 13 protected routes at desktop/tablet/mobile plus login and setup in both themes, compact cards, command search, contextual tabs, mobile navigation, theme and sidebar')
  console.log(`Screenshots: ${out}/index.html (${screenshots.length} full-page images)`)
} finally { await browser.close() }
