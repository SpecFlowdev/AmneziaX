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
  /** Resolves to 'totp' when the password was right but a code is still needed. */
  login: (username: string, password: string, code?: string) => Promise<'ok' | 'totp'>
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

  const login = useCallback(async (username: string, password: string, code?: string) => {
    const res = await api.post<{
      token?: string
      admin?: Admin
      totpRequired?: boolean
      enrolTotp?: boolean
      recoveryCodesLeft?: number
    }>('/api/auth/login', { username, password, code })

    // The password was right, but this is not a session yet — the form has to
    // ask for a code before anything is stored.
    if (res.totpRequired) return 'totp'

    setToken(res.token ?? null)
    setAdmin(res.admin ?? null)
    if (res.enrolTotp) sessionStorage.setItem('amneziax.enrolTotp', '1')
    if (typeof res.recoveryCodesLeft === 'number') {
      sessionStorage.setItem('amneziax.recoveryLeft', String(res.recoveryCodesLeft))
    }
    return 'ok'
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
