import { useEffect, useRef, useState } from 'react'
import { Button } from '@heroui/react'
import { Link } from 'react-router-dom'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { IntentChip } from '../components/ui/IntentChip'
import { EmptyState, InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api, ApiError } from '../lib/api'
import { urgencyIntent, urgencyLabel } from '../lib/labels'
import { fmtDateTime } from '../lib/time'
import type { NoticeRecord } from '../lib/types'

const PAGE_SIZE = 30

export default function Notices() {
  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('')
  const [rows, setRows] = useState<NoticeRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [hasMore, setHasMore] = useState(false)
  const [error, setError] = useState('')
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

  return (
    <Page>
      <PageHeader title="通知归档" description="检索已保留的正式通知，回看时间、地点与待办事项" />
      <SectionCard>
        <form onSubmit={(event) => { event.preventDefault(); void load() }} className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <TextSetting label="关键词" value={query} onChange={setQuery} placeholder="搜索标题、摘要或事项" className="flex-1" />
          <TextSetting label="群号" value={group} onChange={setGroup} inputMode="numeric" placeholder="全部群组" className="sm:w-44" />
          <Button type="submit" isPending={loading}>搜索</Button>
        </form>
      </SectionCard>
      {error ? <InlineError message={error} onRetry={() => { void load() }} /> : null}
      {loading && rows.length === 0 ? <Loader label="正在加载通知…" /> : !error && rows.length === 0 ? (
        <EmptyState icon="fileText" title="暂无通知" description="正式通知通过判断后会在这里保留；也可以换个关键词搜索。" />
      ) : (
        <div className="flex flex-col gap-4">
          {rows.map((notice) => <NoticeCard key={notice.id} notice={notice} />)}
          {hasMore ? <div className="flex justify-center"><Button variant="secondary" onPress={() => { void load(true) }} isPending={loading}>加载更早的通知</Button></div> : null}
        </div>
      )}
    </Page>
  )
}

function NoticeCard({ notice }: { notice: NoticeRecord }) {
  const result = notice.result
  const details = [
    ['时间', result.time], ['地点', result.place], ['事项', result.event], ['截止', result.deadline],
  ].filter(([, value]) => value && value !== '无' && value !== '未提及')
  return (
    <SectionCard
      title={<span className="flex flex-wrap items-center gap-2"><IntentChip intent={urgencyIntent(result.level)}>{urgencyLabel(result.level)}</IntentChip><span>{result.title || '群通知'}</span></span>}
      description={<span className="tabular-nums">{notice.group || `群 ${notice.groupId}`} · {fmtDateTime(notice.createdAt)}</span>}
    >
      <div className="flex flex-col gap-4">
        {result.summary ? <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{result.summary}</p> : null}
        {details.length ? (
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
            {details.map(([label, value]) => <div key={label} className="flex min-w-0 gap-3"><dt className="shrink-0 text-muted">{label}</dt><dd className="break-words">{value}</dd></div>)}
          </dl>
        ) : null}
        <div className="flex flex-wrap items-center gap-3 text-xs text-muted">
          {notice.sources?.length ? <span>来源：{[...new Set(notice.sources.map((source) => source.nickname || String(source.userId)))].join('、')}</span> : null}
          <Link to={`/groups/${notice.groupId}/history`} className="text-accent hover:underline">查看群聊记录</Link>
        </div>
      </div>
    </SectionCard>
  )
}
