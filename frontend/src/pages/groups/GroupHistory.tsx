import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { Avatar, Button, Modal, Spinner } from '@heroui/react'
import { Widget } from '@heroui-pro/react'
import { Page } from '../../components/Page'
import { EmptyState, InlineError } from '../../components/ui/States'
import { Loader } from '../../components/Loader'
import { IntentChip } from '../../components/ui/IntentChip'
import { AppIcon, type IconName } from '../../lib/icons'
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

// Map a filename's extension to a brand-tinted icon so a file card reads at a
// glance (PDF red, Word blue, Excel green, PPT orange, archive amber, code
// indigo). The colored tile uses the same hue as a soft wash + saturated glyph,
// which stays clean in both light and dark themes (item: less-gray file cards).
type FileKind = { icon: IconName; color: string }
function fileKind(name: string): FileKind {
  const ext = (name.split('.').pop() || '').toLowerCase()
  const zipped = /\.(tar\.gz|tar\.bz2|tar\.xz)$/i.test(name)
  if (zipped || ['zip', '7z', 'rar', 'tar', 'gz', 'tgz', 'bz2', 'xz'].includes(ext))
    return { icon: 'fileZip', color: '#D08A2E' }
  if (ext === 'pdf') return { icon: 'fileText', color: '#E5484D' }
  if (['doc', 'docx', 'rtf'].includes(ext)) return { icon: 'fileDoc', color: '#2F6FE0' }
  if (['xls', 'xlsx', 'csv'].includes(ext)) return { icon: 'fileXls', color: '#2E9E5B' }
  if (['ppt', 'pptx'].includes(ext)) return { icon: 'filePpt', color: '#E06C2F' }
  if (['txt', 'md', 'log', 'ini', 'conf'].includes(ext))
    return { icon: 'fileText', color: '#6B7A90' }
  if (['js', 'ts', 'tsx', 'jsx', 'py', 'go', 'java', 'c', 'cpp', 'rs', 'rb', 'php', 'sh', 'json', 'xml', 'yml', 'yaml', 'html', 'css'].includes(ext))
    return { icon: 'fileCode', color: '#5B6EE1' }
  return { icon: 'file', color: '#7A8699' }
}

// Format unix-seconds voice/audio duration as m:ss for the QQ-style player.
function fmtDur(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return ''
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

// ---- Inline media segments: render the real thing, never a [图片]/[语音] tag ----

// A photo opens a floating lightbox on click (never a raw src navigation); the
// overlay carries a download link that saves the same-origin bytes. A sticker
// (大表情) renders small at natural size and is not zoomable, matching QQ.
function ImageSeg({ url, sticker }: { url?: string; sticker?: boolean }) {
  const [open, setOpen] = useState(false)
  if (!url) return null
  const src = api.groups.mediaUrl(url)
  if (sticker) {
    return <img src={src} alt="表情" loading="lazy" className="mt-1 max-h-24 max-w-[6rem] object-contain" />
  }
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="mt-1 block w-fit overflow-hidden rounded-lg border border-border transition-opacity hover:opacity-90"
      >
        <img src={src} alt="图片" loading="lazy" className="max-h-64 max-w-[16rem] object-cover" />
      </button>
      <Modal.Backdrop isOpen={open} onOpenChange={setOpen} isDismissable>
        <Modal.Container>
          <Modal.Dialog className="max-w-[92vw] p-2 sm:max-w-3xl">
            <Modal.CloseTrigger />
            <Modal.Body className="flex flex-col items-center gap-3">
              <img src={src} alt="图片" className="max-h-[78vh] w-auto rounded-md object-contain" />
              <a
                href={src}
                download
                className="inline-flex w-full items-center justify-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-muted/50"
              >
                <AppIcon name="download" className="size-4" />
                下载原图
              </a>
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </>
  )
}

