package output

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"sync"
	"time"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"

	"github.com/gorilla/websocket"
)

// jpegQuality mirrors WebSocketOutput's own choice (websocket.go) — same
// "good enough for a live/replay feed, not archival" trade-off applies to
// a buffered rewind frame just as much as a live one.
const jpegQuality = 80

// maxEntries is a hard safety cap independent of maxAge — protects
// against unbounded growth if Render is ever called at a much higher
// rate than the ~3-5fps this project's own detection cycle normally
// produces (docs/adr/clip-backend.md's own measured timings), e.g. a
// future change that renders every raw input frame instead of one per
// reanchor cycle. Whichever limit (age or count) is hit first evicts.
const maxEntries = 2000

// RingBufferOutput decorates another streamer.OutputStream, buffering the
// last maxAge worth of rendered frames (already JPEG-encoded, paired with
// whatever boxes were current) so it can also serve as a
// streamer.Rewindable — added 2026-08-13, TODO.md § H1 "Pause/reprise +
// buffer de rewind". The live pipeline itself is never paused by this;
// it only lets a *client* look back, per the explicit product requirement
// (docs/gui/spec.md § 1.5bis).
//
// Requires inner to implement streamer.BoxAwareOutputStream — see
// NewRingBufferOutput's own doc comment for why silently allowing a
// non-box-aware inner would be a real footgun, not a defensive
// over-restriction.
//
// Boxes/frame pairing is best-effort, not strictly ordered: Render and
// RenderBoxes are documented (streamer.BoxAwareOutputStream) as not
// having a guaranteed call order relative to each other within one
// render cycle — this decorator always pairs a newly rendered frame with
// whatever boxes were *most recently seen* at that instant, which can be
// one cycle stale in the rare case RenderBoxes for cycle N arrives after
// Render for cycle N+1. Same tolerance already accepted for live display
// (useVideoStream.ts treats the two message types independently) — not a
// new source of imprecision.
type RingBufferOutput struct {
	inner        streamer.OutputStream
	innerBoxes   streamer.BoxAwareOutputStream
	innerBcaster frameBroadcaster // nil if inner doesn't support it (see AddClient's own doc comment)
	maxAge       time.Duration

	mu        sync.Mutex
	entries   []ringEntry
	lastBoxes []streamer.BoxData
}

// frameBroadcaster structurally mirrors transport/adapters/api.
// FrameBroadcaster — duplicated here rather than imported (this package
// must never import transport/*, wrong dependency direction) so
// RingBufferOutput can forward AddClient/RemoveClient to inner and, by
// Go's structural typing, still satisfy transport's own FrameBroadcaster
// interface without either package knowing about the other's type.
//
// Found missing 2026-08-13 the same day this file was added: wrapping
// WebSocketOutput in RingBufferOutput without this made
// session.Manager.Output(id) fail transport/adapters/api's own
// `out.(FrameBroadcaster)` type assertion (handleSessionWebSocket) — no
// client ever got registered, so a session ran with zero visible video
// in the GUI despite the backend processing frames normally. Real bug,
// caught in a live run, not theoretical.
type frameBroadcaster interface {
	AddClient(conn *websocket.Conn)
	RemoveClient(conn *websocket.Conn)
}

type ringEntry struct {
	at    time.Time
	jpeg  []byte
	boxes []streamer.BoxData
}

var (
	_ streamer.OutputStream         = (*RingBufferOutput)(nil)
	_ streamer.BoxAwareOutputStream = (*RingBufferOutput)(nil)
	_ streamer.Rewindable           = (*RingBufferOutput)(nil)
)

// NewRingBufferOutput wraps inner, which must itself implement
// streamer.BoxAwareOutputStream — errors otherwise rather than silently
// dropping every box RenderBoxes ever receives (a RingBufferOutput always
// advertises itself as box-aware to application/uc, which would then
// skip drawer.BoxDrawer entirely and rely on RenderBoxes reaching a real
// sink; an inner that can't receive box data would lose them for good).
// In practice this project only ever wraps
// implementation/streamer/output.WebSocketOutput with it (session.Manager,
// cmd/livesemantic/main.go) — the window/CLI output is never wrapped,
// it composites boxes into pixels itself and has no rewind UI to serve.
func NewRingBufferOutput(inner streamer.OutputStream, maxAge time.Duration) (*RingBufferOutput, error) {
	innerBoxes, ok := inner.(streamer.BoxAwareOutputStream)
	if !ok {
		return nil, fmt.Errorf("output: RingBufferOutput requires a BoxAwareOutputStream inner, got %T", inner)
	}
	// innerBcaster is allowed to be nil (a plain BoxAwareOutputStream that
	// isn't also a FrameBroadcaster) — AddClient/RemoveClient just become
	// no-ops in that case, see their own doc comments. Every real inner
	// this project constructs (WebSocketOutput) satisfies both, so this
	// is defensive, not the expected path.
	innerBcaster, _ := inner.(frameBroadcaster)
	return &RingBufferOutput{inner: inner, innerBoxes: innerBoxes, innerBcaster: innerBcaster, maxAge: maxAge}, nil
}

