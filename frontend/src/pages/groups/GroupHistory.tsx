import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { Avatar, Button, Spinner } from '@heroui/react'
import { Page } from '../../components/Page'
import { SectionCard } from '../../components/ui/SectionCard'
import { EmptyState, InlineError } from '../../components/ui/States'
import { Loader } from '../../components/Loader'
import { IntentChip } from '../../components/ui/IntentChip'
import { AppIcon } from '../../lib/icons'
import { api, ApiError } from '../../lib/api'
import { userAvatar, groupAvatar } from '../../lib/qlogo'
import { fmtDateTime, secToMs } from '../../lib/time'
import { roleLabel, roleIntent } from '../../lib/labels'
import type { HistoryFile, HistoryMsg, MsgSegment, ReplyQuote } from '../../lib/types'

const BATCH = 30

// Human-readable byte size for file cards (empty when unknown).
function fmtBytes(n: number): string {
  if (!n || n <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`
}

// ---- Inline media segments: render the real thing, never a [图片]/[语音] tag ----

function ImageSeg({ url }: { url?: string }) {
  if (!url) return null
  const src = api.groups.mediaUrl(url)
  return (
    <a href={src} target="_blank" rel="noopener noreferrer" className="mt-1 block w-fit">
      <img
        src={src}
        alt="图片"
        loading="lazy"
        className="max-h-64 max-w-[16rem] rounded-lg border border-border object-cover"
      />
    </a>
  )
}

function AudioSeg({ file, url }: { file?: string; url?: string }) {
  // QQ voice is AMR/SILK — browsers can't decode it, and the CDN mislabels the
  // bytes audio/mp3, so a raw <audio src> never loads. Prefer the same-origin
  // transcode (backend get_record → real mp3); fall back to the proxied CDN only
  // if we have no file id. The raw URL stays as a manual download link.
  const src = file ? api.groups.voiceUrl(file) : url ? api.groups.mediaUrl(url) : ''
  if (!src) return null
  return (
    <audio controls src={src} className="mt-1 h-10 w-full max-w-xs" preload="none">
      {url ? (
        <a href={api.groups.mediaUrl(url)} target="_blank" rel="noopener noreferrer">
          下载语音
        </a>
      ) : null}
    </audio>
  )
}

function VideoSeg({ url }: { url?: string }) {
  if (!url) return null
  return (
    <video
      controls
      src={api.groups.mediaUrl(url)}
      preload="metadata"
      className="mt-1 max-h-64 max-w-[20rem] rounded-lg border border-border"
    />
  )
}

// A quoted original (reply target), resolved server-side.
function QuoteSeg({ reply }: { reply?: ReplyQuote }) {
  if (!reply) return null
  return (
    <div className="mt-1 rounded-md border-l-2 border-accent/50 bg-muted/40 px-2 py-1 text-xs text-muted">
      <span className="font-medium text-foreground/80">
        {reply.nickname || `用户 ${reply.userId}`}
      </span>
      <span className="ml-1 break-words">{reply.text}</span>
    </div>
  )
}

// Walk the structured segments: text/@/face/card flow inline; image, voice,
// video and quotes break out as their own blocks. @-mentions show the resolved
// name (never the bare QQ number).
function renderSegments(segs: MsgSegment[]): ReactNode[] {
  const nodes: ReactNode[] = []
  let run: ReactNode[] = []
  let key = 0
  const flush = () => {
    if (run.length === 0) return
    nodes.push(
      <p key={`t${key++}`} className="whitespace-pre-wrap break-words text-sm text-foreground/90">
        {run}
      </p>,
    )
    run = []
  }
  segs.forEach((s, i) => {
    switch (s.type) {
      case 'text':
        run.push(<span key={i}>{s.text}</span>)
        break
      case 'at':
        run.push(
          <span key={i} className="font-medium text-accent">
            @{s.name || s.id}
          </span>,
        )
        break
      case 'face':
      case 'card':
      case 'forward':
        run.push(
          <span key={i} className="text-muted">
            {s.text}
          </span>,
        )
        break
      case 'image':
        flush()
        nodes.push(<ImageSeg key={i} url={s.url} />)
        break
      case 'record':
        flush()
        nodes.push(<AudioSeg key={i} file={s.file} url={s.url} />)
        break
      case 'video':
        flush()
        nodes.push(<VideoSeg key={i} url={s.url} />)
        break
      case 'reply':
        flush()
        nodes.push(<QuoteSeg key={i} reply={s.reply} />)
        break
    }
  })
  flush()
  return nodes
}

// The flat-text fallback (older payloads / the live-feed shape) can carry bracket
// placeholders like [文件:x] / [图片] / [引用]. History messages now render those
// as real segments and file cards, so in the fallback path they are pure noise
// the user asked us to drop — strip them and only show genuine leftover text.
const PLACEHOLDER_RE = /\[(?:图片|语音|视频|引用|表情|卡片|聊天记录|文件:[^\]]*)\]/g
function plainFallback(text?: string): string {
  if (!text) return ''
  return text.replace(PLACEHOLDER_RE, '').trim()
}

// Message body: prefer structured segments; fall back to flat text for older
// payloads (or the live-feed shape) that carry no segments. A file-only message
// has no segments and a "[文件:名]" text, which the file card already conveys —
// so the stripped fallback is empty and only the card shows.
function MessageBody({ m }: { m: HistoryMsg }) {
  if (m.segments && m.segments.length > 0) {
    return <div className="mt-1 space-y-1.5">{renderSegments(m.segments)}</div>
  }
  const text = plainFallback(m.text)
  if (text) {
    return (
      <p className="mt-1 whitespace-pre-wrap break-words text-sm text-foreground/90">{text}</p>
    )
  }
  return null
}

// A single downloadable group file as a compact rectangular card. The href is a
// same-origin proxy that resolves the short-lived NapCat link and re-serves it
// with Content-Disposition, so the browser saves it under its real name+ext
// (fixes the "下载" no-extension bug) — a plain navigation, cookie carried.
function FileCard({ groupId, file }: { groupId: number; file: HistoryFile }) {
  const href = api.groups.fileDownloadUrl(groupId, file.fileId, file.name, file.busid)
  return (
    <a
      href={href}
      download={file.name}
      className="group flex w-56 max-w-full items-start gap-2.5 rounded-xl border border-border bg-muted/40 p-3 transition-colors hover:bg-muted/70"
    >
      <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-default text-muted">
        <AppIcon name="file" className="size-4" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 break-all text-xs font-medium text-foreground">
          {file.name}
        </span>
        {file.size > 0 ? (
          <span className="mt-0.5 block text-[11px] text-muted tabular-nums">
            {fmtBytes(file.size)}
          </span>
        ) : null}
      </span>
      <AppIcon
        name="download"
        className="mt-0.5 size-4 shrink-0 text-muted transition-colors group-hover:text-accent"
      />
    </a>
  )
}

// One message row: speaker avatar + 群内备注名 + 头衔 chips + timestamp, then the
// structured body (inline media, @-names, quotes) and any file cards.
function HistoryItem({ groupId, m }: { groupId: number; m: HistoryMsg }) {
  const showRole = m.role === 'owner' || m.role === 'admin'
  return (
    <li className="flex gap-3 py-3 first:pt-0 last:pb-0">
      <Avatar size="sm" className="shrink-0">
        <Avatar.Image src={userAvatar(m.userId)} alt={m.nickname || String(m.userId)} loading="lazy" />
        <Avatar.Fallback>{(m.nickname || 'Q').slice(0, 1)}</Avatar.Fallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-sm font-medium text-foreground">{m.nickname || m.userId}</span>
          {showRole ? (
            <IntentChip intent={roleIntent[m.role]}>{roleLabel[m.role]}</IntentChip>
          ) : null}
          {m.title ? <IntentChip intent="secondary">{m.title}</IntentChip> : null}
          {m.isSelf ? <IntentChip intent="success">本账号</IntentChip> : null}
          <span className="ml-auto shrink-0 text-xs text-muted tabular-nums">
            {fmtDateTime(secToMs(m.time))}
          </span>
        </div>
        <MessageBody m={m} />
        {m.files && m.files.length > 0 ? (
          <div className="mt-2 flex flex-wrap gap-2">
            {m.files.map((f) => (
              <FileCard key={f.fileId} groupId={groupId} file={f} />
            ))}
          </div>
        ) : null}
      </div>
    </li>
  )
}

// Group chat-history viewer (route: /groups/:groupId/history). QQ-style: oldest
// on top, newest at the bottom, scrolled to the bottom on open. Scrolling UP to
// the top sentinel lazy-loads older batches of 30 via the messageSeq cursor and
// prepends them, holding the viewport steady so reading isn't disrupted. Deduped
// by messageId so overlapping seq windows are safe.
export default function GroupHistory() {
  const { groupId } = useParams()
  const gid = Number(groupId)
  const navigate = useNavigate()
  const { state } = useLocation() as {
    state: { groupName?: string; groupRemark?: string; memberCount?: number } | null
  }

  const [msgs, setMsgs] = useState<HistoryMsg[]>([])
  const [phase, setPhase] = useState<'init' | 'ready' | 'error'>('init')
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [err, setErr] = useState<string>()

  const busyRef = useRef(false)
  const hasMoreRef = useRef(true)
  const cursorRef = useRef(0) // oldest messageSeq loaded so far
  const seenRef = useRef<Set<number>>(new Set())
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const topRef = useRef<HTMLDivElement | null>(null)
  const scrollAction = useRef<'bottom' | 'anchor' | null>(null)
  const distFromBottom = useRef(0)

  const fetchBatch = useCallback(
    async (initial: boolean) => {
      if (busyRef.current) return
      if (!initial && !hasMoreRef.current) return
      if (!Number.isFinite(gid) || gid <= 0) {
        setErr('非法群号')
        setPhase('error')
        return
      }
      if (initial) {
        seenRef.current = new Set()
        cursorRef.current = 0
        hasMoreRef.current = true
        setErr(undefined)
        setPhase('init')
        setHasMore(true)
      } else if (cursorRef.current <= 0) {
        hasMoreRef.current = false
        setHasMore(false)
        return
      } else {
        setLoadingMore(true)
      }
      busyRef.current = true

      try {
        const before = initial ? undefined : cursorRef.current
        const batch = await api.groups.history(gid, BATCH, before)
        // Each batch is oldest-first; an older batch is globally older, so it
        // prepends above what's shown. Dedupe by messageId across windows.
        const fresh = batch.filter((m) => !seenRef.current.has(m.messageId))
        fresh.forEach((m) => seenRef.current.add(m.messageId))
        if (fresh.length > 0) {
          const seqs = batch.map((m) => m.messageSeq).filter((s) => s > 0)
          if (seqs.length > 0) cursorRef.current = Math.min(...seqs)
          if (initial) {
            scrollAction.current = 'bottom'
            setMsgs(fresh)
          } else {
            const el = scrollRef.current
            distFromBottom.current = el ? el.scrollHeight - el.scrollTop : 0
            scrollAction.current = 'anchor'
            setMsgs((prev) => [...fresh, ...prev])
          }
        }
        const more = batch.length >= BATCH && fresh.length > 0 && cursorRef.current > 0
        hasMoreRef.current = more
        setHasMore(more)
        setPhase('ready')
      } catch (e) {
        if (initial) {
          setErr(e instanceof ApiError ? e.message : '加载失败')
          setPhase('error')
        } else {
          // Soft-fail older-batch loads: keep what's shown, stop paging up.
          hasMoreRef.current = false
          setHasMore(false)
        }
      } finally {
        busyRef.current = false
        setLoadingMore(false)
      }
    },
    [gid],
  )

  // Initial load (and reset) whenever the group changes.
  useEffect(() => {
    fetchBatch(true)
  }, [fetchBatch])

  // After each render that changed the list, place the scroll: to the bottom on
  // first load, or hold the reading position steady after an older-batch prepend
  // (keep the same distance from the bottom, so nothing appears to jump).
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el || !scrollAction.current) return
    if (scrollAction.current === 'bottom') el.scrollTop = el.scrollHeight
    else if (scrollAction.current === 'anchor') el.scrollTop = el.scrollHeight - distFromBottom.current
    scrollAction.current = null
  }, [msgs])

  // Scroll-up lazyload: observe a top sentinel within the scroll container. Set
  // up once the list is on screen (root + target both mounted in 'ready').
  useEffect(() => {
    if (phase !== 'ready') return
    const root = scrollRef.current
    const target = topRef.current
    if (!root || !target) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) fetchBatch(false)
      },
      { root, rootMargin: '160px 0px 0px 0px' },
    )
    io.observe(target)
    return () => io.disconnect()
  }, [phase, fetchBatch])

  const title = state?.groupName || String(gid)

  return (
    <Page>
      <div className="mb-6 flex items-center gap-3">
        <Button variant="tertiary" size="sm" onPress={() => navigate('/groups')} aria-label="返回群组">
          <AppIcon name="back" className="size-4" />
          返回
        </Button>
        <Avatar size="md" className="shrink-0">
          <Avatar.Image src={groupAvatar(gid)} alt={title} loading="lazy" />
          <Avatar.Fallback>{title.slice(0, 1)}</Avatar.Fallback>
        </Avatar>
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold tracking-tight text-foreground">{title}</h1>
          <p className="text-xs text-muted tabular-nums">
            群号 {gid}
            {state?.memberCount ? ` · ${state.memberCount} 名成员` : ''}
          </p>
        </div>
      </div>

      <SectionCard title="消息记录" description={`已加载 ${msgs.length} 条`}>
        {phase === 'init' ? (
          <Loader label="正在加载聊天记录…" />
        ) : phase === 'error' ? (
          <InlineError message={err || '加载失败'} onRetry={() => fetchBatch(true)} />
        ) : msgs.length === 0 ? (
          <EmptyState icon="logs" title="暂无历史消息" description="该群最近没有可显示的消息" />
        ) : (
          <div ref={scrollRef} className="max-h-[68vh] overflow-y-auto pr-1">
            <div ref={topRef} className="flex justify-center py-3">
              {loadingMore ? (
                <Spinner size="sm" color="accent" />
              ) : hasMore ? (
                <span className="text-xs text-muted">向上滚动加载更早的消息</span>
              ) : (
                <span className="text-xs text-muted">没有更早的消息了</span>
              )}
            </div>
            <ul className="divide-y divide-border">
              {msgs.map((m) => (
                <HistoryItem key={m.messageId} groupId={gid} m={m} />
              ))}
            </ul>
          </div>
        )}
      </SectionCard>
    </Page>
  )
}




