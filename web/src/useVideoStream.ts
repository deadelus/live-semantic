import { useEffect, useRef, useState } from 'react'
import type { OverlayBox } from './components/VideoOverlayBox'

interface RawBoxData {
  ID: string
  Label: string
  TrackID: string
  X1: number
  Y1: number
  X2: number
  Y2: number
}

interface VideoStreamState {
  frameURL: string | null
  boxes: OverlayBox[]
  connected: boolean
}

// Single /ws/sessions/:id connection, handling both message types the
// backend sends (docs/adr/clip-backend.md § 32): binary JPEG frames
// (undrawn — this hook turns each into an object URL, revoking the
// previous one so a long session doesn't leak one blob URL per frame)
// and text JSON box data ({"boxes":[...]}, sent every cycle even empty).
// One hook/one connection rather than two, so the two message types share
// the exact same underlying WebSocket instead of opening it twice.
//
// Migrated 2026-08-13 from the mono-session /ws endpoint to the per-
// session /ws/sessions/:id one (docs/gui/spec.md § 1.1) — sessionId is
// null until App.tsx has created a session via api.createSession, so no
// connection is attempted before one exists.
export function useVideoStream(sessionId: string | null): VideoStreamState {
  const [frameURL, setFrameURL] = useState<string | null>(null)
  const [boxes, setBoxes] = useState<OverlayBox[]>([])
  const [connected, setConnected] = useState(false)
  const lastURL = useRef<string | null>(null)

  useEffect(() => {
    if (!sessionId) {
      setConnected(false)
      return
    }

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${window.location.host}/ws/sessions/${sessionId}`)
    ws.binaryType = 'blob'

    ws.onopen = () => setConnected(true)
    ws.onclose = () => setConnected(false)
    ws.onerror = () => setConnected(false)
    ws.onmessage = (event: MessageEvent<Blob | string>) => {
      if (typeof event.data === 'string') {
        try {
          const body = JSON.parse(event.data) as { boxes: RawBoxData[] }
          setBoxes(
            body.boxes.map((b) => ({
              id: b.ID,
              label: b.Label,
              trackId: b.TrackID,
              x1: b.X1,
              y1: b.Y1,
              x2: b.X2,
              y2: b.Y2,
            })),
          )
        } catch {
          /* malformed message — keep showing the last good box set rather than clearing it */
        }
        return
      }

      const url = URL.createObjectURL(event.data)
      if (lastURL.current) {
        URL.revokeObjectURL(lastURL.current)
      }
      lastURL.current = url
      setFrameURL(url)
    }

    return () => {
      ws.close()
      if (lastURL.current) {
        URL.revokeObjectURL(lastURL.current)
      }
    }
  }, [sessionId])

  return { frameURL, boxes, connected }
}
