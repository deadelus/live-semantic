import type { OverlayBox } from './components/VideoOverlayBox'
import { VideoOverlayBox } from './components/VideoOverlayBox'
import './VideoStream.css'

// Presentational only — deliberately doesn't call useVideoStream() itself
// (App.tsx does, once, and passes the result down) so only one /ws
// connection ever opens regardless of how many places want the frame/
// boxes. onBoxClick is a hook for a future "click a box to add it to the
// reference gallery" flow (docs/gui/mockups/ screen 1d) — not built yet,
// POST /api/v1/gallery already exists (TODO.md § A) but the
// crop-from-click UI doesn't.
interface VideoStreamProps {
  frameURL: string | null
  boxes: OverlayBox[]
  connected: boolean
  onBoxClick?: (boxId: string) => void
}

export function VideoStream({ frameURL, boxes, connected, onBoxClick }: VideoStreamProps) {
  return (
    <div className="ls-video">
      {frameURL ? (
        <>
          <img src={frameURL} alt="Live recognition feed" />
          {boxes.map((box) => (
            <VideoOverlayBox
              key={box.trackId}
              box={box}
              onClick={onBoxClick ? () => onBoxClick(box.id) : undefined}
            />
          ))}
        </>
      ) : (
        <div className="ls-video__placeholder">
          {connected ? 'Connecté — en attente de frames…' : 'Connexion au flux…'}
        </div>
      )}
    </div>
  )
}
