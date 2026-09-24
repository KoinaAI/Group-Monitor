import { useEffect, useState } from 'react'
import { SectionCard } from '../../components/ui/SectionCard'
import { EmptyState } from '../../components/ui/States'
import { IntentChip } from '../../components/ui/IntentChip'
import { AppIcon } from '../../lib/icons'
import { useLive } from '../../lib/store'
import { roleLabel, urgencyIntent, urgencyLabel } from '../../lib/labels'
import { fmtCountdown, msUntil, relFromMs, relFromSec } from '../../lib/time'

// Live tail of watched-group messages (newest first, capped in the store).
export function MessageFeed() {
  const messages = useLive((s) => s.messages)
  return (
    <SectionCard
      title="实时消息"
      description="监听群组的最新消息"
      contentClassName="max-h-[30rem] overflow-y-auto"
    >
      {messages.length === 0 ? (
        <EmptyState icon="groups" title="暂无消息" description="监听中，等待群消息…" />
      ) : (
        <ul className="divide-y divide-border">
          {messages.map((m) => (
            <li
              key={`${m.messageId}-${m.time}`}
              className="flex items-start gap-3 py-3 first:pt-0 last:pb-0"
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">
                    {m.nickname || m.userId}
                  </span>
                  {m.role !== 'member' ? (
                    <IntentChip intent={m.role === 'owner' ? 'warning' : 'primary'}>
                      {roleLabel[m.role]}
                    </IntentChip>
                  ) : null}
                  <span className="truncate text-xs text-muted">{m.groupName}</span>
                </div>
                <p className="mt-0.5 line-clamp-2 text-sm text-muted">
                  {m.text || (m.hasImage ? '[图片]' : '')}
                </p>
                <div className="mt-1 flex items-center gap-2 text-xs text-muted">
                  <span>{relFromSec(m.time)}</span>
                  {m.atAll ? <span className="text-warning">@全体</span> : null}
                  {m.atSelf ? <span className="text-accent">@我</span> : null}
                  {m.hasImage ? <AppIcon name="image" className="size-3.5" /> : null}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </SectionCard>
  )
}

// Groups currently aggregating inside a quiet window, with a live flush countdown.
export function BufferPanel() {
  const buffers = useLive((s) => s.buffers)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  return (
    <SectionCard title="待发送窗口" description="静默窗口聚合中的群组">
      {buffers.length === 0 ? (
        <EmptyState icon="clock" title="暂无待处理窗口" />
      ) : (
        <ul className="flex flex-col gap-3">
          {buffers.map((b) => {
            const left = msUntil(b.flushAt, now)
            return (
              <li key={b.groupId} className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">
                    {b.groupName}
                  </p>
                  <p className="truncate text-xs text-muted">
                    {b.count} 条 · {b.topLabel}
                  </p>
                </div>
                <span className="shrink-0 rounded-md bg-accent-soft px-2 py-1 text-xs font-medium tabular-nums text-accent">
                  {left > 0 ? fmtCountdown(left) : '即将发送'}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </SectionCard>
  )
}

// Recent escalations that were distilled and pushed to masters.
export function EscalationPanel() {
  const escalations = useLive((s) => s.escalations)
  return (
    <SectionCard title="最新升级" description="已推送给主人的通知">
      {escalations.length === 0 ? (
        <EmptyState icon="bell" title="暂无升级事件" />
      ) : (
        <ul className="flex flex-col gap-3">
          {escalations.slice(0, 6).map((e) => (
            <li key={e.ts} className="rounded-lg border border-border p-3">
              <div className="flex items-center justify-between gap-2">
                <span className="truncate text-sm font-medium text-foreground">
                  {e.title || e.event || '升级事件'}
                </span>
                <IntentChip intent={urgencyIntent(e.level)}>{urgencyLabel(e.level)}</IntentChip>
              </div>
              {e.summary ? (
                <p className="mt-1 line-clamp-2 text-xs text-muted">{e.summary}</p>
              ) : null}
              <div className="mt-1.5 flex items-center gap-2 text-xs text-muted">
                <span className="truncate">{e.group}</span>
                <span>·</span>
                <span>{relFromMs(e.ts)}</span>
                {e.notified ? null : <span className="text-warning">未送达</span>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </SectionCard>
  )
}
