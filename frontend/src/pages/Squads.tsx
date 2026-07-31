import { useState } from 'react'
import { Icon } from '../components/icons'
import {
  CheckList,
  ConfirmDialog,
  EmptyState,
  Field,
  Modal,
  Spinner,
  useAction,
} from '../components/ui'
import { useI18n } from '../i18n'
import { api, type Inbound, type Squad } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useFetch } from '../lib/useApi'

export function Squads() {
  const { t, f } = useI18n()
  const { canWrite } = useAuth()
  const run = useAction()

  const squads = useFetch<Squad[]>('/api/squads')
  const inbounds = useFetch<Inbound[]>('/api/profiles/inbounds')

  const [editing, setEditing] = useState<Squad | 'new' | null>(null)
  const [confirm, setConfirm] = useState<{ squad: Squad; action: 'delete' | 'addAll' | 'removeAll' } | null>(
    null,
  )

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.squads.title}</h2>
          <p>{t.squads.subtitle}</p>
        </div>
        {canWrite && (
          <button className="btn-primary" onClick={() => setEditing('new')}>
            <Icon name="plus" size={16} />
            {t.squads.add}
          </button>
        )}
      </div>

      {squads.loading && !squads.data ? (
        <div className="card card-pad" style={{ display: 'grid', placeItems: 'center', height: 160 }}>
          <Spinner />
        </div>
      ) : (squads.data ?? []).length === 0 ? (
        <div className="card">
          <EmptyState title={t.common.nothingHere} hint={t.squads.subtitle} />
        </div>
      ) : (
        <div className="grid cols-3">
          {(squads.data ?? []).map((squad) => (
            <div key={squad.uuid} className="card">
              <div className="card-head">
                <Icon name="layers" size={17} />
                <h3>{squad.name}</h3>
                <div className="spacer" />
                <span className="pill">
                  <Icon name="users" size={12} />
                  {squad.membersCount}
                </span>
              </div>
              <div className="card-pad stack" style={{ gap: 12 }}>
                {squad.info && <span className="small muted">{squad.info}</span>}
                <div className="split" style={{ flexWrap: 'wrap', gap: 6 }}>
                  {(squad.inbounds ?? []).length === 0 ? (
                    <span className="small dim">{t.common.none}</span>
                  ) : (
                    (squad.inbounds ?? []).map((i) => (
                      <span key={i.uuid} className="pill" title={`${i.profileName} · ${i.type}`}>
                        {i.tag}
                      </span>
                    ))
                  )}
                </div>

                {canWrite && (
                  <>
                    <hr className="sep" />
                    <div className="split" style={{ flexWrap: 'wrap', gap: 6 }}>
                      <button className="btn-sm" onClick={() => setEditing(squad)}>
                        <Icon name="edit" size={14} />
                        {t.common.edit}
                      </button>
                      <button
                        className="btn-sm"
                        onClick={() => setConfirm({ squad, action: 'addAll' })}
                      >
                        <Icon name="plus" size={14} />
                        {t.squads.addAll}
                      </button>
                      <div style={{ flex: 1 }} />
                      <button
                        className="btn-sm btn-ghost btn-icon btn-danger"
                        onClick={() => setConfirm({ squad, action: 'delete' })}
                        title={t.common.delete}
                      >
                        <Icon name="trash" size={15} />
                      </button>
                    </div>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <SquadEditor
          squad={editing === 'new' ? null : editing}
          inbounds={inbounds.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await squads.reload()
          }}
        />
      )}

      {confirm && (
        <ConfirmDialog
          danger={confirm.action !== 'addAll'}
          message={
            confirm.action === 'delete'
              ? f(t.common.deleteConfirm, { name: confirm.squad.name })
              : confirm.action === 'addAll'
                ? f(t.squads.addAllConfirm, { name: confirm.squad.name })
                : f(t.squads.removeAllConfirm, { name: confirm.squad.name })
          }
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            const { squad, action } = confirm
            setConfirm(null)
            void run(async () => {
              if (action === 'delete') await api.del(`/api/squads/${squad.uuid}`)
              else if (action === 'addAll') await api.post(`/api/squads/${squad.uuid}/add-all-users`)
              else await api.post(`/api/squads/${squad.uuid}/remove-all-users`)
              await squads.reload()
            })
          }}
        />
      )}
    </div>
  )
}

function SquadEditor({
  squad,
  inbounds,
  onClose,
  onSaved,
}: {
  squad: Squad | null
  inbounds: Inbound[]
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const run = useAction()
  const [busy, setBusy] = useState(false)
  const [name, setName] = useState(squad?.name ?? '')
  const [info, setInfo] = useState(squad?.info ?? '')
  const [selected, setSelected] = useState<string[]>(squad?.inboundUuids ?? [])

  async function save() {
    setBusy(true)
    const body = { name: name.trim(), info, inboundUuids: selected }
    await run(async () => {
      if (squad) await api.put(`/api/squads/${squad.uuid}`, body)
      else await api.post('/api/squads', body)
      onSaved()
    })
    setBusy(false)
  }

  return (
    <Modal
      title={squad ? t.squads.edit : t.squads.add}
      onClose={onClose}
      footer={
        <>
          <button onClick={onClose}>{t.common.cancel}</button>
          <button className="btn-primary" onClick={() => void save()} disabled={busy || !name.trim()}>
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <Field label={t.common.name}>
        <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Field>
      <Field label={t.squads.info} hint={t.common.optional}>
        <input value={info} onChange={(e) => setInfo(e.target.value)} />
      </Field>
      <Field label={t.squads.pickInbounds}>
        <CheckList
          items={inbounds.map((i) => ({
            value: i.uuid,
            label: `${i.tag}`,
            hint: `${i.profileName} · ${i.type} · ${i.network} · ${i.security}`,
          }))}
          selected={selected}
          onChange={setSelected}
          emptyLabel={t.common.nothingHere}
        />
      </Field>
    </Modal>
  )
}
