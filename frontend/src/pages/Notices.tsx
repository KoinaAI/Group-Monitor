import { useEffect, useRef, useState } from 'react'
import { Button, Input, ListBox, TextField } from '@heroui/react'
import { ListView, Widget } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { IntentChip } from '../components/ui/IntentChip'
import { EmptyState, InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api, ApiError } from '../lib/api'
import { urgencyIntent, urgencyLabel } from '../lib/labels'
import { fmtDateTime } from '../lib/time'
import type { GroupRow, NoticeRecord, SourceConfig } from '../lib/types'

const PAGE_SIZE = 30

export default function Notices() {
  const [query, setQuery] = useState('')
  const [groupQuery, setGroupQuery] = useState('')
  const [groupIds, setGroupIds] = useState<Set<string>>(new Set())
  const [groups, setGroups] = useState<GroupRow[]>([])
  const [groupSearchOpen, setGroupSearchOpen] = useState(false)
  const [rows, setRows] = useState<NoticeRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
  const [sources, setSources] = useState<SourceConfig[]>([])
  const [selectedId, setSelectedId] = useState<string>()
  const requestId = useRef(0)
  const applied = useRef<{ q: string; groupIds: number[] }>({ q: '', groupIds: [] })
  const selectedGroups = groups.filter((row) => groupIds.has(String(row.groupId)))
  const groupSuggestions = groups.filter((row) => {
    const needle = groupQuery.trim().toLowerCase()
    return !needle || [row.groupName, row.groupRemark, String(row.groupId)].some((value) => value?.toLowerCase().includes(needle))
  })

  const load = async (append = false) => {
    const id = ++requestId.current
    const filters = append ? applied.current : { q: query.trim(), groupIds: [...groupIds].map(Number).filter((value) => Number.isSafeInteger(value) && value > 0) }
    setLoading(true)
    setError('')
    try {
      const result = await api.notices({
        ...filters, limit: PAGE_SIZE,
        before: append ? rows.at(-1)?.createdAt : undefined,
        beforeId: append ? rows.at(-1)?.id : undefined,
      })
      if (id !== requestId.current) return
      if (!append) applied.current = filters
      setRows((previous) => append ? [...previous, ...result.filter((row) => !previous.some((item) => item.id === row.id))] : result)
      setHasMore(result.length === PAGE_SIZE)
    } catch (e) {
      if (id === requestId.current) setError(e instanceof ApiError ? e.message : '通知加载失败，请重试')
    } finally {
      if (id === requestId.current) setLoading(false)
    }
  }

  useEffect(() => {
    let canceled = false
    api.sources.list().then((value) => { if (!canceled) setSources(value) }).catch(() => {})
    api.groups.list().then((value) => { if (!canceled) setGroups(value) }).catch(() => {})
    const id = ++requestId.current
    api.notices({ limit: PAGE_SIZE }).then((result) => {
      if (canceled || id !== requestId.current) return
      setRows(result)
      setHasMore(result.length === PAGE_SIZE)
    }).catch((e) => {
      if (!canceled && id === requestId.current) setError(e instanceof ApiError ? e.message : '通知加载失败，请重试')
    }).finally(() => {
      if (!canceled && id === requestId.current) setLoading(false)
    })
    return () => { canceled = true }
  }, [])

  const selected = rows.find((notice) => notice.id === selectedId) || rows[0]

  return (
    <Page>
      <PageHeader title="通知归档" description="集中检索通知，查看时间、地点和待办事项。" actions={<span className="pt-1 text-xs tabular-nums text-muted">已加载 {rows.length} 条</span>} />
      <Widget>
        <Widget.Content>
          <form aria-label="归档检索" onSubmit={(event) => { event.preventDefault(); void load() }} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(12rem,18rem)_auto]">
            <TextField aria-label="关键词" value={query} onChange={setQuery}><Input placeholder="搜索标题、摘要或事项" variant="secondary" /></TextField>
            <div className="relative col-span-2 row-start-2 min-w-0 sm:col-span-1 sm:row-start-auto">
              <TextField
                aria-label="筛选群组"
                value={groupQuery}
                onChange={(value) => { setGroupQuery(value); setGroupSearchOpen(true) }}
                onFocus={() => setGroupSearchOpen(true)}
                onBlur={() => window.setTimeout(() => setGroupSearchOpen(false), 120)}
              >
                <Input placeholder="按群名筛选，可多选" variant="secondary" />
              </TextField>
              {groupSearchOpen ? <div className="workspace-popover absolute inset-x-0 top-full z-30 mt-1 overflow-hidden rounded-lg border border-border">
                <ListBox aria-label="群组筛选结果" className="max-h-64 overflow-y-auto py-1">
                  {groupSuggestions.length === 0 ? <ListBox.Item id="empty" isDisabled textValue="没有匹配的群组">没有匹配的群组</ListBox.Item> : groupSuggestions.map((row) => {
                    const selected = groupIds.has(String(row.groupId))
                    return <ListBox.Item
                      key={row.groupId}
                      id={String(row.groupId)}
                      textValue={`${row.groupName} ${row.groupRemark} ${row.groupId}`}
                      aria-selected={selected}
                      onPointerDown={(event) => event.preventDefault()}
                      onAction={() => {
                        setGroupIds((current) => {
                          const next = new Set(current)
                          if (selected) next.delete(String(row.groupId))
                          else next.add(String(row.groupId))
                          return next
                        })
                        setGroupQuery('')
                      }}
                    >
                      <span className="min-w-0 flex-1">
                        <span className="block truncate">{row.groupName || `群 ${row.groupId}`}</span>
                        <span className="block text-xs text-muted tabular-nums">群号 {row.groupId}{row.groupRemark ? ` · ${row.groupRemark}` : ''}</span>
                      </span>
                      {selected ? <span className="text-accent" aria-hidden>✓</span> : null}
                    </ListBox.Item>
                  })}
                </ListBox>
              </div> : null}
            </div>
            <Button className="col-start-2 row-start-1 sm:col-start-3" type="submit" isPending={loading}>搜索</Button>
          </form>
          {selectedGroups.length ? <div className="mt-3 flex flex-wrap items-center gap-1.5" aria-label="已选群组">
            <span className="mr-1 text-xs text-muted">已选群组</span>
            {selectedGroups.map((row) => <span key={row.groupId} className="inline-flex max-w-full items-center gap-1 rounded-md bg-surface-secondary px-2 py-1 text-xs">
              <span className="truncate">{row.groupName || `群 ${row.groupId}`} · {row.groupId}</span>
              <Button size="sm" variant="ghost" isIconOnly className="size-5 min-w-5 p-0 text-muted" aria-label={`移除 ${row.groupName || row.groupId}`} onPress={() => setGroupIds((current) => {
                const next = new Set(current)
                next.delete(String(row.groupId))
                return next
              })}><span aria-hidden>×</span></Button>
            </span>)}
            <Button size="sm" variant="ghost" onPress={() => setGroupIds(new Set())}>清除选择</Button>
          </div> : null}
        </Widget.Content>
      </Widget>
      {error ? <InlineError message={error} onRetry={() => { void load() }} /> : null}
      {loading && rows.length === 0 ? <Loader label="正在加载通知…" /> : !error && rows.length === 0 ? (
        <EmptyState icon="fileText" title="暂无通知" description="正式通知通过判断后会在这里保留；也可以换个关键词搜索。" />
      ) : (
        <div className="mt-4 grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(18rem,0.9fr)_minmax(0,1.4fr)]">
          <Widget>
            <Widget.Header>
              <Widget.Title>通知列表</Widget.Title>
              <Widget.Description>最近通知在前</Widget.Description>
            </Widget.Header>
            <Widget.Content className="!p-0">
              <ListView
                aria-label="通知归档列表"
                variant="secondary"
                selectionMode="single"
                selectionBehavior="replace"
                disallowEmptySelection
                selectedKeys={selected ? [selected.id] : []}
                onSelectionChange={(keys) => { if (keys !== 'all') setSelectedId([...keys][0] as string) }}
                className="max-h-[22rem] overflow-y-auto lg:max-h-[calc(100dvh-23rem)]"
              >
                {rows.map((notice) => (
                  <ListView.Item id={notice.id} key={notice.id} textValue={notice.result.title || '群通知'} className="items-start px-4 py-2.5">
                    <ListView.ItemContent className="flex-col items-start gap-1">
                      <span className="flex w-full items-center justify-between gap-2">
                        <ListView.Title className="max-w-full">{notice.result.title || '群通知'}</ListView.Title>
                        <span className="text-[11px] tabular-nums text-muted">{fmtDateTime(notice.createdAt)}</span>
                      </span>
                      <span className="flex max-w-full items-center gap-2"><IntentChip intent={urgencyIntent(notice.result.level)}>{urgencyLabel(notice.result.level)}</IntentChip><span className="truncate text-xs text-muted">{notice.group || `群 ${notice.groupId}`}</span></span>
                      {notice.result.summary ? <ListView.Description className="mt-0 max-w-full">{notice.result.summary}</ListView.Description> : null}
                    </ListView.ItemContent>
                  </ListView.Item>
                ))}
              </ListView>
            </Widget.Content>
            {hasMore ? <Widget.Footer className="justify-center pt-1 pb-2"><Button size="sm" variant="ghost" onPress={() => { void load(true) }} isPending={loading}>加载更早的通知</Button></Widget.Footer> : null}
          </Widget>
          {selected ? <NoticeDetail notice={selected} sources={sources} /> : null}
        </div>
      )}
    </Page>
  )
}

