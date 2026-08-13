import { useState } from 'react'
import { SourcesList } from './SourcesList'
import { LiveView } from './LiveView'
import { Library } from './Library'
import { ThemeToggle } from './components/ThemeToggle'
import { ToastProvider } from './toast/ToastProvider'
import type { SessionInfo } from './api'
import './App.css'

type Tab = 'sources' | 'library'

// Top-level view router — two permanent tabs (Sources / Bibliothèque,
// docs/gui/mockups/ screens 4a's own top-bar tab pattern), plus the live
// view (1c/1f) which temporarily replaces whichever tab is active when a
// source is opened, back button returns to it. No react-router: three
// screens still don't justify the dependency (no deep-linking need has
// come up), revisit if that changes. Session lifecycle owned entirely by
// SourcesList (create/list/remove) — App.tsx only tracks which one is
// currently open, LiveView never creates or deletes a session itself.
//
// ToastProvider wraps everything once here (2026-08-13) so every screen
// can raise a clean error toast without knowing about each other —
// deliberately *not* the backend's raw error text (explicitly not
// wanted): call sites translate failures into short human sentences.
function App() {
  const [tab, setTab] = useState<Tab>('sources')
  const [activeSession, setActiveSession] = useState<SessionInfo | null>(null)

  const openTab = (t: Tab) => {
    setActiveSession(null) // switching tabs always leaves the live view
    setTab(t)
  }

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
          </nav>
          <div className="ls-topbar__spacer" />
          <ThemeToggle />
        </header>

        {activeSession ? (
          <LiveView session={activeSession} onBack={() => setActiveSession(null)} />
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
