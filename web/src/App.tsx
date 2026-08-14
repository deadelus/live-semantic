import { useState } from 'react'
import { SourcesList } from './SourcesList'
import { LiveView } from './LiveView'
import { Library } from './Library'
import { ThemeToggle } from './components/ThemeToggle'
import { ToastProvider } from './toast/ToastProvider'
import { sourceLabel, type SessionInfo } from './api'
import './App.css'

type Tab = 'sources' | 'library'

// Live-session status for the tab pill's dot (docs/gui/mockups/ screen
// 1c's own top-bar tab: a small dot, pulsing green while connected,
// steady amber otherwise — color/pulse state, not a full StatusBadge,
// that lives in the tab itself). Lifted up from LiveView via
// onStatusChange since the mockup renders the open session as a *tab in
// this same top bar*, not a separate sub-bar below it (2026-08-14
// fidelity pass — LiveView used to render its own full-width status row,
// removed in favor of matching the mockup's actual structure).
interface LiveStatus {
  connected: boolean
  hasError: boolean
}

// Top-level view router — three tabs in one row, exactly the mockup's own
// pattern (screen 1c's top bar: "Sources" + the currently open source as
// a pill tab with a live status dot + name + ×, screen 4a's "Sources /
// Bibliothèque" pair) — not three independent "screens" the rest of the
// app pretends don't share a bar. No react-router: still just a handful
// of tabs, no deep-linking need has come up.
//
// ToastProvider wraps everything once here (2026-08-13) so every screen
// can raise a clean error toast without knowing about each other —
// deliberately *not* the backend's raw error text (explicitly not
// wanted): call sites translate failures into short human sentences.
function App() {
  const [tab, setTab] = useState<Tab>('sources')
  const [activeSession, setActiveSession] = useState<SessionInfo | null>(null)
  const [liveStatus, setLiveStatus] = useState<LiveStatus>({ connected: false, hasError: false })

  const openTab = (t: Tab) => {
    setActiveSession(null) // switching tabs always leaves the live view
    setTab(t)
  }

  const closeSession = () => setActiveSession(null)

  return (
    <ToastProvider>
      <div className="ls-app">
        <header className="ls-topbar">
          <div className="ls-topbar__brand">
            <span className="ls-topbar__dot" />
            LiveSemantic
          </div>
          <nav className="ls-topbar__tabs">
            <button
              className={`ls-topbar__tab ${!activeSession && tab === 'sources' ? 'ls-topbar__tab--active' : ''}`}
              onClick={() => openTab('sources')}
            >
              Sources
            </button>
            <button
              className={`ls-topbar__tab ${!activeSession && tab === 'library' ? 'ls-topbar__tab--active' : ''}`}
              onClick={() => openTab('library')}
            >
              Bibliothèque
            </button>
            {activeSession && (
              <span className="ls-topbar__tab ls-topbar__tab--active ls-topbar__tab--session">
                <span
                  className={`ls-topbar__session-dot ${liveStatus.hasError ? 'ls-topbar__session-dot--error' : liveStatus.connected ? 'ls-topbar__session-dot--live' : ''}`}
                />
                {sourceLabel(activeSession)}
                <span className="ls-topbar__tab-close" onClick={closeSession}>
                  ×
                </span>
              </span>
            )}
          </nav>
          <div className="ls-topbar__spacer" />
          <ThemeToggle />
        </header>

        {activeSession ? (
          <LiveView session={activeSession} onStatusChange={setLiveStatus} />
        ) : tab === 'sources' ? (
          <SourcesList onOpen={setActiveSession} />
        ) : (
          <Library />
        )}
      </div>
    </ToastProvider>
  )
}

export default App
