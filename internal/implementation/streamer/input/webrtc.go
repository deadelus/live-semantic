// Package input's WebRTCInput — browser camera capture over WebRTC
// (docs/gui/spec.md § 2, TODO.md § H1 "Ingestion WebRTC navigateur"),
// added 2026-08-13 as the richer alternative to BrowserInput's JPEG-over-
// WS v1 (kept, not replaced — still the fallback if WebRTC negotiation
// fails, per the priority already decided in docs/gui/spec.md § 0).
//
// Decode path, and why it needs an external ffmpeg process: pion/webrtc
// is pure Go (no CGo) for signaling/transport, exactly the advantage
// docs/gui/spec.md § 1.3 notes — but it deliberately doesn't decode video
// pixels, only depacketizes RTP into complete encoded frames. There is no
// mature pure-Go VP8/H264 pixel decoder, and reaching for one via CGo
// (libvpx/libx264 bindings) would reintroduce exactly the CGo fragility
// pion was chosen to avoid (docs/adr/inference-runtimes.md's own
// reasoning about gocv/onnxruntime_go's CGo brittleness applies equally
// here). Shelling out to `ffmpeg` instead is the same accepted trade-off
// this project already made for YouTube ingestion (`yt-dlp`, TODO.md §
// H1) — an external binary dependency, not a new Go/CGo one: each
// negotiated video track's RTP packets are muxed into an IVF stream
// (github.com/pion/webrtc/v4/pkg/media/ivfwriter, handles VP8
// depacketization internally — no samplebuilder needed), piped into
// ffmpeg's stdin, and ffmpeg's stdout (-f image2pipe -vcodec mjpeg) is
// read back as a plain JPEG stream — reusing image/jpeg.Decode and
// BrowserInput's exact PushFrame/Start/Stop machinery, since a decoded
// JPEG frame is indistinguishable from one BrowserInput itself receives.
//
// **Verified so far**: builds, vets, and the JPEG-stream framing
// (readJPEGStream) is unit-tested against synthetic byte streams
// (webrtc_test.go). **Not verified**: real SDP/ICE negotiation against
// an actual browser, and real VP8 media flowing through the ffmpeg pipe
// end-to-end — no browser or WebRTC-capable client is available in this
// environment. STUN-only configuration below (no TURN) — a non-local
// peer behind symmetric NAT will fail to connect, documented risk from
// docs/gui/spec.md § 4 point 4, not solved here.
package input

import (
	"bufio"
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"os/exec"
	"sync"
	"time"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
)

var _ streamer.InputStream = (*WebRTCInput)(nil)

// WebRTCInput embeds *BrowserInput purely for its frame-queue/Start/Stop/
// Cleanup mechanics (PushFrame's overwrite-on-full channel, the race-
// safe stop-channel dance documented on BrowserInput itself) — those are
// exactly what's needed here too, decoded JPEG frames just arrive from
// decodeTrack's ffmpeg pipe instead of a WebSocket reader loop. Stop/
// Cleanup are overridden to additionally tear down the PeerConnection
// (see their own doc comments).
type WebRTCInput struct {
	*BrowserInput

	mu sync.Mutex
	pc *webrtc.PeerConnection
}

// NewWebRTCInput creates an unconnected WebRTCInput — HandleOffer must be
// called (via the signaling REST endpoint) before any frames arrive.
func NewWebRTCInput() *WebRTCInput {
	return &WebRTCInput{BrowserInput: NewBrowserInput()}
}

// webrtcConfig is STUN-only (Google's public server, the same default
// used throughout pion's own examples) — no TURN relay, so a peer behind
// a symmetric NAT (a non-local browser) won't be able to connect. Known
// limitation, not a bug: TURN needs a relay server this project doesn't
// operate, see this file's own doc comment.
var webrtcConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
}

// HandleOffer performs one non-trickle signaling exchange: sets offer as
// the remote description, creates and sets a local answer, waits for ICE
// candidate gathering to finish, then returns the complete answer (with
// every gathered candidate already embedded in its SDP) — simpler
// contract for a plain REST offer/answer round trip than a separate
// trickle-ICE candidate-exchange endpoint would be, at the cost of
// waiting for the full gathering timeout on a slow network. A second call
// (e.g. the browser tab reloads and renegotiates) tears down any previous
// PeerConnection first — this WebRTCInput instance is reused across
// sessions/reconnects the same way BrowserInput is.
func (wi *WebRTCInput) HandleOffer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	wi.mu.Lock()
	defer wi.mu.Unlock()

	if wi.pc != nil {
		_ = wi.pc.Close()
		wi.pc = nil
	}

	pc, err := webrtc.NewPeerConnection(webrtcConfig)
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: create peer connection: %w", err)
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		go wi.decodeTrack(track)
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: create answer: %w", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: set local description: %w", err)
	}
	<-gatherComplete

	wi.pc = pc
	return *pc.LocalDescription(), nil
}

