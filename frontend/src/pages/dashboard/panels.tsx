import { useEffect, useState } from 'react'
import { Button, ScrollShadow } from '@heroui/react'
import { Widget } from '@heroui-pro/react'
import { useNavigate } from 'react-router-dom'
import { IntentChip } from '../../components/ui/IntentChip'
import { AppIcon } from '../../lib/icons'
import { useLive } from '../../lib/store'
import { roleLabel, urgencyIntent, urgencyLabel } from '../../lib/labels'
import { fmtCountdown, msUntil, relFromMs, relFromSec } from '../../lib/time'

// Live tail of watched-group messages (newest first, capped in the store).
export function MessageFeed() {
  const messages = useLive((s) => s.messages)
  return (
    <Widget>
      <Widget.Header>
        <Widget.Title>实时消息</Widget.Title>
        <Widget.Description className="tabular-nums">最近 {messages.length} 条</Widget.Description>
      </Widget.Header>
      <Widget.Content className="!p-0">
        {messages.length === 0 ? (
          <div className="dashboard-empty"><AppIcon name="groups" className="size-5 shrink-0 text-muted" /><div><p>暂无消息</p><p className="mt-1 text-xs text-muted">监听中，等待群消息…</p></div></div>
        ) : (
          <ScrollShadow className="max-h-[26rem] overflow-y-auto px-4">
            <ul className="divide-y divide-border/60">
              {messages.map((m) => (
                <li key={`${m.messageId}-${m.time}`} className="py-2.5 first:pt-1">
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 truncate text-sm font-medium">{m.nickname || m.userId}</span>
                    {m.role !== 'member' ? (
                      <IntentChip intent={m.role === 'owner' ? 'warning' : 'primary'}>{roleLabel[m.role]}</IntentChip>
                    ) : null}
                    <span className="hidden min-w-0 truncate text-xs text-muted sm:inline">{m.groupName || m.groupId}</span>
                    <span className="ml-auto shrink-0 text-xs tabular-nums text-muted">{relFromSec(m.time)}</span>
                  </div>
                  <p className="mt-1 line-clamp-2 break-words text-sm leading-relaxed text-foreground/80">
                    {m.text || (m.hasImage ? '[图片]' : '')}
                  </p>
                  <div className="mt-1 flex items-center gap-2 text-xs text-muted">
                    <span className="truncate sm:hidden">{m.groupName || m.groupId}</span>
                    {m.atAll ? <span className="shrink-0 text-warning">@全体</span> : null}
                    {m.atSelf ? <span className="shrink-0 text-accent">@我</span> : null}
                    {m.hasImage ? <AppIcon name="image" className="size-3.5 shrink-0" /> : null}
                  </div>
                </li>
              ))}
            </ul>
          </ScrollShadow>
        )}
      </Widget.Content>
    </Widget>
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
    <Widget>
      <Widget.Header>
        <Widget.Title>待发送窗口</Widget.Title>
        <Widget.Description className="tabular-nums">{buffers.length} 个群组处理中</Widget.Description>
      </Widget.Header>
      <Widget.Content>
        {buffers.length === 0 ? (
          <div className="flex items-center gap-3 py-1 text-sm text-muted">
            <AppIcon name="clock" className="size-4 shrink-0" />
            <p>暂无待处理窗口</p>
          </div>
        ) : (
          <ScrollShadow className="max-h-64 overflow-y-auto">
            <ul className="divide-y divide-border/60">
              {buffers.map((b) => {
                const left = msUntil(b.flushAt, now)
                return (
                  <li key={b.groupId} className="flex items-center justify-between gap-3 py-2.5 first:pt-0 last:pb-0">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{b.groupName}</p>
                      <p className="mt-0.5 truncate text-xs text-muted">{b.count} 条消息 · {b.topLabel}</p>
                    </div>
                    <IntentChip intent="primary">
                      <span className="tabular-nums">{left > 0 ? fmtCountdown(left) : '即将发送'}</span>
                    </IntentChip>
                  </li>
                )
              })}
            </ul>
          </ScrollShadow>
        )}
      </Widget.Content>
    </Widget>
  )
}

// Recent escalations that were distilled and pushed to masters.
export function EscalationPanel() {
  const escalations = useLive((s) => s.escalations)
  const navigate = useNavigate()
  return (
    <Widget>
      <Widget.Header>
        <div className="flex items-center gap-2">
          <Widget.Title>最新升级</Widget.Title>
          <Widget.Description className="tabular-nums">{escalations.length} 条</Widget.Description>
        </div>
        <Button size="sm" variant="ghost" onPress={() => navigate('/notices')}>通知归档</Button>
      </Widget.Header>
      <Widget.Content className="!p-0">
        {escalations.length === 0 ? (
          <div className="dashboard-empty"><AppIcon name="bell" className="size-5 shrink-0 text-accent" /><div><p>暂无升级事件</p><p className="mt-1 text-xs text-muted">重要消息通过判断后，将在这里汇总。</p></div></div>
        ) : (
          <ScrollShadow className="max-h-[38rem] overflow-y-auto px-4">
            <ul className="divide-y divide-border/60">
              {escalations.slice(0, 8).map((e, index) => (
                <li key={`${e.ts}-${e.groupId}-${index}`} className="py-3 first:pt-1">
                  <div className="flex items-start gap-3">
                    <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-surface-secondary text-muted">
                      <AppIcon name={e.urgent ? 'urgent' : 'bell'} className={`size-4 ${e.urgent ? 'text-warning' : ''}`} />
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="min-w-0 break-words text-sm font-semibold">{e.title || e.event || '升级事件'}</span>
                        <IntentChip intent={urgencyIntent(e.level)}>{urgencyLabel(e.level)}</IntentChip>
                      </div>
                      {e.summary ? <p className="mt-1.5 line-clamp-2 text-[13px] leading-relaxed text-muted">{e.summary}</p> : null}
                      {(e.time || e.deadline) ? <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-foreground/80">
                        {e.time ? <span className="flex items-center gap-1.5"><AppIcon name="clock" className="size-3 text-muted" />{e.time}</span> : null}
                        {e.deadline ? <span>截止：{e.deadline}</span> : null}
                      </div> : null}
                      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
                        <span className="min-w-0 truncate">{e.group}</span>
                        <span className="tabular-nums">{relFromMs(e.ts)}</span>
                        <span className={e.notified ? 'text-success' : 'text-warning'}>{e.notified ? '已送达' : '未送达'}</span>
                      </div>
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </ScrollShadow>
        )}
      </Widget.Content>
    </Widget>
  )
}