// A minimal right-click menu: QQ surfaces voice actions on right-click, not a
// chrome control bar. Rendered in a portal at the cursor and dismissed on any
// outside click, scroll, resize, or Esc. Styled with theme tokens.
function DownloadMenu({
  at,
  onClose,
  href,
  label,
}: {
  at: { x: number; y: number } | null
  onClose: () => void
  href: string
  label: string
}) {
  useEffect(() => {
    if (!at) return
    const close = () => onClose()
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
      window.removeEventListener('keydown', onKey)
    }
  }, [at, onClose])
  if (!at) return null
  const x = Math.min(at.x, window.innerWidth - 168)
  const y = Math.min(at.y, window.innerHeight - 56)
  return createPortal(
    <div
      className="fixed inset-0 z-50"
      onClick={onClose}
      onContextMenu={(e) => {
        e.preventDefault()
        onClose()
      }}
    >
      <div
        style={{ left: x, top: y }}
        className="fixed min-w-36 rounded-lg border border-border bg-surface p-1 shadow-lg"
      >
        <a
          href={href}
          download
          onClick={onClose}
          className="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-sm text-foreground transition-colors hover:bg-muted/60"
        >
          <AppIcon name="download" className="size-4 text-muted" />
          {label}
        </a>
      </div>
    </div>,
    document.body,
  )
}

