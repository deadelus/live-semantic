// Package streamer defines the ports for video frame I/O — InputStream
// (camera/file/browser-ingest sources) and OutputStream (window/WebSocket
// sinks) — implemented by implementation/streamer/{input,output}.
package streamer

import (
	"time"

	"live-semantic/internal/domain/entities"
)

// InputStream is the port for a source of video frames (camera, file,
// browser ingest). Initialize sets up the stream, Start reads frames in
// a blocking loop until Stop is called, Cleanup releases resources once
// the stream is discarded.
type InputStream interface {
	// Initialize sets up the input stream (camera, file, cctv, etc.).
	Initialize() error
	// Start begins reading frames from the input stream.
	Start(frameActionCallback func(*entities.Frame) (*entities.Frame, error)) error
	// Stop halts the input stream and releases any resources.
	Stop()
	// Cleanup performs any necessary cleanup before the processor is discarded.
	Cleanup()
}

// OutputStream is the port for a destination that renders/forwards
// analyzed frames (the gocv window, a WebSocket sink...).
type OutputStream interface {
	// Initialize sets up the output stream (websocket, stream server, window, etc.).
	Initialize() error
	// Render outputs a frame to the destination.
	Render(frame *entities.Frame) error
	// HandleKeyEvent processes output-specific key events (optional).
	HandleKeyEvent() int
	// Stop halts the output stream and releases any resources.
	Stop()
	// Cleanup performs any necessary cleanup before the processor is discarded.
	Cleanup()
}

// BoxData is one detected/tracked box as structured data — label, score,
// and coordinates normalized to [0,1] (fraction of frame width/height,
// not pixels), so a client doesn't need to know the source frame's
// resolution to position an overlay. docs/gui/mockups/
// (screens 1c/1d/1h — boxes drawn as separate, hoverable/clickable DOM
// elements, colored by filter term, not baked into the video pixels).
type BoxData struct {
	// ID is the filter term/track identity (trackedBox.FilterKey in
	// application/uc — e.g. "person" or "person with a hat"), used for
	// color assignment client-side, same as drawer.BoxID server-side.
	ID string
	// Label is what to actually display — same string RecognitionUseCase
	// already formats today ("person (89.97%)"), kept as one field rather
	// than separate name/percent so the client doesn't need to duplicate
	// that formatting logic (percent-vs-score-vs-confidence, docs/adr/
	// clip-backend.md § 20-21).
	Label string
	// TrackID identifies the specific track — lets a client correlate a
	// box across successive messages (e.g. to animate a transition)
	// without depending on array order, which reanchor's map iteration
	// doesn't guarantee is stable frame to frame.
	TrackID        string
	X1, Y1, X2, Y2 float32
}

// RewindEntry is one buffered instant a Rewindable OutputStream can hand
// back — an already-JPEG-encoded, undrawn frame (same "undrawn + separate
// boxes" shape as Render/RenderBoxes, docs/adr/clip-backend.md § 32) plus
// the boxes that were current at that instant and how long ago it was
// captured (relative to when At was called, not an absolute timestamp —
// callers only ever reason in "how far back", matching the GUI's own
// "retour en arrière de N secondes" framing, docs/gui/spec.md § 1.5bis).
type RewindEntry struct {
	JPEG   []byte
	Boxes  []BoxData
	AgeAgo time.Duration
}

// Rewindable is an optional capability an OutputStream may implement
// (checked via type assertion, same pattern as BoxAwareOutputStream) —
// pause/reprise + retour en arrière on the GUI's Vue live (docs/gui/spec.md
// § 1.5bis/3.2, TODO.md § H1 "Pause/reprise + buffer de rewind"), added
// 2026-08-13. The live detection/tracking pipeline itself never pauses —
// this only lets a client *look back* at recent already-rendered frames
// without interrupting it, per the explicit product requirement ("le flux
// réel qui continue tourner en arrière-plan").
type Rewindable interface {
	// RewindAt returns the buffered frame closest to offset time ago
	// (e.g. offset=5s -> "what did this flow look like 5 seconds ago") —
	// ok is false if nothing has been buffered yet, or offset exceeds
	// RewindRange().
	RewindAt(offset time.Duration) (RewindEntry, bool)
	// RewindRange reports how far back rewinding is currently possible —
	// grows from zero right after a session starts up to the
	// implementation's configured retention window.
	RewindRange() time.Duration
}

// BoxAwareOutputStream is an optional capability an OutputStream may
// implement alongside OutputStream itself — checked via a type assertion
// in application/uc (not part of the base OutputStream contract, since
// most implementations — the gocv window, a future log-only output —
// have no separate channel for structured data and are fine with boxes
// composited directly into the frame's pixels, drawer.BoxDrawer). An
// OutputStream that DOES implement this receives the frame *undrawn*
// (RecognitionUseCase skips drawer.BoxDrawer for it) and gets box data
// through RenderBoxes instead — see websocket_output.go's implementation.
type BoxAwareOutputStream interface {
	// RenderBoxes delivers the current cycle's boxes as structured data,
	// called once per rendered frame alongside Render (undrawn) — order
	// between the two isn't contractually guaranteed, a client should be
	// able to handle either arriving first.
	RenderBoxes(boxes []BoxData) error
}
