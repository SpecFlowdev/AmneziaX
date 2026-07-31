import { useState } from 'react'
import { Icon } from '../components/icons'
import { Badge, ConfirmDialog, EmptyState, Field, Modal, Spinner, useAction } from '../components/ui'
import { useI18n } from '../i18n'
import { api, type Admin, type AdminRole } from '../lib/api'
import { useAuth } from '../lib/auth'
import { dateTime } from '../lib/format'
import { useFetch } from '../lib/useApi'

const ROLES: AdminRole[] = ['ADMIN', 'VIEWER']

export function Admins() {
  const { t, lang, f } = useI18n()
  const { admin: me, isOwner } = useAuth()
  const run = useAction()

  const admins = useFetch<Admin[]>('/api/admins')
  const [editing, setEditing] = useState<Admin | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<Admin | null>(null)

  const roleLabel = (role: AdminRole) =>
    role === 'OWNER' ? t.admins.roleOwner : role === 'ADMIN' ? t.admins.roleAdmin : t.admins.roleViewer

  if (!isOwner) {
    return (
      <div className="page">
        <div className="card">
          <EmptyState title={t.admins.ownerOnly} />
        </div>
      </div>
    )
  }

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.admins.title}</h2>
          <p>{t.admins.subtitle}</p>
        </div>
        <button className="btn-primary" onClick={() => setEditing('new')}>
          <Icon name="plus" size={16} />
          {t.admins.add}
        </button>
      </div>

      <div className="card">
        {admins.loading && !admins.data ? (
          <div style={{ display: 'grid', placeItems: 'center', height: 160 }}>
            <Spinner />
          </div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t.login.username}</th>
                  <th>{t.admins.role}</th>
                  <th>{t.common.status}</th>
                  <th>{t.admins.lastLogin}</th>
                  <th>{t.common.created}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {(admins.data ?? []).map((a) => (
                  <tr key={a.uuid}>
                    <td style={{ fontWeight: 600 }}>
                      {a.username}
                      {a.uuid === me?.uuid && <span className="pill" style={{ marginLeft: 8 }}>you</span>}
                    </td>
                    <td>
                      <Badge kind={a.role === 'OWNER' ? 'accent' : 'muted'}>{roleLabel(a.role)}</Badge>
                    </td>
                    <td>
                      {a.isDisabled ? (
                        <Badge kind="muted" dot>
                          {t.common.disabled}
                        </Badge>
                      ) : (
                        <Badge kind="ok" dot>
                          {t.common.enabled}
                        </Badge>
                      )}
                    </td>
                    <td className="small">{a.lastLoginAt ? dateTime(a.lastLoginAt, lang) : t.common.never}</td>
                    <td className="small dim">{dateTime(a.createdAt, lang)}</td>
                    <td>
                      <div className="row-actions">
                        {a.role !== 'OWNER' && (
                          <>
                            <button
                              className="btn-sm btn-ghost btn-icon"
                              onClick={() => setEditing(a)}
                              title={t.common.edit}
                            >
                              <Icon name="edit" size={15} />
                            </button>
                            <button
                              className="btn-sm btn-ghost btn-icon btn-danger"
                              onClick={() => setConfirmDelete(a)}
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
        <AdminEditor
          admin={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await admins.reload()
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
              await api.del(`/api/admins/${target.uuid}`)
              await admins.reload()
            })
          }}
        />
      )}
    </div>
  )
}

function AdminEditor({
  admin,
  onClose,
  onSaved,
}: {
  admin: Admin | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const run = useAction()
  const [busy, setBusy] = useState(false)
  const [username, setUsername] = useState(admin?.username ?? '')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<AdminRole>(admin?.role ?? 'ADMIN')
  const [disabled, setDisabled] = useState(admin?.isDisabled ?? false)

  const roleLabel = (r: AdminRole) => (r === 'ADMIN' ? t.admins.roleAdmin : t.admins.roleViewer)

  async function save() {
    setBusy(true)
    await run(async () => {
      if (admin) {
        await api.put(`/api/admins/${admin.uuid}`, {
          username,
          password,
          role,
          isDisabled: disabled,
        })
      } else {
        await api.post('/api/admins', { username: username.trim(), password, role, isDisabled: false })
      }
      onSaved()
    })
    setBusy(false)
  }

  return (
    <Modal
      title={admin ? t.admins.edit : t.admins.add}
      onClose={onClose}
      footer={
        <>
          <button onClick={onClose}>{t.common.cancel}</button>
          <button
            className="btn-primary"
            onClick={() => void save()}
            disabled={busy || !username.trim() || (!admin && password.length < 8)}
          >
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <Field label={t.login.username}>
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          disabled={!!admin}
          autoFocus={!admin}
        />
      </Field>
      <Field label={t.admins.password} hint={admin ? t.admins.passwordKeep : '≥ 8'}>
        <input
          type="password"
          value={password}
          autoComplete="new-password"
          onChange={(e) => setPassword(e.target.value)}
        />
      </Field>
      <Field label={t.admins.role}>
        <select value={role} onChange={(e) => setRole(e.target.value as AdminRole)}>
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {roleLabel(r)}
            </option>
          ))}
        </select>
      </Field>
      {admin && (
        <label className="checkbox">
          <input type="checkbox" checked={disabled} onChange={(e) => setDisabled(e.target.checked)} />
          {t.common.disabled}
        </label>
      )}
    </Modal>
  )
}
