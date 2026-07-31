import { useState } from 'react'
import { Icon } from '../components/icons'
import { EmptyState, Spinner } from '../components/ui'
import { useI18n } from '../i18n'
import type { PanelEvent } from '../lib/api'
import { dateTime, relative } from '../lib/format'
import { useFetch } from '../lib/useApi'
import { EventBadge } from './Dashboard'

const KINDS = [
  'NODE_CONNECTED',
  'NODE_DISCONNECTED',
  'NODE_CONFIG_PUSHED',
  'NODE_ERROR',
  'USER_CREATED',
  'USER_UPDATED',
  'USER_DELETED',
  'USER_LIMITED',
  'USER_EXPIRED',
  'ADMIN_LOGIN',
  'ADMIN_LOGIN_FAILED',
  'PROFILE_UPDATED',
]

export function Events() {
  const { t, lang } = useI18n()
  const [kind, setKind] = useState('')
  const events = useFetch<PanelEvent[]>(
    `/api/system/events?limit=200${kind ? `&kind=${kind}` : ''}`,
    20_000,
  )

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.events.title}</h2>
          <p>{t.events.subtitle}</p>
        </div>
        <select value={kind} onChange={(e) => setKind(e.target.value)} style={{ width: 230 }}>
          <option value="">
            {t.events.filterKind}: {t.common.all}
          </option>
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {k.replaceAll('_', ' ').toLowerCase()}
            </option>
          ))}
        </select>
        <button className="btn-ghost" onClick={() => void events.reload()}>
          <Icon name="refresh" size={16} />
        </button>
      </div>

      <div className="card">
        {events.loading && !events.data ? (
          <div style={{ display: 'grid', placeItems: 'center', height: 160 }}>
            <Spinner />
          </div>
        ) : (events.data ?? []).length === 0 ? (
          <EmptyState title={t.common.nothingHere} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t.events.kind}</th>
                  <th>{t.events.actor}</th>
                  <th>{t.events.subject}</th>
                  <th>{t.events.message}</th>
                  <th>{t.events.when}</th>
                </tr>
              </thead>
              <tbody>
                {(events.data ?? []).map((e) => (
                  <tr key={e.id}>
                    <td>
                      <EventBadge kind={e.kind} />
                    </td>
                    <td className="small">{e.actor || '—'}</td>
                    <td className="small">{e.subject || '—'}</td>
                    <td className="muted">{e.message}</td>
                    <td className="small dim nowrap" title={dateTime(e.createdAt, lang)}>
                      {relative(e.createdAt, lang)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
