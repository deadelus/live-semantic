package input

import (
	"fmt"
	"sync"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"
)

var _ streamer.InputStream = (*BrowserInput)(nil)

// BrowserInput is a streamer.InputStream fed from outside itself — frames
// arrive via PushFrame (called by the /ws/ingest handler,
// transport/adapters/api, as the browser pushes JPEG frames captured from
// its own getUserMedia camera) rather than opened directly by this
// adapter (contrast CameraInput/FileInput, which own a gocv.VideoCapture).
// Browser camera capture — makes the GUI usable as a
// real web app from any device, not just the machine running the backend
// (docs/gui/spec.md § 2's WebRTC path is the eventual richer version of
// this same idea; this is the JPEG-over-WS v1 explicitly allowed as a
// simpler fallback there).
//
// frames is buffer-1, overwrite-on-full — same "always the freshest
// frame, never block the sender" pattern as RecognitionUseCase's own
// detection hand-off (uc_recognition.go), applied here one level
// upstream: a slow/backed-up recognition loop shouldn't make the
// WebSocket ingest handler (and therefore the browser's own capture loop)
// block or buffer up stale frames either.
type BrowserInput struct {
	frames chan *entities.Frame

	// mu guards stop/running, both read and written from different
	// goroutines (Start's own goroutine vs. whoever calls Stop() — the
	// REST /recognition/stop handler, a separate HTTP goroutine). Found
	// necessary via `go test -race` (2026-08-12): the naive unguarded
	// version raced on exactly this.
	mu      sync.Mutex
	stop    chan struct{}
	running bool
}

// NewBrowserInput creates an unstarted BrowserInput. PushFrame can be
// called even before Start (frames just queue up to 1 deep, oldest
// dropped) — the WS ingest handler and the recognition session it feeds
// aren't guaranteed to start in a particular order.
func NewBrowserInput() *BrowserInput {
	return &BrowserInput{
		frames: make(chan *entities.Frame, 1),
		stop:   make(chan struct{}),
	}
}

// Initialize implements streamer.InputStream. Nothing to open — unlike
// CameraInput/FileInput, there's no local device/file handle to acquire.
func (bi *BrowserInput) Initialize() error {
	return nil
}

// Start implements streamer.InputStream — reads from frames until Stop is
// called or the channel is closed, invoking frameActionCallback per frame
// like every other InputStream. No mirroring (unlike CameraInput): a
// browser's own getUserMedia feed is already displayed un-mirrored to the
// user capturing it, mirroring here would flip it unexpectedly relative
// to what they see themselves.
func (bi *BrowserInput) Start(frameActionCallback func(*entities.Frame) (*entities.Frame, error)) error {
	defer bi.Cleanup()

	bi.mu.Lock()
	bi.running = true
	stopCh := bi.stop // stable local reference — see Cleanup's doc comment
	bi.mu.Unlock()

	for {
		select {
		case <-stopCh:
			return nil
		case frame, ok := <-bi.frames:
			if !ok {
				return nil
			}
			if _, err := frameActionCallback(frame); err != nil {
				return err
			}
		}
	}
}

// PushFrame hands a freshly decoded frame to the running (or not-yet-
// started) Start loop — called from the /ws/ingest handler's read loop,
// a different goroutine than Start's. Non-blocking: drops whatever's
// pending and replaces it, same overwrite-on-full pattern as
// uc_recognition.go's own frame hand-off — see the struct doc comment.
func (bi *BrowserInput) PushFrame(frame *entities.Frame) {
	select {
	case bi.frames <- frame:
	default:
		select {
		case <-bi.frames:
		default:
		}
		select {
		case bi.frames <- frame:
		default:
		}
	}
}

// Stop implements streamer.InputStream.
func (bi *BrowserInput) Stop() {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	if !bi.running {
		return
	}
	bi.running = false
	close(bi.stop)
}

// Cleanup implements streamer.InputStream. Recreates the stop channel so
// this instance can Start() again on a future session — Stop() closes the
// previous one (a closed channel can't be reopened), but the same
// *BrowserInput instance is reused across sessions (wired once in
// main.go, like CameraInput). Safe against Start's in-flight stopCh: it
// captured its own reference to the old channel before this runs (Start's
// deferred call to Cleanup only fires after Start has already returned
// and stopped reading stopCh), so replacing bi.stop here never affects an
// already-running Start loop, only the next one.
func (bi *BrowserInput) Cleanup() {
	bi.mu.Lock()
	bi.stop = make(chan struct{})
	bi.mu.Unlock()
	fmt.Println("BrowserInput resources cleaned up successfully.")
}
