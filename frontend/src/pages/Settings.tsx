import { useEffect, useRef, useState } from 'react'
import { Icon } from '../components/icons'
import {
  Badge,
  ConfirmDialog,
  CopyButton,
  EmptyState,
  Field,
  Modal,
  Spinner,
  useAction,
  useToast,
} from '../components/ui'
import { TwoFactorCard } from '../components/TwoFactor'
import { useI18n, type Lang } from '../i18n'
import { api, type ApiToken, type Overview, type Settings as PanelSettings } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useBranding } from '../lib/branding'
import { dateTime, duration } from '../lib/format'
import { useTheme } from '../lib/theme'
import { useFetch } from '../lib/useApi'

const MAX_LOGO_BYTES = 180 * 1024

export function Settings() {
  const { t, lang, setLang } = useI18n()
  const { canWrite, isOwner } = useAuth()
  const { theme, setTheme } = useTheme()

  const overview = useFetch<Overview>('/api/system/overview')

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2 style={{ fontSize: 22 }}>{t.settings.title}</h2>
          <p>{t.settings.subtitle}</p>
        </div>
      </div>

      <div className="grid cols-2">
        <div className="stack" style={{ gap: 16 }}>
          {canWrite && <BrandingCard />}
          <AccountCard />
          <TwoFactorCard />
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

          {isOwner && <TokensCard />}

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

function BrandingCard() {
  const { t } = useI18n()
  const { isOwner } = useAuth()
  const { reload } = useBranding()
  const run = useAction()
  const { push } = useToast()
  const fileInput = useRef<HTMLInputElement>(null)

  const [draft, setDraft] = useState<PanelSettings | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api
      .get<PanelSettings>('/api/settings')
      .then(setDraft)
      .catch(() => setDraft(null))
  }, [])

  const set = <K extends keyof PanelSettings>(key: K, value: PanelSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: value } : d))

  function pickLogo(file: File) {
    if (file.size > MAX_LOGO_BYTES) {
      push(t.settings.brandLogoHint, 'error')
      return
    }
    const reader = new FileReader()
    reader.onload = () => set('brandLogo', String(reader.result ?? ''))
    reader.onerror = () => push(t.settings.brandLogoHint, 'error')
    // A data URI keeps the logo in the database, so a deployment needs no
    // object storage and the logo survives a container rebuild.
    reader.readAsDataURL(file)
  }

  async function save() {
    if (!draft) return
    setBusy(true)
    const ok = await run(async () => {
      const saved = await api.put<PanelSettings>('/api/settings', draft)
      setDraft(saved)
      await reload()
    }, t.settings.saved)
    setBusy(false)
    if (!ok) return
  }

  if (!draft) {
    return (
      <div className="card card-pad" style={{ display: 'grid', placeItems: 'center', height: 160 }}>
        <Spinner />
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-head">
        <Icon name="qr" size={17} />
        <h3>{t.settings.branding}</h3>
      </div>
      <div className="card-pad stack" style={{ gap: 14 }}>
        <p className="small muted" style={{ margin: 0 }}>
          {t.settings.brandingHint}
        </p>

        <div className="split" style={{ gap: 14, alignItems: 'flex-start' }}>
          <div
            style={{
              width: 64,
              height: 64,
              borderRadius: 14,
              display: 'grid',
              placeItems: 'center',
              flex: 'none',
              overflow: 'hidden',
              background: draft.brandLogo ? 'var(--surface-2)' : undefined,
            }}
            className={draft.brandLogo ? '' : 'brand-mark'}
          >
            {draft.brandLogo ? (
              <img
                src={draft.brandLogo}
                alt=""
                style={{ width: '100%', height: '100%', objectFit: 'cover' }}
              />
            ) : (
              <Icon name="qr" size={26} />
            )}
          </div>
          <div className="stack" style={{ gap: 8, flex: 1 }}>
            <input
              ref={fileInput}
              type="file"
              accept="image/png,image/jpeg,image/svg+xml,image/webp"
              style={{ display: 'none' }}
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) pickLogo(file)
                e.target.value = ''
              }}
            />
            <div className="split" style={{ gap: 8 }}>
              <button className="btn-sm" onClick={() => fileInput.current?.click()}>
                <Icon name="download" size={14} />
                {t.settings.uploadLogo}
              </button>
              {draft.brandLogo && (
                <button className="btn-sm btn-ghost" onClick={() => set('brandLogo', '')}>
                  {t.settings.removeLogo}
                </button>
              )}
            </div>
            <span className="hint">{t.settings.brandLogoHint}</span>
          </div>
        </div>

        <Field label={t.settings.brandName}>
          <input value={draft.brandName} onChange={(e) => set('brandName', e.target.value)} />
        </Field>
        <Field label={t.settings.brandTagline} hint={t.settings.brandTaglineHint}>
          <input value={draft.brandTagline} onChange={(e) => set('brandTagline', e.target.value)} />
        </Field>

        <div className="grid cols-2">
          <Field label={t.settings.brandAccent} hint={t.settings.brandAccentHint}>
            <div className="split" style={{ gap: 8 }}>
              <input
                type="color"
                value={draft.brandAccent || '#e11d48'}
                onChange={(e) => set('brandAccent', e.target.value)}
                style={{ width: 46, padding: 3, height: 36 }}
              />
              <input
                value={draft.brandAccent}
                placeholder="#e11d48"
                onChange={(e) => set('brandAccent', e.target.value)}
              />
              {draft.brandAccent && (
                <button className="btn-ghost btn-icon" onClick={() => set('brandAccent', '')}>
                  <Icon name="x" size={15} />
                </button>
              )}
            </div>
          </Field>
          <Field label={t.settings.currency}>
            <input
              value={draft.currency}
              maxLength={8}
              onChange={(e) => set('currency', e.target.value.toUpperCase())}
            />
          </Field>
        </div>

        <Field label={t.settings.subscriptionTitle} hint={t.settings.subscriptionTitleHint}>
          <input
            value={draft.subscriptionTitle}
            placeholder={draft.brandName}
            onChange={(e) => set('subscriptionTitle', e.target.value)}
          />
        </Field>
        <Field label={t.settings.supportUrl} hint={t.common.optional}>
          <input value={draft.supportUrl} onChange={(e) => set('supportUrl', e.target.value)} />
        </Field>
        <Field label={t.settings.subscriptionFormat} hint={t.settings.subscriptionFormatHint}>
          <select
            value={draft.subscriptionFormat ?? ''}
            onChange={(e) => set('subscriptionFormat', e.target.value)}
          >
            <option value="">{t.settings.subscriptionFormatAuto}</option>
            <option value="json">Xray JSON</option>
            <option value="base64">Base64</option>
            <option value="plain">{t.settings.subscriptionFormatPlain}</option>
            <option value="clash">Clash / Mihomo</option>
            <option value="singbox">sing-box</option>
          </select>
        </Field>

        {isOwner && (
          <Field label={t.totp.title} hint={t.totp.requireAllHint}>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={draft.requireTotp ?? false}
                onChange={(e) => set('requireTotp', e.target.checked)}
              />
              {t.totp.requireAll}
            </label>
          </Field>
        )}

        <Field label={t.settings.subPage} hint={t.settings.subPageHint}>
          <div className="stack" style={{ gap: 8 }}>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={draft.subPageShowLinks ?? true}
                onChange={(e) => set('subPageShowLinks', e.target.checked)}
              />
              {t.settings.subPageLinks}
            </label>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={draft.subPageShowFormats ?? true}
                onChange={(e) => set('subPageShowFormats', e.target.checked)}
              />
              {t.settings.subPageFormats}
            </label>
          </div>
        </Field>

        <Field label={t.settings.clashTemplate} hint={t.settings.templateHint}>
          <textarea
            value={draft.clashTemplate ?? ''}
            onChange={(e) => set('clashTemplate', e.target.value)}
            rows={4}
            placeholder={'proxies:\n{{PROXIES}}'}
          />
        </Field>
        <Field label={t.settings.singboxTemplate} hint={t.settings.templateHint}>
          <textarea
            value={draft.singboxTemplate ?? ''}
            onChange={(e) => set('singboxTemplate', e.target.value)}
            rows={4}
            placeholder={'{"outbounds": [{{OUTBOUNDS}}]}'}
          />
        </Field>

        <button className="btn-primary" onClick={() => void save()} disabled={busy}>
          {busy && <Spinner />}
          {t.common.save}
        </button>
      </div>
    </div>
  )
}

