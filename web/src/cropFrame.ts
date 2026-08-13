import type { NormalizedRect } from './VideoStream'

// cropFrame extracts the sub-rectangle rect (normalized [0,1], the same
// coordinate space VideoOverlayBox/VideoStream already use to position
// boxes over the container) out of img — a plain <canvas> crop, no
// network round-trip needed since the frame is already decoded and
// painted client-side. Used by the "sélection runtime + labellisation"
// flow (LiveView.tsx, TODO.md § H1) to turn a clicked/drawn box into the
// JPEG blob POST /api/v1/gallery expects.
//
// Known imprecision, not fixed here: VideoStream.css renders the <img>
// with object-fit: contain, so on an aspect-ratio mismatch the image is
// letterboxed inside its container — rect (container-relative, same as
// every overlay box already drawn on screen) can be slightly offset from
// the true image content in that case. Not a regression introduced by
// this feature: the existing detection-box overlay already has the exact
// same approximation, this crop is only ever as precise as what the user
// already sees drawn on screen.
export function cropFrame(img: HTMLImageElement, rect: NormalizedRect): Promise<Blob> {
  const w = img.naturalWidth
  const h = img.naturalHeight
  if (w === 0 || h === 0) {
    return Promise.reject(new Error('frame not loaded yet'))
  }

  const sx = rect.x1 * w
  const sy = rect.y1 * h
  const sw = Math.max(1, (rect.x2 - rect.x1) * w)
  const sh = Math.max(1, (rect.y2 - rect.y1) * h)

  const canvas = document.createElement('canvas')
  canvas.width = sw
  canvas.height = sh
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    return Promise.reject(new Error('canvas 2d context unavailable'))
  }
  ctx.drawImage(img, sx, sy, sw, sh, 0, 0, sw, sh)

  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error('canvas.toBlob() returned null'))),
      'image/jpeg',
      0.9,
    )
  })
}
