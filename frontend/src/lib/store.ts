import { create } from 'zustand'
import type {
  BufView,
  EscalationEvent,
  Event,
  GroupMessage,
  LogEntry,
  StatusPayload,
} from './types'

// Caps mirror the backend ring buffers (hub.go: maxLogs 200, maxEscs 30). The
// live message feed is display-only, so we keep a shorter tail.
const MAX_LOGS = 200
const MAX_ESCS = 30
const MAX_MSGS = 60

export interface LiveState {
  connected: boolean
  status: StatusPayload | null
  messages: GroupMessage[]
  buffers: BufView[]
  escalations: EscalationEvent[]
  logs: LogEntry[]
  // Backfill from initial REST GETs (escalations/logs survive a refresh; the
  // live SSE stream only carries new events).
  seedEscalations: (e: EscalationEvent[]) => void
  seedLogs: (l: LogEntry[]) => void
  // SSE plumbing.
  apply: (ev: Event) => void
  setConnected: (c: boolean) => void
}

export const useLive = create<LiveState>((set) => ({
  connected: false,
  status: null,
  messages: [],
  buffers: [],
  escalations: [],
  logs: [],

  seedEscalations: (e) => set({ escalations: e.slice(0, MAX_ESCS) }),
  seedLogs: (l) => set({ logs: l.slice(0, MAX_LOGS) }),
  setConnected: (connected) => set({ connected }),

  apply: (ev) =>
    set((s) => {
      switch (ev.type) {
        case 'hello':
          return { status: ev.data as StatusPayload }
        case 'status':
          // A reconnect "status" carries only onebotConnected/selfId, so merge
          // into the existing snapshot rather than replacing it.
          return {
            status: {
              ...s.status,
              ...(ev.data as Partial<StatusPayload>),
            } as StatusPayload,
          }
        case 'message':
          return {
            messages: [ev.data as GroupMessage, ...s.messages].slice(0, MAX_MSGS),
          }
        case 'buffer':
          return { buffers: (ev.data as BufView[]) ?? [] }
        case 'escalation':
          return {
            escalations: [ev.data as EscalationEvent, ...s.escalations].slice(
              0,
              MAX_ESCS,
            ),
          }
        case 'log':
          return { logs: [ev.data as LogEntry, ...s.logs].slice(0, MAX_LOGS) }
        default:
          return {}
      }
    }),
}))

// connectLiveStream opens the SSE feed and pipes events into the store. The
// backend sends every event as an unnamed `data:` frame whose JSON carries the
// discriminating `type`, so a single onmessage handler covers all of them.
// EventSource auto-reconnects; we create it once and close it on teardown.
export function connectLiveStream(): () => void {
  const es = new EventSource('/api/events', { withCredentials: true })
  const { apply, setConnected } = useLive.getState()

  es.onopen = () => setConnected(true)
  es.onmessage = (e) => {
    try {
      apply(JSON.parse(e.data) as Event)
    } catch {
      // Ignore malformed frames (e.g. a stray keep-alive comment).
    }
  }
  es.onerror = () => setConnected(false)

  return () => {
    es.close()
    setConnected(false)
  }
}
