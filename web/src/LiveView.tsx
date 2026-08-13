import { useEffect, useRef, useState } from 'react'
import { getSession, startSessionRecognition, stopSessionRecognition, type SessionInfo } from './api'
import { VideoStream } from './VideoStream'
import { BrowserCamera } from './BrowserCamera'
import { Button } from './components/Button'
import { StatusBadge } from './components/StatusBadge'
import { colorForID } from './components/VideoOverlayBox'
import { useVideoStream } from './useVideoStream'
import { useToast } from './toast/ToastProvider'
import './App.css'

interface LiveViewProps {
  session: SessionInfo
  onBack: () => void
}

// Onglet Vue live (docs/gui/mockups/ screens 1c/1f) for one already-
// created session — "pas un écran indépendant" (docs/gui/spec.md § 3.2),
// opened by clicking a row in SourcesList. Extracted 2026-08-13 from the
// single-screen App.tsx now that session creation lives in SourcesList —
// this component only ever operates on a session that already exists,
// it never creates or deletes one. Source kind (local/browser) is fixed
// at session creation (session.Source has no "change kind" endpoint), so
// unlike the pre-multi-flux version there's no source <select> here
// anymore — switching source means going back and opening/adding a
// different session.
export function LiveView({ session, onBack }: LiveViewProps) {
  const [filter, setFilter] = useState(session.filter ?? 'person')
  const [running, setRunning] = useState(session.running)
  const [hasError, setHasError] = useState(Boolean(session.error))
  const [busy, setBusy] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const { pushError } = useToast()
  // Avoids re-toasting the same backend error on every 2s poll — only a
  // genuinely new failure (message changed) raises a new toast.
  const lastToastedError = useRef<string | null>(session.error ?? null)

  const browserCamera = useRef<BrowserCamera | null>(null)
  const { frameURL, boxes, connected } = useVideoStream(session.id)

  useEffect(() => {
    const poll = () => {
      getSession(session.id)
        .then((s) => {
          setRunning(s.running)
          setHasError(Boolean(s.error))
          if (s.error && lastToastedError.current !== s.error) {
            lastToastedError.current = s.error
            pushError('Cette session a rencontré une erreur.')
          }
        })
        .catch(() => {
          /* transient network hiccup — next poll will retry */
        })
    }
    poll()
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id])

  // Known limitation, not silent: a "browser" session's frame capture
  // only runs while this screen is mounted — navigating back to the
  // sources list stops pushing frames even though the backend session
  // keeps existing (running flips to whatever the last reanchor left it
  // at, no new frames arrive until this screen is reopened). Solving
  // that for real needs the source to keep capturing off-screen, out of
  // scope for this slice.
  useEffect(() => {
    return () => {
      browserCamera.current?.stop()
      browserCamera.current = null
    }
  }, [])

  useEffect(() => {
    if (!running) {
      browserCamera.current?.stop()
      browserCamera.current = null
    }
  }, [running])

  const handleStart = async () => {
    setBusy(true)
    try {
      if (session.source.kind === 'browser' && !browserCamera.current) {
        const cam = new BrowserCamera()
        await cam.start(`/ws/sessions/${session.id}/ingest`)
        browserCamera.current = cam
      }
      await startSessionRecognition(session.id, filter)
      setRunning(true)
    } catch {
      browserCamera.current?.stop()
      browserCamera.current = null
      pushError('Impossible de démarrer la reconnaissance.')
    } finally {
      setBusy(false)
    }
  }

  const handleStop = async () => {
    setBusy(true)
    try {
      await stopSessionRecognition(session.id)
      setRunning(false)
    } catch {
      pushError("Impossible d'arrêter la reconnaissance.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="ls-subbar">
        <button className="ls-back" onClick={onBack}>
          ← Sources
        </button>
        <span className="ls-subbar__label">
          {session.source.kind === 'local' ? `Caméra ${session.source.device ?? 0}` : 'Caméra du navigateur'} ·{' '}
          {session.id}
        </span>
        <div className="ls-topbar__spacer" />
        <StatusBadge status={hasError ? 'error' : connected ? 'connected' : 'disconnected'} />
      </div>

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
            {running ? (
              <Button variant="neutral" onClick={handleStop} disabled={busy}>
                Arrêter
              </Button>
            ) : (
              <Button variant="primary" onClick={handleStart} disabled={busy}>
                Démarrer
              </Button>
            )}
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
    </>
  )
}
