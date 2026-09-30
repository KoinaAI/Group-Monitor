// Mirrors the Go backend's JSON contracts (backend/config.go, api.go, hub.go,
// pipeline.go, onebot.go, llm.go). Field names match the Go `json:"..."` tags
// exactly — keep in sync if the backend contract changes.

// ---- Sender / master levels (backend/pipeline.go) ----
// Sender classification: -1 muted, 0 normal, 1 admin, 2 owner, 3 vip.
export type SenderLevel = -1 | 0 | 1 | 2 | 3
// Master minLevel gate: 0 all useful, 1 normal+, 2 important+, 3 urgent only.
export type MinLevel = 0 | 1 | 2 | 3

// ---- Config (backend/config.go) ----
export interface OneBotConfig {
  httpBase: string
  wsUrl: string
  token: string
}

export interface LLMConfig {
  enabled: boolean
  baseUrl: string
  apiKey: string
  model: string
  timeoutSec: number
  maxTokens: number
  temperature: number
}

export interface JevConfig {
  enabled: boolean
  baseUrl: string
  apiKey: string
  model: string
  threshold: number
  contextN: number
  timeoutSec: number
}

export interface BackupConfig {
  enabled: boolean
  provider: 'r2' | 's3'
  cron: string
  endpoint: string
  bucket: string
  prefix: string
  region: string
  accessKey: string
  secretKey: string
  timeoutSec: number
}

export interface BackupStatus {
  running: boolean
  lastRun: number
  lastSuccess: number
  lastError: string
  filesUploaded: number
  nextRun?: number
}

export interface DocumentConfig {
  enabled: boolean
  baseUrl: string
  apiKey: string
  timeoutSec: number
  maxFileMB: number
  maxTextChars: number
}

// MasterKind gates privilege: 'full' (default) receives escalations AND is a
// login-OTP recipient; 'notify' is notify-only — a sharing target that still
// gets pushes but has no login path. Absent kind means full.
export type MasterKind = 'full' | 'notify'
export interface Master {
  userId: number
  nickname: string
  minLevel: MinLevel
  kind?: MasterKind
}

export interface GroupWatch {
  groupId: number
  groupName: string
  watch: boolean
}

export type SenderOverrideLevel = 'vip' | 'normal' | 'muted'
export interface SenderOverride {
  userId: number
  note: string
  level: SenderOverrideLevel
}

export interface Rules {
  quietWindowSec: number
  maxHoldSec: number
  urgentKeywords: string[]
  atAllUrgent: boolean
  elevateOwnerAdmin: boolean
  senderOverrides: SenderOverride[]
}

export interface Config {
  security: { initialized: boolean }
  sources: SourceConfig[]
  onebot: OneBotConfig
  llm: LLMConfig
  jev: JevConfig
  backup: BackupConfig
  documents: DocumentConfig
  masters: Master[]
  groups: GroupWatch[]
  rules: Rules
  enabled: boolean
}

// ---- Auth (backend/auth.go) — GET /api/auth/status is PUBLIC ----
export interface AuthStatus {
  setupRequired?: boolean
  authed: boolean
  otpAvailable: boolean
  passwordConfigured: boolean
  passwordAvailable: boolean
  onebotConnected: boolean
  masters: number
}

// ---- Status (backend/api.go) — GET /api/status (protected) ----
export interface StatusResponse {
  onebotConnected: boolean
  selfId: number
  account: { user_id: number; nickname: string }
  enabled: boolean
  llmEnabled: boolean
  jevEnabled: boolean
  watchedGroups: number
  totalGroups: number
  masters: number
  quietWindowSec: number
  serverTime: number
}

// ---- Groups (backend/api.go, onebot.go) ----
export interface GroupRow {
  groupId: number
  groupName: string
  groupRemark: string
  memberCount: number
  watch: boolean
}

// GET /api/groups/history entry (backend/onebot.go HistoryMsg).
export interface HistoryFile {
  name: string
  fileId: string
  size: number
  busid: number
  url?: string // present only if the raw segment already carried a link
}

