// Screenshot harness for the design-review loop: log in via the break-glass
// password (works while NapCat is offline), then capture every route at desktop
// and mobile widths. Run against the dev server:
//
//   NAP_PASSWORD=... node scripts/shot.mjs
//
// Env: SHOT_BASE (default http://localhost:5173), NAP_PASSWORD, SHOT_OUT
// (default ./shots). Requires `npx playwright install chromium` once.
import { chromium } from 'playwright'
import { mkdir } from 'node:fs/promises'

const BASE = process.env.SHOT_BASE ?? 'http://localhost:5173'
const PASSWORD = process.env.NAP_PASSWORD ?? ''
const OUT = process.env.SHOT_OUT ?? 'shots'

const ROUTES = [
  ['overview', '/'],
  ['groups', '/groups'],
  ['logs', '/logs'],
  ['masters', '/masters'],
  ['rules', '/rules'],
  ['intelligence', '/intelligence'],
  ['connection', '/connection'],
]
const VIEWPORTS = [
  ['desktop', 1440, 900],
  ['mobile', 390, 844],
]

async function shoot(page, name, vp) {
  await page.screenshot({ path: `${OUT}/${vp}-${name}.png`, fullPage: true })
  console.log(`  ${vp}-${name}.png`)
}

const browser = await chromium.launch()
try {
  await mkdir(OUT, { recursive: true })
  for (const [vp, width, height] of VIEWPORTS) {
    console.log(`[${vp}]`)
    // Login page in a fresh, unauthenticated context.
    const anon = await browser.newContext({ viewport: { width, height } })
    const anonPage = await anon.newPage()
    // Use 'domcontentloaded' not 'networkidle': the authed area holds a
    // long-lived SSE connection (/api/events) that keeps the network "busy"
    // forever, so networkidle would never settle.
    await anonPage.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
    await anonPage.waitForTimeout(500)
    await shoot(anonPage, 'login', vp)
    await anon.close()

    // Authenticated context for the rest.
    const ctx = await browser.newContext({ viewport: { width, height } })
    if (PASSWORD) {
      const res = await ctx.request.post(`${BASE}/api/auth/password`, {
        data: { password: PASSWORD },
      })
      if (!res.ok()) console.warn(`  ! password login failed: ${res.status()}`)
    } else {
      console.warn('  ! NAP_PASSWORD not set — protected routes will redirect')
    }
    const page = await ctx.newPage()
    for (const [name, path] of ROUTES) {
      await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(900) // let SSE seed + fetches + charts settle
      await shoot(page, name, vp)
    }
    await ctx.close()
  }
} finally {
  await browser.close()
}
