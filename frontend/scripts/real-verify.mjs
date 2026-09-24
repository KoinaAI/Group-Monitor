// REAL-backend verification (no mocks, no DMs). Unlike shot-verify.mjs, this does
// NOT fabricate API payloads: it logs into an ISOLATED backend instance on :8799
// (started with onebot.wsUrl blanked → no pipeline → no DMs, but httpBase+token
// kept → read-only history/download work), then proxies every browser /api/**
// call to that real backend carrying the genuine nap_session cookie. So the app
// renders REAL group history — real inline images, real @-names, real reply
// quotes, real downloadable file cards — proving the segments fix end-to-end.
// Run (with the isolated :8799 backend already up): node scripts/real-verify.mjs
import { chromium } from 'playwright'
import { mkdir } from 'node:fs/promises'

const APP = process.env.SHOT_BASE ?? 'http://localhost:5173'
const API = process.env.REAL_API ?? 'http://127.0.0.1:8799'
const OUT = process.env.SHOT_OUT ?? 'shots-real'
const PASS = process.env.NAP_PASSWORD ?? 'napdev2026'

// Log into the isolated backend from Node (not the browser) to mint a session.
async function login() {
  const r = await fetch(`${API}/api/auth/password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: PASS }),
  })
  const setCookie = r.headers.get('set-cookie') || ''
  const m = setCookie.match(/nap_session=[^;]+/)
  if (!m) throw new Error(`login failed: ${r.status} ${await r.text()}`)
  return m[0]
}

const cookie = await login()
console.log('session minted:', cookie.slice(0, 24) + '…')

async function proxy(route) {
  const req = route.request()
  const path = new URL(req.url()).pathname + new URL(req.url()).search
  // Short-circuit the SSE stream: the live feed is not under test and a streamed
  // body can't be fulfilled in one shot. Hand back a single hello event.
  if (path.startsWith('/api/events')) {
    return route.fulfill({
      status: 200,
      headers: { 'content-type': 'text/event-stream' },
      body: 'data: {"type":"hello","data":{"onebotConnected":true}}\n\n',
    })
  }
  try {
    const upstream = await fetch(API + path, {
      method: req.method(),
      headers: { ...req.headers(), cookie },
      body: ['GET', 'HEAD'].includes(req.method()) ? undefined : req.postData() || undefined,
    })
    const buf = Buffer.from(await upstream.arrayBuffer())
    const headers = {}
    upstream.headers.forEach((v, k) => {
      if (!['content-encoding', 'content-length', 'transfer-encoding'].includes(k)) headers[k] = v
    })
    return route.fulfill({ status: upstream.status, headers, body: buf })
  } catch (e) {
    return route.fulfill({ status: 502, body: String(e) })
  }
}

async function shoot(page, name) {
  await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true })
  console.log(`  ${name}.png`)
}

// Groups with the richest real content (found by scanning live history):
//  1061928546 新工科A3 — inline images + @names + a reply quote + a file
//  1106796074 程序设计基础 — reply quotes with real original text + images
//  1051168155 通知群 — many downloadable file cards (.pdf/.mp4/.xlsx/.doc)
const HISTORY = [
  ['history-rich', 1061928546],
  ['history-quotes', 1106796074],
  ['history-files', 1051168155],
]

const browser = await chromium.launch()
try {
  await mkdir(OUT, { recursive: true })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  await page.route('**/api/**', proxy)
  await page.addInitScript(() => localStorage.setItem('nap-theme', 'dark'))

  await page.goto(`${APP}/groups`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1200)
  await shoot(page, 'groups')

  for (const [name, gid] of HISTORY) {
    await page.goto(`${APP}/groups/${gid}/history`, { waitUntil: 'domcontentloaded' })
    // Real OneBot history + member-list + quote resolution takes a beat; then let
    // the inline <img>s fetch from the QQ CDN before the shot.
    await page.waitForTimeout(3500)
    const loaded = await page
      .locator('text=/已加载 \\d+ 条/')
      .first()
      .innerText()
      .catch(() => '(none)')
    const imgs = await page.locator('main img').count()
    const files = await page.locator('a[download]').count()
    console.log(`  ${name} (g=${gid}): ${loaded} | <img>=${imgs} fileCards=${files}`)
    await shoot(page, name)
  }

  // Page polish (real master list): neumorphic-fixed selects on masters, and the
  // number steppers/selects on rules + intelligence.
  await page.goto(`${APP}/masters`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1200)
  await shoot(page, 'masters')

  await page.goto(`${APP}/rules`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1000)
  await shoot(page, 'rules')

  await ctx.close()
} finally {
  await browser.close()
}
