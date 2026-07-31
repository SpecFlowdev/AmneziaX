import type { Lang } from '../i18n'

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

/** Formats a byte count with the largest unit that keeps it readable. */
export function bytes(value: number, digits = 1): string {
  if (!value || value < 0) return '0 B'
  const exp = Math.min(Math.floor(Math.log(value) / Math.log(1024)), UNITS.length - 1)
  const scaled = value / 1024 ** exp
  return `${scaled.toFixed(exp === 0 ? 0 : digits)} ${UNITS[exp]}`
}

/** Parses "10 GB" / "500mb" / "1.5t" into bytes; empty means unlimited. */
export function parseBytes(input: string): number {
  const raw = input.trim().toLowerCase()
  if (!raw) return 0
  const match = raw.match(/^([\d.,]+)\s*([kmgtp]?)b?$/)
  if (!match) return Number.NaN
  const amount = Number.parseFloat(match[1].replace(',', '.'))
  if (Number.isNaN(amount)) return Number.NaN
  const exp = ['', 'k', 'm', 'g', 't', 'p'].indexOf(match[2])
  return Math.round(amount * 1024 ** exp)
}

export function percent(part: number, whole: number): number {
  if (whole <= 0) return 0
  return Math.min(100, (part / whole) * 100)
}

export function dateTime(value: string | null | undefined, lang: Lang): string {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString(lang === 'ru' ? 'ru-RU' : 'en-GB', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function dateOnly(value: string | null | undefined, lang: Lang): string {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-GB', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
  })
}

/** Compact relative time such as "3m ago" / "3 мин назад". */
export function relative(value: string | null | undefined, lang: Lang): string {
  if (!value) return '—'
  const then = new Date(value).getTime()
  if (Number.isNaN(then)) return '—'
  const diff = Date.now() - then
  const rtf = new Intl.RelativeTimeFormat(lang === 'ru' ? 'ru-RU' : 'en-GB', { numeric: 'auto' })
  const steps: [number, Intl.RelativeTimeFormatUnit][] = [
    [1000 * 60, 'minute'],
    [1000 * 60 * 60, 'hour'],
    [1000 * 60 * 60 * 24, 'day'],
    [1000 * 60 * 60 * 24 * 30, 'month'],
    [1000 * 60 * 60 * 24 * 365, 'year'],
  ]
  if (Math.abs(diff) < 60_000) return rtf.format(0, 'minute')
  for (let i = steps.length - 1; i >= 0; i--) {
    const [ms, unit] = steps[i]
    if (Math.abs(diff) >= ms) return rtf.format(-Math.round(diff / ms), unit)
  }
  return rtf.format(0, 'minute')
}

/** Human duration for uptimes, e.g. "4d 3h". */
export function duration(seconds: number): string {
  if (!seconds || seconds < 0) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

export function toDatetimeLocal(value: string | null): string {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function fromDatetimeLocal(value: string): string | null {
  if (!value) return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

/** Turns an ISO 3166-1 alpha-2 code into its flag emoji. */
export function flag(code: string): string {
  const cc = (code || '').toUpperCase()
  if (!/^[A-Z]{2}$/.test(cc) || cc === 'XX') return '🏳️'
  return String.fromCodePoint(...[...cc].map((c) => 0x1f1a5 + c.charCodeAt(0)))
}
