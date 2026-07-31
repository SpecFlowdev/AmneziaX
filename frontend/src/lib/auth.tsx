import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { api, getToken, onUnauthorized, setToken, type Admin } from './api'

type AuthCtx = {
  admin: Admin | null
  ready: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  refresh: () => Promise<void>
  canWrite: boolean
  isOwner: boolean
}

const AuthContext = createContext<AuthCtx | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [admin, setAdmin] = useState<Admin | null>(null)
  const [ready, setReady] = useState(false)

  const refresh = useCallback(async () => {
    if (!getToken()) {
      setAdmin(null)
      setReady(true)
      return
    }
    try {
      setAdmin(await api.get<Admin>('/api/auth/me'))
    } catch {
      setAdmin(null)
    } finally {
      setReady(true)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    // A 401 anywhere in the app drops us back to the sign-in screen.
    const handler = () => setAdmin(null)
    onUnauthorized.add(handler)
    return () => {
      onUnauthorized.delete(handler)
    }
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const res = await api.post<{ token: string; admin: Admin }>('/api/auth/login', {
      username,
      password,
    })
    setToken(res.token)
    setAdmin(res.admin)
  }, [])

  const logout = useCallback(() => {
    setToken(null)
    setAdmin(null)
  }, [])

  const value = useMemo<AuthCtx>(
    () => ({
      admin,
      ready,
      login,
      logout,
      refresh,
      canWrite: admin?.role === 'OWNER' || admin?.role === 'ADMIN',
      isOwner: admin?.role === 'OWNER',
    }),
    [admin, ready, login, logout, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
