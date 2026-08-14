import { useEffect, useRef, useState } from 'react'
import {
  addGalleryImage,
  getRewindBoxes,
  getRewindRange,
  getSession,
  listGallery,
  rewindImageURL,
  startSessionRecognition,
  stopSessionRecognition,
  type GalleryEntry,
  type RewindBox,
  type SessionInfo,
} from './api'
import { VideoStream, type NormalizedRect } from './VideoStream'
import { BrowserCamera } from './BrowserCamera'
import { WebRTCCamera } from './WebRTCCamera'
import { Button } from './components/Button'
import { colorForID } from './components/VideoOverlayBox'
import type { OverlayBox } from './components/VideoOverlayBox'
import { useVideoStream } from './useVideoStream'
import { useToast } from './toast/ToastProvider'
import { cropFrame } from './cropFrame'
import './App.css'

interface LiveViewProps {
  session: SessionInfo
  // Reports connection state up to App.tsx, which renders it on the open
  // session's tab (docs/gui/mockups/ screen 1c — the tab itself carries
  // the live-status dot, there's no separate status row in this
  // component anymore, 2026-08-14 fidelity pass).
  onStatusChange: (status: { connected: boolean; hasError: boolean }) => void
}

// rewindStepMs backs the −10s/+10s transport buttons (docs/gui/mockups/
// screen 1c) — a fixed step, matching the mockup's own labels exactly.
const rewindStepMs = 10000

