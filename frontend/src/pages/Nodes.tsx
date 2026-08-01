import { useEffect, useMemo, useState } from 'react'
import { Icon } from '../components/icons'
import {
  Badge,
  CheckList,
  ConfirmDialog,
  CopyButton,
  EmptyState,
  Field,
  Meter,
  Modal,
  Spinner,
  useAction,
} from '../components/ui'
import { useI18n } from '../i18n'
import {
  api,
  type BillingCycle,
  type ConfigProfile,
  type Node,
  type ResetStrategy,
} from '../lib/api'
import { useAuth } from '../lib/auth'
import {
  bytes,
  dateOnly,
  duration,
  flag,
  fromDatetimeLocal,
  parseBytes,
  relative,
  toDatetimeLocal,
} from '../lib/format'
import { useFetch } from '../lib/useApi'
import { money, NodeHealthBadge } from './Dashboard'

const STRATEGIES: ResetStrategy[] = ['NO_RESET', 'DAY', 'WEEK', 'MONTH']
const CYCLES: BillingCycle[] = ['NONE', 'MONTHLY', 'QUARTERLY', 'YEARLY']

interface Draft {
  name: string
  address: string
  countryCode: string
  description: string
  configProfileUuid: string
  activeInboundTags: string[]
  consumptionMultiplier: string
  trafficLimit: string
  trafficResetStrategy: ResetStrategy
  notifyPercent: string
  isDisabled: boolean
  provider: string
  providerUrl: string
  costAmount: string
  costCurrency: string
  billingCycle: BillingCycle
  nextPaymentAt: string
  billingNotes: string
  tags: string
}

const emptyDraft: Draft = {
  name: '',
  address: '',
  countryCode: '',
  description: '',
  configProfileUuid: '',
  activeInboundTags: [],
  consumptionMultiplier: '1',
  trafficLimit: '',
  trafficResetStrategy: 'NO_RESET',
  notifyPercent: '0',
  isDisabled: false,
  provider: '',
  providerUrl: '',
  costAmount: '',
  costCurrency: '',
  billingCycle: 'NONE',
  nextPaymentAt: '',
  billingNotes: '',
  tags: '',
}

