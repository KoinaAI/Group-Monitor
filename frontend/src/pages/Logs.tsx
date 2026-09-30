import { useMemo, useState } from 'react'
import { Input, ScrollShadow, TextField } from '@heroui/react'
import { Segment, Widget } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/ui/States'
import { IntentChip } from '../components/ui/IntentChip'
import { useLive } from '../lib/store'
import { logLevelIntent, logLevelLabel } from '../lib/labels'
import { fmtDateTime } from '../lib/time'
import type { LogLevel } from '../lib/types'

const FILTERS: { key: LogLevel | 'all'; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'escalate', label: '升级' },
  { key: 'urgent', label: '紧急' },
  { key: 'suppress', label: '抑制' },
  { key: 'info', label: '信息' },
  { key: 'error', label: '错误' },
]

// Preserve server/store order: the newest event remains first after filtering.
export default function Logs() {
  const logs = useLive((s) => s.logs)
  const connected = useLive((s) => s.connected)
  const [filter, setFilter] = useState<LogLevel | 'all'>('all')
  const [query, setQuery] = useState('')
  const rows = useMemo(() => {
    const search = query.trim().toLowerCase()
    return logs.filter((entry) => (filter === 'all' || entry.level === filter)
      && (!search || entry.text.toLowerCase().includes(search) || entry.group?.toLowerCase().includes(search)))
  }, [logs, filter, query])

  return (
    <Page>
      <PageHeader title="运行日志" description="追踪消息处理过程，定位升级与投递问题。" actions={<IntentChip intent={connected ? 'success' : 'warning'}>{connected ? '实时同步' : '正在重连'}</IntentChip>} />
      <Widget>
          <Widget.Header className="flex-wrap gap-y-2 py-1.5">
          <div className="flex items-center gap-3">
            <Widget.Title>事件流</Widget.Title>
            <Widget.Description className="tabular-nums">共 {logs.length} 条 · 最新在前</Widget.Description>
          </div>
          <TextField aria-label="搜索日志" value={query} onChange={setQuery} className="w-full sm:w-64">
            <Input placeholder="搜索事件或群组" variant="secondary" />
          </TextField>
        </Widget.Header>
        <Widget.Content className="!p-0">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 px-4 py-2.5">
            <div className="max-w-full overflow-x-auto">
              <Segment aria-label="日志级别筛选" size="sm" selectedKey={filter} onSelectionChange={(key) => setFilter(key as LogLevel | 'all')}>
                {FILTERS.map((item) => <Segment.Item key={item.key} id={item.key}>{item.label}</Segment.Item>)}
              </Segment>
            </div>
            <span className="text-xs tabular-nums text-muted">匹配 {rows.length} 条</span>
          </div>
          {rows.length === 0 ? (
            <EmptyState icon="logs" title={logs.length ? '没有匹配的日志' : '暂无日志'} description={logs.length ? '尝试其他级别或搜索关键词。' : '等待管道事件…'} />
          ) : (
            <ScrollShadow className="max-h-[calc(100dvh-18rem)] min-h-48 overflow-y-auto px-4">
              <ul className="divide-y divide-border/60">
                {rows.map((entry, index) => (
                  <li key={`${entry.ts}-${index}`} className="grid gap-x-3 gap-y-1 py-2.5 sm:grid-cols-[8.5rem_3.5rem_minmax(0,1fr)]">
                    <span className="text-xs leading-5 tabular-nums text-muted">{fmtDateTime(entry.ts)}</span>
                    <div className="row-start-1 justify-self-end sm:col-start-2 sm:justify-self-start">
                      <IntentChip intent={logLevelIntent[entry.level]}>{logLevelLabel[entry.level]}</IntentChip>
                    </div>
                    <div className="min-w-0 sm:col-start-3 sm:row-start-1">
                      <p className="break-words text-sm leading-5">{entry.text}</p>
                      {entry.group ? <p className="truncate text-xs leading-5 text-muted">{entry.group}</p> : null}
                    </div>
                  </li>
                ))}
              </ul>
            </ScrollShadow>
          )}
        </Widget.Content>
      </Widget>
    </Page>
  )
}
