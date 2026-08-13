import { useRef, useState, type RefObject } from 'react'
import type { OverlayBox } from './components/VideoOverlayBox'
import { VideoOverlayBox } from './components/VideoOverlayBox'
import './VideoStream.css'

export interface NormalizedRect {
  x1: number
  y1: number
  x2: number
  y2: number
}

// Presentational only — deliberately doesn't call useVideoStream() itself
// (App.tsx does, once, and passes the result down) so only one /ws
// connection ever opens regardless of how many places want the frame/
// boxes.
//
// onBoxClick / onRegionSelect back the "sélection runtime + labellisation"
// interaction (docs/gui/mockups/ screen 1d, TODO.md § H1) — added
// 2026-08-13. imgRef is exposed so a caller (LiveView) can crop the
// *currently displayed* frame straight out of this <img> via canvas once
// a selection is confirmed, without a second network round-trip to
// re-fetch the frame.
interface VideoStreamProps {
  frameURL: string | null
  boxes: OverlayBox[]
  connected: boolean
  imgRef?: RefObject<HTMLImageElement | null>
  onBoxClick?: (box: OverlayBox) => void
  onRegionSelect?: (rect: NormalizedRect) => void
}

// minDragFraction filters out an accidental single-pixel "drag" (really
// just a click on empty video, not a hand-drawn box) — screen 1d only
// draws a box "si rien n'est détecté dessus", a deliberate drag, not a
// stray mousedown/mouseup.
const minDragFraction = 0.02

export function VideoStream({
  frameURL,
  boxes,
  connected,
  imgRef,
  onBoxClick,
  onRegionSelect,
}: VideoStreamProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
  const [dragCurrent, setDragCurrent] = useState<{ x: number; y: number } | null>(null)

  const pointerToNormalized = (e: { clientX: number; clientY: number }) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect || rect.width === 0 || rect.height === 0) return null
    return {
      x: Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width)),
      y: Math.min(1, Math.max(0, (e.clientY - rect.top) / rect.height)),
    }
  }

  const handleMouseDown = (e: React.MouseEvent) => {
    if (!onRegionSelect) return
    const p = pointerToNormalized(e)
    if (!p) return
    setDragStart(p)
    setDragCurrent(p)
  }

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!dragStart) return
    const p = pointerToNormalized(e)
    if (p) setDragCurrent(p)
  }

  const handleMouseUp = () => {
    if (!dragStart || !dragCurrent || !onRegionSelect) {
      setDragStart(null)
      setDragCurrent(null)
      return
    }
    const rect: NormalizedRect = {
      x1: Math.min(dragStart.x, dragCurrent.x),
      y1: Math.min(dragStart.y, dragCurrent.y),
      x2: Math.max(dragStart.x, dragCurrent.x),
      y2: Math.max(dragStart.y, dragCurrent.y),
    }
    setDragStart(null)
    setDragCurrent(null)
    if (rect.x2 - rect.x1 >= minDragFraction && rect.y2 - rect.y1 >= minDragFraction) {
      onRegionSelect(rect)
    }
  }

  const dragRect =
    dragStart && dragCurrent
      ? {
          x1: Math.min(dragStart.x, dragCurrent.x),
          y1: Math.min(dragStart.y, dragCurrent.y),
          x2: Math.max(dragStart.x, dragCurrent.x),
          y2: Math.max(dragStart.y, dragCurrent.y),
        }
      : null

  return (
    <div
      className="ls-video"
      ref={containerRef}
      onMouseDown={handleMouseDown}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      onMouseLeave={handleMouseUp}
    >
      {frameURL ? (
        <>
          <img ref={imgRef} src={frameURL} alt="Live recognition feed" draggable={false} />
          {boxes.map((box) => (
            <VideoOverlayBox
              key={box.trackId}
              box={box}
              onClick={onBoxClick ? () => onBoxClick(box) : undefined}
            />
          ))}
          {dragRect && (
            <div
              className="ls-video__draw-rect"
              style={{
                left: `${dragRect.x1 * 100}%`,
                top: `${dragRect.y1 * 100}%`,
                width: `${(dragRect.x2 - dragRect.x1) * 100}%`,
                height: `${(dragRect.y2 - dragRect.y1) * 100}%`,
              }}
            />
          )}
        </>
      ) : (
        <div className="ls-video__placeholder">
          {connected ? 'Connecté — en attente de frames…' : 'Connexion au flux…'}
        </div>
      )}
    </div>
  )
}
