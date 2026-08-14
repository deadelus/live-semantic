// Captures the user's own device camera and streams it to the backend
// over a real WebRTC PeerConnection (implementation/streamer/input.
// WebRTCInput, POST /api/v1/sessions/:id/webrtc/offer) — the richer
// alternative to BrowserCamera.ts's JPEG-over-WS v1 (docs/gui/spec.md §
// 2's own stated priority: WebRTC first, JPEG-over-WS as the fallback).
// Backend added 2026-08-13 (branch feat/gui-webrtc-ingestion) with no
// frontend client until now — this is that client.
//
// Non-trickle signaling to match the backend's own contract
// (WebRTCInput.HandleOffer waits for its *own* ICE gathering to finish
// before answering, and expects the offer it receives to already be
// "complete" too — a single POST/response round trip, no separate
// candidate-exchange channel this REST endpoint doesn't have). Same
// STUN-only limitation as the backend: works for two peers on the same
// local network (the target use case, "webcam de l'ordi qui fait tourner
// le backend" or one on the same LAN), a genuinely remote peer behind a
// symmetric NAT will fail to connect (no TURN relay configured, see
// implementation/streamer/input/webrtc.go's own doc comment).
export class WebRTCCamera {
  private stream: MediaStream | null = null
  private pc: RTCPeerConnection | null = null

  async start(sessionId: string): Promise<void> {
    this.stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false })

    const pc = new RTCPeerConnection({ iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] })
    this.pc = pc
    for (const track of this.stream.getVideoTracks()) {
      pc.addTrack(track, this.stream)
    }

    const offer = await pc.createOffer()
    await pc.setLocalDescription(offer)
    await waitForIceGatheringComplete(pc)

    const localDescription = pc.localDescription
    if (!localDescription) {
      throw new Error('RTCPeerConnection has no local description after setLocalDescription()')
    }

    const res = await fetch(`/api/v1/sessions/${sessionId}/webrtc/offer`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: localDescription.type, sdp: localDescription.sdp }),
    })
    const body = await res.json().catch(() => null)
    if (!res.ok) {
      throw new Error(body?.error ?? `webrtc offer failed (${res.status})`)
    }

    await pc.setRemoteDescription(new RTCSessionDescription(body))
    await waitForConnected(pc)
  }

  stop(): void {
    this.pc?.close()
    this.pc = null
    this.stream?.getTracks().forEach((track) => track.stop())
    this.stream = null
  }
}

function waitForIceGatheringComplete(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === 'complete') return Promise.resolve()
  return new Promise((resolve) => {
    const check = () => {
      if (pc.iceGatheringState === 'complete') {
        pc.removeEventListener('icegatheringstatechange', check)
        resolve()
      }
    }
    pc.addEventListener('icegatheringstatechange', check)
  })
}

function waitForConnected(pc: RTCPeerConnection): Promise<void> {
  if (pc.connectionState === 'connected') return Promise.resolve()
  return new Promise((resolve, reject) => {
    const check = () => {
      if (pc.connectionState === 'connected') {
        pc.removeEventListener('connectionstatechange', check)
        resolve()
      } else if (pc.connectionState === 'failed' || pc.connectionState === 'closed') {
        pc.removeEventListener('connectionstatechange', check)
        reject(new Error(`WebRTC connection ${pc.connectionState}`))
      }
    }
    pc.addEventListener('connectionstatechange', check)
  })
}
