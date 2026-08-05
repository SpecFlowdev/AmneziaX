import { useMemo, useState } from 'react'
import { Icon } from './icons'
import { CheckList, Field, Modal, Spinner, useToast } from './ui'
import { useI18n } from '../i18n'
import { api, type ResetStrategy, type Squad, type UserStatus } from '../lib/api'
import { parseBytes } from '../lib/format'

interface Created {
  username: string
  subscriptionUrl: string
}
interface Failure {
  username: string
  error: string
}

/**
 * Handing access to a class, an office or a reseller's customers means creating
 * dozens of identical users. The form takes either a prefix and a count or a
 * pasted list, because those are the two ways the names already exist: made up
 * on the spot, or sitting in a spreadsheet.
 */
export function BulkCreate({
  squads,
  onClose,
  onDone,
}: {
  squads: Squad[]
  onClose: () => void
  onDone: () => void
}) {
  const { t, f } = useI18n()
  const { push } = useToast()

  const [mode, setMode] = useState<'prefix' | 'list'>('prefix')
  const [prefix, setPrefix] = useState('user-')
  const [count, setCount] = useState(10)
  const [start, setStart] = useState(1)
  const [names, setNames] = useState('')

  const [status, setStatus] = useState<UserStatus>('ACTIVE')
  const [limit, setLimit] = useState('')
  const [reset, setReset] = useState<ResetStrategy>('NO_RESET')
  const [squadIds, setSquadIds] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ created: Created[]; failed: Failure[] } | null>(null)

  // Shown before anything is created, so a wrong prefix is caught by reading
  // rather than by making five hundred users called `usre-1`.
  const preview = useMemo(() => {
    if (mode === 'list') {
      return names
        .split(/[\n,;]+/)
        .map((s) => s.trim())
        .filter(Boolean)
        .slice(0, 3)
    }
    const n = Math.max(1, Math.min(count, 500))
    const width = String(Math.max(1, start) + n - 1).length
    return [0, 1, n - 1]
      .filter((v, i, a) => a.indexOf(v) === i && v < n)
      .map((i) => prefix + String(Math.max(1, start) + i).padStart(width, '0'))
  }, [mode, names, prefix, count, start])

  async function submit() {
    setBusy(true)
    try {
      const body: Record<string, unknown> = {
        status,
        trafficLimitBytes: parseBytes(limit) || 0,
        trafficLimitStrategy: reset,
        squadUuids: squadIds,
      }
      if (mode === 'list') {
        body.names = names.split(/[\n,;]+/).map((s) => s.trim()).filter(Boolean)
      } else {
        body.prefix = prefix
        body.count = count
        body.start = start
      }
      const res = await api.post<{ created: Created[]; failed: Failure[] }>(
        '/api/users/bulk-create',
        body,
      )
      setResult(res)
      if (res.created.length > 0) onDone()
    } catch (err) {
      push(err instanceof Error ? err.message : 'error', 'error')
    } finally {
      setBusy(false)
    }
  }

  function downloadLinks() {
    if (!result) return
    const body = result.created.map((c) => `${c.username}\t${c.subscriptionUrl}`).join('\n')
    const url = URL.createObjectURL(new Blob([body + '\n'], { type: 'text/plain' }))
    const a = document.createElement('a')
    a.href = url
    a.download = 'amneziax-new-users.txt'
    a.click()
    URL.revokeObjectURL(url)
  }

  if (result) {
    return (
      <Modal title={t.bulk.done} onClose={onClose}>
        <div className="stack" style={{ gap: 12 }}>
          <span>{f(t.bulk.createdN, { n: result.created.length })}</span>

          {result.created.length > 0 && (
            <>
              <pre className="code-block" style={{ margin: 0, maxHeight: 220, overflow: 'auto' }}>
                {result.created.map((c) => `${c.username}\t${c.subscriptionUrl}`).join('\n')}
              </pre>
              <button className="btn" onClick={downloadLinks}>
                <Icon name="download" size={15} />
                {t.bulk.downloadLinks}
              </button>
            </>
          )}

          {result.failed.length > 0 && (
            <>
              <span className="small warn">{f(t.bulk.failedN, { n: result.failed.length })}</span>
              <pre className="code-block" style={{ margin: 0, maxHeight: 160, overflow: 'auto' }}>
                {result.failed.map((x) => `${x.username} — ${x.error}`).join('\n')}
              </pre>
            </>
          )}

          <button className="btn-primary" onClick={onClose}>
            {t.common.close}
          </button>
        </div>
      </Modal>
    )
  }

  return (
    <Modal title={t.bulk.title} onClose={onClose}>
      <div className="stack" style={{ gap: 12 }}>
        <div className="tabs">
          <button
            className={`tab${mode === 'prefix' ? ' active' : ''}`}
            onClick={() => setMode('prefix')}
          >
            {t.bulk.byPrefix}
          </button>
          <button
            className={`tab${mode === 'list' ? ' active' : ''}`}
            onClick={() => setMode('list')}
          >
            {t.bulk.byList}
          </button>
        </div>

        {mode === 'prefix' ? (
          <div className="grid cols-3" style={{ gap: 10 }}>
            <Field label={t.bulk.prefix}>
              <input value={prefix} onChange={(e) => setPrefix(e.target.value)} />
            </Field>
            <Field label={t.bulk.count}>
              <input
                type="number"
                min={1}
                max={500}
                value={count}
                onChange={(e) => setCount(Number(e.target.value))}
              />
            </Field>
            <Field label={t.bulk.start}>
              <input
                type="number"
                min={1}
                value={start}
                onChange={(e) => setStart(Number(e.target.value))}
              />
            </Field>
          </div>
        ) : (
          <Field label={t.bulk.names} hint={t.bulk.namesHint}>
            <textarea rows={6} value={names} onChange={(e) => setNames(e.target.value)} />
          </Field>
        )}

        <div className="small dim">
          {t.bulk.preview}: <span className="mono">{preview.join(', ')}</span>
          {mode === 'prefix' && count > 3 ? ' …' : ''}
        </div>

        <div className="grid cols-3" style={{ gap: 10 }}>
          <Field label={t.common.status}>
            <select value={status} onChange={(e) => setStatus(e.target.value as UserStatus)}>
              <option value="ACTIVE">{t.users.statusActive}</option>
              <option value="DISABLED">{t.users.statusDisabled}</option>
            </select>
          </Field>
          <Field label={t.users.trafficLimit}>
            <input
              value={limit}
              onChange={(e) => setLimit(e.target.value)}
              placeholder="100 GB"
            />
          </Field>
          <Field label={t.users.trafficStrategy}>
            <select value={reset} onChange={(e) => setReset(e.target.value as ResetStrategy)}>
              {(['NO_RESET', 'DAY', 'WEEK', 'MONTH'] as ResetStrategy[]).map((s) => (
                <option key={s} value={s}>
                  {t.strategy[s]}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <Field label={t.users.squads}>
          <CheckList
            items={squads.map((s) => ({ value: s.uuid, label: s.name }))}
            selected={squadIds}
            onChange={setSquadIds}
            emptyLabel={t.users.noSquads}
          />
        </Field>

        <div className="split" style={{ gap: 8 }}>
          <button className="btn-primary" onClick={() => void submit()} disabled={busy}>
            {busy ? <Spinner /> : <Icon name="plus" size={15} />}
            {t.bulk.create}
          </button>
          <button className="btn-ghost" onClick={onClose}>
            {t.common.cancel}
          </button>
        </div>
      </div>
    </Modal>
  )
}
