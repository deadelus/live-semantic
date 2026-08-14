import { useState } from 'react'
import type { SessionInfo } from './api'
import { sourceLabel } from './api'
import { StatusBadge } from './components/StatusBadge'
import { VideoOverlayBox } from './components/VideoOverlayBox'
import { useVideoStream } from './useVideoStream'
import './Mosaic.css'

interface MosaicViewProps {
  sessions: SessionInfo[]
  onOpen: (session: SessionInfo) => void
}

// Vue mosaïque (docs/gui/mockups/ screens 1a/1e), added 2026-08-14 —
// each tile is its own WebSocket subscription with server-side-throttled
// FPS and boxes on/off (useVideoStream's own opts, backed by
// implementation/streamer/output.WebSocketOutput's per-client
// streamer.ClientOptions) — not a client-side "drop most frames" hack,
// the backend genuinely sends less over the wire per tile.
//
// Réglages panel (docs/gui/spec.md § 3.1: "la qualité de la mosaïque
// doit être réglable... pas juste un mode preview fixe à 1 fps") applies
// the same fps/boxes settings to every tile at once — no per-tile
// override, matching the mockup's own single settings panel.
export function MosaicView({ sessions, onOpen }: MosaicViewProps) {
  const [fps, setFps] = useState(1)
  const [showBoxes, setShowBoxes] = useState(false)

  return (
    <div className="ls-mosaic">
      <div className="ls-mosaic__settings">
        <label className="ls-mosaic__setting">
          <span>FPS des tuiles</span>
          <input
            type="range"
            min={1}
            max={10}
            step={1}
            value={fps}
            onChange={(e) => setFps(Number(e.target.value))}
          />
          <span className="ls-mosaic__setting-value">{fps} fps</span>
        </label>
        <label className="ls-mosaic__setting ls-mosaic__setting--checkbox">
          <input type="checkbox" checked={showBoxes} onChange={(e) => setShowBoxes(e.target.checked)} />
          <span>Afficher les boxes sur les tuiles</span>
        </label>
        {(fps > 3 || showBoxes) && (
          <p className="ls-mosaic__warning">
            ⚠ FPS élevé et/ou boxes activées sur plusieurs tuiles peut introduire de la latence
            selon le matériel côté serveur — aucun Execution Provider GPU câblé aujourd'hui
            (TODO.md § 1.6).
          </p>
        )}
      </div>

      {sessions.length === 0 ? (
        <p className="ls-muted">Aucune source ajoutée.</p>
      ) : (
        <div className="ls-mosaic__grid">
          {sessions.map((s) => (
            <MosaicTile key={s.id} session={s} fps={fps} showBoxes={showBoxes} onClick={() => onOpen(s)} />
          ))}
        </div>
      )}
    </div>
  )
}

function MosaicTile({
  session,
  fps,
  showBoxes,
  onClick,
}: {
  session: SessionInfo
  fps: number
  showBoxes: boolean
  onClick: () => void
}) {
  const { frameURL, boxes, connected } = useVideoStream(session.id, { fps, boxes: showBoxes })

  return (
    <button className="ls-mosaic__tile" onClick={onClick}>
      <div className="ls-mosaic__tile-video">
        {frameURL ? (
          <>
            <img src={frameURL} alt="" />
            {showBoxes && boxes.map((b) => <VideoOverlayBox key={b.trackId} box={b} />)}
          </>
        ) : (
          <span className="ls-mosaic__tile-placeholder">
            {connected ? 'En attente…' : 'Connexion…'}
          </span>
        )}
      </div>
      <div className="ls-mosaic__tile-footer">
        <StatusBadge status={session.error ? 'error' : connected ? 'connected' : 'disconnected'} />
        <span className="ls-mosaic__tile-label">{sourceLabel(session)}</span>
      </div>
    </button>
  )
}
