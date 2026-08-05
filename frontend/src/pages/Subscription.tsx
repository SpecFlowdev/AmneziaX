import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import QRCode from 'qrcode'
import { Icon } from '../components/icons'
import { Brand } from '../components/Brand'
import { ImportButtons } from '../components/ImportButtons'
import { CopyButton, Meter, Spinner } from '../components/ui'
import { useI18n, type Lang } from '../i18n'
import type { SubscriptionInfo } from '../lib/api'
import { bytes, dateTime } from '../lib/format'
import { useTheme } from '../lib/theme'

/**
 * The subscriber-facing page. It is served from the same SPA but never touches
 * the admin API — the subscription token in the URL is the only credential.
 */
export function Subscription() {
  const { token } = useParams<{ token: string }>()
  const { t, lang, setLang, f } = useI18n()
  const { theme, setTheme } = useTheme()

  const [info, setInfo] = useState<SubscriptionInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [qr, setQr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetch(`/sub/${token}/info`)
      .then(async (res) => {
        if (res.status === 404) throw new Error(t.sub.notFound)
        if (res.status === 403) throw new Error(t.sub.disabled)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return (await res.json()) as SubscriptionInfo
      })
      .then((data) => {
        if (!cancelled) setInfo(data)
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message)
      })
    return () => {
      cancelled = true
    }
  }, [token, t])

  useEffect(() => {
    if (!info) return
    QRCode.toDataURL(info.subscriptionUrl, { margin: 1, width: 260 })
      .then(setQr)
      .catch(() => setQr(null))
  }, [info])

  const remaining =
    info && info.trafficLimitBytes > 0
      ? Math.max(0, info.trafficLimitBytes - info.usedTrafficBytes)
      : null

  return (
    <div className="center-screen">
      <div style={{ width: 'min(560px, 100%)', display: 'flex', flexDirection: 'column', gap: 14 }}>
        <div className="split">
          <Brand showTagline={false} />
          <div style={{ flex: 1 }} />
          <div className="tabs">
            {(['ru', 'en'] as Lang[]).map((l) => (
              <button
                key={l}
                className={`tab${lang === l ? ' active' : ''}`}
                onClick={() => setLang(l)}
              >
                {l === 'ru' ? 'РУС' : 'ENG'}
              </button>
            ))}
          </div>
          <button
            className="btn-ghost btn-icon"
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            aria-label={t.common.theme}
          >
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} size={17} />
          </button>
        </div>

        {error ? (
          <div className="card card-pad">
            <div className="badge badge-danger" style={{ padding: '10px 14px' }}>
              <Icon name="alert" size={16} />
              {error}
            </div>
          </div>
        ) : !info ? (
          <div className="card card-pad" style={{ display: 'grid', placeItems: 'center', height: 200 }}>
            <Spinner />
          </div>
        ) : (
          <>
            <div className="card card-pad stack" style={{ gap: 14 }}>
              <div className="split">
                <strong style={{ fontSize: 17 }}>{info.username}</strong>
                <div style={{ flex: 1 }} />
                <span className={`badge badge-${info.status === 'ACTIVE' ? 'ok' : 'warn'}`}>
                  <span className="dot" />
                  {info.status}
                </span>
              </div>

              <div className="stack" style={{ gap: 7 }}>
                <div className="split small">
                  <span className="muted">{t.sub.used}</span>
                  <div style={{ flex: 1 }} />
                  <span className="nums">
                    {bytes(info.usedTrafficBytes)}
                    {info.trafficLimitBytes > 0 && ` / ${bytes(info.trafficLimitBytes)}`}
                  </span>
                </div>
                {info.trafficLimitBytes > 0 ? (
                  <>
                    <Meter used={info.usedTrafficBytes} limit={info.trafficLimitBytes} />
                    <span className="small dim">
                      {bytes(remaining ?? 0)} {t.sub.remaining}
                    </span>
                  </>
                ) : (
                  <span className="small dim">{t.common.unlimited}</span>
                )}
              </div>

              {info.expireAt && (
                <div className="split small">
                  <span className="muted">{t.sub.expires}</span>
                  <div style={{ flex: 1 }} />
                  <span>
                    {dateTime(info.expireAt, lang)}
                    {typeof info.daysLeft === 'number' && info.daysLeft >= 0 && (
                      <span className="dim"> · {f(t.users.daysLeft, { n: info.daysLeft })}</span>
                    )}
                  </span>
                </div>
              )}
            </div>

            {(info.announcements ?? []).map((a) => (
              <div
                key={a.uuid}
                className="card card-pad stack"
                style={{ gap: 6, borderLeft: `3px solid ${
                  a.level === 'DANGER'
                    ? 'var(--danger)'
                    : a.level === 'WARNING'
                      ? 'var(--warn)'
                      : 'var(--accent)'
                }` }}
              >
                {a.title && <strong>{a.title}</strong>}
                <span className="small" style={{ whiteSpace: 'pre-wrap' }}>{a.body}</span>
              </div>
            ))}

            <div className="card card-pad stack" style={{ gap: 14, alignItems: 'center' }}>
              <span className="small muted">{t.sub.import}</span>
              {qr && (
                <img
                  src={qr}
                  alt="QR"
                  width={230}
                  height={230}
                  style={{ borderRadius: 12, background: '#fff', padding: 8 }}
                />
              )}
              <div className="split" style={{ width: '100%' }}>
                <pre className="code-block" style={{ flex: 1, margin: 0 }}>
                  {info.subscriptionUrl}
                </pre>
                <CopyButton value={info.subscriptionUrl} />
              </div>
              <CopyButton value={info.subscriptionUrl} label={t.sub.copyLink} />

              <ImportButtons subscriptionUrl={info.subscriptionUrl} />

              {/* The link above already gives every client the right format.
                  These pin one explicitly, for an app that asks for a
                  particular file or does not identify itself. */}
              {info.showFormats !== false && (
              <div className="stack" style={{ gap: 6, width: '100%', alignItems: 'center' }}>
                <span className="small dim">{t.sub.formats}</span>
                <div className="split" style={{ gap: 8, flexWrap: 'wrap', justifyContent: 'center' }}>
                  {(
                    [
                      ['base64', 'Base64'],
                      ['json', 'Xray JSON'],
                      ['clash', 'Clash'],
                      ['singbox', 'sing-box'],
                      ['wireguard', 'WireGuard'],
                    ] as const
                  ).map(([key, label]) => (
                    <a
                      key={key}
                      className="pill"
                      href={`${info.subscriptionUrl}?format=${key}`}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {label}
                    </a>
                  ))}
                </div>
              </div>
              )}
            </div>

            {info.showLinks !== false && info.links.length > 0 && (
              <div className="card card-pad stack" style={{ gap: 9 }}>
                <span className="small muted">{t.users.links}</span>
                {info.links.map((link, i) => (
                  <div className="split" key={i}>
                    <pre className="code-block" style={{ flex: 1, margin: 0 }}>
                      {link}
                    </pre>
                    <CopyButton value={link} />
                  </div>
                ))}
              </div>
            )}

            {info.supportUrl && (
              <a className="btn" href={info.supportUrl} target="_blank" rel="noreferrer">
                <Icon name="link" size={15} />
                {info.supportUrl}
              </a>
            )}
          </>
        )}
      </div>
    </div>
  )
}
