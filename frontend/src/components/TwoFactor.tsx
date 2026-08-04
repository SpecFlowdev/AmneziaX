import { useCallback, useEffect, useState } from 'react'
import QRCode from 'qrcode'
import { Icon } from './icons'
import { Badge, CopyButton, Field, Modal, Spinner, useAction, useToast } from './ui'
import { useI18n } from '../i18n'
import { api } from '../lib/api'

interface Status {
  enabled: boolean
  confirmedAt: string | null
  recoveryCodesLeft: number
  requiredByPanel: boolean
}

/**
 * Enrolment is two steps on purpose: the secret is staged, and only a code the
 * app actually produced turns it on. Anything shorter risks an operator
 * scanning a QR into an app that never worked and locking themselves out.
 */
export function TwoFactorCard() {
  const { t, f } = useI18n()
  const run = useAction()
  const { push } = useToast()

  const [status, setStatus] = useState<Status | null>(null)
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null)
  const [qr, setQr] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)

  // Held in state, never re-fetchable: the server shows these once.
  const [recovery, setRecovery] = useState<string[] | null>(null)
  const [password, setPassword] = useState('')
  const [asking, setAsking] = useState<'disable' | 'regenerate' | null>(null)

  const load = useCallback(async () => {
    try {
      setStatus(await api.get<Status>('/api/totp'))
    } catch {
      setStatus(null)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!setup) {
      setQr(null)
      return
    }
    QRCode.toDataURL(setup.uri, { margin: 1, width: 240 })
      .then(setQr)
      .catch(() => setQr(null))
  }, [setup])

  async function start() {
    setBusy(true)
    try {
      setSetup(await api.post<{ secret: string; uri: string }>('/api/totp/start', {}))
      setCode('')
    } catch (err) {
      push(err instanceof Error ? err.message : 'error', 'error')
    } finally {
      setBusy(false)
    }
  }

  async function confirm() {
    setBusy(true)
    try {
      const res = await api.post<{ recoveryCodes: string[] }>('/api/totp/confirm', {
        code: code.trim(),
      })
      setRecovery(res.recoveryCodes)
      setSetup(null)
      setCode('')
      await load()
    } catch (err) {
      push(err instanceof Error ? err.message : 'error', 'error')
    } finally {
      setBusy(false)
    }
  }

  async function submitPassword() {
    if (asking === 'disable') {
      const ok = await run(() => api.post('/api/totp/disable', { password }), t.totp.off)
      if (ok) {
        setAsking(null)
        setPassword('')
        await load()
      }
      return
    }
    setBusy(true)
    try {
      const res = await api.post<{ recoveryCodes: string[] }>('/api/totp/recovery-codes', {
        password,
      })
      setRecovery(res.recoveryCodes)
      setAsking(null)
      setPassword('')
      await load()
    } catch (err) {
      push(err instanceof Error ? err.message : 'error', 'error')
    } finally {
      setBusy(false)
    }
  }

  function download() {
    if (!recovery) return
    const blob = new Blob([recovery.join('\n') + '\n'], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'amneziax-recovery-codes.txt'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="card">
      <div className="card-head">
        <Icon name="shield" size={17} />
        <h3>{t.totp.title}</h3>
        <div style={{ flex: 1 }} />
        {status && (
          <Badge kind={status.enabled ? 'ok' : 'muted'}>
            {status.enabled ? t.totp.on : t.totp.off}
          </Badge>
        )}
      </div>

      <div className="card-pad stack" style={{ gap: 12 }}>
        <p className="small dim" style={{ margin: 0 }}>
          {t.totp.subtitle}
        </p>

        {!status ? (
          <Spinner />
        ) : status.enabled ? (
          <>
            <div className="split small">
              <span className="muted">{t.totp.recoveryTitle}</span>
              <div style={{ flex: 1 }} />
              <span className={status.recoveryCodesLeft <= 2 ? 'warn' : ''}>
                {f(t.totp.recoveryLeft, { n: status.recoveryCodesLeft })}
              </span>
            </div>
            {status.recoveryCodesLeft <= 2 && (
              <span className="small warn">{t.totp.lowCodes}</span>
            )}
            <div className="split" style={{ gap: 8, flexWrap: 'wrap' }}>
              <button className="btn" onClick={() => setAsking('regenerate')}>
                {t.totp.regenerate}
              </button>
              {status.requiredByPanel ? (
                <span className="small dim">{t.totp.requiredByPanel}</span>
              ) : (
                <button className="btn-ghost" onClick={() => setAsking('disable')}>
                  {t.totp.disable}
                </button>
              )}
            </div>
          </>
        ) : setup ? (
          <>
            <span className="small muted">{t.totp.scan}</span>
            {qr && (
              <img
                src={qr}
                alt="QR"
                width={200}
                height={200}
                style={{ borderRadius: 10, background: '#fff', padding: 8, alignSelf: 'center' }}
              />
            )}
            <span className="small dim">{t.totp.manual}</span>
            <div className="split">
              <pre className="code-block" style={{ flex: 1, margin: 0 }}>
                {setup.secret}
              </pre>
              <CopyButton value={setup.secret} />
            </div>
            <Field label={t.totp.confirm}>
              <input
                value={code}
                onChange={(e) => setCode(e.target.value)}
                autoComplete="one-time-code"
                style={{ fontFamily: 'var(--mono)', letterSpacing: '0.14em' }}
              />
            </Field>
            <div className="split" style={{ gap: 8 }}>
              <button className="btn-primary" onClick={() => void confirm()} disabled={busy}>
                {busy ? <Spinner /> : null}
                {t.totp.confirmBtn}
              </button>
              <button className="btn-ghost" onClick={() => setSetup(null)}>
                {t.totp.cancel}
              </button>
            </div>
          </>
        ) : (
          <>
            {status.requiredByPanel && <span className="small warn">{t.totp.enrolNow}</span>}
            <button className="btn-primary" onClick={() => void start()} disabled={busy}>
              {busy ? <Spinner /> : <Icon name="shield" size={15} />}
              {t.totp.enable}
            </button>
          </>
        )}
      </div>

      {/* Shown once. Closing this dialog is the last chance to write them down. */}
      {recovery !== null && (
      <Modal title={t.totp.recoveryTitle} onClose={() => setRecovery(null)}>
        <div className="stack" style={{ gap: 12 }}>
          <span className="small warn">{t.totp.recoveryHint}</span>
          <pre className="code-block" style={{ margin: 0, lineHeight: 1.8 }}>
            {(recovery ?? []).join('\n')}
          </pre>
          <div className="split" style={{ gap: 8 }}>
            <button className="btn" onClick={download}>
              <Icon name="download" size={15} />
              {t.totp.download}
            </button>
            <CopyButton value={(recovery ?? []).join('\n')} />
            <div style={{ flex: 1 }} />
            <button className="btn-primary" onClick={() => setRecovery(null)}>
              {t.totp.done}
            </button>
          </div>
        </div>
      </Modal>
      )}

      {asking !== null && (
      <Modal title={t.totp.passwordPrompt} onClose={() => setAsking(null)}>
        <div className="stack" style={{ gap: 12 }}>
          <Field label={t.login.password}>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              autoFocus
            />
          </Field>
          <div className="split" style={{ gap: 8 }}>
            <button className="btn-primary" onClick={() => void submitPassword()} disabled={busy}>
              {busy ? <Spinner /> : null}
              {t.common.save}
            </button>
            <button className="btn-ghost" onClick={() => setAsking(null)}>
              {t.totp.cancel}
            </button>
          </div>
        </div>
      </Modal>
      )}
    </div>
  )
}