export function Nodes() {
  const { t, lang, f } = useI18n()
  const { canWrite } = useAuth()
  const run = useAction()

  const nodes = useFetch<Node[]>('/api/nodes', 5_000)
  const profiles = useFetch<ConfigProfile[]>('/api/profiles')

  const [editing, setEditing] = useState<Node | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Node | null>(null)
  const [install, setInstall] = useState<{ command: string; token: string } | null>(null)
  const [logsFor, setLogsFor] = useState<Node | null>(null)
  const [configFor, setConfigFor] = useState<Node | null>(null)

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.nodes.title}</h2>
          <p>{t.nodes.subtitle}</p>
        </div>
        <button className="btn-ghost" onClick={() => void nodes.reload()}>
          <Icon name="refresh" size={16} />
          {t.common.refresh}
        </button>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.nodes.add}
          </button>
        )}
      </div>

      {nodes.loading && !nodes.data ? (
        <div className="card card-pad" style={{ display: 'grid', placeItems: 'center', height: 180 }}>
          <Spinner />
        </div>
      ) : (nodes.data ?? []).length === 0 ? (
        <div className="card">
          <EmptyState title={t.common.nothingHere} hint={t.nodes.subtitle} />
        </div>
      ) : (
        <div className="grid cols-2">
          {(nodes.data ?? []).map((node) => (
            <div key={node.uuid} className="card">
              <div className="card-head">
                <span style={{ fontSize: 20 }}>{flag(node.countryCode)}</span>
                <div className="stack">
                  <h3>{node.name}</h3>
                  <span className="small dim mono">{node.address || '—'}</span>
                </div>
                <div className="spacer" />
                <NodeHealthBadge node={node} />
              </div>

              <div className="card-pad" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                <div className="grid cols-2" style={{ gap: 12 }}>
                  <Metric label={t.nodes.xray} value={node.xrayVersion || '—'} sub={
                    node.xrayRunning && node.xrayStartedAt
                      ? `${t.nodes.uptime}: ${duration((Date.now() - new Date(node.xrayStartedAt).getTime()) / 1000)}`
                      : node.statusMessage || t.common.offline
                  } />
                  <Metric
                    label="CPU / RAM"
                    value={node.xrayRunning ? `${node.cpuUsagePercent.toFixed(0)}%` : '—'}
                    sub={
                      node.totalRamBytes > 0
                        ? `${bytes(node.usedRamBytes)} / ${bytes(node.totalRamBytes)}`
                        : t.nodes.notConnected
                    }
                  />
                </div>

                <div>
                  <div className="split small" style={{ marginBottom: 6 }}>
                    <span className="dim">{t.nodes.traffic}</span>
                    <div style={{ flex: 1 }} />
                    <span className="nums">
                      {bytes(node.trafficUsedBytes)}
                      {node.trafficLimitBytes > 0 && ` / ${bytes(node.trafficLimitBytes)}`}
                    </span>
                  </div>
                  {node.trafficLimitBytes > 0 ? (
                    <Meter used={node.trafficUsedBytes} limit={node.trafficLimitBytes} />
                  ) : (
                    <div className="small dim">{t.common.unlimited}</div>
                  )}
                </div>

                <div className="split" style={{ flexWrap: 'wrap', gap: 7 }}>
                  <span className="pill">
                    <Icon name="code" size={13} />
                    {node.configProfileName || t.nodes.noProfile}
                  </span>
                  {node.activeInboundTags.length > 0 ? (
                    node.activeInboundTags.map((tag) => (
                      <span key={tag} className="pill">
                        {tag}
                      </span>
                    ))
                  ) : (
                    <span className="pill dim">{t.common.all}</span>
                  )}
                  {node.consumptionMultiplier !== 1 && (
                    <span className="pill">×{node.consumptionMultiplier}</span>
                  )}
                  {(node.tags ?? []).map((tag) => (
                    <span key={tag} className="pill">
                      {tag}
                    </span>
                  ))}
                </div>

                {(node.provider || node.costAmount > 0) && (
                  <div className="split small" style={{ gap: 8, flexWrap: 'wrap' }}>
                    {node.provider && (
                      <span className="pill">
                        <Icon name="server" size={12} />
                        {node.providerUrl ? (
                          <a href={node.providerUrl} target="_blank" rel="noreferrer">
                            {node.provider}
                          </a>
                        ) : (
                          node.provider
                        )}
                      </span>
                    )}
                    {node.costAmount > 0 && (
                      <span className="pill">
                        {money(node.costAmount, node.costCurrency || 'USD')}
                        <span className="dim">
                          {t.billing[`cycle${node.billingCycle}` as 'cycleNONE'].toLowerCase()}
                        </span>
                      </span>
                    )}
                    {node.nextPaymentAt && (
                      <span
                        className={`pill${
                          new Date(node.nextPaymentAt).getTime() - Date.now() < 7 * 864e5
                            ? ' badge-warn'
                            : ''
                        }`}
                      >
                        <Icon name="clock" size={12} />
                        {dateOnly(node.nextPaymentAt, lang)}
                      </span>
                    )}
                  </div>
                )}

                {node.hostname && (
                  <div className="small dim">
                    {node.hostname} · {node.os}/{node.arch} · {node.cpuCount} vCPU
                    {node.lastConnectedAt && ` · ${relative(node.lastConnectedAt, lang)}`}
                  </div>
                )}

                <hr className="sep" />

                <div className="split" style={{ flexWrap: 'wrap', gap: 7 }}>
                  {canWrite && (
                    <>
                      <button
                        className="btn-sm"
                        disabled={!node.isConnected}
                        onClick={() =>
                          void run(() => api.post(`/api/nodes/${node.uuid}/restart`), t.nodes.restart)
                        }
                      >
                        <Icon name="refresh" size={14} />
                        {t.nodes.restart}
                      </button>
                      <button
                        className="btn-sm"
                        onClick={() =>
                          void run(async () => {
                            await api.post(
                              `/api/nodes/${node.uuid}/${node.isDisabled ? 'enable' : 'disable'}`,
                            )
                            await nodes.reload()
                          })
                        }
                      >
                        <Icon name="power" size={14} />
                        {node.isDisabled ? t.common.enable : t.common.disable}
                      </button>
                    </>
                  )}
                  <button className="btn-sm" onClick={() => setLogsFor(node)} disabled={!node.isConnected}>
                    <Icon name="terminal" size={14} />
                    {t.nodes.logs}
                  </button>
                  <button className="btn-sm" onClick={() => setConfigFor(node)}>
                    <Icon name="code" size={14} />
                    {t.nodes.viewConfig}
                  </button>
                  <div style={{ flex: 1 }} />
                  {canWrite && (
                    <>
                      <button
                        className="btn-sm btn-ghost btn-icon"
                        onClick={() => setEditing(node)}
                        title={t.common.edit}
                      >
                        <Icon name="edit" size={15} />
                      </button>
                      <button
                        className="btn-sm btn-ghost btn-icon btn-danger"
                        onClick={() => setConfirmDelete(node)}
                        title={t.common.delete}
                      >
                        <Icon name="trash" size={15} />
                      </button>
                    </>
                  )}
                </div>

                {canWrite && (
                  <div className="split small" style={{ gap: 10 }}>
                    <button
                      className="btn-ghost btn-sm"
                      onClick={() =>
                        void run(async () => {
                          const res = await api.post<{ token: string; installCommand: string }>(
                            `/api/nodes/${node.uuid}/rotate-token`,
                          )
                          setInstall({ command: res.installCommand, token: res.token })
                        })
                      }
                    >
                      <Icon name="key" size={14} />
                      {t.nodes.rotateToken}
                    </button>
                    <button
                      className="btn-ghost btn-sm"
                      onClick={() =>
                        void run(async () => {
                          await api.post(`/api/nodes/${node.uuid}/reset-traffic`)
                          await nodes.reload()
                        }, t.nodes.resetTraffic)
                      }
                    >
                      <Icon name="refresh" size={14} />
                      {t.nodes.resetTraffic}
                    </button>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <NodeEditor
          node={editing === 'new' ? null : editing}
          profiles={profiles.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async (created) => {
            setEditing(null)
            await nodes.reload()
            if (created) setInstall(created)
          }}
        />
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
              await api.del(`/api/nodes/${target.uuid}`)
              await nodes.reload()
            })
          }}
        />
      )}

      {install && <InstallDialog install={install} onClose={() => setInstall(null)} />}
      {logsFor && <LogsDialog node={logsFor} onClose={() => setLogsFor(null)} />}
      {configFor && <ConfigDialog node={configFor} onClose={() => setConfigFor(null)} />}
    </div>
  )
}

