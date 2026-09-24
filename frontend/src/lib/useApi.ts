import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from './api'

export interface AsyncState<T> {
  data: T | undefined
  error: string | undefined
  loading: boolean
  reload: () => void
  setData: (updater: (prev: T | undefined) => T) => void
}

// One-shot GET helper with loading/error/reload. A monotonic request id guards
// against out-of-order responses when reload() is called rapidly. `deps` are
// the values that should refetch when changed (pass [] for load-once).
export function useApi<T>(
  fn: () => Promise<T>,
  deps: unknown[] = [],
): AsyncState<T> {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(true)
  const fnRef = useRef(fn)
  fnRef.current = fn
  const idRef = useRef(0)

  const run = useCallback(() => {
    const id = ++idRef.current
    setLoading(true)
    setError(undefined)
    fnRef
      .current()
      .then((d) => {
        if (id === idRef.current) setData(d)
      })
      .catch((e) => {
        if (id !== idRef.current) return
        setError(e instanceof ApiError ? e.message : '加载失败，请重试')
      })
      .finally(() => {
        if (id === idRef.current) setLoading(false)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  useEffect(() => {
    run()
  }, [run])

  const patch = useCallback((updater: (prev: T | undefined) => T) => {
    setData((prev) => updater(prev))
  }, [])

  return { data, error, loading, reload: run, setData: patch }
}
