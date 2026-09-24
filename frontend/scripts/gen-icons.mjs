// Build a minimal Gravity UI Iconify collection containing only the icons the
// app actually references, so we ship ~40 glyphs instead of the full ~1500-icon
// set (which alone was ~440 KB of JSON / ~578 KB in the bundle).
//
// Source of truth for the used ids is the ICONS map in src/lib/icons.tsx — this
// script parses those values, so adding an icon there and re-running keeps the
// subset in sync. Regenerate with:  node scripts/gen-icons.mjs
import { readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const full = require('@iconify-json/gravity-ui/icons.json')

const src = readFileSync(new URL('../src/lib/icons.tsx', import.meta.url), 'utf8')
const block = src.slice(src.indexOf('export const ICONS'), src.indexOf('} as const'))
const ids = [...block.matchAll(/:\s*'([a-z0-9-]+)'/g)].map((m) => m[1])
const used = [...new Set(ids)].sort()

const icons = {}
const missing = []
for (const id of used) {
  if (full.icons[id]) icons[id] = full.icons[id]
  else missing.push(id)
}
if (missing.length) {
  console.error(`Missing icons in @iconify-json/gravity-ui: ${missing.join(', ')}`)
  process.exit(1)
}

const subset = { prefix: full.prefix, icons }
const out = new URL('../src/lib/gravity-subset.json', import.meta.url)
writeFileSync(out, JSON.stringify(subset))
console.log(`Wrote ${used.length} icons to src/lib/gravity-subset.json`)
