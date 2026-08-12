import { useEffect, useRef, useState } from 'react'
import { getStatus, startRecognition, stopRecognition, type Source } from './api'
import { VideoStream } from './VideoStream'
import { BrowserCamera } from './BrowserCamera'
import { Button } from './components/Button'
import { StatusBadge } from './components/StatusBadge'
import { ThemeToggle } from './components/ThemeToggle'
import { colorForID } from './components/VideoOverlayBox'
import { useVideoStream } from './useVideoStream'
import './App.css'

// Live view screen (TODO.md § H2), restyled 2026-08-12 against
// docs/gui/mockups/ screens 1c/1f — design tokens/components (theme.css,
// components/) rather than the earlier minimal CSS. Deliberately still
// not a pixel-for-pixel copy: the mockup's transport bar (pause/rewind)
// and per-term threshold slider+sparkline need backend features not
// built yet (ring buffer, TODO.md § C "V2"; per-term threshold,
// docs/adr/clip-backend.md § 23's own open question) — building their
// UI now would be non-functional set dressing, not a real feature
// (TODO.md § H's own stated risk: "travail jetable"). What's here is
// real: filter control, source toggle, live video with real overlay
// boxes (§ 32), a genuine detected-objects list, and read-only backend
// config already hardcoded server-side (§ 3.4 of docs/gui/spec.md).
function App() {
  const [filter, setFilter] = useState('person')
  const [source, setSource] = useState<Source>('local')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)

  const browserCamera = useRef<BrowserCamera | null>(null)
  const { frameURL, boxes, connected } = useVideoStream()

  useEffect(() => {
    const poll = () => {
      getStatus()
        .then((s) => {
          setRunning(s.running)
          if (s.error) {
            setError(s.error)
          }
        })
        .catch(() => {
          /* transient network hiccup — next poll will retry */
        })
    }
    poll()
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
  }, [])

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
    <div className="ls-app">
      <header className="ls-topbar">
        <div className="ls-topbar__brand">
          <span className="ls-topbar__dot" />
          LiveSemantic
        </div>
        <div className="ls-topbar__spacer" />
        {/* Reflects the /ws transport itself (frame stream alive or not)
            — a separate concept from `running` (a recognition session
            active), already communicated by the Démarrer/Arrêter button
            below. Conflating the two would misrepresent a real state:
            the WS upgrade succeeds and stays open with no session
            running too (no frames/boxes arrive until one starts, but
            the transport itself is still "connected"). */}
        <StatusBadge status={connected ? 'connected' : 'disconnected'} />
        <ThemeToggle />
      </header>

      <main className="ls-main">
        <VideoStream frameURL={frameURL} boxes={boxes} connected={connected} />

        <aside className="ls-panel">
          <section className="ls-panel__section">
            <h2>Filtre</h2>
            <input
              className="ls-input"
              type="text"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder='ex: person*1, person%+%backpack'
              disabled={running}
            />
            <select
              className="ls-input"
              value={source}
              onChange={(e) => setSource(e.target.value as Source)}
              disabled={running}
            >
              <option value="local">Caméra du serveur</option>
              <option value="browser">Caméra du navigateur</option>
            </select>
            {running ? (
              <Button variant="neutral" onClick={handleStop} disabled={busy}>
                Arrêter
              </Button>
            ) : (
              <Button variant="primary" onClick={handleStart} disabled={busy}>
                Démarrer
              </Button>
            )}
            {error && <p className="ls-error">{error}</p>}
          </section>

          <section className="ls-panel__section">
            <h2>Objets détectés — {boxes.length}</h2>
            {boxes.length === 0 ? (
              <p className="ls-muted">Aucun objet suivi actuellement.</p>
            ) : (
              <ul className="ls-detections">
                {boxes.map((b) => (
                  <li key={b.trackId}>
                    <span className="ls-detections__dot" style={{ background: colorForID(b.id) }} />
                    <span className="ls-detections__label">{b.label}</span>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="ls-panel__section ls-panel__section--advanced">
            <button className="ls-panel__toggle" onClick={() => setAdvancedOpen((v) => !v)}>
              <span>Avancés</span>
              <span>{advancedOpen ? '▾' : '▸'}</span>
            </button>
            {advancedOpen && (
              <dl className="ls-advanced">
                <dt>Algorithme de tracking</dt>
                <dd>KCF</dd>
                <dt>IoU d'association</dt>
                <dd>0.3</dd>
                <dt>Frames avant perte</dt>
                <dd>2</dd>
                <dt>Matériel d'inférence</dt>
                <dd>CPU</dd>
                <dt>Variante de modèle</dt>
                <dd>YOLO11s</dd>
              </dl>
            )}
          </section>
        </aside>
      </main>
    </div>
  )
}

export default App
