import { scopedURL } from './accounts'
import type {
  AgentKey,
  NotificationTarget,
  SourceConfig,
  SourceStatus,
  AuthStatus,
  BackupConfig,
  BackupStatus,
  Config,
  DocumentConfig,
  EscalationEvent,
  GroupRow,
  GroupWatch,
  HistoryMsg,
  JevConfig,
  JevTestResponse,
  LLMConfig,
  LLMTestResponse,
  LogEntry,
  Master,
  MasterKind,
  NoticeRecord,
  OneBotConfig,
  Rules,
  StatusResponse,
  StrangerInfo,
  TestNotifyResponse,
} from './types'

// Every call is same-origin (`/api/...`): in dev via the Vite proxy, in prod via
// a reverse proxy in front of the static host (see frontend-heroui-stack memory).
// `credentials: 'include'` carries the HttpOnly `nap_session` cookie the Go
// backend issues; the SPA never handles the cookie itself.

export class ApiError extends Error {
  status: number
  body: unknown
  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

// A 401 on any protected call means the session lapsed (backend restart, TTL).
// App registers a handler that returns the user to /login.
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: (() => void) | null) {
  onUnauthorized = fn
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(scopedURL(`/api${path}`), {
    credentials: 'include',
    ...init,
    headers: {
      Accept: 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  const ct = res.headers.get('content-type') ?? ''
  const body: unknown = ct.includes('application/json')
    ? await res.json()
    : await res.text()
  if (res.status === 401) {
    onUnauthorized?.()
    throw new ApiError(401, '未登录或会话已过期', body)
  }
  if (!res.ok) {
    const msg =
      body && typeof body === 'object' && 'error' in body
        ? String((body as { error: unknown }).error)
        : `请求失败 (${res.status})`
    throw new ApiError(res.status, msg, body)
  }
  return body as T
}

const get = <T>(path: string) => request<T>(path)
const post = <T>(path: string, data?: unknown) =>
  request<T>(path, {
    method: 'POST',
    body: data === undefined ? undefined : JSON.stringify(data),
  })

// Grouped by resource. Config sections (rules/llm/jev/onebot) are POST-only on
// the backend — there is no per-section GET, so pages load their initial values
// from `api.config()` (the full Config) and save through these section calls.
export const api = {
  agentKeys: {
    list: () => get<AgentKey[]>('/agent-keys'),
    create: (name: string, accountIds: string[]) => post<{ apiKey: string; key: AgentKey; keys: AgentKey[] }>('/agent-keys', { name, accountIds }),
    revoke: (id: string) => post<AgentKey[]>('/agent-keys/revoke', { id }),
  },
  notifications: {
    list: () => get<NotificationTarget[]>('/notifications'),
    save: (targets: NotificationTarget[]) => post<NotificationTarget[]>('/notifications', { targets }),
    test: (id: string) => post<{ ok: boolean }>('/notifications/test', { id }),
  },
  setup: {
    status: () => get<{ required: boolean; tokenRequired: boolean }>('/setup/status'),
    complete: (body: { setupToken: string; password: string; llm?: LLMConfig; sources?: SourceConfig[] }) => post<{ ok: boolean }>('/setup', body),
  },
  sources: {
    list: () => get<SourceConfig[]>('/sources'),
    save: (sources: SourceConfig[]) => post<SourceConfig[]>('/sources', { sources }),
    status: () => get<SourceStatus[]>('/sources/status'),
  },
  auth: {
    status: () => get<AuthStatus>('/auth/status'),
    otpRequest: () =>
      post<{ sent: number; failed: string[]; ttlSec: number }>(
        '/auth/otp/request',
      ),
    otpVerify: (code: string) =>
      post<{ ok: boolean; reason?: string }>('/auth/otp/verify', { code }),
    password: (password: string) =>
      post<{ ok: boolean; reason?: string }>('/auth/password', { password }),
    logout: () => post<{ ok: boolean }>('/auth/logout'),
  },

  status: () => get<StatusResponse>('/status'),
  config: () => get<Config>('/config'),

  groups: {
    list: () => get<GroupRow[]>('/groups'),
    saveWatch: (groups: GroupWatch[]) =>
      post<GroupWatch[]>('/groups/watch', { groups }),
    history: (groupId: number, count = 30, beforeSeq?: number) =>
      get<HistoryMsg[]>(
        `/groups/history?groupId=${groupId}&count=${count}` +
          (beforeSeq ? `&beforeSeq=${beforeSeq}` : ''),
      ),
    fileUrl: (groupId: number, fileId: string, busid?: number) =>
      get<{ url: string }>(
        `/groups/file-url?groupId=${groupId}&fileId=${encodeURIComponent(fileId)}` +
          (busid ? `&busid=${busid}` : ''),
      ),
    // Same-origin download link: the backend resolves the short-lived NapCat URL
    // and re-serves it with Content-Disposition, so an <a href> saves the file
    // under its real name instead of a bare "下载". Not a fetch — a URL to visit.
    fileDownloadUrl: (groupId: number, fileId: string, name: string, busid?: number) =>
      scopedURL(`/api/groups/file-download?groupId=${groupId}&fileId=${encodeURIComponent(fileId)}` +
      `&name=${encodeURIComponent(name)}` +
      (busid ? `&busid=${busid}` : '')),
    // Same-origin proxy for an inline history image/voice/video. A raw QQ CDN URL
    // handed to <img>/<audio> is blocked by Chrome (net::ERR_BLOCKED_BY_ORB) and
    // renders broken; routing it through /api makes it same-origin and served with
    // a clean media content-type. Not a fetch — a URL to put in a src.
    mediaUrl: (url: string) => scopedURL(`/api/groups/media?u=${encodeURIComponent(url)}`),
    // Same-origin, transcoded voice: QQ voice is AMR/SILK (no browser decodes it)
    // and the CDN mislabels it audio/mp3, so an <audio> pointed at the raw URL just
    // fails to load. The backend runs it through NapCat get_record (ffmpeg) and
    // serves real mp3. Not a fetch — a URL to put in <audio src>.
    voiceUrl: (file: string) => scopedURL(`/api/groups/voice?file=${encodeURIComponent(file)}`),
  },

  masters: {
    list: () => get<Master[]>('/masters'),
    save: (masters: Master[]) => post<Master[]>('/masters', { masters }),
    // Task 3: privileged bind requires the candidate QQ to read back a 3-min
    // one-time code we DM them. `verifyRequest` sends it (returning the resolved
    // nickname so the operator sees who was contacted); `verifyConfirm` submits
    // the code and, on success, binds and echoes the fresh master set.
    verifyRequest: (userId: number) =>
      post<{ nickname: string; ttlSec: number }>('/masters/verify/request', {
        userId,
      }),
    verifyConfirm: (userId: number, code: string, minLevel: number, kind: MasterKind = 'full') =>
      post<{ ok: boolean; reason?: string; masters?: Master[] }>(
        '/masters/verify/confirm',
        { userId, code, minLevel, kind },
      ),
  },

  rules: { save: (rules: Rules) => post<Rules>('/rules', rules) },

  llm: {
    save: (llm: LLMConfig) => post<LLMConfig>('/llm', llm),
    test: (llm: LLMConfig) => post<LLMTestResponse>('/llm/test', llm),
  },

  jev: {
    save: (jev: JevConfig) => post<JevConfig>('/jev', jev),
    test: (jev: JevConfig) => post<JevTestResponse>('/jev/test', jev),
  },

  backup: {
    save: (backup: BackupConfig) => post<BackupConfig>('/backup', backup),
    run: () => post<{ ok: boolean }>('/backup/run'),
    status: () => get<BackupStatus>('/backup/status'),
  },

  documents: {
    save: (documents: DocumentConfig) => post<DocumentConfig>('/documents', documents),
  },

  notices: (options: { groupId?: number; groupIds?: number[]; q?: string; limit?: number; before?: number; beforeId?: string } = {}) => {
    const params = new URLSearchParams()
    if (options.groupIds?.length) {
      options.groupIds.forEach((id) => params.append('groupId', String(id)))
    } else if (options.groupId) {
      params.set('groupId', String(options.groupId))
    }
    if (options.q) params.set('q', options.q)
    if (options.limit) params.set('limit', String(options.limit))
    if (options.before) params.set('before', String(options.before))
    if (options.beforeId) params.set('beforeId', options.beforeId)
    return get<NoticeRecord[]>(`/notices?${params}`)
  },

  onebot: { save: (onebot: OneBotConfig) => post<OneBotConfig>('/onebot', onebot) },

  setEnabled: (enabled: boolean) =>
    post<{ enabled: boolean }>('/enabled', { enabled }),
  testNotify: () => post<TestNotifyResponse>('/test-notify'),
  lookup: (userId: number) => get<StrangerInfo>(`/lookup?userId=${userId}`),
  logs: () => get<LogEntry[]>('/logs'),
  escalations: () => get<EscalationEvent[]>('/escalations'),
}
