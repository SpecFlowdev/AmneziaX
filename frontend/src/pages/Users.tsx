import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import QRCode from 'qrcode'
import { Sparkbars } from '../components/Chart'
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
import { api, type Device, type ResetStrategy, type Squad, type User, type UserStatus } from '../lib/api'
import { useAuth } from '../lib/auth'
import {
  bytes,
  dateTime,
  fromDatetimeLocal,
  parseBytes,
  relative,
  toDatetimeLocal,
} from '../lib/format'
import { useFetch } from '../lib/useApi'

const STRATEGIES: ResetStrategy[] = ['NO_RESET', 'DAY', 'WEEK', 'MONTH']
const STATUSES: UserStatus[] = ['ACTIVE', 'DISABLED', 'LIMITED', 'EXPIRED']
const PAGE_SIZE = 25

export function UserStatusBadge({ status }: { status: UserStatus }) {
  const { t } = useI18n()
  const map = {
    ACTIVE: { kind: 'ok' as const, label: t.users.statusActive },
    DISABLED: { kind: 'muted' as const, label: t.users.statusDisabled },
    LIMITED: { kind: 'warn' as const, label: t.users.statusLimited },
    EXPIRED: { kind: 'danger' as const, label: t.users.statusExpired },
  }
  const entry = map[status]
  return (
    <Badge kind={entry.kind} dot>
      {entry.label}
    </Badge>
  )
}

