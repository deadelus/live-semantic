// Package session adds multiple, independently addressable recognition
// sessions ("multi-flux") on top of application/uc.UseCase —
// docs/gui/spec.md § 1.2 ("la demande qui a le plus d'impact
// architectural"). Each Session wraps its own uc.UseCases instance (its
// own InputStream/OutputStream, own trackManager per call — unchanged
// from the single-session path) while sharing the expensive ONNX
// resources (objectDetector, semanticEncoder), the notifier, the tracker
// factory, and the reference gallery (storage.GalleryStorage — deliberately
// shared, not one per session: a named reference should mean the same
// thing everywhere, not be re-added per source).
//
// Deliberately additive, not a replacement for the existing single-
// session REST/WS surface (transport/adapters/api's recognitionController,
// /ws, /ws/ingest): that path is already tested and the current web/
// frontend depends on it. Multi-session is a new, parallel capability;
// migrating the frontend onto it is a separate, later effort.
package session

import (
	"context"
	"fmt"
	"sync"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/application/uc"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/notifier"
	"live-semantic/internal/infrastructure/storage"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// Source describes where a session's frames come from — enough
// information for an InputFactory (below) to build the right concrete
// streamer.InputStream, without this package needing to know about any
// of them concretely (CameraInput/FileInput/BrowserInput all live in
// implementation/streamer/input, which this package never imports —
// same dependency-inversion discipline as the rest of application/*).
type Source struct {
	// Kind is "local" (the backend's own camera, Device selects which),
	// "browser" (frames pushed over a per-session ingest endpoint — see
	// transport/adapters/api), "webrtc" (browser camera over a real
	// WebRTC PeerConnection instead of repeated JPEG-over-WS snapshots —
	// implementation/streamer/input.WebRTCInput, negotiated via a
	// per-session SDP offer/answer REST endpoint, added 2026-08-13), or
	// "file" (local video file, RTSP, or HTTP — URI, all three resolved
	// the same way by gocv, see implementation/streamer/input.FileInput's
	// own doc comment).
	Kind   string `json:"kind"`
	Device int    `json:"device,omitempty"`
	URI    string `json:"uri,omitempty"`
}

// InputFactory builds a fresh streamer.InputStream for a session's
// Source — supplied by the composition root (cmd/livesemantic/main.go),
// the only place allowed to know about concrete adapters. Called once
// per CreateSession.
type InputFactory func(Source) (streamer.InputStream, error)

// OutputFactory builds a fresh streamer.OutputStream for a new session —
// in practice always a new *implementation/streamer/output.WebSocketOutput
// (one per session, its own client list, so /ws traffic for one source
// never reaches a client subscribed to another) — same rationale as
// InputFactory for keeping the concrete type out of this package.
type OutputFactory func() streamer.OutputStream

// Info is the read-only, REST-facing view of a session — no internal
// handles (InputStream/OutputStream/UseCases), just what a client needs
// to render a sources list (docs/gui/mockups/ screens 1a/1b).
type Info struct {
	ID      string `json:"id"`
	Source  Source `json:"source"`
	Running bool   `json:"running"`
	Filter  string `json:"filter,omitempty"`
	Error   string `json:"error,omitempty"`
}

type entry struct {
	source   Source
	input    streamer.InputStream
	output   streamer.OutputStream
	useCases uc.UseCases

	mu        sync.Mutex
	running   bool
	filter    string
	lastError string
	// done is closed when the most recently started Recognize
	// goroutine returns — created fresh, synchronously, right before that
	// goroutine is spawned (StartRecognition), so RemoveSession/anything
	// else can wait on it correctly. Deliberately NOT e.useCases.WaitRecognition():
	// its sync.WaitGroup.Add(1) happens *inside* Recognize itself
	// (uc_recognition.go), i.e. on the spawned goroutine — no
	// happens-before guarantee versus a concurrent Wait() call racing
	// ahead of it from a different goroutine. go test -race caught this
	// for real on RemoveSession (2026-08-12), not hypothetical. A channel
	// has no such precondition: it exists before the goroutine does and is
	// safe to receive from at any point, including before the goroutine
	// has run its first statement. Starts pre-closed so an idle,
	// never-started session doesn't block a wait.
	done chan struct{}
}

// Manager owns every active session, addressable by ID. Safe for
// concurrent use — CreateSession/StartRecognition/etc. may be called
// from different HTTP handler goroutines.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*entry
	nextID   int

	inputFactory  InputFactory
	outputFactory OutputFactory

	logger                logger.Logger
	notifier              notifier.AlertSender
	objectDetector        inference.ObjectDetector
	semanticEncoder       inference.SemanticEncoder
	trackerFactoryBuilder func() tracking.TrackerFactory
	gallery               storage.GalleryStorage
	collections           storage.CollectionStorage
}

