import { useRef, useState } from 'react'
import { Icon } from '../components/icons'
import { ConfirmDialog, Spinner, useAction, useToast } from '../components/ui'
import { useI18n } from '../i18n'
import { useFetch } from '../lib/useApi'
import { getToken } from '../lib/api'

interface BackupSummary {
  schema: string
  counts: Record<string, number>
}

interface RestoreResult {
  schema: string
  restored: Record<string, number>
}

export function Backup() {
  const { t } = useI18n()
  const { push } = useToast()
  const run = useAction()
  const summary = useFetch<BackupSummary>('/api/backup')
  const fileInput = useRef<HTMLInputElement>(null)

  const [pending, setPending] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)

  const counts = Object.entries(summary.data?.counts ?? {})
    .filter(([, n]) => n > 0)
    .sort(([a], [b]) => a.localeCompare(b))

  // The export is a plain authenticated GET, so it is fetched rather than
  // linked: an <a href> carries no Authorization header.
  const download = () =>
    run(async () => {
      setBusy(true)
      try {
        const res = await fetch('/api/backup/export', {
          headers: { Authorization: `Bearer ${getToken() ?? ''}` },
        })
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const blob = await res.blob()
        const name =
          res.headers
            .get('Content-Disposition')
            ?.match(/filename="([^"]+)"/)?.[1] ?? 'amneziax-backup.json'

        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = name
        a.click()
        URL.revokeObjectURL(url)
      } finally {
        setBusy(false)
      }
    })

  const restore = (file: File) =>
    run(async () => {
      setBusy(true)
      try {
        const res = await fetch('/api/backup/import', {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${getToken() ?? ''}`,
            'Content-Type': 'application/json',
          },
          body: await file.text(),
        })
        const body = (await res.json()) as RestoreResult & { error?: string }
        if (!res.ok) throw new Error(body.error ?? `HTTP ${res.status}`)

        const total = Object.values(body.restored ?? {}).reduce((a, b) => a + b, 0)
        push(`${t.backup.restored} — ${total}`, 'success')
        await summary.reload()
      } finally {
        setBusy(false)
        if (fileInput.current) fileInput.current.value = ''
      }
    })

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.backup.title}</h2>
          <p>{t.backup.subtitle}</p>
        </div>
      </div>

      <div className="grid cols-2">
        <div className="card card-pad stack" style={{ gap: 12 }}>
          <strong style={{ fontSize: 15 }}>{t.backup.exportTitle}</strong>
          <p className="small dim" style={{ margin: 0 }}>
            {t.backup.exportHint}
          </p>

          {/* Saying plainly what is in the file, because it is not a report —
              it is every credential the deployment holds. */}
          <div className="badge badge-warn" style={{ alignSelf: 'flex-start' }}>
            <Icon name="alert" size={14} />
            {t.backup.secretsWarning}
          </div>

          {summary.loading && !summary.data ? (
            <Spinner />
          ) : (
            <>
              <div className="small dim">
                {t.backup.schema}: <span className="mono">{summary.data?.schema ?? '—'}</span>
              </div>
              <div className="table-wrap">
                <table>
                  <tbody>
                    {counts.map(([table, n]) => (
                      <tr key={table}>
                        <td className="small mono">{table}</td>
                        <td className="right small nums">{n}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          <button className="btn-primary" onClick={() => void download()} disabled={busy}>
            {busy ? <Spinner /> : <Icon name="download" size={15} />}
            {t.backup.download}
          </button>
        </div>

        <div className="card card-pad stack" style={{ gap: 12 }}>
          <strong style={{ fontSize: 15 }}>{t.backup.importTitle}</strong>
          <p className="small dim" style={{ margin: 0 }}>
            {t.backup.importHint}
          </p>

          <div className="badge badge-danger" style={{ alignSelf: 'flex-start' }}>
            <Icon name="alert" size={14} />
            {t.backup.replaceWarning}
          </div>

          <input
            ref={fileInput}
            type="file"
            accept="application/json,.json"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) setPending(file)
            }}
          />

          <p className="small dim" style={{ margin: 0 }}>
            {t.backup.schemaNote}
          </p>
        </div>
      </div>

      {pending && (
        <ConfirmDialog
          message={`${t.backup.confirmRestore} ${pending.name}`}
          danger
          onCancel={() => {
            setPending(null)
            if (fileInput.current) fileInput.current.value = ''
          }}
          onConfirm={async () => {
            const file = pending
            setPending(null)
            await restore(file)
          }}
        />
      )}
    </div>
  )
}
