import { useState } from 'react'
import { Icon } from '../components/icons'
import { Badge, Field, Spinner, useAction, useToast } from '../components/ui'
import { useI18n, type Lang } from '../i18n'
import { api, type Overview } from '../lib/api'
import { useAuth } from '../lib/auth'
import { dateTime, duration } from '../lib/format'
import { useTheme } from '../lib/theme'
import { useFetch } from '../lib/useApi'

export function Settings() {
  const { t, lang, setLang } = useI18n()
  const { admin } = useAuth()
  const { theme, setTheme } = useTheme()
  const run = useAction()
  const { push } = useToast()

  const overview = useFetch<Overview>('/api/system/overview')

  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [busy, setBusy] = useState(false)

  async function changePassword() {
    if (next !== repeat) {
      push(t.settings.passwordMismatch, 'error')
      return
    }
    setBusy(true)
    const ok = await run(
      () => api.post('/api/auth/password', { currentPassword: current, newPassword: next }),
      t.settings.passwordChanged,
    )
    setBusy(false)
    if (ok) {
      setCurrent('')
      setNext('')
      setRepeat('')
    }
  }

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2 style={{ fontSize: 22 }}>{t.settings.title}</h2>
          <p>{t.settings.subtitle}</p>
        </div>
      </div>

      <div className="grid cols-2">
        <div className="card">
          <div className="card-head">
            <Icon name="shield" size={17} />
            <h3>{t.settings.account}</h3>
          </div>
          <div className="card-pad stack" style={{ gap: 12 }}>
            <div className="split">
              <span className="muted">{t.login.username}</span>
              <div style={{ flex: 1 }} />
              <strong>{admin?.username}</strong>
            </div>
            <div className="split">
              <span className="muted">{t.admins.role}</span>
              <div style={{ flex: 1 }} />
              <Badge kind={admin?.role === 'OWNER' ? 'accent' : 'muted'}>
                {admin?.role === 'OWNER'
                  ? t.admins.roleOwner
                  : admin?.role === 'ADMIN'
                    ? t.admins.roleAdmin
                    : t.admins.roleViewer}
              </Badge>
            </div>
            <div className="split">
              <span className="muted">{t.admins.lastLogin}</span>
              <div style={{ flex: 1 }} />
              <span className="small">
                {admin?.lastLoginAt ? dateTime(admin.lastLoginAt, lang) : t.common.never}
              </span>
            </div>

            <hr className="sep" />
            <h4 style={{ fontSize: 13.5 }}>{t.settings.changePassword}</h4>
            <Field label={t.settings.currentPassword}>
              <input
                type="password"
                value={current}
                autoComplete="current-password"
                onChange={(e) => setCurrent(e.target.value)}
              />
            </Field>
            <Field label={t.settings.newPassword} hint="≥ 8">
              <input
                type="password"
                value={next}
                autoComplete="new-password"
                onChange={(e) => setNext(e.target.value)}
              />
            </Field>
            <Field label={t.settings.repeatPassword}>
              <input
                type="password"
                value={repeat}
                autoComplete="new-password"
                onChange={(e) => setRepeat(e.target.value)}
              />
            </Field>
            <button
              className="btn-primary"
              onClick={() => void changePassword()}
              disabled={busy || next.length < 8 || !current}
            >
              {busy && <Spinner />}
              {t.settings.changePassword}
            </button>
          </div>
        </div>

        <div className="stack" style={{ gap: 16 }}>
          <div className="card">
            <div className="card-head">
              <Icon name="sun" size={17} />
              <h3>{t.settings.appearance}</h3>
            </div>
            <div className="card-pad stack" style={{ gap: 14 }}>
              <Field label={t.common.theme}>
                <div className="tabs">
                  {(
                    [
                      ['dark', t.settings.themeDark],
                      ['light', t.settings.themeLight],
                    ] as const
                  ).map(([value, label]) => (
                    <button
                      key={value}
                      className={`tab${theme === value ? ' active' : ''}`}
                      onClick={() => setTheme(value)}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </Field>
              <Field label={t.common.language}>
                <div className="tabs">
                  {(
                    [
                      ['ru', 'Русский'],
                      ['en', 'English'],
                    ] as [Lang, string][]
                  ).map(([value, label]) => (
                    <button
                      key={value}
                      className={`tab${lang === value ? ' active' : ''}`}
                      onClick={() => setLang(value)}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </Field>
            </div>
          </div>

          <div className="card">
            <div className="card-head">
              <Icon name="info" size={17} />
              <h3>{t.settings.about}</h3>
            </div>
            <div className="card-pad stack" style={{ gap: 9 }}>
              {overview.data && (
                <>
                  <Row label={t.dashboard.panelVersion} value={overview.data.panel.version} />
                  <Row label="Commit" value={overview.data.panel.commit} />
                  <Row label="Go" value={overview.data.panel.goVersion} />
                  <Row
                    label={t.dashboard.uptime}
                    value={duration(overview.data.panel.uptimeSeconds)}
                  />
                  <Row
                    label={t.nav.nodes}
                    value={`${overview.data.counters.nodesOnline} / ${overview.data.counters.nodesTotal}`}
                  />
                  <Row label={t.nav.users} value={String(overview.data.counters.usersTotal)} />
                  <Row label={t.nav.hosts} value={String(overview.data.counters.hostsTotal)} />
                  <Row label={t.nav.squads} value={String(overview.data.counters.squadsTotal)} />
                </>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="split">
      <span className="muted small">{label}</span>
      <div style={{ flex: 1 }} />
      <span className="mono small">{value}</span>
    </div>
  )
}
