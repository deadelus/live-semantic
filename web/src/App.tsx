import { useEffect, useState } from 'react'
import { getStatus, startRecognition, stopRecognition } from './api'
import { VideoStream } from './VideoStream'
import './App.css'

// First real GUI screen (TODO.md § H2, vertical slice decided 2026-08-12):
// single source (the backend's own local camera, already wired), a filter
// text field, start/stop, live video. Deliberately not the full mockup
// (no sources list/mosaic, no reference gallery, no multi-flux) — those
// depend on H1 work not done yet (Multi-flux, WebRTC, galerie de
// références). This slice validates the transport (REST + WS) against a
// real UI before building the rest on top of it.
function App() {
  const [filter, setFilter] = useState('person')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // Polls status rather than trusting only the optimistic state set after
  // start/stop: a session can end on its own (e.g. an invalid filter
  // caught server-side after the 202 Accepted response, TODO.md § A's
  // "l'API REST ne remonte pas au client un filtre invalide" — status
  // polling is the only way this UI finds out, until that's fixed).
  useEffect(() => {
    const poll = () => {
      getStatus()
        .then((s) => setRunning(s.running))
        .catch(() => {
          /* transient network hiccup — next poll will retry */
        })
    }
    poll()
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
  }, [])

  const handleStart = async () => {
    setBusy(true)
    setError(null)
    try {
      await startRecognition(filter)
      setRunning(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const handleStop = async () => {
    setBusy(true)
    setError(null)
    try {
      await stopRecognition()
      setRunning(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="app">
      <header className="app-header">
        <h1>LiveSemantic</h1>
        <span className={`status-badge ${running ? 'status-running' : 'status-stopped'}`}>
          {running ? 'En cours' : 'Arrêté'}
        </span>
      </header>

      <main className="app-main">
        <VideoStream />

        <div className="controls">
          <input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder='ex: person*1, person%+%backpack'
            disabled={running}
          />
          {running ? (
            <button onClick={handleStop} disabled={busy}>
              Arrêter
            </button>
          ) : (
            <button onClick={handleStart} disabled={busy}>
              Démarrer
            </button>
          )}
        </div>

        {error && <p className="error">{error}</p>}
      </main>
    </div>
  )
}

export default App
