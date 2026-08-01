import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export interface Branding {
  brandName: string
  brandTagline: string
  brandLogo: string
  brandAccent: string
  supportUrl: string
}

const FALLBACK: Branding = {
  brandName: 'AmneziaX',
  brandTagline: '',
  brandLogo: '',
  brandAccent: '',
  supportUrl: '',
}

const BrandingContext = createContext<{
  branding: Branding
  reload: () => Promise<void>
} | null>(null)

/**
 * Branding is fetched from a public endpoint so the sign-in screen and the
 * subscription page are already white-labelled before anyone authenticates.
 */
export function BrandingProvider({ children }: { children: ReactNode }) {
  const [branding, setBranding] = useState<Branding>(FALLBACK)

  const reload = useCallback(async () => {
    try {
      const res = await fetch('/api/branding')
      if (!res.ok) return
      const data = (await res.json()) as Partial<Branding>
      setBranding({ ...FALLBACK, ...data })
    } catch {
      // Keep the defaults; the panel is still perfectly usable unbranded.
    }
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  useEffect(() => {
    document.title = branding.brandName || 'AmneziaX'
    const root = document.documentElement
    if (branding.brandAccent) {
      root.style.setProperty('--accent', branding.brandAccent)
      root.style.setProperty('--accent-hover', branding.brandAccent)
    } else {
      root.style.removeProperty('--accent')
      root.style.removeProperty('--accent-hover')
    }
  }, [branding])

  const value = useMemo(() => ({ branding, reload }), [branding, reload])
  return <BrandingContext.Provider value={value}>{children}</BrandingContext.Provider>
}

export function useBranding() {
  const ctx = useContext(BrandingContext)
  if (!ctx) throw new Error('useBranding must be used inside BrandingProvider')
  return ctx
}
