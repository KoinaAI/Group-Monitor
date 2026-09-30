import { useEffect, useRef, useState } from 'react'
import { Button } from '@heroui/react'
import { ListView, Widget } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { TextSetting } from '../components/ui/TextSetting'
import { IntentChip } from '../components/ui/IntentChip'
import { EmptyState, InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api, ApiError } from '../lib/api'
import { urgencyIntent, urgencyLabel } from '../lib/labels'
import { fmtDateTime } from '../lib/time'
import type { NoticeRecord, SourceConfig } from '../lib/types'

const PAGE_SIZE = 30

export default function Notices() {
  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('')
  const [rows, setRows] = useState<NoticeRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
  const [sources, setSources] = useState<SourceConfig[]>([])
  const [selectedId, setSelectedId] = useState<string>()
  const requestId = useRef(0)
  const applied = useRef({ q: '', groupId: 0 })

  const load = async (append = false) => {
    if (!append && group.trim() && (!/^\d+$/.test(group.trim()) || !Number.isSafeInteger(Number(group)) || Number(group) <= 0)) {
      setError('请输入有效群号')
      return
    }
    const id = ++requestId.current
    const filters = append ? applied.current : { q: query.trim(), groupId: Number(group) || 0 }
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
    <Page className="flex flex-col gap-4">
      <PageHeader title="通知归档" description="集中检索通知，查看时间、地点和待办事项。" />
      <Widget>
        <Widget.Header>
          <Widget.Title>归档检索</Widget.Title>
          <Widget.Description className="tabular-nums">已加载 {rows.length} 条</Widget.Description>
        </Widget.Header>
        <Widget.Content>
          <form onSubmit={(event) => { event.preventDefault(); void load() }} className="grid grid-cols-[1fr_auto] items-end gap-3 sm:grid-cols-[minmax(0,1fr)_10rem_auto]">
            <TextSetting label="关键词" value={query} onChange={setQuery} placeholder="搜索标题、摘要或事项" className="col-span-2 sm:col-span-1" />
            <TextSetting label="群号" value={group} onChange={setGroup} inputMode="numeric" placeholder="全部群组" />
            <Button type="submit" isPending={loading}>搜索</Button>
          </form>
        </Widget.Content>
      </Widget>
      {error ? <InlineError message={error} onRetry={() => { void load() }} /> : null}
      {loading && rows.length === 0 ? <Loader label="正在加载通知…" /> : !error && rows.length === 0 ? (
        <EmptyState icon="fileText" title="暂无通知" description="正式通知通过判断后会在这里保留；也可以换个关键词搜索。" />
      ) : (
        <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(18rem,0.85fr)_minmax(0,1.4fr)]">
          <Widget>
            <Widget.Header>
              <Widget.Title>通知列表</Widget.Title>
              <Widget.Description>最近通知在前</Widget.Description>
            </Widget.Header>
            <Widget.Content className="p-0">
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
                  <ListView.Item id={notice.id} key={notice.id} textValue={notice.result.title || '群通知'} className="items-start px-3 py-3">
                    <ListView.ItemContent className="flex-col items-start gap-1.5">
                      <span className="flex w-full items-center justify-between gap-2">
                        <IntentChip intent={urgencyIntent(notice.result.level)}>{urgencyLabel(notice.result.level)}</IntentChip>
                        <span className="text-[11px] tabular-nums text-muted">{fmtDateTime(notice.createdAt)}</span>
                      </span>
                      <ListView.Title className="max-w-full">{notice.result.title || '群通知'}</ListView.Title>
                      <ListView.Description className="mt-0 max-w-full">{notice.group || `群 ${notice.groupId}`}{notice.result.summary ? ` · ${notice.result.summary}` : ''}</ListView.Description>
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
