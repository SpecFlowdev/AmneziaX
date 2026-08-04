import { useState, type FormEvent } from 'react'
import { Icon } from '../components/icons'
import { Brand } from '../components/Brand'
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
  const [code, setCode] = useState('')
  // The second step is its own screen rather than an always-visible field, so
  // an account without two-factor never sees a box it cannot fill in.
  const [needCode, setNeedCode] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const result = await login(username.trim(), password, needCode ? code.trim() : undefined)
      if (result === 'totp') {
        setNeedCode(true)
        setError(null)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t.login.failed)
      // A rejected code leaves the operator on the code step to try again;
      // only a rejected password sends them back to the start.
      if (!needCode) setPassword('')
      setCode('')
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
          <Brand />

          <div className="stack">
            <h2 style={{ fontSize: 20 }}>{needCode ? t.login.codeTitle : t.login.title}</h2>
            <p className="muted small" style={{ margin: 0 }}>
              {needCode ? t.login.codeSubtitle : t.login.subtitle}
            </p>
          </div>

          {needCode ? (
            <Field label={t.login.code} hint={t.login.codeHint}>
              <input
                value={code}
                onChange={(e) => setCode(e.target.value)}
                autoComplete="one-time-code"
                inputMode="text"
                autoFocus
                required
                style={{ fontFamily: 'var(--mono)', letterSpacing: '0.14em' }}
              />
            </Field>
          ) : (
            <>
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
            </>
          )}

          {error && (
            <div className="badge badge-danger" style={{ padding: '8px 12px' }}>
              <Icon name="alert" size={15} />
              {error}
            </div>
          )}

          <button className="btn-primary" type="submit" disabled={busy}>
            {busy ? <Spinner /> : <Icon name={needCode ? 'shield' : 'logout'} size={16} />}
            {needCode ? t.login.verify : t.login.submit}
          </button>

          {needCode && (
            <button
              type="button"
              className="btn-ghost"
              onClick={() => {
                setNeedCode(false)
                setCode('')
                setPassword('')
                setError(null)
              }}
            >
              {t.common.back}
            </button>
          )}
        </form>
      </div>
    </div>
  )
}
