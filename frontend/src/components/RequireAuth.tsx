import { useEffect, useState, type ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { api } from '../lib/api'
import { Loader } from './Loader'

// Route gate for the authenticated area. GET /api/auth/status is PUBLIC, so we
// can probe it without tripping the 401 → /login handler; it reports whether the
// HttpOnly nap_session cookie is currently valid. The SPA never sees the cookie
// and never re-implements auth — it only asks the backend "am I in?".
export function RequireAuth({ children }: { children: ReactNode }) {
  const [state, setState] = useState<'loading' | 'authed' | 'anon'>('loading')

  useEffect(() => {
    let alive = true
    api.auth
      .status()
      .then((s) => alive && setState(s.authed ? 'authed' : 'anon'))
      .catch(() => alive && setState('anon'))
    return () => {
      alive = false
    }
  }, [])

  if (state === 'loading') return <Loader fullscreen label="正在校验会话…" />
  if (state === 'anon') return <Navigate to="/login" replace />
  return <>{children}</>
}