// NewManager creates an empty Manager. galleryRepo is shared across every
// session this Manager creates (and, per the caller's choice, can be the
// same instance passed to a separate single-session uc.UseCase too — see
// uc.NewUseCase's doc comment) — pass a fresh
// implementation/storage/inmemory.New() if no sharing with anything else
// is wanted.
//
// trackerFactoryBuilder, unlike objectDetector/semanticEncoder/gallery, is
// called fresh once per CreateSession rather than shared as-is — found
// necessary 2026-08-13 by a real SIGSEGV: gocvtracker.Factory owns a
// frame-conversion cache (gocvtracker.frameCache) that's only safe to
// share among Trackers *within one session*, never across sessions
// running concurrently (see that package's own doc comment for the crash
// trace). The composition root (cmd/livesemantic/main.go) passes a
// closure that builds a brand new *gocvtracker.Factory each call, not a
// shared instance.
func NewManager(
	inputFactory InputFactory,
	outputFactory OutputFactory,
	logger logger.Logger,
	notifier notifier.AlertSender,
	objectDetector inference.ObjectDetector,
	semanticEncoder inference.SemanticEncoder,
	trackerFactoryBuilder func() tracking.TrackerFactory,
	galleryRepo storage.GalleryStorage,
	collectionRepo storage.CollectionStorage,
) *Manager {
	return &Manager{
		sessions:              make(map[string]*entry),
		inputFactory:          inputFactory,
		outputFactory:         outputFactory,
		logger:                logger,
		notifier:              notifier,
		objectDetector:        objectDetector,
		semanticEncoder:       semanticEncoder,
		trackerFactoryBuilder: trackerFactoryBuilder,
		gallery:               galleryRepo,
		collections:           collectionRepo,
	}
}

// CreateSession builds a new session's input/output via the injected
// factories and wraps them in their own uc.UseCases instance — the
// expensive shared resources (detector/encoder/tracker factory/gallery)
// are handed the *same* pointers as every other session, only
// input/output/trackManager are per-session. Does not start recognition
// — see StartRecognition.
func (m *Manager) CreateSession(ctx context.Context, src Source) (Info, error) {
	in, err := m.inputFactory(src)
	if err != nil {
		return Info{}, fmt.Errorf("create input for source %+v: %w", src, err)
	}
	out := m.outputFactory()

	// localInput/browserInput: a per-session UseCase always has exactly
	// one real input (in), never a runtime choice between two (unlike
	// the single-session path's dto.RecognitionRequest.Source, which
	// exists because *that* UseCase can be asked for either — see its
	// own doc comment). Passed as both slots so the "local" default path
	// in Recognize (Source == "" -> localInput) always resolves
	// to the right thing regardless of what kind of source this actually
	// is (camera, file/RTSP, or browser ingest).
	useCases, err := uc.NewUseCase(ctx, m.logger, in, in, out, m.notifier, m.objectDetector, m.semanticEncoder, m.trackerFactoryBuilder(), m.gallery, m.collections)
	if err != nil {
		return Info{}, fmt.Errorf("create use cases: %w", err)
	}

	preClosed := make(chan struct{})
	close(preClosed)

	m.mu.Lock()
	m.nextID++
	id := fmt.Sprintf("session-%d", m.nextID)
	m.sessions[id] = &entry{source: src, input: in, output: out, useCases: useCases, done: preClosed}
	m.mu.Unlock()

	return Info{ID: id, Source: src}, nil
}

