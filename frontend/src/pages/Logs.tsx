import { useMemo, useState } from 'react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { EmptyState } from '../components/ui/States'
import { IntentChip } from '../components/ui/IntentChip'
import { useLive } from '../lib/store'
import { logLevelIntent, logLevelLabel } from '../lib/labels'
import { fmtDateTime } from '../lib/time'
import { cn } from '../lib/cn'
import type { LogLevel } from '../lib/types'

const FILTERS: { key: LogLevel | 'all'; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'escalate', label: '升级' },
  { key: 'urgent', label: '紧急' },
  { key: 'suppress', label: '抑制' },
  { key: 'info', label: '信息' },
  { key: 'error', label: '错误' },
]

// Live log stream (seeded by AppLayout via GET /api/logs, then appended from the
// SSE "log" events). Client-side level filter over the store-capped buffer.
export default function Logs() {
  const logs = useLive((s) => s.logs)
  const [filter, setFilter] = useState<LogLevel | 'all'>('all')
  const rows = useMemo(
    () => (filter === 'all' ? logs : logs.filter((l) => l.level === filter)),
    [logs, filter],
  )
  return (
    <Page>
      <PageHeader title="运行日志" description="管道处理与升级决策的实时流水" />
      <SectionCard
        title="事件流"
        description={`共 ${logs.length} 条`}
        contentClassName="max-h-[36rem] overflow-y-auto"
        actions={
          <div className="flex flex-wrap justify-end gap-1">
            {FILTERS.map((f) => (
              <button
                key={f.key}
                type="button"
                onClick={() => setFilter(f.key)}
                className={cn(
                  'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  filter === f.key
                    ? 'bg-accent-soft text-accent'
                    : 'text-muted hover:text-foreground',
                )}
              >
                {f.label}
              </button>
            ))}
          </div>
        }
      >
        {rows.length === 0 ? (
          <EmptyState icon="logs" title="暂无日志" description="等待管道事件…" />
        ) : (
          <ul className="divide-y divide-border">
            {rows.map((l, i) => (
              <li key={`${l.ts}-${i}`} className="flex items-start gap-3 py-2.5 first:pt-0 last:pb-0">
                <IntentChip intent={logLevelIntent[l.level]}>{logLevelLabel[l.level]}</IntentChip>
                <div className="min-w-0 flex-1">
                  <p className="text-sm text-foreground">{l.text}</p>
                  <div className="mt-0.5 flex items-center gap-2 text-xs text-muted">
                    <span className="tabular-nums">{fmtDateTime(l.ts)}</span>
                    {l.group ? (
                      <>
                        <span>·</span>
                        <span className="truncate">{l.group}</span>
                      </>
                    ) : null}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </SectionCard>
    </Page>
  )
}
