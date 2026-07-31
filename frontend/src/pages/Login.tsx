import { useState, type FormEvent } from 'react'
import { BrandMark, Icon } from '../components/icons'
import { Field, Spinner } from '../components/ui'
import { useI18n, type Lang } from '../i18n'
import { useAuth } from '../lib/auth'
import { useTheme } from '../lib/theme'

export function Login() {
  const { t, lang, setLang } = useI18n()
  const { login } = useAuth()
  const { theme, setTheme } = useTheme()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await login(username.trim(), password)
    } catch (err) {
      setError(err instanceof Error ? err.message : t.login.failed)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="center-screen">
      <div style={{ display: 'flex', flexDirection: 'column', gap: 14, width: 'min(410px, 100%)' }}>
        <div className="split" style={{ justifyContent: 'flex-end' }}>
          <div className="tabs">
            {(['ru', 'en'] as Lang[]).map((l) => (
              <button
                key={l}
                className={`tab${lang === l ? ' active' : ''}`}
                onClick={() => setLang(l)}
                type="button"
              >
                {l === 'ru' ? 'РУС' : 'ENG'}
              </button>
            ))}
          </div>
          <button
            type="button"
            className="btn-ghost btn-icon"
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            aria-label={t.common.theme}
          >
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} size={17} />
          </button>
        </div>

        <form className="card auth-card" onSubmit={submit}>
          <div className="split">
            <div className="brand-mark">
              <BrandMark size={19} />
            </div>
            <div className="stack">
              <span className="brand-name">{t.common.appName}</span>
              <span className="brand-sub">{t.common.tagline}</span>
            </div>
          </div>

          <div className="stack">
            <h2 style={{ fontSize: 20 }}>{t.login.title}</h2>
            <p className="muted small" style={{ margin: 0 }}>
              {t.login.subtitle}
            </p>
          </div>

          <Field label={t.login.username}>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
            />
          </Field>

          <Field label={t.login.password}>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </Field>

          {error && (
            <div className="badge badge-danger" style={{ padding: '8px 12px' }}>
              <Icon name="alert" size={15} />
              {error}
            </div>
          )}

          <button className="btn-primary" type="submit" disabled={busy}>
            {busy ? <Spinner /> : <Icon name="logout" size={16} />}
            {t.login.submit}
          </button>
        </form>
      </div>
    </div>
  )
}
