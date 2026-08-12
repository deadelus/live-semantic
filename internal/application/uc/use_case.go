// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"image"
	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain"
	galleryport "live-semantic/internal/infrastructure/gallery"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/notifier"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"
	"sync"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// Recognition is the use case around running/controlling a single
// continuous video-analysis session — see uc_recognition.go/
// uc_control.go for the concrete implementation. Split out from the
// former single flat UseCases interface (2026-08-12) so a consumer that
// only needs recognition control (transport/adapters/api's
// recognitionController/session.Manager) can depend on this narrow
// interface instead of the wider UseCases composition below — interface
// segregation, and it also stops the gallery CRUD methods from being
// mixed in with recognition control in the same interface, which made
// the previous UseCases hard to read at a glance.
type Recognition interface {
	// Recognize starts the continuous analysis of the video stream and
	// blocks until it stops — see uc_recognition.go's own doc comment for
	// the full loop architecture. Named Recognize (not RecognitionUseCase)
	// to avoid stuttering against this interface's own name/package
	// (uc.Recognition.RecognitionUseCase read worse than uc.Recognition.
	// Recognize) — "UseCase" doesn't need to be repeated in every method
	// name once it's already the package/interface's job to say that.
	Recognize(ctx context.Context, req dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error)
	// StopRecognition halts the currently running Recognize call, if any —
	// see uc_control.go. Named to make clear it belongs to Recognition
	// specifically (a bare Stop() read as if it might stop *anything* —
	// found confusing once GalleryReferences existed alongside it in the
	// same interface). H1 minimal scope (TODO.md § H1): a single shared
	// UseCase serves one recognition session at a time (streamingInput/
	// streamingOutput are fields set once at construction, not per-call);
	// the caller (transport/adapters/api) is responsible for not starting
	// a second session concurrently. Revisit once multi-flux gives each
	// session its own UseCase/InputStream.
	StopRecognition()
	// WaitRecognition blocks until any in-flight Recognize call has fully
	// returned. Found necessary 2026-08-11 (real crash, not hypothetical —
	// docs/adr/clip-backend.md § 15, TODO.md § H1): main.go's graceful
	// shutdown handler used to call objectDetector.Cleanup()/
	// semanticEncoder.Cleanup() unconditionally on SIGTERM, destroying the
	// shared ONNX sessions while Recognize's detection goroutine could
	// still be mid-EncodeImage/AnalyzeFrame on those exact sessions
	// (SIGSEGV inside the CGo call). Callers should call StopRecognition()
	// first (to actually unstick a session that's still blocked reading
	// frames), then WaitRecognition(), before tearing down
	// objectDetector/semanticEncoder.
	WaitRecognition()
}

// GalleryReferences is CRUD over the "recognize by reference image"
// gallery (TODO.md § D "reconnaissance par référence image" / § H1,
// docs/adr/clip-backend.md § 24 — a gallery entry's name becomes usable
// directly as a filter term, matched by image↔image similarity instead
// of text↔image) — split from Recognition above for the same interface-
// segregation reason (transport/adapters/api's galleryController only
// needs this, not recognition control). See uc_gallery.go for the
// concrete implementation and infrastructure/gallery.Repository for the
// storage port it delegates to (application/uc.UseCase no longer holds a
// concrete gallery type directly — dependency inversion, same shape as
// every other port in this application).
type GalleryReferences interface {
	// AddGalleryReference encodes crop (a JPEG-decoded image, typically a
	// user-selected box from the live view) and stores it under name. See
	// uc_gallery.go for the validation rules (name can't be empty or
	// collide with a COCO class/an existing entry). ctx is checked before
	// the CLIP EncodeImage call (a real CGo/ONNX inference, not free) —
	// every GalleryReferences method takes ctx even where the underlying
	// work is normally fast/in-memory (Remove/Rename/SetEnabled/List
	// below), so a caller's cancellation (e.g. an HTTP client
	// disconnecting mid-request) is always honorable at this layer instead
	// of being silently ignored by some methods and not others (found
	// 2026-08-12 — Recognition's ctx parameter existed but every gallery
	// method had none at all).
	AddGalleryReference(ctx context.Context, name string, crop image.Image) error
	// RemoveGalleryReference deletes a gallery entry — see
	// gallery.Repository.Remove (idempotent, not an error if absent).
	RemoveGalleryReference(ctx context.Context, name string)
	// RenameGalleryReference — see uc_gallery.go/gallery.Repository.Rename.
	RenameGalleryReference(ctx context.Context, oldName, newName string) error
	// SetGalleryReferenceEnabled — see gallery.Repository.SetEnabled.
	SetGalleryReferenceEnabled(ctx context.Context, name string, enabled bool) error
	// ListGalleryReferences — see gallery.Repository.List.
	ListGalleryReferences(ctx context.Context) []GalleryEntryInfo
}

