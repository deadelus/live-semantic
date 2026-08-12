// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"image"
	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/notifier"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"
	"sync"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// UseCases defines the interface for the use cases in the application.
type UseCases interface {
	RecognitionUseCase(ctx context.Context, req dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error)
	// Stop halts the currently running RecognitionUseCase call, if any —
	// see uc_control.go. H1 minimal scope (TODO.md § H1): a single shared
	// UseCase serves one recognition session at a time (streamingInput/
	// streamingOutput are fields set once at construction, not per-call);
	// the caller (transport/adapters/api) is responsible for not starting
	// a second session concurrently. Revisit once multi-flux gives each
	// session its own UseCase/InputStream.
	Stop()
	// Wait blocks until any in-flight RecognitionUseCase call has fully
	// returned. Found necessary 2026-08-11 (real crash, not hypothetical —
	// docs/adr/clip-backend.md § 15, TODO.md § H1): main.go's graceful
	// shutdown handler used to call objectDetector.Cleanup()/
	// semanticEncoder.Cleanup() unconditionally on SIGTERM, destroying the
	// shared ONNX sessions while RecognitionUseCase's detection goroutine
	// could still be mid-EncodeImage/AnalyzeFrame on those exact sessions
	// (SIGSEGV inside the CGo call). Callers should call Stop() first (to
	// actually unstick a session that's still blocked reading frames),
	// then Wait(), before tearing down objectDetector/semanticEncoder.
	Wait()

	// AddGalleryReference encodes crop (a JPEG-decoded image, typically a
	// user-selected box from the live view) and stores it under name in
	// the reference gallery (TODO.md § D "reconnaissance par référence
	// image" / § H1, docs/adr/clip-backend.md § 24 — a gallery entry's
	// name becomes usable directly as a filter term, matched by
	// image↔image similarity instead of text↔image). See
	// ReferenceGallery.Add for the validation rules (name can't be empty
	// or collide with a COCO class/an existing entry).
	AddGalleryReference(name string, crop image.Image) error
	// RemoveGalleryReference deletes a gallery entry — see
	// ReferenceGallery.Remove (idempotent, not an error if absent).
	RemoveGalleryReference(name string)
	// RenameGalleryReference — see ReferenceGallery.Rename.
	RenameGalleryReference(oldName, newName string) error
	// SetGalleryReferenceEnabled — see ReferenceGallery.SetEnabled.
	SetGalleryReferenceEnabled(name string, enabled bool) error
	// ListGalleryReferences — see ReferenceGallery.List.
	ListGalleryReferences() []GalleryEntryInfo
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

	// activeSessions tracks in-flight RecognitionUseCase calls — see
	// Wait's doc comment. Incremented/decremented in RecognitionUseCase
	// itself (uc_recognition.go), not here.
	activeSessions sync.WaitGroup

	// mu guards activeInput — set by RecognitionUseCase to whichever of
	// localInput/browserInput this call picked, read by Stop() (a
	// different goroutine, e.g. the REST /recognition/stop handler) so it
	// stops the *actual* running input rather than always localInput.
	mu          sync.Mutex
	activeInput streamer.InputStream

	// gallery is the {name, embedding} store for reference-image filter
	// terms (TODO.md § D/§ H1, docs/adr/clip-backend.md § 24) — see
	// uc_gallery.go for the UseCases methods around it. Created
	// internally (not a NewUseCase parameter): pure application-layer
	// state, no external adapter to inject.
	gallery *ReferenceGallery
}

// NewUseCase initializes your use cases with all the necessary
// dependencies. browserInput may be nil (CLI/interactive mode — see the
// UseCase struct's doc comment). gallery may also be nil — a fresh empty
// one is created internally — but callers that want the reference
// gallery *shared* across multiple UseCase instances (TODO.md § H1
// "Multi-flux", internal/application/session: every session should see
// the same named references, not one gallery each) must pass the same
// *ReferenceGallery explicitly. Every other parameter is required.
func NewUseCase(ctx context.Context, logger logger.Logger, localInput streamer.InputStream, browserInput streamer.InputStream, streamingOutput streamer.OutputStream, notifier notifier.AlertSender, objectDetector inference.ObjectDetector, semanticEncoder inference.SemanticEncoder, trackerFactory tracking.TrackerFactory, gallery *ReferenceGallery) (UseCases, error) {

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

	if gallery == nil {
		gallery = NewReferenceGallery()
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
		gallery:         gallery,
	}, nil
}
