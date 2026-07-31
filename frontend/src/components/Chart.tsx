import { useMemo, useState } from 'react'
import { bytes } from '../lib/format'

export interface Series {
  id: string
  name: string
  points: { at: string; bytes: number }[]
}

// The palette walks the brand's crimson-to-rose range so a stack of nodes still
// reads as one family, with two cool accents to break up long lists.
const PALETTE = [
  '#e11d48',
  '#fb7195',
  '#9f1239',
  '#fda4b8',
  '#7f1d1d',
  '#f472b6',
  '#c2410c',
  '#7dd3fc',
  '#a78bfa',
  '#34d399',
]

/**
 * A stacked area chart drawn as plain SVG. Every series is aligned onto the
 * union of timestamps so the stack stays continuous even when a node reported
 * nothing during a bucket.
 */
export function StackedAreaChart({
  series,
  height = 240,
  formatX,
}: {
  series: Series[]
  height?: number
  formatX: (iso: string) => string
}) {
  const [hover, setHover] = useState<number | null>(null)

  const model = useMemo(() => {
    let stamps = Array.from(new Set(series.flatMap((s) => s.points.map((p) => p.at)))).sort()

    let lookup = series.map((s) => {
      const map = new Map(s.points.map((p) => [p.at, p.bytes]))
      return stamps.map((ts) => map.get(ts) ?? 0)
    })

    // A single bucket has no width to draw an area over, so mirror it into a
    // second column and let it render as a full-width band.
    if (stamps.length === 1) {
      stamps = [stamps[0], stamps[0]]
      lookup = lookup.map(([v]) => [v, v])
    }

    const totals = stamps.map((_, i) => lookup.reduce((sum, values) => sum + values[i], 0))
    const peak = Math.max(1, ...totals)
    return { stamps, lookup, totals, peak }
  }, [series])

  const { stamps, lookup, totals, peak } = model
  if (stamps.length === 0) return null

  const W = 1000
  const H = height
  const padTop = 12
  const padBottom = 26
  const plotH = H - padTop - padBottom

  const x = (i: number) => (i / (stamps.length - 1)) * W
  const y = (value: number) => padTop + plotH - (value / peak) * plotH

  // Build cumulative bands from the bottom up.
  const running = new Array(stamps.length).fill(0)
  const bands = lookup.map((values) => {
    const lower = [...running]
    values.forEach((v, i) => {
      running[i] += v
    })
    return { lower, upper: [...running] }
  })

  const areaPath = (band: { lower: number[]; upper: number[] }) => {
    const up = band.upper.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    const down = band.lower
      .map((v, i) => ({ v, i }))
      .reverse()
      .map(({ v, i }) => `L${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    return `${up.join('')}${down.join('')}Z`
  }

  const gridLines = [0, 0.25, 0.5, 0.75, 1]

  return (
    <div style={{ position: 'relative' }}>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="none"
        style={{ width: '100%', height, display: 'block' }}
        onMouseLeave={() => setHover(null)}
        onMouseMove={(e) => {
          const rect = e.currentTarget.getBoundingClientRect()
          const ratio = (e.clientX - rect.left) / rect.width
          setHover(Math.max(0, Math.min(stamps.length - 1, Math.round(ratio * (stamps.length - 1)))))
        }}
      >
        <defs>
          {series.map((s, i) => (
            <linearGradient key={s.id} id={`grad-${s.id}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={PALETTE[i % PALETTE.length]} stopOpacity="0.75" />
              <stop offset="100%" stopColor={PALETTE[i % PALETTE.length]} stopOpacity="0.22" />
            </linearGradient>
          ))}
        </defs>

        {gridLines.map((g) => (
          <line
            key={g}
            x1={0}
            x2={W}
            y1={padTop + plotH * g}
            y2={padTop + plotH * g}
            stroke="currentColor"
            strokeOpacity="0.1"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
        ))}

        {bands.map((band, i) => (
          <path
            key={series[i].id}
            d={areaPath(band)}
            fill={`url(#grad-${series[i].id})`}
            stroke={PALETTE[i % PALETTE.length]}
            strokeWidth="1.4"
            vectorEffect="non-scaling-stroke"
          />
        ))}

        {hover !== null && (
          <line
            x1={x(hover)}
            x2={x(hover)}
            y1={padTop}
            y2={padTop + plotH}
            stroke="currentColor"
            strokeOpacity="0.45"
            strokeDasharray="3 3"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
        )}
      </svg>

      <div
        className="small dim"
        style={{ display: 'flex', justifyContent: 'space-between', marginTop: -18 }}
      >
        <span>{formatX(stamps[0])}</span>
        <span>{formatX(stamps[stamps.length - 1])}</span>
      </div>

      {hover !== null && (
        <div
          style={{
            position: 'absolute',
            top: 8,
            left: `${(x(hover) / W) * 100}%`,
            transform: `translateX(${hover > stamps.length / 2 ? '-105%' : '5%'})`,
            background: 'var(--surface-2)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            padding: '8px 11px',
            fontSize: 12,
            pointerEvents: 'none',
            boxShadow: 'var(--shadow-lg)',
            minWidth: 150,
            zIndex: 5,
          }}
        >
          <div className="dim" style={{ marginBottom: 5 }}>
            {formatX(stamps[hover])}
          </div>
          <div style={{ fontWeight: 650, marginBottom: 5 }}>{bytes(totals[hover])}</div>
          {series.map((s, i) =>
            lookup[i][hover] > 0 ? (
              <div key={s.id} style={{ display: 'flex', gap: 7, alignItems: 'center' }}>
                <span
                  className="swatch"
                  style={{
                    width: 8,
                    height: 8,
                    borderRadius: 2,
                    background: PALETTE[i % PALETTE.length],
                    display: 'inline-block',
                  }}
                />
                <span className="dim" style={{ flex: 1 }}>
                  {s.name}
                </span>
                <span>{bytes(lookup[i][hover])}</span>
              </div>
            ) : null,
          )}
        </div>
      )}
    </div>
  )
}

export function ChartLegend({ series }: { series: Series[] }) {
  return (
    <div className="chart-legend">
      {series.map((s, i) => (
        <span key={s.id}>
          <span className="swatch" style={{ background: PALETTE[i % PALETTE.length] }} />
          {s.name}
        </span>
      ))}
    </div>
  )
}

/** Tiny inline bar chart used for per-user history. */
export function Sparkbars({ points }: { points: { at: string; bytes: number }[] }) {
  if (points.length === 0) return null
  const peak = Math.max(1, ...points.map((p) => p.bytes))
  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 56 }}>
      {points.map((p) => (
        <div
          key={p.at}
          title={`${new Date(p.at).toLocaleString()} — ${bytes(p.bytes)}`}
          style={{
            flex: 1,
            minWidth: 2,
            // Capped so a handful of buckets read as bars, not as one slab.
            maxWidth: 22,
            height: `${Math.max(3, (p.bytes / peak) * 100)}%`,
            background: 'linear-gradient(180deg, var(--rose-400), var(--rose-700))',
            borderRadius: 2,
          }}
        />
      ))}
    </div>
  )
}
