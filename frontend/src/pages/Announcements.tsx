import { useState } from 'react'
import { Icon } from '../components/icons'
import { ConfirmDialog, EmptyState, Field, Modal, Spinner, useAction } from '../components/ui'
import { useI18n } from '../i18n'
import { api, type Announcement } from '../lib/api'
import { dateTime } from '../lib/format'
import { useFetch } from '../lib/useApi'
import { useAuth } from '../lib/auth'

const LEVELS = ['INFO', 'WARNING', 'DANGER'] as const

/** Maps a level onto the badge class the rest of the panel already uses. */
const LEVEL_CLASS: Record<string, string> = {
  INFO: 'badge',
  WARNING: 'badge badge-warn',
  DANGER: 'badge badge-danger',
}

/** datetime-local wants `YYYY-MM-DDTHH:mm` with no zone, in local time. */
function toLocalInput(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function fromLocalInput(value: string): string | null {
  if (!value) return null
  return new Date(value).toISOString()
}

export function Announcements() {
  const { t, lang } = useI18n()
  const { canWrite } = useAuth()
  const list = useFetch<Announcement[]>('/api/announcements', 30_000)

  const [editing, setEditing] = useState<Announcement | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Announcement | null>(null)

  const items = list.data ?? []
  const now = Date.now()

  // The window is the whole point of scheduling one, so say plainly where each
  // notice sits relative to it rather than only whether it is switched on.
  const state = (a: Announcement) => {
    if (!a.isEnabled) return { cls: 'badge', text: t.common.disabled }
    if (a.startsAt && new Date(a.startsAt).getTime() > now)
      return { cls: 'badge', text: t.announcements.scheduled }
    if (a.endsAt && new Date(a.endsAt).getTime() < now)
      return { cls: 'badge', text: t.announcements.finished }
    return { cls: 'badge badge-ok', text: t.announcements.live }
  }

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.announcements.title}</h2>
          <p>{t.announcements.subtitle}</p>
        </div>
        <button className="btn-ghost" onClick={() => void list.reload()}>
          <Icon name="refresh" size={16} />
        </button>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.announcements.add}
          </button>
        )}
      </div>

      {list.loading && !list.data ? (
        <div className="card" style={{ display: 'grid', placeItems: 'center', height: 180 }}>
          <Spinner />
        </div>
      ) : items.length === 0 ? (
        <div className="card">
          <EmptyState title={t.announcements.empty} hint={t.announcements.emptyHint} />
        </div>
      ) : (
        <div className="stack" style={{ gap: 12 }}>
          {items.map((a) => {
            const s = state(a)
            return (
              <div key={a.uuid} className="card card-pad stack" style={{ gap: 10 }}>
                <div className="split" style={{ gap: 10, flexWrap: 'wrap' }}>
                  <span className={LEVEL_CLASS[a.level] ?? 'badge'}>{a.level.toLowerCase()}</span>
                  {a.title && <strong style={{ fontSize: 15 }}>{a.title}</strong>}
                  <div style={{ flex: 1 }} />
                  <span className={s.cls}>
                    <span className="dot" />
                    {s.text}
                  </span>
                  {canWrite && (
                    <>
                      <button
                        className="btn-sm btn-ghost btn-icon"
                        onClick={() => setEditing(a)}
                      >
                        <Icon name="edit" size={14} />
                      </button>
                      <button
                        className="btn-sm btn-ghost btn-icon btn-danger"
                        onClick={() => setConfirmDelete(a)}
                      >
                        <Icon name="trash" size={14} />
                      </button>
                    </>
                  )}
                </div>
                <p style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{a.body}</p>
                {(a.startsAt || a.endsAt) && (
                  <span className="small dim">
                    {a.startsAt ? dateTime(a.startsAt, lang) : t.announcements.immediately}
                    {' → '}
                    {a.endsAt ? dateTime(a.endsAt, lang) : t.announcements.indefinitely}
                  </span>
                )}
              </div>
            )
          })}
        </div>
      )}

      {editing && (
        <AnnouncementForm
          announcement={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await list.reload()
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          message={`${t.announcements.deleteTitle} ${confirmDelete.title || confirmDelete.body.slice(0, 60)}`}
          danger
          onCancel={() => setConfirmDelete(null)}
          onConfirm={async () => {
            await api.del(`/api/announcements/${confirmDelete.uuid}`)
            setConfirmDelete(null)
            await list.reload()
          }}
        />
      )}
    </div>
  )
}

function AnnouncementForm({
  announcement,
  onClose,
  onSaved,
}: {
  announcement: Announcement | null
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const { t } = useI18n()
  const run = useAction()

  const [title, setTitle] = useState(announcement?.title ?? '')
  const [body, setBody] = useState(announcement?.body ?? '')
  const [level, setLevel] = useState<string>(announcement?.level ?? 'INFO')
  const [enabled, setEnabled] = useState(announcement?.isEnabled ?? true)
  const [startsAt, setStartsAt] = useState(toLocalInput(announcement?.startsAt ?? null))
  const [endsAt, setEndsAt] = useState(toLocalInput(announcement?.endsAt ?? null))
  const [busy, setBusy] = useState(false)

  const save = () =>
    run(async () => {
      setBusy(true)
      try {
        const payload = {
          title,
          body,
          level,
          isEnabled: enabled,
          startsAt: fromLocalInput(startsAt),
          endsAt: fromLocalInput(endsAt),
        }
        if (announcement) await api.put(`/api/announcements/${announcement.uuid}`, payload)
        else await api.post('/api/announcements', payload)
        await onSaved()
      } finally {
        setBusy(false)
      }
    })

  return (
    <Modal
      title={announcement ? t.announcements.editTitle : t.announcements.addTitle}
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
        <Field label={t.announcements.titleField} hint={t.common.optional}>
          <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={120} />
        </Field>
        <Field label={t.announcements.level}>
          <select value={level} onChange={(e) => setLevel(e.target.value)}>
            {LEVELS.map((l) => (
              <option key={l} value={l}>
                {l.toLowerCase()}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <Field label={t.announcements.body} hint={t.announcements.bodyHint}>
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={4}
          maxLength={2000}
        />
      </Field>

      <div className="grid cols-2">
        <Field label={t.announcements.startsAt} hint={t.announcements.startsAtHint}>
          <input
            type="datetime-local"
            value={startsAt}
            onChange={(e) => setStartsAt(e.target.value)}
          />
        </Field>
        <Field label={t.announcements.endsAt} hint={t.announcements.endsAtHint}>
          <input
            type="datetime-local"
            value={endsAt}
            onChange={(e) => setEndsAt(e.target.value)}
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