// AddClient forwards to inner's own AddClient — see frameBroadcaster's
// doc comment for why this exists at all (structural satisfaction of
// transport/adapters/api.FrameBroadcaster). No-op if inner doesn't
// support it.
func (r *RingBufferOutput) AddClient(conn *websocket.Conn) {
	if r.innerBcaster != nil {
		r.innerBcaster.AddClient(conn)
	}
}

// RemoveClient — see AddClient's doc comment.
func (r *RingBufferOutput) RemoveClient(conn *websocket.Conn) {
	if r.innerBcaster != nil {
		r.innerBcaster.RemoveClient(conn)
	}
}

// Initialize — see streamer.OutputStream.
func (r *RingBufferOutput) Initialize() error { return r.inner.Initialize() }

// HandleKeyEvent — see streamer.OutputStream.
func (r *RingBufferOutput) HandleKeyEvent() int { return r.inner.HandleKeyEvent() }

// Stop — see streamer.OutputStream.
func (r *RingBufferOutput) Stop() { r.inner.Stop() }

// Cleanup — see streamer.OutputStream. Also drops every buffered entry —
// a RingBufferOutput isn't reused across sessions the way BrowserInput
// is (session.Manager's OutputFactory builds a fresh one per session),
// but clearing is cheap and avoids holding megabytes of JPEG bytes any
// longer than necessary.
func (r *RingBufferOutput) Cleanup() {
	r.mu.Lock()
	r.entries = nil
	r.mu.Unlock()
	r.inner.Cleanup()
}

// Render — see streamer.OutputStream. Encodes frame once here (JPEG,
// same quality as WebSocketOutput's own live encode) — inner does its
// own separate encode for the live path, a small duplicated cost
// (docs/adr/clip-backend.md's own measured per-cycle timings dwarf a
// single JPEG encode) traded for keeping this decorator fully independent
// of inner's internals rather than trying to intercept/share its
// already-encoded bytes.
func (r *RingBufferOutput) Render(frame *entities.Frame) error {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, frame.Image, &jpeg.Options{Quality: jpegQuality}); err == nil {
		r.mu.Lock()
		now := time.Now()
		r.entries = append(r.entries, ringEntry{at: now, jpeg: buf.Bytes(), boxes: r.lastBoxes})
		r.evictLocked(now)
		r.mu.Unlock()
	}
	return r.inner.Render(frame)
}

// RenderBoxes — see streamer.BoxAwareOutputStream. See the struct doc
// comment for the pairing-isn't-strictly-ordered caveat.
func (r *RingBufferOutput) RenderBoxes(boxes []streamer.BoxData) error {
	r.mu.Lock()
	r.lastBoxes = boxes
	r.mu.Unlock()
	return r.innerBoxes.RenderBoxes(boxes)
}

// evictLocked drops entries older than r.maxAge or beyond maxEntries —
// must be called with r.mu held. Entries are append-only/time-ordered, so
// eviction is always from the front.
func (r *RingBufferOutput) evictLocked(now time.Time) {
	cutoff := now.Add(-r.maxAge)
	i := 0
	for i < len(r.entries) && r.entries[i].at.Before(cutoff) {
		i++
	}
	if excess := len(r.entries) - i; excess > maxEntries {
		i += excess - maxEntries
	}
	if i > 0 {
		r.entries = r.entries[i:]
	}
}

// RewindAt — see streamer.Rewindable. Picks the entry closest to
// (now - offset) — buffered timestamps rarely land exactly on the
// requested offset (detection cycles aren't perfectly periodic), closest
// match is the only sensible contract.
func (r *RingBufferOutput) RewindAt(offset time.Duration) (streamer.RewindEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) == 0 {
		return streamer.RewindEntry{}, false
	}

	target := time.Now().Add(-offset)
	best := 0
	bestDelta := absDuration(r.entries[0].at.Sub(target))
	for i := 1; i < len(r.entries); i++ {
		if d := absDuration(r.entries[i].at.Sub(target)); d < bestDelta {
			best, bestDelta = i, d
		}
	}

	e := r.entries[best]
	return streamer.RewindEntry{JPEG: e.jpeg, Boxes: e.boxes, AgeAgo: time.Since(e.at)}, true
}

// RewindRange — see streamer.Rewindable.
func (r *RingBufferOutput) RewindRange() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return 0
	}
	return time.Since(r.entries[0].at)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
