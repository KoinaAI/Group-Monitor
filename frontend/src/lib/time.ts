// Timestamp helpers. Backend uses two shapes: unix SECONDS (GroupMessage.time,
// HistoryMsg.time) and ms epoch (LogEntry.ts, EscalationEvent.ts, BufView
// flushAt). Keep the seconds-vs-ms distinction explicit at every call site via
// the function name so we never multiply by 1000 in the wrong place.

const pad = (n: number) => String(n).padStart(2, '0')

export const secToMs = (sec: number) => sec * 1000

/** "HH:MM:SS" in local time. */
export function fmtClock(ms: number): string {
  const d = new Date(ms)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** "MM-DD HH:MM" in local time. */
export function fmtDateTime(ms: number): string {
  const d = new Date(ms)
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(
    d.getMinutes(),
  )}`
}

/** Chinese relative time from a ms-epoch instant in the past. */
export function relFromMs(ms: number, now = Date.now()): string {
  const diff = Math.max(0, now - ms)
  const s = Math.floor(diff / 1000)
  if (s < 10) return '刚刚'
  if (s < 60) return `${s} 秒前`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} 分钟前`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} 小时前`
  return fmtDateTime(ms)
}

/** Same as relFromMs but for a unix-seconds instant. */
export const relFromSec = (sec: number, now = Date.now()) =>
  relFromMs(secToMs(sec), now)

/** Milliseconds until a future ms-epoch instant (never negative). */
export const msUntil = (msEpoch: number, now = Date.now()) =>
  Math.max(0, msEpoch - now)

/** "M:SS" countdown label from a millisecond duration. */
export function fmtCountdown(ms: number): string {
  const total = Math.ceil(ms / 1000)
  return `${Math.floor(total / 60)}:${pad(total % 60)}`
}