// UseCases composes every use case this application exposes. The
// concrete *UseCase implements all of them, and the composition root
// (cmd/livesemantic/main.go) still wires a single UseCases value where
// convenient — but most consumers should depend on the narrower
// interface they actually need (Recognition or GalleryReferences) rather
// than this one, per the interface segregation principle these two were
// split out for.
type UseCases interface {
	Recognition
	GalleryReferences
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger logger.Logger
	// localInput is the backend's own camera/file/RTSP source — always
	// set. browserInput feeds frames pushed over /ws/ingest (TODO.md § H2
	// "capture caméra navigateur") — nil in CLI/interactive mode, where
	// there's no web server to receive an ingest connection at all; a
	// request with Source: "browser" against a nil browserInput fails
	// clearly (uc_recognition.go) rather than nil-panicking.
	localInput      streamer.InputStream
	browserInput    streamer.InputStream
	streamingOutput streamer.OutputStream
	notifier        notifier.AlertSender
	objectDetector  inference.ObjectDetector
	semanticEncoder inference.SemanticEncoder
	trackerFactory  tracking.TrackerFactory

	// activeSessions tracks in-flight Recognize calls — see
	// WaitRecognition's doc comment. Incremented/decremented in Recognize
	// itself (uc_recognition.go), not here.
	activeSessions sync.WaitGroup

	// mu guards activeInput — set by Recognize to whichever of
	// localInput/browserInput this call picked, read by StopRecognition()
	// (a different goroutine, e.g. the REST /recognition/stop handler) so
	// it stops the *actual* running input rather than always localInput.
	mu          sync.Mutex
	activeInput streamer.InputStream

	// gallery is the {name, embedding} storage port for reference-image
	// filter terms (TODO.md § D/§ H1, docs/adr/clip-backend.md § 24) —
	// see uc_gallery.go for the GalleryReferences methods around it.
	// gallery.Repository, not a concrete type (dependency inversion,
	// 2026-08-12 — this used to be *ReferenceGallery, a concrete struct
	// defined in this very package, holding both storage and its own
	// validation logic; the port now lives in infrastructure/gallery, the
	// only implementation in implementation/gallery/inmemory, business
	// validation moved to uc_gallery.go where the rest of this
	// application's filter-grammar rules already live).
	gallery galleryport.Repository
}

// NewUseCase initializes your use cases with all the necessary
// dependencies. browserInput may be nil (CLI/interactive mode — see the
// UseCase struct's doc comment). galleryRepo is required, same as every
// other port here — deliberately *not* defaulted to an internal fallback
// anymore (2026-08-12 port extraction: application/uc used to construct
// its own concrete gallery when nil, which is exactly the kind of
// implementation-detail-leaking-into-application-code the
// interface/repository split was meant to remove — this package must
// not know inmemory.New() exists). The composition root
// (cmd/livesemantic/main.go) constructs the concrete
// implementation/gallery/inmemory.Gallery and passes it in — callers
// that want the reference gallery *shared* across multiple UseCase
// instances (TODO.md § H1 "Multi-flux", internal/application/session:
// every session should see the same named references, not one gallery
// each) construct one such Repository and pass the same instance to
// every NewUseCase call.
func NewUseCase(ctx context.Context, logger logger.Logger, localInput streamer.InputStream, browserInput streamer.InputStream, streamingOutput streamer.OutputStream, notifier notifier.AlertSender, objectDetector inference.ObjectDetector, semanticEncoder inference.SemanticEncoder, trackerFactory tracking.TrackerFactory, galleryRepo galleryport.Repository) (UseCases, error) {

	if ctx == nil {
		return nil, domain.ErrNilContext
	}

	if logger == nil {
		return nil, domain.ErrNilLogger
	}

	if localInput == nil {
		return nil, domain.ErrNilStreamingProcessor
	}
	if streamingOutput == nil {
		return nil, domain.ErrNilStreamingProcessor
	}

	if notifier == nil {
		return nil, domain.ErrNilNotifier
	}

	if objectDetector == nil {
		return nil, domain.ErrNilObjectDetector
	}

	if semanticEncoder == nil {
		return nil, domain.ErrNilSemanticEncoder
	}

	if trackerFactory == nil {
		return nil, domain.ErrNilTrackerFactory
	}

	if galleryRepo == nil {
		return nil, domain.ErrNilGalleryRepo
	}

	return &UseCase{
		logger:          logger,
		localInput:      localInput,
		browserInput:    browserInput,
		streamingOutput: streamingOutput,
		notifier:        notifier,
		objectDetector:  objectDetector,
		semanticEncoder: semanticEncoder,
		trackerFactory:  trackerFactory,
		gallery:         galleryRepo,
	}, nil
}
