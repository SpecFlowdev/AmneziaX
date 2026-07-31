import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useI18n } from '../i18n'
import { Icon } from './icons'

// ------------------------------------------------------------------- toasts

type Toast = { id: number; message: string; kind: 'info' | 'success' | 'error' }
type ToastCtx = { push: (message: string, kind?: Toast['kind']) => void }

const ToastContext = createContext<ToastCtx | null>(null)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const push = useCallback((message: string, kind: Toast['kind'] = 'info') => {
    const id = Date.now() + Math.random()
    setToasts((prev) => [...prev, { id, message, kind }])
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 4200)
  }, [])

  const value = useMemo(() => ({ push }), [push])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toasts">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.kind}`}>
            <Icon
              name={t.kind === 'error' ? 'alert' : t.kind === 'success' ? 'check' : 'info'}
              size={16}
            />
            <span>{t.message}</span>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used inside ToastProvider')
  return ctx
}

/** Runs an async action and surfaces any error as a toast. */
export function useAction() {
  const { push } = useToast()
  return useCallback(
    async (fn: () => Promise<unknown>, successMessage?: string) => {
      try {
        await fn()
        if (successMessage) push(successMessage, 'success')
        return true
      } catch (err) {
        push(err instanceof Error ? err.message : String(err), 'error')
        return false
      }
    },
    [push],
  )
}

// ------------------------------------------------------------------- modal

export function Modal({
  title,
  onClose,
  children,
  footer,
  wide,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    }
  }, [onClose])

  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`modal${wide ? ' wide' : ''}`} role="dialog" aria-modal="true">
        <div className="modal-head">
          <h2>{title}</h2>
          <div style={{ flex: 1 }} />
          <button className="btn-ghost btn-icon" onClick={onClose} aria-label="Close">
            <Icon name="x" size={17} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  )
}

export function ConfirmDialog({
  message,
  onConfirm,
  onCancel,
  danger,
}: {
  message: string
  onConfirm: () => void
  onCancel: () => void
  danger?: boolean
}) {
  const { t } = useI18n()
  return (
    <Modal
      title={t.common.confirm}
      onClose={onCancel}
      footer={
        <>
          <button onClick={onCancel}>{t.common.cancel}</button>
          <button className={danger ? 'btn-danger' : 'btn-primary'} onClick={onConfirm}>
            {t.common.confirm}
          </button>
        </>
      }
    >
      <p style={{ margin: 0 }}>{message}</p>
    </Modal>
  )
}

// ------------------------------------------------------------------- bits

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="field">
      <label>{label}</label>
      {children}
      {hint && <span className="hint">{hint}</span>}
    </div>
  )
}

export function Badge({
  kind = 'muted',
  children,
  dot,
}: {
  kind?: 'ok' | 'warn' | 'danger' | 'info' | 'muted' | 'accent'
  children: ReactNode
  dot?: boolean
}) {
  return (
    <span className={`badge badge-${kind}`}>
      {dot && <span className="dot" />}
      {children}
    </span>
  )
}

export function Meter({ used, limit }: { used: number; limit: number }) {
  if (limit <= 0) return null
  const pct = Math.min(100, (used / limit) * 100)
  return (
    <div className={`meter${pct >= 90 ? ' over' : ''}`}>
      <span style={{ width: `${pct}%` }} />
    </div>
  )
}

export function Spinner() {
  return <div className="spinner" />
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="empty">
      <h4>{title}</h4>
      {hint && <div className="small">{hint}</div>}
    </div>
  )
}

export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useI18n()
  const { push } = useToast()
  return (
    <button
      className={label ? '' : 'btn-ghost btn-icon'}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value)
        } catch {
          // Clipboard access needs a secure context; fall back to a selection.
          const area = document.createElement('textarea')
          area.value = value
          document.body.appendChild(area)
          area.select()
          document.execCommand('copy')
          area.remove()
        }
        push(t.common.copied, 'success')
      }}
      title={t.common.copy}
    >
      <Icon name="copy" size={15} />
      {label}
    </button>
  )
}

export function Tabs<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div className="tabs">
      {options.map((o) => (
        <button
          key={o.value}
          className={`tab${o.value === value ? ' active' : ''}`}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** A multi-select rendered as a scrollable list of checkboxes. */
export function CheckList({
  items,
  selected,
  onChange,
  emptyLabel,
}: {
  items: { value: string; label: string; hint?: string }[]
  selected: string[]
  onChange: (next: string[]) => void
  emptyLabel: string
}) {
  if (items.length === 0) {
    return <div className="small dim">{emptyLabel}</div>
  }
  const toggle = (value: string) => {
    onChange(selected.includes(value) ? selected.filter((v) => v !== value) : [...selected, value])
  }
  return (
    <div
      style={{
        maxHeight: 210,
        overflowY: 'auto',
        border: '1px solid var(--border-soft)',
        borderRadius: 'var(--radius-sm)',
        padding: 10,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
      }}
    >
      {items.map((item) => (
        <label key={item.value} className="checkbox">
          <input
            type="checkbox"
            checked={selected.includes(item.value)}
            onChange={() => toggle(item.value)}
          />
          <span className="stack">
            <span>{item.label}</span>
            {item.hint && <span className="small dim">{item.hint}</span>}
          </span>
        </label>
      ))}
    </div>
  )
}
