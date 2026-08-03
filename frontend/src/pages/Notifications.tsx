import { useMemo, useState } from 'react'
import { Icon } from '../components/icons'
import {
  ConfirmDialog,
  EmptyState,
  Field,
  Modal,
  Spinner,
  useAction,
  useToast,
} from '../components/ui'
import { useI18n } from '../i18n'
import { api, type NotificationChannel, type NotificationDelivery } from '../lib/api'
import { dateTime, relative } from '../lib/format'
import { useFetch } from '../lib/useApi'
import { useAuth } from '../lib/auth'

const KIND_LABEL: Record<string, string> = {
  WEBHOOK: 'Webhook',
  TELEGRAM: 'Telegram',
}

export function Notifications() {
  const { t, lang } = useI18n()
  const { canWrite } = useAuth()
  const channels = useFetch<NotificationChannel[]>('/api/notifications/channels', 30_000)
  const kinds = useFetch<string[]>('/api/notifications/events')

  const [editing, setEditing] = useState<NotificationChannel | 'new' | null>(null)
  const [inspecting, setInspecting] = useState<NotificationChannel | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<NotificationChannel | null>(null)

  const list = channels.data ?? []

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.notifications.title}</h2>
          <p>{t.notifications.subtitle}</p>
        </div>
        <button className="btn-ghost" onClick={() => void channels.reload()}>
          <Icon name="refresh" size={16} />
        </button>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.notifications.add}
          </button>
        )}
      </div>

      {channels.loading && !channels.data ? (
        <div className="card" style={{ display: 'grid', placeItems: 'center', height: 180 }}>
          <Spinner />
        </div>
      ) : list.length === 0 ? (
        <div className="card">
          <EmptyState title={t.notifications.empty} hint={t.notifications.emptyHint} />
        </div>
      ) : (
        <div className="grid cols-2">
          {list.map((c) => (
            <ChannelCard
              key={c.uuid}
              channel={c}
              onEdit={() => setEditing(c)}
              onInspect={() => setInspecting(c)}
              onDelete={() => setConfirmDelete(c)}
              onChanged={() => void channels.reload()}
            />
          ))}
        </div>
      )}

      {editing && (
        <ChannelForm
          channel={editing === 'new' ? null : editing}
          kinds={kinds.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await channels.reload()
          }}
        />
      )}

      {inspecting && (
        <DeliveryLog channel={inspecting} onClose={() => setInspecting(null)} />
      )}

      {confirmDelete && (
        <ConfirmDialog
          message={`${t.notifications.deleteTitle} ${confirmDelete.name}`}
          danger
          onCancel={() => setConfirmDelete(null)}
          onConfirm={async () => {
            await api.del(`/api/notifications/channels/${confirmDelete.uuid}`)
            setConfirmDelete(null)
            await channels.reload()
          }}
        />
      )}
    </div>
  )

  function ChannelCard({
    channel,
    onEdit,
    onInspect,
    onDelete,
    onChanged,
  }: {
    channel: NotificationChannel
    onEdit: () => void
    onInspect: () => void
    onDelete: () => void
    onChanged: () => void
  }) {
    const run = useAction()
    const { push } = useToast()
    const [testing, setTesting] = useState(false)

    // A channel that has never been tried reads differently from one that was
    // tried and refused — that distinction is the whole point of the log.
    const health =
      channel.lastOk === null || channel.lastOk === undefined
        ? { cls: 'badge', text: t.notifications.neverSent }
        : channel.lastOk
          ? { cls: 'badge badge-ok', text: t.notifications.healthy }
          : { cls: 'badge badge-danger', text: t.notifications.failing }

    return (
      <div className="card card-pad stack" style={{ gap: 12 }}>
        <div className="split" style={{ gap: 10 }}>
          <strong style={{ fontSize: 15 }}>{channel.name}</strong>
          <span className="pill">{KIND_LABEL[channel.kind] ?? channel.kind}</span>
          <div style={{ flex: 1 }} />
          {!channel.isEnabled && <span className="badge">{t.common.disabled}</span>}
          <span className={health.cls}>
            <span className="dot" />
            {health.text}
          </span>
        </div>

        <div className="small dim truncate" title={channel.config.url ?? ''}>
          {channel.kind === 'WEBHOOK'
            ? channel.config.url
            : `chat ${channel.config.chatId ?? '—'}`}
          {channel.hasSecret && ` · ${t.notifications.signed}`}
        </div>

        <div className="small dim">
          {channel.eventCount === 0
            ? t.notifications.allEvents
            : `${channel.eventCount} ${t.notifications.eventsSelected}`}
          {channel.lastSentAt && ` · ${relative(channel.lastSentAt, lang)}`}
        </div>

        {channel.lastOk === false && channel.lastDetail && (
          <div className="small" style={{ color: 'var(--danger)' }}>
            {channel.lastDetail}
          </div>
        )}

        <div className="split" style={{ gap: 8, flexWrap: 'wrap' }}>
          {canWrite && (
            <button
              className="btn-sm"
              disabled={testing}
              onClick={() =>
                void run(async () => {
                  setTesting(true)
                  try {
                    const res = await api.post<{ ok: boolean; detail: string }>(
                      `/api/notifications/channels/${channel.uuid}/test`,
                    )
                    push(res.ok ? t.notifications.testOk : res.detail, res.ok ? 'success' : 'error')
                    onChanged()
                  } finally {
                    setTesting(false)
                  }
                })
              }
            >
              {testing ? <Spinner /> : <Icon name="activity" size={14} />}
              {t.notifications.test}
            </button>
          )}
          <button className="btn-sm btn-ghost" onClick={onInspect}>
            <Icon name="clock" size={14} />
            {t.notifications.deliveries}
          </button>
          <div style={{ flex: 1 }} />
          {canWrite && (
            <>
              <button className="btn-sm btn-ghost btn-icon" onClick={onEdit}>
                <Icon name="edit" size={14} />
              </button>
              <button className="btn-sm btn-ghost btn-icon btn-danger" onClick={onDelete}>
                <Icon name="trash" size={14} />
              </button>
            </>
          )}
        </div>
      </div>
    )
  }
}

