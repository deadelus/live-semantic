import { useEffect, useRef, useState } from 'react'
import { getStatus, startRecognition, stopRecognition, type Source } from './api'
import { VideoStream } from './VideoStream'
import { BrowserCamera } from './BrowserCamera'
import './App.css'

// First real GUI screen (TODO.md § H2, vertical slice decided 2026-08-12):
// single source, a filter text field, start/stop, live video — plus,
// since 2026-08-12, a source toggle (local backend camera vs. this
// device's own browser camera, TODO.md § H2 "capture caméra navigateur").
// Deliberately not the full mockup (no sources list/mosaic, no reference
// gallery, no multi-flux) — those depend on H1 work not done yet.
function App() {
  const [filter, setFilter] = useState('person')
  const [source, setSource] = useState<Source>('local')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const browserCamera = useRef<BrowserCamera | null>(null)

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

  // If the recognition session ends on its own (server-side error, or the
  // user clicking "stop") while a browser camera capture is active, stop
  // it too rather than leaving getUserMedia running invisibly.
  useEffect(() => {
    if (!running) {
      browserCamera.current?.stop()
      browserCamera.current = null
    }
  }, [running])

  const handleStart = async () => {
    setBusy(true)
    setError(null)
    try {
      if (source === 'browser') {
        const cam = new BrowserCamera()
        await cam.start()
        browserCamera.current = cam
      }
      await startRecognition(filter, source)
      setRunning(true)
    } catch (e) {
      browserCamera.current?.stop()
      browserCamera.current = null
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
          <select
            value={source}
            onChange={(e) => setSource(e.target.value as Source)}
            disabled={running}
          >
            <option value="local">Caméra du serveur</option>
            <option value="browser">Caméra du navigateur</option>
          </select>
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
