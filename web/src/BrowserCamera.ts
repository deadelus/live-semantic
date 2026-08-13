// Captures the user's own device camera (getUserMedia) and streams it to
// the backend's per-session ingest endpoint (/ws/sessions/:id/ingest) as
// one JPEG frame per tick — the "JPEG-over-WS" v1 the backend's
// docs/gui/spec.md § 2 explicitly allows as a simpler fallback before a
// full WebRTC (pion/webrtc) integration. Migrated 2026-08-13 from the
// mono-session /ws/ingest endpoint — the ingest path is now passed to
// start() rather than hardcoded, since it depends on which session this
// capture feeds.
// This is what makes the GUI a real web app usable from any device's
// camera, not just the machine running the Go backend (TODO.md § H2,
// clarified with the user 2026-08-12 — the earlier "local camera" slice
// only ever showed the backend's own machine's camera).
//
// Deliberately simple: a hidden <video> fed by getUserMedia, a <canvas>
// snapshot on an interval, encoded as JPEG and sent as a binary WS
// message. No adaptive bitrate/framerate, no reconnect-with-backoff — a
// dropped ingest connection just stops sending, same "acceptable for a
// first vertical slice" scope as VideoStream.tsx's own simplifications.
export class BrowserCamera {
  private stream: MediaStream | null = null
  private video: HTMLVideoElement | null = null
  private canvas: HTMLCanvasElement | null = null
  private ws: WebSocket | null = null
  private intervalId: number | null = null

  // captureIntervalMs: how often a frame is captured and sent. 150ms
  // (~6-7 fps) — deliberately well under the backend's own reanchor pace
  // (~150-270ms measured, docs/adr/clip-backend.md § 8) since there's no
  // point pushing frames faster than the backend can ever act on them,
  // and JPEG-encoding + sending every frame at full camera framerate
  // (usually 30fps) would waste bandwidth/CPU on the browser side for no
  // visible benefit.
  private static readonly captureIntervalMs = 150
  private static readonly jpegQuality = 0.8

  // ingestPath: e.g. `/ws/sessions/${sessionId}/ingest` — the caller
  // (App.tsx) owns knowing which session this capture feeds, this class
  // stays session-agnostic.
  async start(ingestPath: string): Promise<void> {
    this.stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false })

    this.video = document.createElement('video')
    this.video.srcObject = this.stream
    this.video.muted = true
    await this.video.play()

    this.canvas = document.createElement('canvas')
    this.canvas.width = this.video.videoWidth || 640
    this.canvas.height = this.video.videoHeight || 480

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    this.ws = new WebSocket(`${proto}://${window.location.host}${ingestPath}`)
    this.ws.binaryType = 'arraybuffer'

    await new Promise<void>((resolve, reject) => {
      if (!this.ws) return reject(new Error('WebSocket not created'))
      this.ws.onopen = () => resolve()
      this.ws.onerror = () => reject(new Error(`failed to connect to ${ingestPath}`))
    })

    this.intervalId = window.setInterval(() => this.captureAndSend(), BrowserCamera.captureIntervalMs)
  }

  private captureAndSend(): void {
    if (!this.video || !this.canvas || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }
    const ctx = this.canvas.getContext('2d')
    if (!ctx) return

    ctx.drawImage(this.video, 0, 0, this.canvas.width, this.canvas.height)
    this.canvas.toBlob(
      (blob) => {
        if (blob && this.ws?.readyState === WebSocket.OPEN) {
          blob.arrayBuffer().then((buf) => this.ws?.send(buf))
        }
      },
      'image/jpeg',
      BrowserCamera.jpegQuality,
    )
  }

  stop(): void {
    if (this.intervalId !== null) {
      window.clearInterval(this.intervalId)
      this.intervalId = null
    }
    this.ws?.close()
    this.ws = null
    this.stream?.getTracks().forEach((track) => track.stop())
    this.stream = null
    this.video = null
    this.canvas = null
  }
}