function NoticeDetail({ notice, sources }: { notice: NoticeRecord; sources: SourceConfig[] }) {
  const accountId = notice.accountId || 'legacy-default'
  const source = sources.find((s) => s.accounts.some((a) => a.id === accountId))
  const account = source?.accounts.find((a) => a.id === accountId)
  const origin = source && account ? `${source.name} / ${account.name}` : `${notice.sourceId || '旧版来源'} / ${notice.accountId || '旧版账号'}`
  const result = notice.result
  const details = [
    ['时间', result.time], ['地点', result.place], ['事项', result.event], ['截止', result.deadline],
  ].filter(([, value]) => value && value !== '无' && value !== '未提及')
  return (
    <Widget className="lg:sticky lg:top-4">
      <Widget.Header>
        <Widget.Title>通知详情</Widget.Title>
        <IntentChip intent={urgencyIntent(result.level)}>{urgencyLabel(result.level)}</IntentChip>
      </Widget.Header>
      <Widget.Content>
        <article className="flex min-w-0 flex-col gap-4" aria-label="选中的通知">
          <div>
            <h2 className="break-words text-lg font-semibold tracking-tight">{result.title || '群通知'}</h2>
            <p className="mt-1 text-xs tabular-nums text-muted">{notice.group || `群 ${notice.groupId}`} · {fmtDateTime(notice.createdAt)}</p>
          </div>
          {result.summary ? <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{result.summary}</p> : null}
          {details.length ? (
            <dl className="grid gap-3 text-sm">
              {details.map(([label, value]) => <div key={label} className="grid min-w-0 grid-cols-[2rem_minmax(0,1fr)] gap-3"><dt className="text-muted">{label}</dt><dd className="whitespace-pre-wrap break-words leading-relaxed">{value}</dd></div>)}
            </dl>
          ) : null}
          <div className="flex flex-col gap-1.5 border-t border-separator pt-3 text-xs text-muted">
            <span className="break-words">{origin} · {notice.group || `群 ${notice.groupId}`}</span>
            {notice.sources?.length ? <span className="break-words">来源：{[...new Set(notice.sources.map((source) => source.nickname || String(source.userId)))].join('、')}</span> : null}
          </div>
        </article>
      </Widget.Content>
      <Widget.Footer className="pt-1 pb-3">
        <div className="text-xs text-muted">
          {account ? <a href={`/groups/${notice.groupId}/history?accountId=${encodeURIComponent(accountId)}`} className="font-medium text-accent hover:underline">查看群聊记录</a> : <span>原账号不可用</span>}
        </div>
      </Widget.Footer>
    </Widget>
  )
}
