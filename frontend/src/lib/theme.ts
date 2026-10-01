import { useEffect, useState } from 'react'

// Theme preference: an explicit mode or "follow the OS". The resolved theme
// (what actually gets painted) is derived from this plus the system setting.
// A tiny inline script in index.html applies the same logic before first paint
// to avoid a flash — keep STORAGE_KEY and the resolve logic in sync with it.
export type ThemePref = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'nap-theme'

const systemQuery = () => window.matchMedia('(prefers-color-scheme: dark)')

// Read the persisted preference, defaulting to "system" when unset/corrupt.
export function getThemePref(): ThemePref {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    // localStorage may be unavailable (private mode, etc.) — fall through.
  }
  return 'system'
}

// Collapse a preference to the concrete theme to paint.
export function resolveTheme(pref: ThemePref): 'light' | 'dark' {
  if (pref === 'system') return systemQuery().matches ? 'dark' : 'light'
  return pref
}

// Share the resolved mode with HeroUI and the workspace palette.
export function applyTheme(pref: ThemePref): void {
  const resolved = resolveTheme(pref)
  const root = document.documentElement
  root.classList.toggle('dark', resolved === 'dark')
  root.style.colorScheme = resolved
}

// Persist a preference and apply it immediately.
export function setThemePref(pref: ThemePref): void {
  try {
    localStorage.setItem(STORAGE_KEY, pref)
  } catch {
    // Best-effort persistence; still apply for this session.
  }
  applyTheme(pref)
}

// React binding for the theme control. Tracks the current preference and, while
// in "system" mode, re-applies when the OS theme flips.
export function useTheme(): { pref: ThemePref; setPref: (pref: ThemePref) => void } {
  const [pref, setPrefState] = useState<ThemePref>(getThemePref)

  useEffect(() => {
    const mql = systemQuery()
    const onChange = () => {
      if (getThemePref() === 'system') applyTheme('system')
    }
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])

  const setPref = (next: ThemePref) => {
    setPrefState(next)
    setThemePref(next)
  }

  return { pref, setPref }
}
