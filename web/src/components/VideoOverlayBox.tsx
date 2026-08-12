import './VideoOverlayBox.css'

// Matches docs/gui/mockups/ screen 1h's "Box vidéo overlay — couleur
// portée par le filtre" — color comes from the filter/track identity
// (id), never the object category (that rule is stated explicitly in the
// mockup's own "Hypothèses & parti pris" note, § 1a). Positioned as a
// percentage of its (relatively-positioned) video container — coordinates
// arrive normalized to [0,1] from the backend (streamer.BoxData,
// docs/adr/clip-backend.md § 32), which is exactly what a CSS percentage
// needs, no pixel math required client-side.
export interface OverlayBox {
  id: string
  label: string
  trackId: string
  x1: number
  y1: number
  x2: number
  y2: number
}

// Deterministic id -> color, same rationale as the backend's own
// entities.BoxColor(id) (internal/domain/entities/class_color.go) —
// doesn't need to match it bit-for-bit (this is a purely client-side
// rendering concern), just needs to be *stable* for a given id across
// renders so a box doesn't visibly change color frame to frame.
const PALETTE = ['#4d9ef8', '#2ec27e', '#e8a13c', '#c77dff', '#e4574f']

export function colorForID(id: string): string {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0
  }
  return PALETTE[hash % PALETTE.length]
}

export function VideoOverlayBox({ box, onClick }: { box: OverlayBox; onClick?: () => void }) {
  const color = colorForID(box.id)
  const style = {
    left: `${box.x1 * 100}%`,
    top: `${box.y1 * 100}%`,
    width: `${(box.x2 - box.x1) * 100}%`,
    height: `${(box.y2 - box.y1) * 100}%`,
    borderColor: color,
  }
  return (
    <div
      className="ls-overlay-box"
      style={style}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      title={box.label}
    >
      <span className="ls-overlay-box__label" style={{ background: color }}>
        {box.label}
      </span>
    </div>
  )
}
