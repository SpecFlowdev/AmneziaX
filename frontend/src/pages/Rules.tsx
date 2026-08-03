import { useState } from 'react'
import { Icon } from '../components/icons'
import { ConfirmDialog, EmptyState, Field, Modal, Spinner, useAction } from '../components/ui'
import { useI18n } from '../i18n'
import { api } from '../lib/api'
import { useFetch } from '../lib/useApi'
import { useAuth } from '../lib/auth'

interface Rule {
  uuid: string
  name: string
  matchUserAgent: string
  format: string
  isEnabled: boolean
  priority: number
  hits: number
}

const FORMATS = ['base64', 'json', 'clash', 'singbox', 'plain'] as const

export function Rules() {
  const { t } = useI18n()
  const { canWrite } = useAuth()
  const rules = useFetch<Rule[]>('/api/rules', 30_000)

  const [editing, setEditing] = useState<Rule | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Rule | null>(null)
  const [probe, setProbe] = useState('')
  const [probeResult, setProbeResult] = useState<string | null>(null)

  const list = rules.data ?? []

  const runProbe = async (ua: string) => {
    if (!ua.trim()) return setProbeResult(null)
    const res = await api.get<{ format: string }>(`/api/rules/test?ua=${encodeURIComponent(ua)}`)
    setProbeResult(res.format)
  }

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.rules.title}</h2>
          <p>{t.rules.subtitle}</p>
        </div>
        <button className="btn-ghost" onClick={() => void rules.reload()}>
          <Icon name="refresh" size={16} />
        </button>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.rules.add}
          </button>
        )}
      </div>

      {/* The only question worth asking while writing a rule is what a given
          client would actually be served — including when a built-in match or
          the panel default wins instead. */}
      <div className="card card-pad stack" style={{ gap: 10 }}>
        <span className="small dim">{t.rules.probeTitle}</span>
        <div className="split" style={{ gap: 10, flexWrap: 'wrap' }}>
          <input
            value={probe}
            onChange={(e) => setProbe(e.target.value)}
            placeholder="v2rayNG/1.8.5"
            style={{ flex: 1, minWidth: 240 }}
            onKeyDown={(e) => e.key === 'Enter' && void runProbe(probe)}
          />
          <button onClick={() => void runProbe(probe)}>{t.rules.probe}</button>
          {probeResult && (
            <span className="badge badge-ok">
              {t.rules.wouldGet}: {probeResult}
            </span>
          )}
        </div>
        <span className="small dim">{t.rules.probeHint}</span>
      </div>

      <div className="card">
        {rules.loading && !rules.data ? (
          <div style={{ display: 'grid', placeItems: 'center', height: 160 }}>
            <Spinner />
          </div>
        ) : list.length === 0 ? (
          <EmptyState title={t.rules.empty} hint={t.rules.emptyHint} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t.rules.priority}</th>
                  <th>{t.rules.name}</th>
                  <th>{t.rules.match}</th>
                  <th>{t.rules.format}</th>
                  <th className="right">{t.rules.hits}</th>
                  {canWrite && <th />}
                </tr>
              </thead>
              <tbody>
                {list.map((r) => (
                  <tr key={r.uuid} style={r.isEnabled ? undefined : { opacity: 0.55 }}>
                    <td className="small dim nums">{r.priority}</td>
                    <td>
                      {r.name || '—'}
                      {!r.isEnabled && (
                        <span className="badge" style={{ marginLeft: 8 }}>
                          {t.common.disabled}
                        </span>
                      )}
                    </td>
                    <td className="mono small">{r.matchUserAgent}</td>
                    <td className="small">{r.format}</td>
                    {/* A rule that has never fired looks exactly like a correct
                        one until this number says otherwise. */}
                    <td className="right small dim nums">{r.hits}</td>
                    {canWrite && (
                      <td style={{ width: 1 }} className="nowrap">
                        <button className="btn-sm btn-ghost btn-icon" onClick={() => setEditing(r)}>
                          <Icon name="edit" size={14} />
                        </button>
                        <button
                          className="btn-sm btn-ghost btn-icon btn-danger"
                          onClick={() => setConfirmDelete(r)}
                        >
                          <Icon name="trash" size={14} />
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {editing && (
        <RuleForm
          rule={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await rules.reload()
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          message={`${t.rules.deleteTitle} ${confirmDelete.name || confirmDelete.matchUserAgent}`}
          danger
          onCancel={() => setConfirmDelete(null)}
          onConfirm={async () => {
            await api.del(`/api/rules/${confirmDelete.uuid}`)
            setConfirmDelete(null)
            await rules.reload()
          }}
        />
      )}
    </div>
  )
}

function RuleForm({
  rule,
  onClose,
  onSaved,
}: {
  rule: Rule | null
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const { t } = useI18n()
  const run = useAction()

  const [name, setName] = useState(rule?.name ?? '')
  const [match, setMatch] = useState(rule?.matchUserAgent ?? '')
  const [format, setFormat] = useState(rule?.format ?? 'base64')
  const [priority, setPriority] = useState(rule?.priority ?? 100)
  const [enabled, setEnabled] = useState(rule?.isEnabled ?? true)
  const [busy, setBusy] = useState(false)

  const save = () =>
    run(async () => {
      setBusy(true)
      try {
        const body = {
          name,
          matchUserAgent: match,
          format,
          priority: Number(priority) || 100,
          isEnabled: enabled,
        }
        if (rule) await api.put(`/api/rules/${rule.uuid}`, body)
        else await api.post('/api/rules', body)
        await onSaved()
      } finally {
        setBusy(false)
      }
    })

  return (
    <Modal
      title={rule ? t.rules.editTitle : t.rules.addTitle}
      onClose={onClose}
      footer={
        <>
          <button onClick={onClose}>{t.common.cancel}</button>
          <button className="btn-primary" onClick={() => void save()} disabled={busy}>
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <Field label={t.rules.name} hint={t.common.optional}>
        <input value={name} onChange={(e) => setName(e.target.value)} maxLength={64} />
      </Field>
      <Field label={t.rules.match} hint={t.rules.matchHint}>
        <input
          value={match}
          onChange={(e) => setMatch(e.target.value)}
          placeholder="Streisand"
          maxLength={200}
        />
      </Field>
      <div className="grid cols-2">
        <Field label={t.rules.format}>
          <select value={format} onChange={(e) => setFormat(e.target.value)}>
            {FORMATS.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t.rules.priority} hint={t.rules.priorityHint}>
          <input
            type="number"
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
          />
        </Field>
      </div>
      <label className="checkbox">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        {t.common.enabled}
      </label>
    </Modal>
  )
}
