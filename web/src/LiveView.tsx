import { useEffect, useRef, useState } from 'react'
import {
  addGalleryImage,
  getRewindBoxes,
  getRewindRange,
  getSession,
  rewindImageURL,
  startSessionRecognition,
  stopSessionRecognition,
  type RewindBox,
  type SessionInfo,
} from './api'
import { VideoStream, type NormalizedRect } from './VideoStream'
import { BrowserCamera } from './BrowserCamera'
import { Button } from './components/Button'
import { StatusBadge } from './components/StatusBadge'
import { colorForID } from './components/VideoOverlayBox'
import type { OverlayBox } from './components/VideoOverlayBox'
import { useVideoStream } from './useVideoStream'
import { useToast } from './toast/ToastProvider'
import { cropFrame } from './cropFrame'
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

  // Sélection runtime + labellisation (docs/gui/mockups/ screen 1d,
  // TODO.md § H1) — added 2026-08-13. selection is either an existing
  // box the user clicked (rect + a suggested label from the detection
  // itself) or a hand-drawn region (rect only, empty label) — see
  // VideoStream's onBoxClick/onRegionSelect. imgRef gives cropFrame a
  // live DOM handle on the currently-displayed frame, no extra fetch.
  const imgRef = useRef<HTMLImageElement | null>(null)
  const [selection, setSelection] = useState<{ rect: NormalizedRect; label: string } | null>(null)
  const [labelSaving, setLabelSaving] = useState(false)

  // Pause/reprise + retour en arrière (docs/gui/spec.md § 1.5bis,
  // TODO.md § H1) — added 2026-08-13. Pausing never stops the real
  // flow/detection running server-side (explicit product requirement),
  // it only stops *this component* from displaying newly arrived WS
  // frames, and offers stepping back into what the backend's
  // RingBufferOutput already buffered instead.
  const [paused, setPaused] = useState(false)
  const [rewindRangeMs, setRewindRangeMs] = useState(0)
  const [rewindOffsetMs, setRewindOffsetMs] = useState(0)
  const [rewindBoxes, setRewindBoxes] = useState<RewindBox[]>([])

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

  const handlePause = async () => {
    try {
      const { rangeMs } = await getRewindRange(session.id)
      setRewindRangeMs(rangeMs)
      setRewindOffsetMs(0)
      setPaused(true)
    } catch {
      pushError('Impossible de figer le flux — rien de mis en mémoire pour cette source.')
    }
  }

  const handleResumeLive = () => {
    setPaused(false)
    setRewindOffsetMs(0)
    setRewindBoxes([])
  }

  // Refetch the buffered boxes for the current offset whenever it (or
  // pause state) changes — the <img> src itself (rewindImageURL) doesn't
  // need a matching effect, its URL already encodes the offset and the
  // browser refetches on src change.
  useEffect(() => {
    if (!paused) return
    getRewindBoxes(session.id, rewindOffsetMs)
      .then((r) => setRewindBoxes(r.boxes))
      .catch(() => {
        /* a transient miss (e.g. offset just past the buffered range) — keep the last boxes shown */
      })
  }, [paused, rewindOffsetMs, session.id])

  const handleBoxClick = (box: OverlayBox) => {
    setSelection({ rect: { x1: box.x1, y1: box.y1, x2: box.x2, y2: box.y2 }, label: box.label })
  }

  const handleRegionSelect = (rect: NormalizedRect) => {
    setSelection({ rect, label: '' })
  }

  const handleConfirmSelection = async () => {
    if (!selection || !imgRef.current) return
    const name = selection.label.trim()
    if (!name) return
    setLabelSaving(true)
    try {
      const blob = await cropFrame(imgRef.current, selection.rect)
      await addGalleryImage(name, blob)
      setSelection(null)
    } catch {
      pushError("Impossible d'ajouter ce Terme à la Bibliothèque.")
    } finally {
      setLabelSaving(false)
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
        {paused ? (
          <Button variant="primary" onClick={handleResumeLive}>
            ▶ Retour au direct
          </Button>
        ) : (
          <Button variant="secondary" onClick={handlePause}>
            ⏸ Pause
          </Button>
        )}
        <div className="ls-topbar__spacer" />
        <StatusBadge status={hasError ? 'error' : connected ? 'connected' : 'disconnected'} />
      </div>

      {paused && (
        <div className="ls-rewind-bar">
          <span className="ls-rewind-bar__label">
            −{(rewindOffsetMs / 1000).toFixed(1)}s
          </span>
          <input
            className="ls-rewind-bar__slider"
            type="range"
            min={0}
            max={Math.max(rewindRangeMs, 1)}
            step={100}
            value={rewindOffsetMs}
            onChange={(e) => setRewindOffsetMs(Number(e.target.value))}
          />
          <span className="ls-rewind-bar__label">
            {(rewindRangeMs / 1000).toFixed(1)}s disponibles
          </span>
        </div>
      )}

      <main className="ls-main">
        <div className="ls-video-wrap">
          <VideoStream
            frameURL={paused ? rewindImageURL(session.id, rewindOffsetMs) : frameURL}
            boxes={paused ? rewindBoxes : boxes}
            connected={connected}
            imgRef={imgRef}
            onBoxClick={handleBoxClick}
            onRegionSelect={handleRegionSelect}
          />

          {selection && (
            <div className="ls-label-form">
              <span className="ls-label-form__title">Ajouter à la Bibliothèque</span>
              <input
                className="ls-input"
                autoFocus
                type="text"
                value={selection.label}
                onChange={(e) => setSelection({ ...selection, label: e.target.value })}
                onKeyDown={(e) => e.key === 'Enter' && handleConfirmSelection()}
                placeholder="Nom du Terme…"
              />
              <div className="ls-label-form__actions">
                <Button variant="neutral" onClick={() => setSelection(null)} disabled={labelSaving}>
                  Annuler
                </Button>
                <Button
                  variant="primary"
                  onClick={handleConfirmSelection}
                  disabled={labelSaving || !selection.label.trim()}
                >
                  Ajouter
                </Button>
              </div>
            </div>
          )}
        </div>

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