function ChannelForm({
  channel,
  kinds,
  onClose,
  onSaved,
}: {
  channel: NotificationChannel | null
  kinds: string[]
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const { t } = useI18n()
  const run = useAction()

  const [name, setName] = useState(channel?.name ?? '')
  const [kind, setKind] = useState(channel?.kind ?? 'WEBHOOK')
  const [url, setUrl] = useState(channel?.config.url ?? '')
  const [secret, setSecret] = useState('')
  const [botToken, setBotToken] = useState('')
  const [chatId, setChatId] = useState(channel?.config.chatId ?? '')
  const [events, setEvents] = useState<string[]>(channel?.events ?? [])
  const [enabled, setEnabled] = useState(channel?.isEnabled ?? true)
  const [busy, setBusy] = useState(false)

  const toggle = (k: string) =>
    setEvents((cur) => (cur.includes(k) ? cur.filter((x) => x !== k) : [...cur, k]))

  const save = () =>
    run(async () => {
      setBusy(true)
      try {
        const config =
          kind === 'WEBHOOK' ? { url, secret } : { botToken, chatId }
        const body = { name, kind, config, events, isEnabled: enabled }
        if (channel) await api.put(`/api/notifications/channels/${channel.uuid}`, body)
        else await api.post('/api/notifications/channels', body)
        await onSaved()
      } finally {
        setBusy(false)
      }
    })

  return (
    <Modal
      title={channel ? t.notifications.editTitle : t.notifications.addTitle}
      onClose={onClose}
      wide
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
      <div className="grid cols-2">
        <Field label={t.notifications.name}>
          <input value={name} onChange={(e) => setName(e.target.value)} maxLength={64} />
        </Field>
        <Field label={t.notifications.kind}>
          <select value={kind} onChange={(e) => setKind(e.target.value as typeof kind)}>
            <option value="WEBHOOK">Webhook</option>
            <option value="TELEGRAM">Telegram</option>
          </select>
        </Field>
      </div>

      {kind === 'WEBHOOK' ? (
        <>
          <Field label={t.notifications.url} hint={t.notifications.urlHint}>
            <input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/hooks/amneziax"
            />
          </Field>
          <Field
            label={t.notifications.secret}
            hint={channel?.hasSecret ? t.notifications.secretKept : t.notifications.secretHint}
          >
            <input
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              type="password"
              autoComplete="new-password"
              placeholder={channel?.hasSecret ? '••••••••' : ''}
            />
          </Field>
        </>
      ) : (
        <>
          <Field
            label={t.notifications.botToken}
            hint={channel?.hasSecret ? t.notifications.secretKept : t.notifications.botTokenHint}
          >
            <input
              value={botToken}
              onChange={(e) => setBotToken(e.target.value)}
              type="password"
              autoComplete="new-password"
              placeholder={channel?.hasSecret ? '••••••••' : '123456:ABC-DEF...'}
            />
          </Field>
          <Field label={t.notifications.chatId} hint={t.notifications.chatIdHint}>
            <input value={chatId} onChange={(e) => setChatId(e.target.value)} />
          </Field>
        </>
      )}

      <Field label={t.notifications.events} hint={t.notifications.eventsHint}>
        <div className="split" style={{ gap: 8, flexWrap: 'wrap' }}>
          {kinds.map((k) => (
            <button
              key={k}
              type="button"
              className={`pill${events.includes(k) ? ' pill-active' : ''}`}
              onClick={() => toggle(k)}
            >
              {k.replaceAll('_', ' ').toLowerCase()}
            </button>
          ))}
        </div>
      </Field>

      <label className="checkbox">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        {t.common.enabled}
      </label>
    </Modal>
  )
}

function DeliveryLog({
  channel,
  onClose,
}: {
  channel: NotificationChannel
  onClose: () => void
}) {
  const { t, lang } = useI18n()
  const log = useFetch<NotificationDelivery[]>(
    `/api/notifications/channels/${channel.uuid}/deliveries?limit=100`,
    15_000,
  )

  const summary = useMemo(() => {
    const items = log.data ?? []
    const failed = items.filter((d) => !d.ok).length
    return { total: items.length, failed }
  }, [log.data])

  return (
    <Modal title={`${channel.name} — ${t.notifications.deliveries}`} onClose={onClose} wide
      footer={<button className="btn-primary" onClick={onClose}>{t.common.close}</button>}>
      <div className="split small dim" style={{ gap: 14 }}>
        <span>{summary.total} {t.notifications.attempts}</span>
        {summary.failed > 0 && (
          <span style={{ color: 'var(--danger)' }}>
            {summary.failed} {t.notifications.failed}
          </span>
        )}
      </div>

      {log.loading && !log.data ? (
        <div style={{ display: 'grid', placeItems: 'center', height: 140 }}>
          <Spinner />
        </div>
      ) : (log.data ?? []).length === 0 ? (
        <EmptyState title={t.notifications.noDeliveries} hint={t.notifications.noDeliveriesHint} />
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t.events.kind}</th>
                <th>{t.notifications.result}</th>
                <th className="right">{t.notifications.attemptsShort}</th>
                <th className="right">{t.notifications.duration}</th>
                <th>{t.events.when}</th>
              </tr>
            </thead>
            <tbody>
              {(log.data ?? []).map((d) => (
                <tr key={d.id}>
                  <td className="small nowrap">{d.eventKind.replaceAll('_', ' ').toLowerCase()}</td>
                  <td className={d.ok ? 'small' : 'small'} style={d.ok ? undefined : { color: 'var(--danger)' }}>
                    {d.ok ? t.notifications.delivered : d.detail}
                  </td>
                  <td className="right small dim">{d.attempts}</td>
                  <td className="right small dim nowrap">{d.durationMs} ms</td>
                  <td className="small dim nowrap" title={dateTime(d.createdAt, lang)}>
                    {relative(d.createdAt, lang)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Modal>
  )
}
