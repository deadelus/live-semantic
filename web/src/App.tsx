import { useState } from 'react'
import { SourcesList } from './SourcesList'
import { LiveView } from './LiveView'
import { ThemeToggle } from './components/ThemeToggle'
import { ToastProvider } from './toast/ToastProvider'
import type { SessionInfo } from './api'
import './App.css'

// Top-level view router — sources list (home, docs/gui/mockups/ screen
// 1b) → click a row → live view (1c/1f), back button returns. No
// react-router: two screens don't justify the dependency yet, revisit if
// a third one (standalone gallery, historique, TODO.md § H2) needs
// deep-linking. Session lifecycle owned entirely by SourcesList
// (create/list/remove) — App.tsx only tracks which one is currently
// open, LiveView never creates or deletes a session itself.
//
// ToastProvider wraps everything once here (2026-08-13) so both screens
// can raise a clean error toast without knowing about each other —
// deliberately *not* the backend's raw error text (explicitly not
// wanted): call sites in SourcesList/LiveView translate failures into
// short human sentences.
function App() {
  const [activeSession, setActiveSession] = useState<SessionInfo | null>(null)

  return (
    <ToastProvider>
      <div className="ls-app">
        <header className="ls-topbar">
          <div className="ls-topbar__brand">
            <span className="ls-topbar__dot" />
            LiveSemantic
          </div>
          <div className="ls-topbar__spacer" />
          <ThemeToggle />
        </header>

        {activeSession ? (
          <LiveView session={activeSession} onBack={() => setActiveSession(null)} />
        ) : (
          <SourcesList onOpen={setActiveSession} />
        )}
      </div>
    </ToastProvider>
  )
}

export default App
