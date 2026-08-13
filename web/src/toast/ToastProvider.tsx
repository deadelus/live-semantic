import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import './Toast.css'

interface Toast {
  id: number
  // Short, human-readable message written per call site — deliberately
  // not the backend's raw error string (explicitly not wanted here,
  // 2026-08-13): call sites translate whatever failed into a clean
  // sentence instead of surfacing JSON/Go error text verbatim.
  message: string
}

interface ToastContextValue {
  pushError: (message: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

// Auto-dismiss delay. Manual close (×) always available too.
const AUTO_DISMISS_MS = 6000

// Single provider for the whole app (mounted once in App.tsx) rather
// than a toast per screen — SourcesList and LiveView both need to raise
// errors (failed REST calls, and backend-reported async failures picked
// up by polling — docs/gui/spec.md's own "GUI has no way to show the
// user why nothing happened" gap this responds to) and neither should
// have to know about the other's toast stack.
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(0)

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const pushError = useCallback(
    (message: string) => {
      const id = nextId.current++
      setToasts((prev) => [...prev, { id, message }])
      window.setTimeout(() => dismiss(id), AUTO_DISMISS_MS)
    },
    [dismiss],
  )

  return (
    <ToastContext.Provider value={{ pushError }}>
      {children}
      <div className="ls-toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className="ls-toast">
            <span className="ls-toast__message">{t.message}</span>
            <button className="ls-toast__close" onClick={() => dismiss(t.id)} aria-label="Fermer">
              ×
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider')
  }
  return ctx
}
