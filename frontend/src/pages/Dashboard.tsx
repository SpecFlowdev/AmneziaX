import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChartLegend, StackedAreaChart, type Series } from '../components/Chart'
import { Icon } from '../components/icons'
import { Badge, EmptyState, Spinner, Tabs } from '../components/ui'
import { useI18n } from '../i18n'
import type { Node, Overview, PanelEvent, SpendSummary, TrafficStats } from '../lib/api'
import { bytes, dateOnly, duration, flag, percent, relative } from '../lib/format'
import { useFetch } from '../lib/useApi'

function Stat({
  label,
  value,
  hint,
  icon,
}: {
  label: string
  value: string
  hint?: string
  icon: string
}) {
  return (
    <div className="card stat">
      <Icon name={icon} size={19} className="stat-icon" />
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
      {hint && <div className="stat-hint">{hint}</div>}
    </div>
  )
}

export function Dashboard() {
  const { t, lang, f } = useI18n()
  const [days, setDays] = useState<'1' | '7' | '30'>('7')

  const overview = useFetch<Overview>('/api/system/overview', 10_000)
  const traffic = useFetch<TrafficStats>(`/api/system/stats/traffic?days=${days}`, 30_000)
  const topUsers = useFetch<{ uuid: string; username: string; bytes: number }[]>(
    `/api/system/stats/top-users?days=${days}&limit=8`,
    60_000,
  )
  const nodes = useFetch<Node[]>('/api/nodes', 10_000)
  const events = useFetch<PanelEvent[]>('/api/system/events?limit=12', 30_000)
  const spend = useFetch<SpendSummary>('/api/system/spend', 60_000)

  const series = useMemo<Series[]>(
    () =>
      (traffic.data?.nodes ?? []).map((n) => ({
        id: n.nodeUuid,
        name: n.nodeName,
        points: n.points,
      })),
    [traffic.data],
  )

  const formatX = (iso: string) =>
    new Date(iso).toLocaleString(lang === 'ru' ? 'ru-RU' : 'en-GB', {
      month: 'short',
      day: '2-digit',
      ...(traffic.data?.interval === 'hour' ? { hour: '2-digit', minute: '2-digit' } : {}),
    })

  const c = overview.data?.counters

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2 style={{ fontSize: 22 }}>{t.dashboard.title}</h2>
          <p>{t.dashboard.subtitle}</p>
        </div>
      </div>

      <div className="grid cols-4">
        <Stat
          label={t.dashboard.usersTotal}
          value={String(c?.usersTotal ?? '—')}
          hint={c ? `${c.usersActive} ${t.dashboard.usersActive}` : undefined}
          icon="users"
        />
        <Stat
          label={t.dashboard.usersOnline}
          value={String(c?.usersOnline ?? '—')}
          hint={t.dashboard.onlineHint}
          icon="activity"
        />
        <Stat
          label={t.dashboard.nodes}
          value={String(c?.nodesTotal ?? '—')}
          hint={c ? `${c.nodesOnline} ${t.dashboard.nodesOnline}` : undefined}
          icon="server"
        />
        <Stat
          label={t.dashboard.traffic24}
          value={c ? bytes(c.trafficLast24hBytes) : '—'}
          hint={c ? `${bytes(c.trafficLast7dBytes)} ${t.dashboard.traffic7}` : undefined}
          icon="chart"
        />
      </div>

      <div className="card">
        <div className="card-head">
          <h3>{t.dashboard.trafficChart}</h3>
          <div className="spacer" />
          <Tabs
            value={days}
            onChange={setDays}
            options={[
              { value: '1', label: t.dashboard.range1 },
              { value: '7', label: t.dashboard.range7 },
              { value: '30', label: t.dashboard.range30 },
            ]}
          />
        </div>
        <div className="card-pad">
          {traffic.loading && !traffic.data ? (
            <div style={{ display: 'grid', placeItems: 'center', height: 240 }}>
              <Spinner />
            </div>
          ) : series.length === 0 ? (
            <EmptyState title={t.dashboard.noTraffic} />
          ) : (
            <>
              <StackedAreaChart series={series} formatX={formatX} />
              <div style={{ marginTop: 14 }}>
                <ChartLegend series={series} />
              </div>
            </>
          )}
        </div>
      </div>

      {spend.data && spend.data.billedNodes > 0 && (
        <div className="grid cols-2">
          <div className="card">
            <div className="card-head">
              <Icon name="chart" size={17} />
              <h3>{t.billing.title}</h3>
              <div className="spacer" />
              {spend.data.overdue > 0 && (
                <Badge kind="danger" dot>
                  {spend.data.overdue} {t.billing.overdue}
                </Badge>
              )}
            </div>
            <div className="card-pad stack" style={{ gap: 14 }}>
              <div className="grid cols-2" style={{ gap: 12 }}>
                <div className="stack">
                  <span className="small dim">{t.billing.monthly}</span>
                  <span style={{ fontSize: 24, fontWeight: 700 }}>
                    {money(spend.data.monthlyTotal, spend.data.currency)}
                  </span>
                  <span className="small dim">
                    {money(spend.data.yearlyTotal, spend.data.currency)} {t.billing.yearly}
                  </span>
                </div>
                <div className="stack">
                  <span className="small dim">{t.billing.costPerTb}</span>
                  <span style={{ fontSize: 24, fontWeight: 700 }}>
                    {spend.data.costPerTb > 0
                      ? money(spend.data.costPerTb, spend.data.currency)
                      : '—'}
                  </span>
                  <span className="small dim">
                    {spend.data.trafficThisMonthTb.toFixed(2)} TB {t.billing.trafficThisMonth}
                  </span>
                </div>
              </div>

              {spend.data.byProvider.length > 0 && (
                <>
                  <hr className="sep" />
                  <span className="small dim">{t.billing.byProvider}</span>
                  {spend.data.byProvider.map((p) => (
                    <div className="split small" key={p.provider}>
                      <span>{p.provider}</span>
                      <span className="dim">×{p.nodes}</span>
                      <div style={{ flex: 1 }} />
                      <span className="nums">{money(p.monthly, spend.data!.currency)}</span>
                    </div>
                  ))}
                </>
              )}
            </div>
          </div>

          <div className="card">
            <div className="card-head">
              <Icon name="clock" size={17} />
              <h3>{t.billing.upcoming}</h3>
            </div>
            {spend.data.upcoming.length === 0 ? (
              <EmptyState title={t.common.nothingHere} />
            ) : (
              <div className="table-wrap">
                <table>
                  <tbody>
                    {spend.data.upcoming.map((b) => (
                      <tr key={b.nodeUuid}>
                        <td>
                          <div className="stack">
                            <strong>{b.nodeName}</strong>
                            <span className="small dim">{b.provider || '—'}</span>
                          </div>
                        </td>
                        <td className="right nums nowrap">{money(b.amount, b.currency)}</td>
                        <td className="right nowrap" style={{ width: 1 }}>
                          <div className="stack" style={{ alignItems: 'flex-end' }}>
                            <span className="small">{dateOnly(b.dueAt, lang)}</span>
                            <Badge
                              kind={b.daysLeft < 0 ? 'danger' : b.daysLeft <= 7 ? 'warn' : 'muted'}
                            >
                              {b.daysLeft < 0
                                ? f(t.billing.dueLate, { n: -b.daysLeft })
                                : b.daysLeft === 0
                                  ? t.billing.dueToday
                                  : f(t.billing.dueIn, { n: b.daysLeft })}
                            </Badge>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="grid cols-2">
        <div className="card">
          <div className="card-head">
            <h3>{t.nav.nodes}</h3>
            <div className="spacer" />
            <Link to="/nodes" className="btn btn-sm btn-ghost">
              {t.nav.nodes}
              <Icon name="chevronRight" size={14} />
            </Link>
          </div>
          {(nodes.data ?? []).length === 0 ? (
            <EmptyState title={t.common.nothingHere} />
          ) : (
            <div className="table-wrap">
              <table>
                <tbody>
                  {(nodes.data ?? []).map((n) => (
                    <tr key={n.uuid}>
                      <td>
                        <div className="split">
                          <span style={{ fontSize: 17 }}>{flag(n.countryCode)}</span>
                          <div className="stack">
                            <span style={{ fontWeight: 600 }}>{n.name}</span>
                            <span className="small dim">{n.address || '—'}</span>
                          </div>
                        </div>
                      </td>
                      <td className="right nums small">
                        <div className="stack">
                          <span>{bytes(n.trafficUsedBytes)}</span>
                          <span className="dim">
                            {n.xrayRunning ? `${n.cpuUsagePercent.toFixed(0)}% CPU` : '—'}
                          </span>
                        </div>
                      </td>
                      <td style={{ width: 1 }}>
                        <NodeHealthBadge node={n} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="card">
          <div className="card-head">
            <h3>{t.dashboard.topUsers}</h3>
          </div>
          {(topUsers.data ?? []).length === 0 ? (
            <EmptyState title={t.common.nothingHere} />
          ) : (
            <div className="table-wrap">
              <table>
                <tbody>
                  {(topUsers.data ?? []).map((u, i) => (
                    <tr key={u.uuid}>
                      <td style={{ width: 1 }} className="dim nums">
                        {i + 1}
                      </td>
                      <td>
                        <Link to={`/users?search=${encodeURIComponent(u.username)}`}>
                          {u.username}
                        </Link>
                      </td>
                      <td style={{ width: '45%' }}>
                        <div className="meter">
                          <span
                            style={{
                              width: `${percent(u.bytes, topUsers.data?.[0]?.bytes ?? 1)}%`,
                            }}
                          />
                        </div>
                      </td>
                      <td className="right nums nowrap">{bytes(u.bytes)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h3>{t.dashboard.recentEvents}</h3>
          <div className="spacer" />
          <Link to="/events" className="btn btn-sm btn-ghost">
            {t.nav.events}
            <Icon name="chevronRight" size={14} />
          </Link>
        </div>
        {(events.data ?? []).length === 0 ? (
          <EmptyState title={t.common.nothingHere} />
        ) : (
          <div className="table-wrap">
            <table>
              <tbody>
                {(events.data ?? []).map((e) => (
                  <tr key={e.id}>
                    <td style={{ width: 1 }}>
                      <EventBadge kind={e.kind} />
                    </td>
                    <td>
                      {e.subject && <strong>{e.subject}</strong>} <span className="muted">{e.message}</span>
                    </td>
                    <td className="right dim small nowrap">{relative(e.createdAt, lang)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {overview.data && (
        <div className="split small dim" style={{ justifyContent: 'center', gap: 18 }}>
          <span>
            {t.dashboard.panelVersion}: {overview.data.panel.version}
          </span>
          <span>
            {t.dashboard.uptime}: {duration(overview.data.panel.uptimeSeconds)}
          </span>
          <span>{f(t.dashboard.connectedNodes, { n: overview.data.panel.connectedNodes })}</span>
        </div>
      )}
    </div>
  )
}

/** Formats an amount with its currency, falling back to a plain number. */
export function money(amount: number, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: currency || 'USD',
      maximumFractionDigits: amount < 100 ? 2 : 0,
    }).format(amount)
  } catch {
    return `${amount.toFixed(2)} ${currency}`
  }
}

export function NodeHealthBadge({ node }: { node: Node }) {
  const { t } = useI18n()
  const map: Record<string, { kind: 'ok' | 'warn' | 'danger' | 'muted' | 'info'; label: string }> = {
    ONLINE: { kind: 'ok', label: t.nodes.healthOnline },
    CONNECTING: { kind: 'info', label: t.nodes.healthConnecting },
    DEGRADED: { kind: 'warn', label: t.nodes.healthDegraded },
    OFFLINE: { kind: 'danger', label: t.nodes.healthOffline },
    DISABLED: { kind: 'muted', label: t.nodes.healthDisabled },
    TRAFFIC_LIMITED: { kind: 'warn', label: t.nodes.healthTrafficLimited },
    UNKNOWN: { kind: 'muted', label: t.nodes.healthUnknown },
  }
  const entry = map[node.health] ?? map.UNKNOWN
  return (
    <Badge kind={entry.kind} dot>
      {entry.label}
    </Badge>
  )
}

export function EventBadge({ kind }: { kind: string }) {
  const tone: 'ok' | 'warn' | 'danger' | 'info' | 'muted' | 'accent' = kind.includes('ERROR')
    ? 'danger'
    : kind.includes('DISCONNECT') || kind.includes('FAILED') || kind.includes('LIMITED')
      ? 'warn'
      : kind.includes('CONNECTED')
        ? 'ok'
        : 'muted'
  return (
    <Badge kind={tone}>{kind.replaceAll('_', ' ').toLowerCase()}</Badge>
  )
}