// decodeTrack owns one negotiated video track's whole decode pipeline —
// see this file's package doc comment for the ffmpeg rationale. Returns
// once track.ReadRTP() errors (peer disconnected, or Stop()/Cleanup()
// closed the PeerConnection) — ffmpeg is signaled to flush and exit by
// closing its stdin (ivfw.Close()), not killed.
func (wi *WebRTCInput) decodeTrack(track *webrtc.TrackRemote) {
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "ivf", "-i", "pipe:0",
		"-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "5", "pipe:1",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	defer func() { _ = cmd.Wait() }() // reap once both pipes are drained/closed below

	ivfw, err := ivfwriter.NewWith(stdin, ivfwriter.WithCodec(webrtc.MimeTypeVP8))
	if err != nil {
		_ = stdin.Close()
		return
	}

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		readJPEGStream(stdout, wi.PushFrame)
	}()

	for {
		pkt, _, readErr := track.ReadRTP()
		if readErr != nil {
			break
		}
		if writeErr := ivfw.WriteRTP(pkt); writeErr != nil {
			break
		}
	}
	_ = ivfw.Close() // also closes stdin -> ffmpeg flushes remaining frames and exits -> stdout EOF
	<-stdoutDone
}

// readJPEGStream splits ffmpeg's `-f image2pipe -vcodec mjpeg` stdout —
// complete JPEG images written back-to-back with no extra framing — into
// individual frames and calls push for each one, exactly like a
// browser's own JPEG-over-WS frame would be handed to BrowserInput.
// PushFrame (decodeTrack passes that method directly as push — a plain
// function taking a callback rather than a *WebRTCInput method so it can
// be unit-tested against a synthetic byte stream without needing a real
// channel/goroutine on the other end, see webrtc_test.go).
//
// Two-phase per frame: first resynchronize to the next SOI (0xFFD8)
// before accumulating anything (self-healing if a previous frame ended
// on anything other than a clean EOI — shouldn't happen against real
// ffmpeg output, but costs nothing to be defensive about), then
// accumulate until EOI (0xFFD9). Splitting on a literal 0xFFD9 once
// inside a frame is safe, not a heuristic: the JPEG spec requires any
// literal 0xFF byte inside entropy-coded scan data to be immediately
// followed by a stuffed 0x00 ("byte stuffing"), so the two-byte sequence
// FF D9 can only legitimately appear as a real end-of-image marker, never
// inside image data — the same technique common MJPEG-over-HTTP
// streaming servers rely on.
func readJPEGStream(r io.Reader, push func(*entities.Frame)) {
	br := bufio.NewReaderSize(r, 64*1024)

	for {
		if !seekSOI(br) {
			return
		}
		frame := []byte{0xFF, 0xD8}
		for {
			b, err := br.ReadByte()
			if err != nil {
				return
			}
			frame = append(frame, b)
			n := len(frame)
			if frame[n-2] == 0xFF && frame[n-1] == 0xD9 {
				break
			}
		}

		img, decodeErr := jpeg.Decode(bytes.NewReader(frame))
		if decodeErr != nil {
			continue // a corrupt frame despite well-formed markers — drop it, keep reading
		}
		push(&entities.Frame{Image: img, Timestamp: time.Now()})
	}
}

// seekSOI discards bytes from br until it has just consumed a JPEG SOI
// marker (0xFFD8), leaving br positioned right after it — false if the
// stream ended first.
func seekSOI(br *bufio.Reader) bool {
	var prev byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return false
		}
		if prev == 0xFF && b == 0xD8 {
			return true
		}
		prev = b
	}
}

// Stop closes the active PeerConnection (if any) in addition to
// BrowserInput's own Stop — this unblocks decodeTrack's track.ReadRTP()
// (which would otherwise keep the ffmpeg subprocess and decode goroutine
// running after recognition has stopped) and forces the browser to
// renegotiate on the next Start, same "torn down between sessions" shape
// as CameraInput's device handle.
func (wi *WebRTCInput) Stop() {
	wi.mu.Lock()
	if wi.pc != nil {
		_ = wi.pc.Close()
		wi.pc = nil
	}
	wi.mu.Unlock()
	wi.BrowserInput.Stop()
}

// Cleanup — see Stop's doc comment for why the PeerConnection is also
// closed here (belt and suspenders: Cleanup can run without a prior
// explicit Stop, e.g. via BrowserInput.Start's own deferred call).
func (wi *WebRTCInput) Cleanup() {
	wi.mu.Lock()
	if wi.pc != nil {
		_ = wi.pc.Close()
		wi.pc = nil
	}
	wi.mu.Unlock()
	wi.BrowserInput.Cleanup()
}
