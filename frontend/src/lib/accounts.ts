import type { SourceAccount } from './types'
const key = 'xunshu-account'
export function activeAccount() { return localStorage.getItem(key) ?? '' }
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
