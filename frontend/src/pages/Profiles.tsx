import { useState } from 'react'
import { Icon } from '../components/icons'
import {
  ConfirmDialog,
  CopyButton,
  EmptyState,
  Field,
  Modal,
  Spinner,
  useAction,
  useToast,
} from '../components/ui'
import { useI18n } from '../i18n'
import { api, profileKinds, type ConfigProfile, type ProfileKind } from '../lib/api'
import { useAuth } from '../lib/auth'
import { dateTime } from '../lib/format'
import { useFetch } from '../lib/useApi'

export function Profiles() {
  const { t, lang, f } = useI18n()
  const { canWrite } = useAuth()
  const run = useAction()

  const profiles = useFetch<ConfigProfile[]>('/api/profiles')
  const [editing, setEditing] = useState<ConfigProfile | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<ConfigProfile | null>(null)
  const [keys, setKeys] = useState<{ privateKey: string; publicKey: string; shortIds: string[] } | null>(
    null,
  )

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.profiles.title}</h2>
          <p>{t.profiles.subtitle}</p>
        </div>
        {canWrite && (
          <>
            <button
              onClick={() =>
                void run(async () => {
                  setKeys(await api.post('/api/profiles/tools/reality-keys'))
                })
              }
            >
              <Icon name="key" size={16} />
              {t.profiles.generateKeys}
            </button>
            <button className="btn-primary" onClick={() => setEditing('new')}>
              <Icon name="plus" size={16} />
              {t.profiles.add}
            </button>
          </>
        )}
      </div>

      {profiles.loading && !profiles.data ? (
        <div className="card card-pad" style={{ display: 'grid', placeItems: 'center', height: 160 }}>
          <Spinner />
        </div>
      ) : (profiles.data ?? []).length === 0 ? (
        <div className="card">
          <EmptyState title={t.common.nothingHere} hint={t.profiles.subtitle} />
        </div>
      ) : (
        <div className="grid cols-2">
          {(profiles.data ?? []).map((p) => (
            <div key={p.uuid} className="card">
              <div className="card-head">
                <Icon name="code" size={17} />
                <h3>{p.name}</h3>
                <span className="pill">{t.profiles.kinds[p.kind ?? 'xray']}</span>
                <div className="spacer" />
                <span className="small dim">{dateTime(p.updatedAt, lang)}</span>
              </div>
              <div className="card-pad stack" style={{ gap: 12 }}>
                <div>
                  <div className="small dim" style={{ marginBottom: 6 }}>
                    {t.profiles.inbounds}
                  </div>
                  <div className="split" style={{ flexWrap: 'wrap', gap: 6 }}>
                    {(p.inbounds ?? []).length === 0 ? (
                      <span className="small dim">{t.common.none}</span>
                    ) : (
                      (p.inbounds ?? []).map((i) => (
                        <span key={i.uuid} className="pill">
                          <strong>{i.tag}</strong>
                          <span className="dim">
                            {i.type}/{i.network}/{i.security}:{i.port}
                          </span>
                        </span>
                      ))
                    )}
                  </div>
                </div>

                {canWrite && (
                  <>
                    <hr className="sep" />
                    <div className="split">
                      <button className="btn-sm" onClick={() => setEditing(p)}>
                        <Icon name="edit" size={14} />
                        {t.common.edit}
                      </button>
                      <div style={{ flex: 1 }} />
                      <button
                        className="btn-sm btn-ghost btn-icon btn-danger"
                        onClick={() => setConfirmDelete(p)}
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
        <ProfileEditor
          profile={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await profiles.reload()
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
              await api.del(`/api/profiles/${target.uuid}`)
              await profiles.reload()
            })
          }}
        />
      )}

      {keys && (
        <Modal
          title={t.profiles.keysTitle}
          onClose={() => setKeys(null)}
          footer={
            <button className="btn-primary" onClick={() => setKeys(null)}>
              {t.common.close}
            </button>
          }
        >
          <p className="muted small" style={{ margin: 0 }}>
            {t.profiles.keysHint}
          </p>
          {(
            [
              [t.profiles.privateKey, keys.privateKey],
              [t.profiles.publicKey, keys.publicKey],
              [t.profiles.shortIds, keys.shortIds.join('", "')],
            ] as const
          ).map(([label, value]) => (
            <div key={label} className="stack" style={{ gap: 6 }}>
              <span className="small dim">{label}</span>
              <div className="split">
                <pre className="code-block" style={{ flex: 1, margin: 0 }}>
                  {value}
                </pre>
                <CopyButton value={value} />
              </div>
            </div>
          ))}
        </Modal>
      )}
    </div>
  )
}

