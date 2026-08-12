import { useEffect, useRef, useState } from 'react'

// Consumes the backend's /ws endpoint: one binary WebSocket message per
// rendered frame, JPEG bytes with boxes already drawn server-side
// (websocket.go/websocket_output.go — H1 minimal scope, no separate JSON
// for boxes/scores yet, docs/gui/spec.md § 2's richer protocol is a later
// iteration). Each frame is turned into an object URL for an <img> — the
// previous URL is revoked right after so we don't leak one blob URL per
// frame (this receives frames continuously, easily hundreds within a
// short session).
//
// Deliberately no reconnect-with-backoff logic yet — a dropped connection
// just freezes the last frame until the page is reloaded. Acceptable for
// this first vertical slice (single local session, TODO.md § H2), not for
// a real multi-source/networked deployment.
export function VideoStream() {
  const [frameURL, setFrameURL] = useState<string | null>(null)
  const [connected, setConnected] = useState(false)
  const lastURL = useRef<string | null>(null)

  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${window.location.host}/ws`)
    ws.binaryType = 'blob'

    ws.onopen = () => setConnected(true)
    ws.onclose = () => setConnected(false)
    ws.onerror = () => setConnected(false)
    ws.onmessage = (event: MessageEvent<Blob>) => {
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
  }, [])

  return (
    <div className="video-stream">
      {frameURL ? (
        <img src={frameURL} alt="Live recognition feed" />
      ) : (
        <div className="video-stream-placeholder">
          {connected ? 'Connecté — en attente de frames…' : 'Connexion au flux…'}
        </div>
      )}
    </div>
  )
}
