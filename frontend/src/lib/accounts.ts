import type { SourceAccount } from './types'
const key = 'xunshu-account'
let pageAccount = localStorage.getItem(key) ?? ''
// Archive links use full navigation so every request and media URL starts in
// the destination account, including when the link is opened in a new tab.
export function initializeAccountFromURL() {
  if (!/^\/groups\/\d+\/history$/.test(window.location.pathname)) return
  const id = new URLSearchParams(window.location.search).get('accountId')
  if (id && /^[A-Za-z0-9_-]{1,64}$/.test(id)) {
    pageAccount = id
    localStorage.setItem(key, id)
  }
}
export function activeAccount() { return pageAccount }
export function selectAccount(id: string) {
  if (id) localStorage.setItem(key, id)
  else localStorage.removeItem(key)
  window.location.assign('/')
}
const paths = new Set(['/api/status', '/api/config', '/api/groups', '/api/groups/watch', '/api/groups/history', '/api/groups/file-url', '/api/groups/file-download', '/api/groups/media', '/api/groups/voice', '/api/masters', '/api/masters/verify/request', '/api/masters/verify/confirm', '/api/rules', '/api/onebot', '/api/test-notify', '/api/lookup', '/api/logs', '/api/escalations', '/api/notices', '/api/events'])
export function scopedURL(path: string) {
  const id = activeAccount()
  if (!id || !paths.has(path.split('?')[0])) return path
  return `${path}${path.includes('?') ? '&' : '?'}accountId=${encodeURIComponent(id)}`
}

export function newAccount(): SourceAccount {
  return { id: crypto.randomUUID(), name: '新账号', enabled: true, onebot: { httpBase: '', wsUrl: '', token: '' }, groups: [], masters: [] }
}
