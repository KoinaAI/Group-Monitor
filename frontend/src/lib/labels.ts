import type { LogLevel, MinLevel, SenderLevel, SenderOverrideLevel } from './types'

// Domain vocabulary → operator-facing Chinese labels + a semantic color intent.
// `Intent` matches HeroUI's `color` prop union so a label map value can be
// passed straight to <Chip color={...}> etc. Mirrors backend/pipeline.go labels.
export type Intent =
  | 'default'
  | 'primary'
  | 'secondary'
  | 'success'
  | 'warning'
  | 'danger'

// Sender classification (pipeline.go: -1 muted … 3 vip).
export const senderLevelLabel: Record<SenderLevel, string> = {
  [-1]: '已屏蔽',
  0: '普通成员',
  1: '管理员',
  2: '群主',
  3: '重点人物',
}
export const senderLevelIntent: Record<SenderLevel, Intent> = {
  [-1]: 'default',
  0: 'default',
  1: 'primary',
  2: 'secondary',
  3: 'warning',
}

// Master notification gate.
export const minLevelLabel: Record<MinLevel, string> = {
  0: '全部有用',
  1: '普通及以上',
  2: '重要及以上',
  3: '仅紧急',
}

// Distilled-event urgency (pipeline.go levelLabel: 3 紧急 / 2 重要 / else 一般).
export const urgencyLabel = (level: number) =>
  level >= 3 ? '紧急' : level >= 2 ? '重要' : '一般'
export const urgencyIntent = (level: number): Intent =>
  level >= 3 ? 'danger' : level >= 2 ? 'warning' : 'default'

// QQ group role (onebot.go).
export const roleLabel: Record<'owner' | 'admin' | 'member', string> = {
  owner: '群主',
  admin: '管理员',
  member: '成员',
}
export const roleIntent: Record<'owner' | 'admin' | 'member', Intent> = {
  owner: 'warning',
  admin: 'primary',
  member: 'default',
}

// Log stream level (hub.go LogEntry).
export const logLevelLabel: Record<LogLevel, string> = {
  info: '信息',
  escalate: '升级',
  suppress: '抑制',
  urgent: '紧急',
  error: '错误',
}
export const logLevelIntent: Record<LogLevel, Intent> = {
  info: 'default',
  escalate: 'primary',
  suppress: 'secondary',
  urgent: 'danger',
  error: 'danger',
}

// Per-sender override (rules.senderOverrides).
export const overrideLevelLabel: Record<SenderOverrideLevel, string> = {
  vip: '重点',
  normal: '正常',
  muted: '屏蔽',
}
export const overrideLevelIntent: Record<SenderOverrideLevel, Intent> = {
  vip: 'warning',
  normal: 'default',
  muted: 'secondary',
}

// Master privilege kind (config.go Master.Kind; absent kind = full). "完整主人"
// receives escalations AND can log in; "仅通知" is a share-only push target with
// no bot authority.
export const masterKindLabel: Record<'full' | 'notify', string> = {
  full: '完整主人',
  notify: '仅通知',
}
export const masterKindIntent: Record<'full' | 'notify', Intent> = {
  full: 'primary',
  notify: 'secondary',
}