// QQ-style voice bubble: a play/pause control + progress track + duration, backed
// by a hidden <audio> (custom player, no default browser control bar). The voice
// route is the same-origin mp3 transcode. Right-click opens the download menu.
function AudioSeg({ file, url }: { file?: string; url?: string }) {
  const ref = useRef<HTMLAudioElement | null>(null)
  const [playing, setPlaying] = useState(false)
  const [dur, setDur] = useState(0)
  const [cur, setCur] = useState(0)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const src = file ? api.groups.voiceUrl(file) : url ? api.groups.mediaUrl(url) : ''
  if (!src) return null
  const toggle = () => {
    const a = ref.current
    if (!a) return
    if (a.paused) {
      void a.play()
      setPlaying(true)
    } else {
      a.pause()
      setPlaying(false)
    }
  }
  const pct = dur > 0 ? Math.min(100, (cur / dur) * 100) : 0
  return (
    <div
      className="mt-1 w-fit"
      onContextMenu={(e) => {
        e.preventDefault()
        setMenu({ x: e.clientX, y: e.clientY })
      }}
    >
      <div className="flex items-center gap-2.5 rounded-2xl rounded-tl-md bg-surface px-3 py-2 shadow-sm">
        <button
          type="button"
          onClick={toggle}
          aria-label={playing ? '暂停' : '播放'}
          className="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent transition-opacity hover:opacity-80"
        >
          <AppIcon name={playing ? 'pause' : 'play'} className="size-4" />
        </button>
        <div className="h-1 w-28 overflow-hidden rounded-full bg-muted">
          <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
        </div>
        <span className="min-w-[2.5rem] text-xs text-muted tabular-nums">{fmtDur(cur || dur)}</span>
      </div>
      <audio
        ref={ref}
        src={src}
        preload="metadata"
        onLoadedMetadata={(e) => setDur(e.currentTarget.duration)}
        onTimeUpdate={(e) => setCur(e.currentTarget.currentTime)}
        onEnded={() => {
          setPlaying(false)
          setCur(0)
        }}
        className="hidden"
      />
      <DownloadMenu at={menu} onClose={() => setMenu(null)} href={src} label="下载语音" />
    </div>
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

// A quoted original (reply target), resolved server-side. Lighter than the body
// (muted, thin left rule, no fill) with the name and text on separate lines, so
// it reads as context rather than competing with the message itself.
function QuoteSeg({ reply }: { reply?: ReplyQuote }) {
  if (!reply) return null
  return (
    <div className="mt-1 border-l-2 border-border pl-2.5">
      <div className="text-xs font-medium text-muted">
        {reply.nickname || `用户 ${reply.userId}`}
      </div>
      <div className="line-clamp-2 break-words text-xs text-muted/70">{reply.text}</div>
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
        nodes.push(<ImageSeg key={i} url={s.url} sticker={s.sticker} />)
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
// (fixes the "下载" no-extension bug) — a plain navigation, cookie carried. The
// leading tile is tinted by file type (per-extension icon + brand hue) on a
// clean surface card, so it reads at a glance without the old gray wash.
function FileCard({ groupId, file }: { groupId: number; file: HistoryFile }) {
  const href = api.groups.fileDownloadUrl(groupId, file.fileId, file.name, file.busid)
  const kind = fileKind(file.name)
  return (
    <a
      href={href}
      download={file.name}
      className="group flex w-64 max-w-full items-start gap-2.5 rounded-xl bg-surface-secondary p-3 hover:bg-default"
    >
      <span
        className="mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-lg"
        style={{ backgroundColor: `${kind.color}1f`, color: kind.color }}
      >
        <AppIcon name={kind.icon} className="size-5" />
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

// A message whose whole body is image/sticker/video/voice (no text, no files)
// renders bare — QQ shows such media without a chat-bubble background (and the
// voice player already carries its own bubble, so a wrapper would double it).
function isBareMedia(m: HistoryMsg): boolean {
  const segs = m.segments
  if (!segs || segs.length === 0) return false
  if (m.files && m.files.length > 0) return false
  return segs.every((s) => s.type === 'image' || s.type === 'video' || s.type === 'record')
}

// One QQ-style message: avatar + 群内备注名 + 头衔 chips + time above a chat
// bubble (own messages align right on a soft-accent bubble). Consecutive
// messages from the same speaker are grouped — avatar and header hidden — and
// pure media/voice render bare, without a bubble. File cards sit below.
function HistoryItem({ groupId, m, prev }: { groupId: number; m: HistoryMsg; prev?: HistoryMsg }) {
  const showRole = m.role === 'owner' || m.role === 'admin'
  const grouped = !!prev && prev.userId === m.userId && m.time - prev.time < 300
  const self = m.isSelf
  const bare = isBareMedia(m)
  const hasFiles = !!m.files && m.files.length > 0
  const bodyEmpty = (!m.segments || m.segments.length === 0) && plainFallback(m.text) === ''
  return (
    <li className={`flex gap-2.5 first:mt-0 ${grouped ? 'mt-0.5' : 'mt-3'} ${self ? 'flex-row-reverse' : ''}`}>
      {grouped ? (
        <span className="w-8 shrink-0" aria-hidden />
      ) : (
        <Avatar size="sm" className="shrink-0">
          <Avatar.Image src={userAvatar(m.userId)} alt={m.nickname || String(m.userId)} loading="lazy" />
          <Avatar.Fallback>{(m.nickname || 'Q').slice(0, 1)}</Avatar.Fallback>
        </Avatar>
      )}
      <div className={`flex min-w-0 max-w-[84%] flex-col sm:max-w-[76%] ${self ? 'items-end' : 'items-start'}`}>
        {grouped ? null : (
          <div className={`mb-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 ${self ? 'flex-row-reverse' : ''}`}>
            <span className="text-xs font-medium text-muted">{m.nickname || m.userId}</span>
            {showRole ? <IntentChip intent={roleIntent[m.role]}>{roleLabel[m.role]}</IntentChip> : null}
            {m.title ? <IntentChip intent="secondary">{m.title}</IntentChip> : null}
            {self ? <IntentChip intent="success">本账号</IntentChip> : null}
            <span className="text-xs tabular-nums text-muted/70">{fmtDateTime(secToMs(m.time))}</span>
          </div>
        )}
        {bodyEmpty ? null : bare ? (
          <MessageBody m={m} />
        ) : (
          <div
            className={`w-fit rounded-2xl px-3 py-1.5 ${self ? 'rounded-tr-md bg-accent-soft' : 'rounded-tl-md bg-surface-secondary'}`}
          >
            <MessageBody m={m} />
          </div>
        )}
        {hasFiles ? (
          <div className="mt-2 flex flex-wrap gap-2">
            {m.files!.map((f) => (
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
        // Each batch is oldest-first (NapCat reverseOrder), so batch[0] is the
        // oldest message and its messageSeq is the anchor for the next-older page.
        // An older batch overlaps the previous anchor by one message; dedupe by
        // messageId so that overlap (and any window overlap) collapses cleanly.
        const fresh = batch.filter((m) => !seenRef.current.has(m.messageId))
        fresh.forEach((m) => seenRef.current.add(m.messageId))
        const oldestSeq = batch.length > 0 ? batch[0].messageSeq : 0
        if (fresh.length > 0) {
          if (oldestSeq > 0) cursorRef.current = oldestSeq
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
        // Keep paging while older messages keep arriving and we still hold a valid
        // backward anchor. At the earliest retained message the anchor returns only
        // itself (all dupes) → fresh 0 → stop, showing "没有更早的消息了".
        const more = fresh.length > 0 && oldestSeq > 0
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
    <Page className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <Avatar size="md" className="shrink-0">
            <Avatar.Image src={groupAvatar(gid)} alt={title} loading="lazy" />
            <Avatar.Fallback>{title.slice(0, 1)}</Avatar.Fallback>
          </Avatar>
          <div className="min-w-0">
            <h1 className="truncate text-xl font-semibold tracking-tight text-foreground">{title}</h1>
            <p className="mt-0.5 text-xs text-muted tabular-nums">
              群号 {gid}
              {state?.memberCount ? ` · ${state.memberCount} 名成员` : ''}
              {state?.groupRemark ? ` · ${state.groupRemark}` : ''}
            </p>
          </div>
        </div>
        <Button variant="ghost" size="sm" onPress={() => navigate('/groups')}>
          <AppIcon name="back" className="size-4" />
          返回群组
        </Button>
      </div>

      <Widget>
        <Widget.Header className="py-1">
          <div className="flex items-center gap-3">
            <Widget.Title>消息记录</Widget.Title>
            <Widget.Description className="tabular-nums">已加载 {msgs.length} 条</Widget.Description>
          </div>
          <Button size="sm" variant="ghost" isDisabled={phase !== 'ready' || msgs.length === 0} onPress={() => { const el = scrollRef.current; if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' }) }}>回到最新</Button>
        </Widget.Header>
        <Widget.Content className="p-0">
        {phase === 'init' ? (
          <Loader label="正在加载聊天记录…" />
        ) : phase === 'error' ? (
          <InlineError message={err || '加载失败'} onRetry={() => fetchBatch(true)} />
        ) : msgs.length === 0 ? (
          <EmptyState icon="logs" title="暂无历史消息" description="该群最近没有可显示的消息" />
        ) : (
          <div
            ref={scrollRef}
            aria-label="群聊历史消息"
            tabIndex={0}
            className={`overflow-y-auto overscroll-contain px-3 pb-4 sm:px-5 ${msgs.length < 6 ? 'min-h-44 max-h-96' : 'h-[calc(100dvh-15rem)] min-h-64 max-h-[52rem]'}`}
          >
            <div ref={topRef} className="flex justify-center py-4">
              {loadingMore ? (
                <Spinner size="sm" color="accent" />
              ) : hasMore ? (
                <span className="text-xs text-muted">向上滚动加载更早的消息</span>
              ) : (
                <span className="text-xs text-muted">没有更早的消息了</span>
              )}
            </div>
            <ul className="pb-1">
              {msgs.map((m, i) => (
                <HistoryItem key={m.messageId} groupId={gid} m={m} prev={msgs[i - 1]} />
              ))}
            </ul>
          </div>
        )}
        </Widget.Content>
        <Widget.Footer className="justify-between gap-2 pt-1 pb-2 text-xs text-muted">
          <span>向上滚动查看更早记录</span>
          <span>支持图片、语音与文件</span>
        </Widget.Footer>
      </Widget>
    </Page>
  )
}

