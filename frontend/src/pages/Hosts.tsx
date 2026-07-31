import { useState } from 'react'
import { Icon } from '../components/icons'
import {
  Badge,
  ConfirmDialog,
  EmptyState,
  Field,
  Modal,
  Spinner,
  useAction,
} from '../components/ui'
import { useI18n } from '../i18n'
import { api, type Host, type Inbound } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useFetch } from '../lib/useApi'

const SECURITIES = ['none', 'tls', 'reality']
const FINGERPRINTS = ['', 'chrome', 'firefox', 'safari', 'ios', 'android', 'edge', 'random', 'randomized']

interface Draft {
  inboundUuid: string
  remark: string
  address: string
  port: string
  path: string
  sni: string
  hostHeader: string
  alpn: string
  fingerprint: string
  publicKey: string
  shortId: string
  spiderX: string
  flow: string
  security: string
  allowInsecure: boolean
  tags: string
  isDisabled: boolean
}

export function Hosts() {
  const { t, f } = useI18n()
  const { canWrite } = useAuth()
  const run = useAction()

  const hosts = useFetch<Host[]>('/api/hosts')
  const inbounds = useFetch<Inbound[]>('/api/profiles/inbounds')

  const [editing, setEditing] = useState<Host | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Host | null>(null)

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.hosts.title}</h2>
          <p>{t.hosts.subtitle}</p>
        </div>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.hosts.add}
          </button>
        )}
      </div>

      <div className="card">
        {hosts.loading && !hosts.data ? (
          <div style={{ display: 'grid', placeItems: 'center', height: 160 }}>
            <Spinner />
          </div>
        ) : (hosts.data ?? []).length === 0 ? (
          <EmptyState title={t.common.nothingHere} hint={t.hosts.subtitle} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t.hosts.remark}</th>
                  <th>{t.hosts.address}</th>
                  <th>{t.hosts.inbound}</th>
                  <th>{t.hosts.security}</th>
                  <th>{t.common.status}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {(hosts.data ?? []).map((h) => (
                  <tr key={h.uuid}>
                    <td>
                      <div className="stack">
                        <span style={{ fontWeight: 600 }}>{h.remark || h.inboundTag}</span>
                        {h.tags.length > 0 && (
                          <div className="split" style={{ gap: 4 }}>
                            {h.tags.map((tag) => (
                              <span key={tag} className="pill">
                                {tag}
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="mono">
                      {h.address}:{h.port}
                    </td>
                    <td>
                      <div className="stack">
                        <span>{h.inboundTag}</span>
                        <span className="small dim">{h.configProfileName}</span>
                      </div>
                    </td>
                    <td>
                      <div className="split" style={{ gap: 5, flexWrap: 'wrap' }}>
                        <span className="pill">{h.security || 'none'}</span>
                        {h.sni && <span className="pill">SNI {h.sni}</span>}
                        {h.flow && <span className="pill">{h.flow}</span>}
                      </div>
                    </td>
                    <td>
                      {h.isDisabled ? (
                        <Badge kind="muted" dot>
                          {t.common.disabled}
                        </Badge>
                      ) : (
                        <Badge kind="ok" dot>
                          {t.common.enabled}
                        </Badge>
                      )}
                    </td>
                    <td>
                      <div className="row-actions">
                        {canWrite && (
                          <>
                            <button
                              className="btn-sm btn-ghost btn-icon"
                              onClick={() => setEditing(h)}
                              title={t.common.edit}
                            >
                              <Icon name="edit" size={15} />
                            </button>
                            <button
                              className="btn-sm btn-ghost btn-icon btn-danger"
                              onClick={() => setConfirmDelete(h)}
                              title={t.common.delete}
                            >
                              <Icon name="trash" size={15} />
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {editing && (
        <HostEditor
          host={editing === 'new' ? null : editing}
          inbounds={inbounds.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await hosts.reload()
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          danger
          message={f(t.common.deleteConfirm, {
            name: confirmDelete.remark || confirmDelete.address,
          })}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => {
            const target = confirmDelete
            setConfirmDelete(null)
            void run(async () => {
              await api.del(`/api/hosts/${target.uuid}`)
              await hosts.reload()
            })
          }}
        />
      )}
    </div>
  )
}

function HostEditor({
  host,
  inbounds,
  onClose,
  onSaved,
}: {
  host: Host | null
  inbounds: Inbound[]
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const run = useAction()
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState<Draft>(() => ({
    inboundUuid: host?.inboundUuid ?? inbounds[0]?.uuid ?? '',
    remark: host?.remark ?? '',
    address: host?.address ?? '',
    port: String(host?.port ?? 443),
    path: host?.path ?? '',
    sni: host?.sni ?? '',
    hostHeader: host?.hostHeader ?? '',
    alpn: host?.alpn ?? '',
    fingerprint: host?.fingerprint ?? 'chrome',
    publicKey: host?.publicKey ?? '',
    shortId: host?.shortId ?? '',
    spiderX: host?.spiderX ?? '',
    flow: host?.flow ?? '',
    security: host?.security ?? 'reality',
    allowInsecure: host?.allowInsecure ?? false,
    tags: (host?.tags ?? []).join(', '),
    isDisabled: host?.isDisabled ?? false,
  }))

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((d) => ({ ...d, [key]: value }))

  const selectedInbound = inbounds.find((i) => i.uuid === draft.inboundUuid)
  const isReality = draft.security === 'reality'

  async function save() {
    setBusy(true)
    const body = {
      inboundUuid: draft.inboundUuid,
      remark: draft.remark,
      address: draft.address.trim(),
      port: Number.parseInt(draft.port, 10) || 443,
      path: draft.path,
      sni: draft.sni,
      hostHeader: draft.hostHeader,
      alpn: draft.alpn,
      fingerprint: draft.fingerprint,
      publicKey: draft.publicKey.trim(),
      shortId: draft.shortId.trim(),
      spiderX: draft.spiderX,
      flow: draft.flow,
      security: draft.security,
      allowInsecure: draft.allowInsecure,
      tags: draft.tags
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
      isDisabled: draft.isDisabled,
      viewPosition: host?.viewPosition ?? 0,
    }
    await run(async () => {
      if (host) await api.put(`/api/hosts/${host.uuid}`, body)
      else await api.post('/api/hosts', body)
      onSaved()
    })
    setBusy(false)
  }

  return (
    <Modal
      title={host ? t.hosts.edit : t.hosts.add}
      onClose={onClose}
      wide
      footer={
        <>
          <button onClick={onClose}>{t.common.cancel}</button>
          <button
            className="btn-primary"
            onClick={() => void save()}
            disabled={busy || !draft.address.trim() || !draft.inboundUuid}
          >
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <Field label={t.hosts.inbound}>
        <select value={draft.inboundUuid} onChange={(e) => set('inboundUuid', e.target.value)}>
          <option value="">—</option>
          {inbounds.map((i) => (
            <option key={i.uuid} value={i.uuid}>
              {i.profileName} · {i.tag} ({i.type}/{i.network}/{i.security})
            </option>
          ))}
        </select>
      </Field>

      <div className="grid cols-2">
        <Field label={t.hosts.remark} hint={t.hosts.remarkHint}>
          <input value={draft.remark} onChange={(e) => set('remark', e.target.value)} />
        </Field>
        <Field label={t.hosts.tags} hint={t.hosts.tagsHint}>
          <input value={draft.tags} onChange={(e) => set('tags', e.target.value)} />
        </Field>
        <Field label={t.hosts.address}>
          <input
            value={draft.address}
            placeholder="nl.example.com"
            onChange={(e) => set('address', e.target.value)}
          />
        </Field>
        <Field label={t.hosts.port}>
          <input
            type="number"
            min="1"
            max="65535"
            value={draft.port}
            onChange={(e) => set('port', e.target.value)}
          />
        </Field>
        <Field label={t.hosts.security}>
          <select value={draft.security} onChange={(e) => set('security', e.target.value)}>
            {SECURITIES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t.hosts.fingerprint}>
          <select value={draft.fingerprint} onChange={(e) => set('fingerprint', e.target.value)}>
            {FINGERPRINTS.map((fp) => (
              <option key={fp} value={fp}>
                {fp || '—'}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t.hosts.sni}>
          <input value={draft.sni} onChange={(e) => set('sni', e.target.value)} />
        </Field>
        <Field label={t.hosts.flow} hint={selectedInbound?.type === 'vless' ? 'xtls-rprx-vision' : ''}>
          <input value={draft.flow} onChange={(e) => set('flow', e.target.value)} />
        </Field>
      </div>

      {isReality && (
        <div className="grid cols-3">
          <Field label={t.hosts.publicKey}>
            <input value={draft.publicKey} onChange={(e) => set('publicKey', e.target.value)} />
          </Field>
          <Field label={t.hosts.shortId}>
            <input value={draft.shortId} onChange={(e) => set('shortId', e.target.value)} />
          </Field>
          <Field label={t.hosts.spiderX}>
            <input value={draft.spiderX} onChange={(e) => set('spiderX', e.target.value)} />
          </Field>
        </div>
      )}

      <div className="grid cols-3">
        <Field label={t.hosts.path} hint={t.common.optional}>
          <input value={draft.path} onChange={(e) => set('path', e.target.value)} />
        </Field>
        <Field label={t.hosts.hostHeader} hint={t.common.optional}>
          <input value={draft.hostHeader} onChange={(e) => set('hostHeader', e.target.value)} />
        </Field>
        <Field label={t.hosts.alpn} hint={t.common.optional}>
          <input value={draft.alpn} onChange={(e) => set('alpn', e.target.value)} />
        </Field>
      </div>

      <div className="split" style={{ gap: 20 }}>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={draft.allowInsecure}
            onChange={(e) => set('allowInsecure', e.target.checked)}
          />
          {t.hosts.allowInsecure}
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={draft.isDisabled}
            onChange={(e) => set('isDisabled', e.target.checked)}
          />
          {t.common.disabled}
        </label>
      </div>
    </Modal>
  )
}
