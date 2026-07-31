import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { en, type Dictionary } from './en'
import { ru } from './ru'

export type Lang = 'en' | 'ru'

const dictionaries: Record<Lang, Dictionary> = { en, ru }

const STORAGE_KEY = 'amneziax.lang'

function detectLang(): Lang {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'en' || stored === 'ru') return stored
  // Russian is the more likely default for this audience, but only when the
  // browser actually asks for it.
  return navigator.language.toLowerCase().startsWith('ru') ? 'ru' : 'en'
}

type Ctx = {
  lang: Lang
  setLang: (l: Lang) => void
  t: Dictionary
  /** Fills {placeholders} in a translated string. */
  f: (template: string, vars: Record<string, string | number>) => string
}

const I18nContext = createContext<Ctx | null>(null)

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLang)

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, lang)
    document.documentElement.lang = lang
  }, [lang])

  const f = useCallback((template: string, vars: Record<string, string | number>) => {
    return template.replace(/\{(\w+)\}/g, (match, key) =>
      key in vars ? String(vars[key]) : match,
    )
  }, [])

  const value = useMemo<Ctx>(
    () => ({ lang, setLang: setLangState, t: dictionaries[lang], f }),
    [lang, f],
  )

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n(): Ctx {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used inside I18nProvider')
  return ctx
}