export function Users() {
  const { t, lang, f } = useI18n()
  const { canWrite } = useAuth()
  const run = useAction()
  const [params, setParams] = useSearchParams()

  const [search, setSearch] = useState(params.get('search') ?? '')
  const [debounced, setDebounced] = useState(search)
  const [status, setStatus] = useState('')
  const [squadFilter, setSquadFilter] = useState('')
  const [page, setPage] = useState(0)
  const [selection, setSelection] = useState<string[]>([])

  const [editing, setEditing] = useState<User | 'new' | null>(null)
  const [detail, setDetail] = useState<User | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<User | null>(null)

  useEffect(() => {
    const id = setTimeout(() => {
      setDebounced(search)
      setPage(0)
      setParams(search ? { search } : {}, { replace: true })
    }, 300)
    return () => clearTimeout(id)
  }, [search, setParams])

  const query = new URLSearchParams({
    limit: String(PAGE_SIZE),
    offset: String(page * PAGE_SIZE),
    sortBy: 'createdAt',
    desc: 'true',
  })
  if (debounced) query.set('search', debounced)
  if (status) query.set('status', status)
  if (squadFilter) query.set('squadUuid', squadFilter)

  const users = useFetch<{ items: User[]; total: number }>(`/api/users?${query}`, 15_000)
  const squads = useFetch<Squad[]>('/api/squads')

  const items = users.data?.items ?? []
  const total = users.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const allSelected = items.length > 0 && items.every((u) => selection.includes(u.uuid))

  async function bulk(action: string) {
    const ok = await run(async () => {
      const res = await api.post<{ requested: number }>('/api/users/bulk', {
        uuids: selection,
        action,
      })
      setSelection([])
      await users.reload()
      return res
    })
    if (ok) return
  }

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.users.title}</h2>
          <p>{t.users.subtitle}</p>
        </div>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.users.add}
          </button>
        )}
      </div>

      <div className="card card-pad toolbar">
        <input
          className="grow"
          placeholder={t.common.search}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          value={status}
          onChange={(e) => {
            setStatus(e.target.value)
            setPage(0)
          }}
          style={{ width: 170 }}
        >
          <option value="">
            {t.users.filterStatus}: {t.common.all}
          </option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {t.users[`status${s.charAt(0)}${s.slice(1).toLowerCase()}` as 'statusActive']}
            </option>
          ))}
        </select>
        <select
          value={squadFilter}
          onChange={(e) => {
            setSquadFilter(e.target.value)
            setPage(0)
          }}
          style={{ width: 190 }}
        >
          <option value="">
            {t.users.filterSquad}: {t.common.all}
          </option>
          {(squads.data ?? []).map((s) => (
            <option key={s.uuid} value={s.uuid}>
              {s.name}
            </option>
          ))}
        </select>
        <button className="btn-ghost" onClick={() => void users.reload()}>
          <Icon name="refresh" size={16} />
        </button>
      </div>

      {selection.length > 0 && canWrite && (
        <div className="card card-pad toolbar">
          <strong>{f(t.users.selected, { n: selection.length })}</strong>
          <div style={{ flex: 1 }} />
          <button className="btn-sm" onClick={() => void bulk('enable')}>
            {t.users.bulkEnable}
          </button>
          <button className="btn-sm" onClick={() => void bulk('disable')}>
            {t.users.bulkDisable}
          </button>
          <button className="btn-sm" onClick={() => void bulk('reset-traffic')}>
            {t.users.bulkReset}
          </button>
          <button className="btn-sm btn-danger" onClick={() => void bulk('delete')}>
            {t.users.bulkDelete}
          </button>
          <button className="btn-sm btn-ghost" onClick={() => setSelection([])}>
            <Icon name="x" size={14} />
          </button>
        </div>
      )}

      <div className="card">
        {users.loading && !users.data ? (
          <div style={{ display: 'grid', placeItems: 'center', height: 180 }}>
            <Spinner />
          </div>
        ) : items.length === 0 ? (
          <EmptyState title={t.common.nothingHere} hint={t.users.subtitle} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  {canWrite && (
                    <th style={{ width: 1 }}>
                      <input
                        type="checkbox"
                        checked={allSelected}
                        style={{ width: 16, height: 16 }}
                        onChange={(e) =>
                          setSelection(e.target.checked ? items.map((u) => u.uuid) : [])
                        }
                      />
                    </th>
                  )}
                  <th>{t.users.username}</th>
                  <th>{t.common.status}</th>
                  <th>{t.users.traffic}</th>
                  <th>{t.users.squads}</th>
                  <th>{t.users.expireAt}</th>
                  <th>{t.users.lastOnline}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {items.map((u) => (
                  <tr key={u.uuid}>
                    {canWrite && (
                      <td>
                        <input
                          type="checkbox"
                          style={{ width: 16, height: 16 }}
                          checked={selection.includes(u.uuid)}
                          onChange={(e) =>
                            setSelection((prev) =>
                              e.target.checked
                                ? [...prev, u.uuid]
                                : prev.filter((id) => id !== u.uuid),
                            )
                          }
                        />
                      </td>
                    )}
                    <td>
                      <button
                        className="btn-ghost"
                        style={{ padding: 0, fontWeight: 600 }}
                        onClick={() => setDetail(u)}
                      >
                        {u.username}
                      </button>
                      {u.tag && <div className="small dim">{u.tag}</div>}
                    </td>
                    <td>
                      <UserStatusBadge status={u.status} />
                    </td>
                    <td style={{ minWidth: 150 }}>
                      <div className="small nums">
                        {bytes(u.usedTrafficBytes)}
                        {u.trafficLimitBytes > 0 ? ` / ${bytes(u.trafficLimitBytes)}` : ''}
                      </div>
                      <Meter used={u.usedTrafficBytes} limit={u.trafficLimitBytes} />
                    </td>
                    <td>
                      {u.squads && u.squads.length > 0 ? (
                        <div className="split" style={{ flexWrap: 'wrap', gap: 5 }}>
                          {u.squads.map((s) => (
                            <span key={s.uuid} className="pill">
                              {s.name}
                            </span>
                          ))}
                        </div>
                      ) : (
                        <span className="dim small">{t.users.noSquads}</span>
                      )}
                    </td>
                    <td className="small nowrap">
                      {u.expireAt ? dateTime(u.expireAt, lang) : t.common.never}
                    </td>
                    <td className="small dim nowrap">{relative(u.onlineAt, lang)}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          className="btn-sm btn-ghost btn-icon"
                          onClick={() => setDetail(u)}
                          title={t.users.subscription}
                        >
                          <Icon name="link" size={15} />
                        </button>
                        {canWrite && (
                          <>
                            <button
                              className="btn-sm btn-ghost btn-icon"
                              onClick={() => setEditing(u)}
                              title={t.common.edit}
                            >
                              <Icon name="edit" size={15} />
                            </button>
                            <button
                              className="btn-sm btn-ghost btn-icon btn-danger"
                              onClick={() => setConfirmDelete(u)}
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

        {total > PAGE_SIZE && (
          <div className="card-head" style={{ borderTop: '1px solid var(--border-soft)', borderBottom: 'none' }}>
            <span className="small dim">
              {total} {t.users.title.toLowerCase()}
            </span>
            <div className="spacer" />
            <button className="btn-sm" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
              <Icon name="chevronLeft" size={14} />
              {t.common.prev}
            </button>
            <span className="small nums">
              {page + 1} {t.common.of} {pages}
            </span>
            <button
              className="btn-sm"
              disabled={page + 1 >= pages}
              onClick={() => setPage((p) => p + 1)}
            >
              {t.common.next}
              <Icon name="chevronRight" size={14} />
            </button>
          </div>
        )}
      </div>

      {editing && (
        <UserEditor
          user={editing === 'new' ? null : editing}
          squads={squads.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await users.reload()
          }}
        />
      )}

      {detail && (
        <UserDetail
          user={detail}
          onClose={() => setDetail(null)}
          onChanged={async () => {
            await users.reload()
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          danger
          message={f(t.common.deleteConfirm, { name: confirmDelete.username })}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => {
            const target = confirmDelete
            setConfirmDelete(null)
            void run(async () => {
              await api.del(`/api/users/${target.uuid}`)
              await users.reload()
            })
          }}
        />
      )}
    </div>
  )
}

function UserEditor({
  user,
  squads,
  onClose,
  onSaved,
}: {
  user: User | null
  squads: Squad[]
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const run = useAction()
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState(() => ({
    username: user?.username ?? '',
    status: user?.status ?? ('ACTIVE' as UserStatus),
    trafficLimit: user?.trafficLimitBytes ? bytes(user.trafficLimitBytes, 0) : '',
    trafficLimitStrategy: user?.trafficLimitStrategy ?? ('NO_RESET' as ResetStrategy),
    expireAt: toDatetimeLocal(user?.expireAt ?? null),
    description: user?.description ?? '',
    tag: user?.tag ?? '',
    email: user?.email ?? '',
    telegramId: user?.telegramId ? String(user.telegramId) : '',
    hwidDeviceLimit: String(user?.hwidDeviceLimit ?? 0),
    squadUuids: user?.squadUuids ?? [],
  }))

  const set = <K extends keyof typeof draft>(key: K, value: (typeof draft)[K]) =>
    setDraft((d) => ({ ...d, [key]: value }))

  async function save() {
    const limit = parseBytes(draft.trafficLimit)
    if (Number.isNaN(limit)) return
    setBusy(true)
    const body = {
      username: draft.username.trim(),
      status: draft.status,
      trafficLimitBytes: limit,
      trafficLimitStrategy: draft.trafficLimitStrategy,
      expireAt: fromDatetimeLocal(draft.expireAt),
      description: draft.description,
      tag: draft.tag.trim(),
      email: draft.email.trim(),
      telegramId: draft.telegramId ? Number.parseInt(draft.telegramId, 10) : null,
      hwidDeviceLimit: Number.parseInt(draft.hwidDeviceLimit, 10) || 0,
      squadUuids: draft.squadUuids,
    }
    const ok = await run(async () => {
      if (user) await api.put(`/api/users/${user.uuid}`, body)
      else await api.post('/api/users', body)
      onSaved()
    })
    setBusy(false)
    if (!ok) return
  }

  return (
    <Modal
      title={user ? t.users.edit : t.users.add}
      onClose={onClose}
      footer={
        <>
          <button onClick={onClose}>{t.common.cancel}</button>
          <button
            className="btn-primary"
            onClick={() => void save()}
            disabled={busy || !draft.username.trim()}
          >
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <div className="grid cols-2">
        <Field label={t.users.username}>
          <input value={draft.username} onChange={(e) => set('username', e.target.value)} autoFocus />
        </Field>
        <Field label={t.common.status}>
          <select value={draft.status} onChange={(e) => set('status', e.target.value as UserStatus)}>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t.users[`status${s.charAt(0)}${s.slice(1).toLowerCase()}` as 'statusActive']}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t.users.trafficLimit} hint="50 GB / 1 TB">
          <input
            value={draft.trafficLimit}
            placeholder={t.common.unlimited}
            onChange={(e) => set('trafficLimit', e.target.value)}
          />
        </Field>
        <Field label={t.users.trafficStrategy}>
          <select
            value={draft.trafficLimitStrategy}
            onChange={(e) => set('trafficLimitStrategy', e.target.value as ResetStrategy)}
          >
            {STRATEGIES.map((s) => (
              <option key={s} value={s}>
                {t.strategy[s]}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t.users.expireAt} hint={t.common.optional}>
          <input
            type="datetime-local"
            value={draft.expireAt}
            onChange={(e) => set('expireAt', e.target.value)}
          />
        </Field>
        <Field label={t.users.tag} hint={t.common.optional}>
          <input value={draft.tag} onChange={(e) => set('tag', e.target.value)} />
        </Field>
        <Field label={t.users.email} hint={t.common.optional}>
          <input value={draft.email} onChange={(e) => set('email', e.target.value)} />
        </Field>
        <Field label={t.users.telegram} hint={t.common.optional}>
          <input
            value={draft.telegramId}
            inputMode="numeric"
            onChange={(e) => set('telegramId', e.target.value.replace(/[^\d-]/g, ''))}
          />
        </Field>
        <Field label={t.users.deviceLimit} hint={t.common.unlimited + ': 0'}>
          <input
            type="number"
            min="0"
            value={draft.hwidDeviceLimit}
            onChange={(e) => set('hwidDeviceLimit', e.target.value)}
          />
        </Field>
      </div>

      <Field label={t.common.description}>
        <input value={draft.description} onChange={(e) => set('description', e.target.value)} />
      </Field>

      <Field label={t.users.squads}>
        <CheckList
          items={squads.map((s) => ({
            value: s.uuid,
            label: s.name,
            hint: s.inbounds?.map((i) => i.tag).join(', '),
          }))}
          selected={draft.squadUuids}
          onChange={(next) => set('squadUuids', next)}
          emptyLabel={t.common.nothingHere}
        />
      </Field>
    </Modal>
  )
}

function UserDetail({
  user,
  onClose,
  onChanged,
}: {
  user: User
  onClose: () => void
  onChanged: () => Promise<void>
}) {
  const { t, lang, f } = useI18n()
  const { canWrite } = useAuth()
  const run = useAction()
  const [current, setCurrent] = useState(user)
  const [confirmRevoke, setConfirmRevoke] = useState(false)

  const links = useFetch<{ links: string[]; subscriptionUrl: string }>(
    `/api/users/${current.uuid}/links`,
  )
  const usage = useFetch<{ at: string; bytes: number }[]>(`/api/users/${current.uuid}/usage?days=30`)
  const devices = useFetch<Device[]>(`/api/users/${current.uuid}/devices`)

  const [qr, setQr] = useState<string | null>(null)
  useEffect(() => {
    QRCode.toDataURL(current.subscriptionUrl, {
      margin: 1,
      width: 220,
      color: { dark: '#000000ff', light: '#ffffffff' },
    })
      .then(setQr)
      .catch(() => setQr(null))
  }, [current.subscriptionUrl])

  const remaining = useMemo(
    () =>
      current.trafficLimitBytes > 0
        ? Math.max(0, current.trafficLimitBytes - current.usedTrafficBytes)
        : null,
    [current],
  )

  // The human-readable page lives in this SPA; the raw link is what apps import.
  const pageUrl = `${window.location.origin}/s/${current.subscriptionUuid}`

  return (
    <Modal
      title={current.username}
      onClose={onClose}
      wide
      footer={
        <>
          {canWrite && (
            <>
              <button
                onClick={() =>
                  void run(async () => {
                    const updated = await api.post<User>(`/api/users/${current.uuid}/reset-traffic`)
                    setCurrent(updated)
                    await onChanged()
                  }, t.users.resetTraffic)
                }
              >
                <Icon name="refresh" size={15} />
                {t.users.resetTraffic}
              </button>
              <button
                onClick={() =>
                  void run(async () => {
                    const updated = await api.post<User>(
                      `/api/users/${current.uuid}/${current.status === 'ACTIVE' ? 'disable' : 'enable'}`,
                    )
                    setCurrent(updated)
                    await onChanged()
                  })
                }
              >
                <Icon name="power" size={15} />
                {current.status === 'ACTIVE' ? t.common.disable : t.common.enable}
              </button>
              <button className="btn-danger" onClick={() => setConfirmRevoke(true)}>
                <Icon name="key" size={15} />
                {t.users.revoke}
              </button>
            </>
          )}
          <button className="btn-primary" onClick={onClose}>
            {t.common.close}
          </button>
        </>
      }
    >
      <div className="split" style={{ gap: 10, flexWrap: 'wrap' }}>
        <UserStatusBadge status={current.status} />
        {current.tag && <span className="pill">{current.tag}</span>}
        {current.expireAt && (
          <span className="pill">
            <Icon name="clock" size={13} />
            {dateTime(current.expireAt, lang)}
          </span>
        )}
        {current.email && <span className="pill">{current.email}</span>}
      </div>

      <div className="grid cols-2">
        <div className="card card-pad stack" style={{ gap: 8 }}>
          <span className="small dim">{t.users.traffic}</span>
          <span style={{ fontSize: 20, fontWeight: 650 }}>{bytes(current.usedTrafficBytes)}</span>
          {current.trafficLimitBytes > 0 ? (
            <>
              <Meter used={current.usedTrafficBytes} limit={current.trafficLimitBytes} />
              <span className="small dim">
                {bytes(remaining ?? 0)} {t.sub.remaining} {t.common.of}{' '}
                {bytes(current.trafficLimitBytes)}
              </span>
            </>
          ) : (
            <span className="small dim">{t.common.unlimited}</span>
          )}
          <span className="small dim">
            {t.common.total}: {bytes(current.lifetimeUsedTrafficBytes)}
          </span>
        </div>

        <div className="card card-pad stack" style={{ gap: 8 }}>
          <span className="small dim">{t.users.usage}</span>
          {(usage.data ?? []).length > 0 ? (
            <Sparkbars points={usage.data ?? []} />
          ) : (
            <span className="small dim">{t.dashboard.noTraffic}</span>
          )}
          <span className="small dim">
            {t.users.lastOnline}: {relative(current.onlineAt, lang)}
          </span>
        </div>
      </div>

      <div className="card card-pad" style={{ display: 'flex', gap: 18, flexWrap: 'wrap' }}>
        <div style={{ flex: '1 1 300px', display: 'flex', flexDirection: 'column', gap: 10 }}>
          <span className="small dim">{t.users.subscriptionUrl}</span>
          <div className="split">
            <pre className="code-block" style={{ flex: 1, margin: 0 }}>
              {current.subscriptionUrl}
            </pre>
            <CopyButton value={current.subscriptionUrl} />
          </div>
          <span className="small dim">{t.sub.title}</span>
          <div className="split">
            <pre className="code-block" style={{ flex: 1, margin: 0 }}>
              {pageUrl}
            </pre>
            <CopyButton value={pageUrl} />
          </div>
          <span className="small dim">
            {t.users.squads}:{' '}
            {current.squads && current.squads.length > 0
              ? current.squads.map((s) => s.name).join(', ')
              : t.users.noSquads}
          </span>
          {current.subLastOpenedAt && (
            <span className="small dim truncate" title={current.subLastUserAgent}>
              {current.subLastUserAgent} · {relative(current.subLastOpenedAt, lang)}
            </span>
          )}
        </div>
        {qr && (
          <div className="stack" style={{ alignItems: 'center', gap: 6 }}>
            <img
              src={qr}
              alt="QR"
              width={180}
              height={180}
              style={{ borderRadius: 10, background: '#fff', padding: 6 }}
            />
            <span className="small dim">{t.users.scanQr}</span>
          </div>
        )}
      </div>

      <div className="card card-pad stack" style={{ gap: 10 }}>
        <div className="split">
          <span className="small dim">{t.devices.title}</span>
          <div style={{ flex: 1 }} />
          <span className="small dim">
            {(devices.data ?? []).length}
            {current.hwidDeviceLimit > 0 ? ` / ${current.hwidDeviceLimit}` : ''}
          </span>
          {canWrite && (devices.data ?? []).length > 0 && (
            <button
              className="btn-sm btn-ghost"
              onClick={() =>
                void run(async () => {
                  await api.del(`/api/users/${current.uuid}/devices`)
                  await devices.reload()
                })
              }
            >
              {t.devices.reset}
            </button>
          )}
        </div>
        {(devices.data ?? []).length === 0 ? (
          <span className="small dim">{t.devices.none}</span>
        ) : (
          <div className="table-wrap">
            <table>
              <tbody>
                {(devices.data ?? []).map((d) => (
                  <tr key={d.hwid}>
                    <td>
                      <div className="stack">
                        <span className="mono small">{d.hwid.slice(0, 24)}</span>
                        <span className="small dim truncate">{d.platform || d.userAgent}</span>
                      </div>
                    </td>
                    <td className="right small dim nowrap">{relative(d.lastSeen, lang)}</td>
                    {canWrite && (
                      <td style={{ width: 1 }}>
                        <button
                          className="btn-sm btn-ghost btn-icon btn-danger"
                          title={t.devices.forget}
                          onClick={() =>
                            void run(async () => {
                              await api.del(
                                `/api/users/${current.uuid}/devices/${encodeURIComponent(d.hwid)}`,
                              )
                              await devices.reload()
                            })
                          }
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

      <div className="stack" style={{ gap: 8 }}>
        <span className="small dim">{t.users.links}</span>
        {(links.data?.links ?? []).length === 0 ? (
          <div className="small dim">{t.users.noLinks}</div>
        ) : (
          (links.data?.links ?? []).map((link, i) => (
            <div className="split" key={i}>
              <pre className="code-block" style={{ flex: 1, margin: 0 }}>
                {link}
              </pre>
              <CopyButton value={link} />
            </div>
          ))
        )}
      </div>

      {confirmRevoke && (
        <ConfirmDialog
          danger
          message={f(t.users.revokeConfirm, { name: current.username })}
          onCancel={() => setConfirmRevoke(false)}
          onConfirm={() => {
            setConfirmRevoke(false)
            void run(async () => {
              const updated = await api.post<User>(`/api/users/${current.uuid}/revoke`)
              setCurrent(updated)
              await links.reload()
              await onChanged()
            })
          }}
        />
      )}
    </Modal>
  )
}
