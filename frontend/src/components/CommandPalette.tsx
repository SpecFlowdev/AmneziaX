import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Icon } from './icons'
import { useI18n } from '../i18n'
import { api } from '../lib/api'

// The icon set is keyed by name rather than exported as a union.
type IconName = string

interface Hit {
  kind: 'user' | 'node' | 'host' | 'squad'
  uuid: string
  label: string
  hint?: string
}

interface Item {
  id: string
  label: string
  hint?: string
  icon: IconName
  go: () => void
}

const KIND_ICON: Record<Hit['kind'], IconName> = {
  user: 'users',
  node: 'server',
  host: 'globe',
  squad: 'layers',
}

/**
 * Ctrl+K / ⌘K. Every page is reachable in two keystrokes and a few letters, and
 * the same box finds a user, node, host or squad by name — the thing an
 * operator actually opens the panel to do.
 */
export function CommandPalette() {
  const { t } = useI18n()
  const navigate = useNavigate()

  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<Hit[]>([])
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const pages = useMemo<Item[]>(
    () =>
      (
        [
          ['/', t.nav.dashboard, 'dashboard'],
          ['/nodes', t.nav.nodes, 'server'],
          ['/hosts', t.nav.hosts, 'globe'],
          ['/profiles', t.nav.profiles, 'code'],
          ['/squads', t.nav.squads, 'layers'],
          ['/users', t.nav.users, 'users'],
          ['/admins', t.nav.admins, 'shield'],
          ['/notifications', t.nav.notifications, 'bell'],
          ['/announcements', t.nav.announcements, 'info'],
          ['/rules', t.nav.rules, 'link'],
          ['/inspect', t.nav.inspect, 'chart'],
          ['/events', t.nav.events, 'activity'],
          ['/backup', t.nav.backup, 'download'],
          ['/settings', t.nav.settings, 'settings'],
        ] as [string, string, IconName][]
      ).map(([path, label, icon]) => ({
        id: 'page:' + path,
        label,
        icon,
        go: () => navigate(path),
      })),
    [t, navigate],
  )

  const close = useCallback(() => {
    setOpen(false)
    setQ('')
    setHits([])
    setActive(0)
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((v) => !v)
        return
      }
      if (e.key === 'Escape') close()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [close])

  useEffect(() => {
    if (open) inputRef.current?.focus()
  }, [open])

  // Debounced, and every in-flight answer is discarded once a newer keystroke
  // has happened — otherwise a slow early request lands after a fast later one
  // and the list flips back to what was typed two letters ago.
  useEffect(() => {
    if (!open) return
    const term = q.trim()
    if (term.length < 2) {
      setHits([])
      return
    }
    let cancelled = false
    const timer = setTimeout(() => {
      api
        .get<Hit[]>(`/api/search?q=${encodeURIComponent(term)}`)
        .then((res) => {
          if (!cancelled) setHits(res)
        })
        .catch(() => {
          if (!cancelled) setHits([])
        })
    }, 140)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [q, open])

  const items = useMemo<Item[]>(() => {
    const term = q.trim().toLowerCase()
    const matchedPages = term
      ? pages.filter((p) => p.label.toLowerCase().includes(term))
      : pages
    const found: Item[] = hits.map((h) => ({
      id: `${h.kind}:${h.uuid}`,
      label: h.label,
      hint: h.hint,
      icon: KIND_ICON[h.kind],
      go: () => navigate(h.kind === 'user' ? `/users?open=${h.uuid}` : `/${h.kind}s`),
    }))
    return [...matchedPages, ...found]
  }, [pages, hits, q, navigate])

  useEffect(() => {
    setActive(0)
  }, [q])

  if (!open) return null

  function run(item: Item) {
    item.go()
    close()
  }

  return (
    <div className="overlay palette-overlay" onClick={close}>
      <div
        className="card palette"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-label={t.palette.title}
      >
        <div className="palette-input">
          <Icon name="qr" size={16} />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t.palette.placeholder}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') {
                e.preventDefault()
                setActive((i) => Math.min(i + 1, items.length - 1))
              } else if (e.key === 'ArrowUp') {
                e.preventDefault()
                setActive((i) => Math.max(i - 1, 0))
              } else if (e.key === 'Enter' && items[active]) {
                e.preventDefault()
                run(items[active])
              }
            }}
          />
          <kbd>esc</kbd>
        </div>

        <div className="palette-list">
          {items.length === 0 ? (
            <div className="palette-empty small dim">{t.palette.nothing}</div>
          ) : (
            items.map((item, i) => (
              <button
                key={item.id}
                className={`palette-item${i === active ? ' active' : ''}`}
                onMouseEnter={() => setActive(i)}
                onClick={() => run(item)}
              >
                <Icon name={item.icon} size={15} />
                <span>{item.label}</span>
                {item.hint && <span className="small dim">{item.hint}</span>}
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
