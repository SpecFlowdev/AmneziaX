import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from './api'

/**
 * Fetches a GET endpoint and re-fetches on demand or on an interval. Returns
 * the last successful payload while a refresh is in flight so live pages do not
 * flash empty between polls.
 */
export function useFetch<T>(path: string | null, pollMs?: number) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(path !== null)
  const generation = useRef(0)

  const reload = useCallback(async () => {
    if (path === null) {
      setData(null)
      setLoading(false)
      return
    }
    const mine = ++generation.current
    try {
      const result = await api.get<T>(path)
      if (mine === generation.current) {
        setData(result)
        setError(null)
      }
    } catch (err) {
      // A 401 is handled globally by the auth provider; don't surface it twice.
      if (mine === generation.current && !(err instanceof ApiError && err.status === 401)) {
        setError(err instanceof Error ? err.message : String(err))
      }
    } finally {
      if (mine === generation.current) setLoading(false)
    }
  }, [path])

  useEffect(() => {
    setLoading(true)
    void reload()
  }, [reload])

  useEffect(() => {
    if (!pollMs || path === null) return
    const id = setInterval(() => void reload(), pollMs)
    return () => clearInterval(id)
  }, [pollMs, reload, path])

  return { data, error, loading, reload }
}
