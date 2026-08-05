import { Link } from 'react-router-dom'
import { Icon } from './icons'
import { useI18n } from '../i18n'
import { useFetch } from '../lib/useApi'

interface Warning {
  userUuid: string
  username: string
  daysLeft?: number
  percent?: number
}

interface AttentionData {
  nodesOffline: number
  nodesDegraded: number
  nodesOverQuota: number
  usersExpiring: number
  usersNearQuota: number
  usersLimited: number
  paymentsDueSoon: number
  expiring: Warning[]
  nearQuota: Warning[]
}

/**
 * One card answering "is anything wrong right now". When nothing is, it says so
 * and takes up almost no room — a panel that always shows a wall of warnings
 * teaches the operator to stop reading them.
 */
export function Attention() {
  const { t, f } = useI18n()
  const { data } = useFetch<AttentionData>('/api/system/attention', 30_000)

  if (!data) return null

  const rows: { key: string; label: string; to: string; bad: boolean }[] = []
  const add = (n: number, label: string, to: string, bad = true) => {
    if (n > 0) rows.push({ key: label, label, to, bad })
  }

  add(data.nodesOffline, f(t.attention.nodesOffline, { n: data.nodesOffline }), '/nodes')
  add(data.nodesDegraded, f(t.attention.nodesDegraded, { n: data.nodesDegraded }), '/nodes')
  add(data.nodesOverQuota, f(t.attention.nodesOverQuota, { n: data.nodesOverQuota }), '/nodes')
  add(data.usersLimited, f(t.attention.usersLimited, { n: data.usersLimited }), '/users')
  add(data.usersExpiring, f(t.attention.usersExpiring, { n: data.usersExpiring }), '/users')
  add(data.usersNearQuota, f(t.attention.usersNearQuota, { n: data.usersNearQuota }), '/users')
  add(data.paymentsDueSoon, f(t.attention.paymentsDue, { n: data.paymentsDueSoon }), '/nodes')

  if (rows.length === 0) {
    return (
      <div className="card card-pad split" style={{ gap: 10 }}>
        <Icon name="check" size={16} className="ok" />
        <span className="small">{t.attention.allClear}</span>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-head">
        <Icon name="alert" size={17} />
        <h3>{t.attention.title}</h3>
      </div>
      <div className="card-pad stack" style={{ gap: 10 }}>
        <div className="split" style={{ gap: 8, flexWrap: 'wrap' }}>
          {rows.map((r) => (
            <Link key={r.key} to={r.to} className={`pill${r.bad ? ' pill-warn' : ''}`}>
              {r.label}
            </Link>
          ))}
        </div>

        {/* Names, not just counts: a number the operator still has to go
            looking for is barely better than no number. */}
        {(data.expiring.length > 0 || data.nearQuota.length > 0) && (
          <div className="stack small dim" style={{ gap: 4 }}>
            {data.expiring.map((w) => (
              <span key={'e' + w.userUuid}>
                {w.username} — {f(t.attention.expiresIn, { n: w.daysLeft ?? 0 })}
              </span>
            ))}
            {data.nearQuota.map((w) => (
              <span key={'q' + w.userUuid}>
                {w.username} — {f(t.attention.usedPercent, { n: w.percent ?? 0 })}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
