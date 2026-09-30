// Start Vite, then run SHOT_BASE=http://127.0.0.1:5178 node scripts/verify-notice-storage.mjs.
// All API traffic is mocked; no uploads or real notifications are sent.
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } })
const errors = []
page.on('pageerror', (error) => errors.push(error.message))
const backup = { enabled: true, provider: 'r2', cron: '0 3 * * *', endpoint: 'https://account.r2.cloudflarestorage.com', bucket: 'notices', prefix: 'group-monitor', region: 'auto', accessKey: '', secretKey: '', timeoutSec: 60 }
const documents = { enabled: false, baseUrl: 'https://mineru.net/api/v4', apiKey: '', timeoutSec: 120, maxFileMB: 20, maxTextChars: 12000 }
const config = { backup, documents, llm: { enabled: true, baseUrl: '', apiKey: '', model: '', timeoutSec: 30, maxTokens: 1000, temperature: 0 }, jev: { enabled: true, baseUrl: '', apiKey: '', model: '', threshold: 0.4, contextN: 3, timeoutSec: 30 } }
const notices = Array.from({ length: 30 }, (_, i) => ({ id: String(i), createdAt: Date.now() - i * 1000, groupId: 42, group: '项目群', messageIds: [i + 1], sources: [{ userId: 2, nickname: '老师', time: 100 }], result: { useful: true, level: 2, title: `通知 ${i}`, summary: '周五前提交确认', time: '周五', place: '会议室', event: '提交确认', deadline: '17:00' }, urgent: false }))
let failSave = true
let failRun = true
let backupRuns = 0
const noticeRequests = []
let documentBody

await page.route('**/api/**', async (route) => {
  const url = new URL(route.request().url())
  const path = url.pathname
  let status = 200
  let body = {}
  if (path === '/api/auth/status') body = { authed: true }
  else if (path === '/api/sources') body = []
  else if (path === '/api/config') body = config
  else if (path === '/api/status') body = { account: { nickname: '测试账号' } }
  else if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': ready\n\n' })
  else if (path === '/api/logs' || path === '/api/escalations') body = []
  else if (path === '/api/backup/status') body = { running: false, lastRun: 0, lastSuccess: 0, lastError: '', filesUploaded: 0 }
  else if (path === '/api/backup') {
    if (failSave) { status = 400; body = { error: '无效的备份配置' } }
    else { Object.assign(backup, route.request().postDataJSON(), { accessKey: '', secretKey: '' }); body = backup }
  } else if (path === '/api/backup/run') {
    backupRuns++
    if (failRun) { status = 502; body = { error: '存储桶连接失败' } }
    else body = { ok: true }
  } else if (path === '/api/documents') {
    documentBody = route.request().postDataJSON()
    body = { ...documentBody, apiKey: '' }
  } else if (path === '/api/notices') {
    noticeRequests.push(url)
    body = url.searchParams.has('q') ? [{ ...notices[0], id: 'search', result: { ...notices[0].result, title: '搜索命中的通知' } }] : url.searchParams.has('before') ? [] : notices
  } else throw new Error(`Unexpected request: ${path}`)
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
})

try {
  const base = process.env.SHOT_BASE ?? 'http://127.0.0.1:5178'
  await page.goto(`${base}/storage`)
  await page.getByRole('heading', { name: '存储与备份', exact: true }).waitFor()
  await page.getByLabel('Cloudflare Account ID').fill('a'.repeat(32))
  await page.getByRole('button', { name: '应用 R2 预设' }).click()
  assert.equal(await page.getByLabel('Endpoint', { exact: true }).inputValue(), `https://${'a'.repeat(32)}.r2.cloudflarestorage.com`)
  assert.equal(await page.getByRole('button', { name: '立即备份', exact: true }).isDisabled(), true)
  await page.getByLabel('Secret Access Key', { exact: true }).fill('test-only-secret')
  await page.getByRole('button', { name: '保存更改', exact: true }).click()
  await page.getByRole('alert').filter({ hasText: '无效的备份配置' }).waitFor()
  assert.equal(backupRuns, 0)
  failSave = false
  await page.getByRole('button', { name: '保存更改', exact: true }).click()
  await page.getByText('配置已保存', { exact: true }).waitFor()
  assert.equal(await page.getByLabel('Secret Access Key', { exact: true }).inputValue(), '')
  await page.getByRole('button', { name: '立即备份', exact: true }).click()
  await page.getByRole('alert').filter({ hasText: '存储桶连接失败' }).waitFor()
  failRun = false
  await page.getByRole('button', { name: '立即备份', exact: true }).click()
  await page.getByText('备份已完成', { exact: true }).waitFor()

  await page.goto(`${base}/intelligence`)
  await page.getByLabel('MinerU API Key', { exact: true }).fill('test-mineru-key')
  await page.getByText('读取通知附件', { exact: true }).click()
  await page.getByRole('button', { name: '保存更改', exact: true }).click()
  await page.getByText('附件阅读配置已保存').waitFor()
  assert.equal(documentBody.enabled, true)
  assert.equal(documentBody.apiKey, 'test-mineru-key')
  assert.equal(await page.getByLabel('MinerU API Key', { exact: true }).inputValue(), '')

  await page.goto(`${base}/notices`)
  await page.getByText('通知 0', { exact: true }).waitFor()
  await page.getByRole('button', { name: '加载更早的通知' }).click()
  await page.waitForFunction(() => ![...document.querySelectorAll('button')].some((button) => button.textContent.includes('加载更早的通知')))
  assert.equal(noticeRequests.at(-1).searchParams.get('before'), String(notices.at(-1).createdAt))
  await page.getByLabel('关键词', { exact: true }).fill('提交')
  await page.getByLabel('群号', { exact: true }).fill('42')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await page.getByText('搜索命中的通知', { exact: true }).waitFor()
  assert.equal(noticeRequests.at(-1).searchParams.get('q'), '提交')
  assert.equal(noticeRequests.at(-1).searchParams.get('groupId'), '42')
  await page.setViewportSize({ width: 390, height: 844 })
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true)
  assert.deepEqual(errors, [])
  console.log('PASS: R2 preset, unsaved backup guard, visible save/run failures, credential redaction, document settings, notice search/pagination, mobile width, no browser errors')
} finally {
  await browser.close()
}