// A quoted original, resolved server-side from a reply segment.
export interface ReplyQuote {
  userId: number
  nickname: string
  text: string
}
// One structured piece of a history message (backend/onebot.go MsgSegment).
// The renderer walks these so @-mentions show names, images/voice/video render
// inline, and replies show the quoted original — no [图片]/[语音]/[引用] text.
export interface MsgSegment {
  type: 'text' | 'at' | 'image' | 'face' | 'record' | 'video' | 'reply' | 'card' | 'forward'
  text?: string // text body; face/card/forward label
  name?: string // at: resolved display name
  id?: number // at: target QQ (0 = @全体成员)
  url?: string // image/record/video source
  file?: string // record: NapCat voice file id, for server-side mp3 transcode
  sticker?: boolean // image: a sticker/大表情 (render small, natural size)
  reply?: ReplyQuote // reply: the quoted original
}
export interface HistoryMsg {
  messageId: number
  messageSeq: number // pagination cursor for older batches
  userId: number
  nickname: string // card if present, else nickname
  role: 'owner' | 'admin' | 'member'
  title: string // custom group title (头衔), if any
  time: number // unix seconds
  text: string
  hasImage: boolean
  files?: HistoryFile[]
  segments?: MsgSegment[] // structured render; falls back to text when absent
  isSelf: boolean
}

// ---- Test endpoints (backend/api.go, llm.go) ----
export interface LLMResult {
  useful: boolean
  level: number
  title: string
  summary: string
  time: string
  place: string
  event: string
  deadline: string
  reason: string
}

export interface NoticeRecord {
  sourceId?: string
  accountId?: string
  id: string
  createdAt: number
  groupId: number
  group: string
  messageIds?: number[]
  sources?: { messageId?: number; time: number; userId: number; nickname?: string }[]
  result: LLMResult
  urgent: boolean
}
export interface LLMTestResponse {
  ok: boolean
  result?: LLMResult
  raw?: string
  preview?: string
  error?: string
}
export interface JevSample {
  label: string
  text: string
  noul: number
  important: boolean
}
export interface JevTestResponse {
  ok: boolean
  threshold?: number
  samples?: JevSample[]
  error?: string
}
// POST /api/test-notify — 400 when no masters. `failed` = "id: error" strings.
export interface TestNotifyResponse {
  sent: number
  failed: string[]
}

// GET /api/lookup?userId= → NapCat stranger info (backend/onebot.go StrangerInfo).
export interface StrangerInfo {
  user_id: number
  nickname: string
  [k: string]: unknown
}

// ---- SSE stream (backend/hub.go, pipeline.go) ----
export type EventType =
  | 'hello'
  | 'status'
  | 'message'
  | 'buffer'
  | 'escalation'
  | 'log'
export interface Event<T = unknown> {
  type: EventType
  data: T
  ts: number
}

// SSE "hello" / "status" payload — a SUBSET of StatusResponse (backend statusPayload).
export interface StatusPayload {
  onebotConnected: boolean
  selfId: number
  enabled: boolean
  llmEnabled: boolean
  jevEnabled: boolean
  watchedGroups: number
  masters: number
}

// SSE "message" — every watched-group message, for the live feed
// (backend/onebot.go GroupMessage). Fired per-message by the pipeline.
export interface GroupMessage {
  time: number // unix seconds
  groupId: number
  groupName: string
  userId: number
  nickname: string // card if present, else nickname
  role: 'owner' | 'admin' | 'member'
  text: string
  atAll: boolean
  atSelf: boolean
  hasImage: boolean
  messageId: number
}

// SSE "buffer" — pending quiet-window buffers (array; empty when none).
export interface BufView {
  groupId: number
  groupName: string
  count: number
  flushAt: number // ms epoch
  windowMs: number
  topLabel: string
}

// SSE "escalation".
export interface EscalationEvent {
  groupId: number
  group: string
  level: number
  title: string
  summary: string
  time: string
  place: string
  event: string
  deadline: string
  notified: boolean
  urgent: boolean
  ts: number // ms epoch
}

// SSE "log" / GET /api/logs entry (backend/hub.go LogEntry).
export type LogLevel = 'info' | 'escalate' | 'suppress' | 'urgent' | 'error'
export interface LogEntry {
  ts: number // ms epoch
  level: LogLevel
  groupId?: number
  group?: string
  text: string
}

export interface SourceAccount {
  id: string
  name: string
  enabled: boolean
  onebot: OneBotConfig
  groups: GroupWatch[]
  masters: Master[]
  rules?: Rules
}
export interface SourceConfig {
  id: string
  name: string
  kind: 'napcat'
  accounts: SourceAccount[]
}
export interface SourceStatus { sourceId: string; accountId: string; connected: boolean; selfId: number }

export interface NotificationTarget {
  id: string; name: string; kind: 'ntfy' | 'bark'; enabled: boolean; url: string
  topic?: string; token?: string; deviceKey?: string; group?: string
  minLevel: number; accountIds?: string[]
}

export interface AgentKey { id: string; name: string; accountIds: string[]; createdAt: number }