// Onglet Vue live (docs/gui/mockups/ screens 1c/1f) for one already-
// created session — "pas un écran indépendant" (docs/gui/spec.md § 3.2),
// opened by clicking a row in SourcesList. This component only ever
// operates on a session that already exists, it never creates or deletes
// one — closing the tab (App.tsx) doesn't remove the backend session,
// same as going back to Sources always did.
export function LiveView({ session, onStatusChange }: LiveViewProps) {
  const [filter, setFilter] = useState(session.filter ?? 'person')
  const [running, setRunning] = useState(session.running)
  const [hasError, setHasError] = useState(Boolean(session.error))
  const [busy, setBusy] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const { pushError } = useToast()
  // Avoids re-toasting the same backend error on every 2s poll — only a
  // genuinely new failure (message changed) raises a new toast.
  const lastToastedError = useRef<string | null>(session.error ?? null)

  // browserCamera (JPEG-over-WS) and webrtcCamera (real WebRTC) are
  // mutually exclusive — session.source.kind fixes which one a given
  // session actually uses (decided at createSession time, no "change
  // kind" endpoint), each ref only ever gets populated for its own kind.
  const browserCamera = useRef<BrowserCamera | null>(null)
  const webrtcCamera = useRef<WebRTCCamera | null>(null)
  const { frameURL, boxes, connected } = useVideoStream(session.id)

  useEffect(() => {
    onStatusChange({ connected, hasError })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connected, hasError])

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

  // Galerie de références, inline in the right panel (docs/gui/mockups/
  // screen 1c) — a quick-glance view while watching a stream, distinct
  // from the full-CRUD Bibliothèque tab (Library.tsx). Polled on the
  // same 2s cadence as session state, not per-frame.
  const [galleryEntries, setGalleryEntries] = useState<GalleryEntry[]>([])

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
      listGallery()
        .then((r) => setGalleryEntries(r.entries))
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
  const stopClientCapture = () => {
    browserCamera.current?.stop()
    browserCamera.current = null
    webrtcCamera.current?.stop()
    webrtcCamera.current = null
  }

  useEffect(() => stopClientCapture, [])

  useEffect(() => {
    if (!running) stopClientCapture()
  }, [running])

  const handleStart = async () => {
    setBusy(true)
    try {
      if (session.source.kind === 'browser' && !browserCamera.current) {
        const cam = new BrowserCamera()
        await cam.start(`/ws/sessions/${session.id}/ingest`)
        browserCamera.current = cam
      } else if (session.source.kind === 'webrtc' && !webrtcCamera.current) {
        const cam = new WebRTCCamera()
        await cam.start(session.id)
        webrtcCamera.current = cam
      }
      await startSessionRecognition(session.id, filter)
      setRunning(true)
    } catch {
      stopClientCapture()
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

  // handleStep backs both −10s/+10s buttons (docs/gui/mockups/ screen
  // 1c) — stepping back from the live edge implicitly pauses first (same
  // buffered range fetch as the Pause button), stepping past offset 0
  // resumes live instead of going "into the future".
  const handleStep = async (deltaMs: number) => {
    if (!paused) {
      if (deltaMs >= 0) return // already live, +10s is a no-op
      await handlePause()
      return
    }
    const next = rewindOffsetMs - deltaMs
    if (next <= 0) {
      handleResumeLive()
      return
    }
    setRewindOffsetMs(Math.min(next, rewindRangeMs))
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

  // filterTerms is a read-only chip view of the raw filter string — see
  // .ls-filter-chips's own CSS comment for why this doesn't parse into
  // interactive per-term controls (threshold/toggle) yet.
  const filterTerms = filter
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)

  const timelinePct = rewindRangeMs > 0 ? Math.max(0, Math.min(100, (rewindOffsetMs / rewindRangeMs) * 100)) : 0

  return (
    <main className="ls-main">
      <div className="ls-video-column">
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

        {/* Transport (docs/gui/mockups/ screen 1c) — timeline scrubber
            reflects the rewind buffer's occupied range while paused (0%
            while live, matching the mockup's own "direct" edge state). */}
        <div className="ls-transport">
          <div className="ls-transport__timeline">
            <div className="ls-transport__timeline-fill" style={{ width: `${100 - timelinePct}%` }} />
            <div className="ls-transport__timeline-marker" style={{ left: `${100 - timelinePct}%` }} />
          </div>
          <div className="ls-transport__timeline-labels">
            <span>−{Math.round(rewindRangeMs / 1000)} s</span>
            <span>{paused ? `−${(rewindOffsetMs / 1000).toFixed(1)} s` : 'direct'}</span>
          </div>
          <div className="ls-transport__controls">
            <Button variant="secondary" onClick={() => handleStep(-rewindStepMs)}>
              −10 s
            </Button>
            {paused ? (
              <Button variant="primary" onClick={handleResumeLive}>
                ▶ Lecture
              </Button>
            ) : (
              <Button variant="primary" onClick={handlePause}>
                ⏸ Pause
              </Button>
            )}
            <Button variant="secondary" onClick={() => handleStep(rewindStepMs)} disabled={!paused}>
              +10 s
            </Button>
            {!paused && (
              <span className="ls-transport__live-pill">
                <span className="ls-transport__live-dot" />
                EN DIRECT
              </span>
            )}
            <div className="ls-topbar__spacer" />
            <span className="ls-transport__buffer">
              Tampon de retour :
              <span className="ls-transport__buffer-value">
                {paused ? `${(rewindRangeMs / 1000).toFixed(0)} s dispo.` : '30 s'}
              </span>
            </span>
          </div>
        </div>
      </div>

      <aside className="ls-panel">
        <section className="ls-panel__section">
          <h2>Filtres sémantiques</h2>
          {filterTerms.length > 0 && (
            <div className="ls-filter-chips">
              {filterTerms.map((t) => (
                <span key={t} className="ls-filter-chip">
                  <span className="ls-filter-chip__dot" style={{ background: colorForID(t) }} />
                  {t}
                </span>
              ))}
            </div>
          )}
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
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
            <h2>Galerie de références</h2>
            <span className="ls-panel-gallery__meta">{galleryEntries.length}</span>
          </div>
          {galleryEntries.length === 0 ? (
            <p className="ls-muted">Aucune référence pour l'instant.</p>
          ) : (
            <ul className="ls-panel-gallery">
              {galleryEntries.map((g) => (
                <li key={g.name} className="ls-panel-gallery__row">
                  {g.images[0] ? (
                    <img
                      className="ls-panel-gallery__thumb"
                      src={`/api/v1/gallery/${encodeURIComponent(g.name)}/images/${encodeURIComponent(g.images[0].id)}`}
                      alt=""
                    />
                  ) : (
                    <span className="ls-panel-gallery__thumb" />
                  )}
                  <span className="ls-panel-gallery__label">
                    <span className="ls-panel-gallery__name">{g.name}</span>
                    <span className="ls-panel-gallery__meta">
                      {g.images.length} photo{g.images.length > 1 ? 's' : ''}
                      {g.cocoClass ? ` · COCO ${g.cocoClass}` : ''}
                    </span>
                  </span>
                </li>
              ))}
            </ul>
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
  )
}
