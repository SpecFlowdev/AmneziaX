import { useState } from 'react'
import { Icon } from '../components/icons'
import { EmptyState, Spinner, Tabs } from '../components/ui'
import { useI18n } from '../i18n'
import { bytes, dateTime, relative } from '../lib/format'
import { useFetch } from '../lib/useApi'

interface DeviceRow {
  userUuid: string
  username: string
  hwid: string
  userAgent: string
  platform: string
  firstSeen: string
  lastSeen: string
}

interface RequestRow {
  id: number
  userUuid: string | null
  username: string
  token: string
  ip: string
  userAgent: string
  format: string
  status: number
  hwid: string
  at: string
}

interface SessionRow {
  userUuid: string
  username: string
  status: string
  nodeName: string
  countryCode: string
  bytes: number
  lastSeen: string
}

type Tab = 'devices' | 'requests' | 'sessions'

export function Inspect() {
  const { t, lang } = useI18n()
  const [tab, setTab] = useState<Tab>('requests')
  const [query, setQuery] = useState('')
  const [failedOnly, setFailedOnly] = useState(false)

  const devices = useFetch<DeviceRow[]>(
    `/api/inspect/devices?limit=200${query ? `&q=${encodeURIComponent(query)}` : ''}`,
    30_000,
  )
  const sessions = useFetch<SessionRow[]>('/api/inspect/sessions?minutes=15', 15_000)
  const requests = useFetch<RequestRow[]>(
    `/api/inspect/subscriptions?limit=200${failedOnly ? '&failed=1' : ''}`,
    15_000,
  )

  const active = tab === 'devices' ? devices : tab === 'sessions' ? sessions : requests

  return (
    <div className="page">
      <div className="page-head">
        <div style={{ flex: 1 }}>
          <h2 style={{ fontSize: 22 }}>{t.inspect.title}</h2>
          <p>{t.inspect.subtitle}</p>
        </div>
        <button className="btn-ghost" onClick={() => void active.reload()}>
          <Icon name="refresh" size={16} />
        </button>
      </div>

      <div className="split" style={{ gap: 12, flexWrap: 'wrap' }}>
        <Tabs
          value={tab}
          onChange={setTab}
          options={[
            { value: 'requests', label: t.inspect.requests },
            { value: 'devices', label: t.inspect.devices },
            { value: 'sessions', label: t.inspect.sessions },
          ]}
        />
        <div style={{ flex: 1 }} />
        {tab === 'sessions' ? (
          <span className="small dim">{t.inspect.sessionsWindow}</span>
        ) : tab === 'devices' ? (
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t.inspect.searchDevices}
            style={{ width: 280 }}
          />
        ) : (
          <label className="checkbox">
            <input
              type="checkbox"
              checked={failedOnly}
              onChange={(e) => setFailedOnly(e.target.checked)}
            />
            {t.inspect.failedOnly}
          </label>
        )}
      </div>

      <div className="card">
        {active.loading && !active.data ? (
          <div style={{ display: 'grid', placeItems: 'center', height: 180 }}>
            <Spinner />
          </div>
        ) : tab === 'sessions' ? (
          (sessions.data ?? []).length === 0 ? (
            <EmptyState title={t.common.nothingHere} hint={t.inspect.sessionsHint} />
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t.inspect.user}</th>
                    <th>{t.nav.nodes}</th>
                    <th className="right">{t.inspect.moved}</th>
                    <th>{t.inspect.lastSeen}</th>
                  </tr>
                </thead>
                <tbody>
                  {(sessions.data ?? []).map((x) => (
                    <tr key={`${x.userUuid}-${x.nodeName}`}>
                      <td>{x.username}</td>
                      <td className="small">{x.nodeName}</td>
                      <td className="right small nums">{bytes(x.bytes)}</td>
                      <td className="small dim nowrap" title={dateTime(x.lastSeen, lang)}>
                        {relative(x.lastSeen, lang)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        ) : tab === 'devices' ? (
          (devices.data ?? []).length === 0 ? (
            <EmptyState title={t.common.nothingHere} hint={t.inspect.devicesHint} />
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t.inspect.user}</th>
                    <th>HWID</th>
                    <th>{t.inspect.platform}</th>
                    <th>{t.inspect.firstSeen}</th>
                    <th>{t.inspect.lastSeen}</th>
                  </tr>
                </thead>
                <tbody>
                  {(devices.data ?? []).map((d) => (
                    <tr key={`${d.userUuid}-${d.hwid}`}>
                      <td>{d.username}</td>
                      <td className="mono small">{d.hwid.slice(0, 28)}</td>
                      <td className="small dim truncate" title={d.userAgent}>
                        {d.platform || d.userAgent || '—'}
                      </td>
                      <td className="small dim nowrap">{relative(d.firstSeen, lang)}</td>
                      <td className="small dim nowrap" title={dateTime(d.lastSeen, lang)}>
                        {relative(d.lastSeen, lang)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        ) : (requests.data ?? []).length === 0 ? (
          <EmptyState title={t.common.nothingHere} hint={t.inspect.requestsHint} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t.inspect.user}</th>
                  <th>{t.inspect.result}</th>
                  <th>{t.inspect.format}</th>
                  <th>{t.inspect.client}</th>
                  <th>IP</th>
                  <th>{t.events.when}</th>
                </tr>
              </thead>
              <tbody>
                {(requests.data ?? []).map((r) => (
                  <tr key={r.id}>
                    {/* A request that resolved to nobody has no username to
                        show, so the token stands in — that is the row an
                        operator is looking for. */}
                    <td className={r.username ? '' : 'mono small dim'}>
                      {r.username || r.token.slice(0, 16) || '—'}
                    </td>
                    <td>
                      <span
                        className={`badge${r.status >= 400 ? ' badge-danger' : ' badge-ok'}`}
                      >
                        {r.status}
                      </span>
                    </td>
                    <td className="small dim">{r.format || '—'}</td>
                    <td className="small dim truncate" title={r.userAgent}>
                      {r.userAgent || '—'}
                    </td>
                    <td className="small dim mono nowrap">{r.ip || '—'}</td>
                    <td className="small dim nowrap" title={dateTime(r.at, lang)}>
                      {relative(r.at, lang)}
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