// StartRecognition runs uc.UseCases.Recognize for the given
// session in a goroutine and returns immediately — same "don't block the
// HTTP response on a call that blocks until the stream stops" rationale
// as the single-session recognitionController.start.
func (m *Manager) StartRecognition(ctx context.Context, id string, req dto.RecognitionRequest) error {
	e, ok := m.get(id)
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}

	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("session %q already has a recognition session running", id)
	}
	e.running = true
	e.filter = req.Filter
	e.lastError = ""
	doneCh := make(chan struct{})
	e.done = doneCh
	e.mu.Unlock()

	go func() {
		defer close(doneCh)
		defer func() {
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
		}()

		result, err := e.useCases.Recognize(ctx, req)
		if err != nil || !result.Success {
			errMsg := result.Error
			if errMsg == "" && err != nil {
				errMsg = err.Error()
			}
			m.logger.Error("Session recognition ended with error", map[string]interface{}{"session": id, "error": errMsg})
			e.mu.Lock()
			e.lastError = errMsg
			e.mu.Unlock()
			return
		}
		m.logger.Info("Session recognition ended", map[string]interface{}{"session": id})
	}()

	return nil
}

// StopRecognition signals the session's running recognition (if any) to
// halt — safe to call on an idle session (InputStream.Stop() already is,
// same contract as every InputStream implementation).
//
// Calls e.input.Stop() directly rather than e.useCases.StopRecognition() — found via
// a real race in this package's own tests (2026-08-12), not hypothetical:
// uc.UseCase.StopRecognition() reads uc.activeInput, which Recognize only
// assigns *inside* the goroutine StartRecognition spawns, asynchronously,
// a moment after StartRecognition itself already returned and flipped
// e.running to true. Calling StopRecognition fast enough after
// StartRecognition (exactly what a tight test loop does, polling every
// 5ms) can land before that assignment happens — uc.UseCase.StopRecognition() then
// finds activeInput still nil and does nothing, leaving the session
// stuck running forever. Manager already holds e.input directly (unlike
// the single-session recognitionController, which only has a uc.UseCases
// handle and therefore has no choice but to go through activeInput) — no
// reason to route through the same race-prone indirection when a stable,
// immediately-available reference exists. The same underlying race is
// believed to exist in the single-session path too (docs/adr/
// clip-backend.md, not fixed there in this pass — out of scope here,
// noted for later).
func (m *Manager) StopRecognition(id string) error {
	e, ok := m.get(id)
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	e.input.Stop()
	return nil
}

// RemoveSession stops (if running), waits for it to actually finish,
// cleans up its input/output, and forgets the session. Errors if the
// session doesn't exist; does NOT error if it was already idle. Uses
// e.input.Stop() directly — see StopRecognition's doc comment for why —
// and waits on e.done rather than e.useCases.WaitRecognition() — see entry.done's
// doc comment (the sync.WaitGroup version raced for real under -race).
func (m *Manager) RemoveSession(id string) error {
	m.mu.Lock()
	e, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}

	e.mu.Lock()
	doneCh := e.done
	e.mu.Unlock()

	e.input.Stop()
	<-doneCh
	e.input.Cleanup()
	e.output.Cleanup()
	return nil
}

// Info returns a session's current read-only state.
func (m *Manager) Info(id string) (Info, bool) {
	e, ok := m.get(id)
	if !ok {
		return Info{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return Info{ID: id, Source: e.source, Running: e.running, Filter: e.filter, Error: e.lastError}, true
}

// List returns every session's current state, ordered by ID.
func (m *Manager) List() []Info {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	infos := make([]Info, 0, len(ids))
	for _, id := range ids {
		if info, ok := m.Info(id); ok {
			infos = append(infos, info)
		}
	}
	return infos
}

// Output returns a session's OutputStream — callers (transport/adapters/api)
// type-assert it to whatever narrow capability interface they need
// (FrameBroadcaster, streamer.BoxAwareOutputStream), same pattern already
// used for the single-session path (cmd/livesemantic/main.go,
// uc_recognition.go) rather than this package knowing about those
// interfaces itself.
func (m *Manager) Output(id string) (streamer.OutputStream, bool) {
	e, ok := m.get(id)
	if !ok {
		return nil, false
	}
	return e.output, true
}

// Input returns a session's InputStream — callers type-assert it (e.g.
// to a FrameReceiver-shaped interface for browser ingest routing), same
// rationale as Output.
func (m *Manager) Input(id string) (streamer.InputStream, bool) {
	e, ok := m.get(id)
	if !ok {
		return nil, false
	}
	return e.input, true
}

// get looks up a session's entry by ID under the Manager-level lock —
// the shared read path behind every exported method that needs one.
func (m *Manager) get(id string) (*entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.sessions[id]
	return e, ok
}