function AccountCard() {
  const { t, lang } = useI18n()
  const { admin } = useAuth()
  const run = useAction()
  const { push } = useToast()

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
  )
}

function TokensCard() {
  const { t, lang, f } = useI18n()
  const run = useAction()
  const tokens = useFetch<ApiToken[]>('/api/tokens')

  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [issued, setIssued] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<ApiToken | null>(null)

  return (
    <div className="card">
      <div className="card-head">
        <Icon name="key" size={17} />
        <h3>{t.settings.apiTokens}</h3>
        <div className="spacer" />
        <button className="btn-sm" onClick={() => setCreating(true)}>
          <Icon name="plus" size={14} />
          {t.settings.newToken}
        </button>
      </div>
      {(tokens.data ?? []).length === 0 ? (
        <EmptyState title={t.common.nothingHere} hint={t.settings.apiTokensHint} />
      ) : (
        <div className="table-wrap">
          <table>
            <tbody>
              {(tokens.data ?? []).map((tok) => (
                <tr key={tok.uuid}>
                  <td>
                    <div className="stack">
                      <strong>{tok.name}</strong>
                      <span className="small dim mono">{tok.tokenPreview}</span>
                    </div>
                  </td>
                  <td className="small dim">
                    {tok.lastUsedAt
                      ? `${t.settings.lastUsed}: ${dateTime(tok.lastUsedAt, lang)}`
                      : t.common.never}
                  </td>
                  <td style={{ width: 1 }}>
                    <button
                      className="btn-sm btn-ghost btn-icon btn-danger"
                      onClick={() => setConfirmDelete(tok)}
                    >
                      <Icon name="trash" size={15} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {creating && (
        <Modal
          title={t.settings.newToken}
          onClose={() => setCreating(false)}
          footer={
            <>
              <button onClick={() => setCreating(false)}>{t.common.cancel}</button>
              <button
                className="btn-primary"
                disabled={!name.trim()}
                onClick={() =>
                  void run(async () => {
                    const res = await api.post<{ token: string }>('/api/tokens', {
                      name: name.trim(),
                      expiresAt: null,
                    })
                    setIssued(res.token)
                    setCreating(false)
                    setName('')
                    await tokens.reload()
                  })
                }
              >
                {t.common.create}
              </button>
            </>
          }
        >
          <p className="small muted" style={{ margin: 0 }}>
            {t.settings.apiTokensHint}
          </p>
          <Field label={t.settings.tokenName}>
            <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          </Field>
        </Modal>
      )}

      {issued && (
        <Modal
          title={t.settings.newToken}
          onClose={() => setIssued(null)}
          footer={
            <button className="btn-primary" onClick={() => setIssued(null)}>
              {t.common.close}
            </button>
          }
        >
          <div className="badge badge-warn" style={{ padding: '8px 12px' }}>
            <Icon name="alert" size={15} />
            {t.settings.tokenCreated}
          </div>
          <div className="split">
            <pre className="code-block" style={{ flex: 1, margin: 0 }}>
              {issued}
            </pre>
            <CopyButton value={issued} />
          </div>
        </Modal>
      )}

      {confirmDelete && (
        <ConfirmDialog
          danger
          message={f(t.common.deleteConfirm, { name: confirmDelete.name })}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => {
            const target = confirmDelete
            setConfirmDelete(null)
            void run(async () => {
              await api.del(`/api/tokens/${target.uuid}`)
              await tokens.reload()
            })
          }}
        />
      )}
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