function ProfileEditor({
  profile,
  onClose,
  onSaved,
}: {
  profile: ConfigProfile | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const run = useAction()
  const { push } = useToast()
  const [busy, setBusy] = useState(false)
  const [name, setName] = useState(profile?.name ?? '')
  const [kind, setKind] = useState<ProfileKind>(profile?.kind ?? 'xray')
  const [domain, setDomain] = useState('')
  const [text, setText] = useState(() =>
    profile ? JSON.stringify(profile.config, null, 2) : '',
  )
  // Filling the box is a two-click action once there is something in it: the
  // starter replaces the whole document, and a single misplaced click should
  // not throw away a config someone just typed.
  const [confirmStarter, setConfirmStarter] = useState(false)

  function format() {
    try {
      setText(JSON.stringify(JSON.parse(text), null, 2))
    } catch {
      push(t.profiles.invalidJson, 'error')
    }
  }

  async function loadStarter() {
    if (text.trim() && !confirmStarter) {
      setConfirmStarter(true)
      return
    }
    setConfirmStarter(false)
    await run(async () => {
      const q = new URLSearchParams({ kind })
      if (domain.trim()) q.set('domain', domain.trim())
      const res = await api.get<{ config: unknown }>(`/api/profiles/tools/starter?${q}`)
      setText(JSON.stringify(res.config, null, 2))
    })
  }

  async function save() {
    let parsed: unknown
    if (text.trim()) {
      try {
        parsed = JSON.parse(text)
      } catch {
        push(t.profiles.invalidJson, 'error')
        return
      }
    }
    setBusy(true)
    await run(async () => {
      const body = { name: name.trim(), kind, config: parsed }
      if (profile) {
        await api.put(`/api/profiles/${profile.uuid}`, body)
        push(t.profiles.savedApplied, 'success')
      } else {
        await api.post('/api/profiles', body)
      }
      onSaved()
    })
    setBusy(false)
  }

  return (
    <Modal
      title={profile ? t.profiles.edit : t.profiles.add}
      onClose={onClose}
      wide
      footer={
        <>
          <button onClick={() => void loadStarter()}>
            <Icon name="plus" size={14} />
            {confirmStarter ? t.profiles.starterReplace : t.profiles.starter}
          </button>
          <button onClick={format}>{t.profiles.format}</button>
          <div style={{ flex: 1 }} />
          <button onClick={onClose}>{t.common.cancel}</button>
          <button className="btn-primary" onClick={() => void save()} disabled={busy || !name.trim()}>
            {busy && <Spinner />}
            {t.common.save}
          </button>
        </>
      }
    >
      <div className="grid cols-2">
        <Field label={t.common.name}>
          <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        <Field label={t.profiles.kind} hint={t.profiles.kindHints[kind]}>
          <select value={kind} onChange={(e) => setKind(e.target.value as ProfileKind)}>
            {profileKinds.map((k) => (
              <option key={k} value={k}>
                {t.profiles.kinds[k]}
              </option>
            ))}
          </select>
        </Field>
      </div>
      {kind !== 'xray' && (
        <Field label={t.profiles.starterDomain} hint={t.profiles.starterDomainHint}>
          <input
            value={domain}
            placeholder="example.com"
            onChange={(e) => setDomain(e.target.value)}
          />
        </Field>
      )}
      <Field label={t.profiles.config} hint={t.profiles.configHints[kind]}>
        <textarea
          rows={22}
          value={text}
          spellCheck={false}
          placeholder={profile ? '' : '{ … }'}
          onChange={(e) => setText(e.target.value)}
        />
      </Field>
    </Modal>
  )
}