function Metric({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="stack">
      <span className="small dim">{label}</span>
      <span style={{ fontWeight: 600 }}>{value}</span>
      {sub && <span className="small dim truncate">{sub}</span>}
    </div>
  )
}

function NodeEditor({
  node,
  profiles,
  onClose,
  onSaved,
}: {
  node: Node | null
  profiles: ConfigProfile[]
  onClose: () => void
  onSaved: (created: { command: string; token: string } | null) => void
}) {
  const { t } = useI18n()
  const run = useAction()
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState<Draft>(() =>
    node
      ? {
          name: node.name,
          address: node.address,
          countryCode: node.countryCode === 'XX' ? '' : node.countryCode,
          description: node.description,
          configProfileUuid: node.configProfileUuid ?? '',
          activeInboundTags: node.activeInboundTags,
          consumptionMultiplier: String(node.consumptionMultiplier),
          trafficLimit: node.trafficLimitBytes ? bytes(node.trafficLimitBytes, 0) : '',
          trafficResetStrategy: node.trafficResetStrategy,
          notifyPercent: String(node.notifyPercent),
          isDisabled: node.isDisabled,
          provider: node.provider,
          providerUrl: node.providerUrl,
          costAmount: node.costAmount ? String(node.costAmount) : '',
          costCurrency: node.costCurrency,
          billingCycle: node.billingCycle ?? 'NONE',
          nextPaymentAt: toDatetimeLocal(node.nextPaymentAt),
          billingNotes: node.billingNotes,
          tags: (node.tags ?? []).join(', '),
        }
      : emptyDraft,
  )

  const profile = profiles.find((p) => p.uuid === draft.configProfileUuid)
  const inbounds = profile?.inbounds ?? []

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((d) => ({ ...d, [key]: value }))

  async function save() {
    const limit = parseBytes(draft.trafficLimit)
    if (Number.isNaN(limit)) return
    setBusy(true)
    const body = {
      name: draft.name.trim(),
      address: draft.address.trim(),
      countryCode: draft.countryCode.trim() || 'XX',
      description: draft.description,
      isDisabled: draft.isDisabled,
      configProfileUuid: draft.configProfileUuid || null,
      activeInboundTags: draft.activeInboundTags,
      consumptionMultiplier: Number.parseFloat(draft.consumptionMultiplier) || 1,
      trafficLimitBytes: limit,
      trafficResetStrategy: draft.trafficResetStrategy,
      notifyPercent: Number.parseInt(draft.notifyPercent, 10) || 0,
      viewPosition: node?.viewPosition ?? 0,
      provider: draft.provider.trim(),
      providerUrl: draft.providerUrl.trim(),
      costAmount: Number.parseFloat(draft.costAmount.replace(',', '.')) || 0,
      costCurrency: draft.costCurrency.trim().toUpperCase(),
      billingCycle: draft.billingCycle,
      nextPaymentAt: fromDatetimeLocal(draft.nextPaymentAt),
      billingNotes: draft.billingNotes,
      tags: draft.tags
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    }
    const ok = await run(async () => {
      if (node) {
        await api.put(`/api/nodes/${node.uuid}`, body)
        onSaved(null)
      } else {
        const res = await api.post<{ token: string; installCommand: string }>('/api/nodes', body)
        onSaved({ command: res.installCommand, token: res.token })
      }
    })
    setBusy(false)
    if (!ok) return
  }

  return (
    <Modal
      title={node ? t.nodes.edit : t.nodes.add}
      onClose={onClose}
      footer={
        <>
          <button onClick={onClose}>{t.common.cancel}</button>
          <button className="btn-primary" onClick={() => void save()} disabled={busy || !draft.name.trim()}>
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <div className="grid cols-2">
        <Field label={t.common.name}>
          <input value={draft.name} onChange={(e) => set('name', e.target.value)} autoFocus />
        </Field>
        <Field label={t.nodes.address} hint="IP / hostname">
          <input value={draft.address} onChange={(e) => set('address', e.target.value)} />
        </Field>
        <Field label={t.nodes.country} hint="NL, DE, US…">
          <input
            value={draft.countryCode}
            maxLength={2}
            onChange={(e) => set('countryCode', e.target.value.toUpperCase())}
          />
        </Field>
        <Field label={t.nodes.multiplier} hint={t.nodes.multiplierHint}>
          <input
            type="number"
            step="0.1"
            min="0.1"
            value={draft.consumptionMultiplier}
            onChange={(e) => set('consumptionMultiplier', e.target.value)}
          />
        </Field>
      </div>

      <Field label={t.common.description}>
        <input value={draft.description} onChange={(e) => set('description', e.target.value)} />
      </Field>

      <Field label={t.nodes.profile}>
        <select
          value={draft.configProfileUuid}
          onChange={(e) => set('configProfileUuid', e.target.value)}
        >
          <option value="">{t.common.none}</option>
          {profiles.map((p) => (
            <option key={p.uuid} value={p.uuid}>
              {p.name}
            </option>
          ))}
        </select>
      </Field>

      <Field label={t.nodes.inbounds} hint={t.nodes.inboundsHint}>
        <CheckList
          items={inbounds.map((i) => ({
            value: i.tag,
            label: i.tag,
            hint: `${i.type} · ${i.network} · ${i.security} · :${i.port}`,
          }))}
          selected={draft.activeInboundTags}
          onChange={(next) => set('activeInboundTags', next)}
          emptyLabel={t.nodes.noProfile}
        />
      </Field>

      <div className="grid cols-3">
        <Field label={t.nodes.trafficLimit} hint="500 GB / 2 TB">
          <input
            value={draft.trafficLimit}
            placeholder={t.common.unlimited}
            onChange={(e) => set('trafficLimit', e.target.value)}
          />
        </Field>
        <Field label={t.nodes.trafficReset}>
          <select
            value={draft.trafficResetStrategy}
            onChange={(e) => set('trafficResetStrategy', e.target.value as ResetStrategy)}
          >
            {STRATEGIES.map((s) => (
              <option key={s} value={s}>
                {t.strategy[s]}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t.nodes.notifyPercent}>
          <input
            type="number"
            min="0"
            max="100"
            value={draft.notifyPercent}
            onChange={(e) => set('notifyPercent', e.target.value)}
          />
        </Field>
      </div>

      <hr className="sep" />
      <div className="split">
        <Icon name="chart" size={16} />
        <strong style={{ fontSize: 13.5 }}>{t.billing.title}</strong>
        <span className="small dim">{t.billing.subtitle}</span>
      </div>

      <div className="grid cols-2">
        <Field label={t.billing.provider} hint="Hetzner, Aeza, Vultr…">
          <input value={draft.provider} onChange={(e) => set('provider', e.target.value)} />
        </Field>
        <Field label={t.billing.providerUrl} hint={t.common.optional}>
          <input value={draft.providerUrl} onChange={(e) => set('providerUrl', e.target.value)} />
        </Field>
      </div>

      <div className="grid cols-3">
        <Field label={t.billing.cost}>
          <input
            value={draft.costAmount}
            inputMode="decimal"
            placeholder="0"
            onChange={(e) => set('costAmount', e.target.value.replace(/[^\d.,]/g, ''))}
          />
        </Field>
        <Field label={t.settings.currency} hint={t.common.optional}>
          <input
            value={draft.costCurrency}
            maxLength={8}
            placeholder="EUR"
            onChange={(e) => set('costCurrency', e.target.value.toUpperCase())}
          />
        </Field>
        <Field label={t.billing.cycle}>
          <select
            value={draft.billingCycle}
            onChange={(e) => set('billingCycle', e.target.value as BillingCycle)}
          >
            {CYCLES.map((c) => (
              <option key={c} value={c}>
                {t.billing[`cycle${c}` as 'cycleNONE']}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <div className="grid cols-2">
        <Field label={t.billing.nextPayment} hint={t.common.optional}>
          <input
            type="datetime-local"
            value={draft.nextPaymentAt}
            onChange={(e) => set('nextPaymentAt', e.target.value)}
          />
        </Field>
        <Field label={t.billing.tags} hint={t.hosts.tagsHint}>
          <input value={draft.tags} onChange={(e) => set('tags', e.target.value)} />
        </Field>
      </div>

      <Field label={t.billing.notes} hint={t.common.optional}>
        <input value={draft.billingNotes} onChange={(e) => set('billingNotes', e.target.value)} />
      </Field>

      <label className="checkbox">
        <input
          type="checkbox"
          checked={draft.isDisabled}
          onChange={(e) => set('isDisabled', e.target.checked)}
        />
        {t.common.disabled}
      </label>
    </Modal>
  )
}

function InstallDialog({
  install,
  onClose,
}: {
  install: { command: string; token: string }
  onClose: () => void
}) {
  const { t } = useI18n()
  return (
    <Modal
      title={t.nodes.installTitle}
      onClose={onClose}
      wide
      footer={
        <button className="btn-primary" onClick={onClose}>
          {t.common.close}
        </button>
      }
    >
      <p className="muted" style={{ margin: 0 }}>
        {t.nodes.installHint}
      </p>
      <div className="split" style={{ alignItems: 'flex-start' }}>
        <pre className="code-block" style={{ flex: 1 }}>
          {install.command}
        </pre>
        <CopyButton value={install.command} />
      </div>
      <div className="badge badge-warn" style={{ padding: '8px 12px' }}>
        <Icon name="alert" size={15} />
        {t.nodes.tokenWarning}
      </div>
    </Modal>
  )
}

function LogsDialog({ node, onClose }: { node: Node; onClose: () => void }) {
  const { t } = useI18n()
  const [lines, setLines] = useState<string[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useMemo(
    () => async () => {
      setError(null)
      try {
        const res = await api.get<{ lines: string[] }>(`/api/nodes/${node.uuid}/logs?lines=300`)
        setLines(res.lines)
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err))
      }
    },
    [node.uuid],
  )

  useEffect(() => {
    void load()
  }, [load])

  return (
    <Modal
      title={`${t.nodes.logs} — ${node.name}`}
      onClose={onClose}
      wide
      footer={
        <>
          <button onClick={() => void load()}>
            <Icon name="refresh" size={15} />
            {t.common.refresh}
          </button>
          <button className="btn-primary" onClick={onClose}>
            {t.common.close}
          </button>
        </>
      }
    >
      {error ? (
        <div className="badge badge-danger" style={{ padding: '8px 12px' }}>
          <Icon name="alert" size={15} />
          {error}
        </div>
      ) : lines === null ? (
        <div style={{ display: 'grid', placeItems: 'center', height: 120 }}>
          <Spinner />
        </div>
      ) : lines.length === 0 ? (
        <EmptyState title={t.nodes.logsEmpty} />
      ) : (
        <div className="log-view">{lines.join('\n')}</div>
      )}
    </Modal>
  )
}

function ConfigDialog({ node, onClose }: { node: Node; onClose: () => void }) {
  const { t } = useI18n()
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .get<unknown>(`/api/nodes/${node.uuid}/config`)
      .then((cfg) => setText(JSON.stringify(cfg, null, 2)))
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
  }, [node.uuid])

  return (
    <Modal
      title={`${t.nodes.configPreview} — ${node.name}`}
      onClose={onClose}
      wide
      footer={
        <>
          {text && <CopyButton value={text} label={t.common.copy} />}
          <button className="btn-primary" onClick={onClose}>
            {t.common.close}
          </button>
        </>
      }
    >
      <p className="muted small" style={{ margin: 0 }}>
        {t.nodes.configHint}
      </p>
      {error ? (
        <div className="badge badge-danger" style={{ padding: '8px 12px' }}>
          <Icon name="alert" size={15} />
          {error}
        </div>
      ) : text === null ? (
        <div style={{ display: 'grid', placeItems: 'center', height: 120 }}>
          <Spinner />
        </div>
      ) : (
        <div className="log-view">{text}</div>
      )}
    </Modal>
  )
}

export { Badge }
