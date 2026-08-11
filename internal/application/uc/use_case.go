// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
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
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger          logger.Logger
	streamingInput  streamer.InputStream
	streamingOutput streamer.OutputStream
	notifier        notifier.AlertSender
	objectDetector  inference.ObjectDetector
	semanticEncoder inference.SemanticEncoder
	trackerFactory  tracking.TrackerFactory

	// activeSessions tracks in-flight RecognitionUseCase calls — see
	// Wait's doc comment. Incremented/decremented in RecognitionUseCase
	// itself (uc_recognition.go), not here.
	activeSessions sync.WaitGroup
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(ctx context.Context, logger logger.Logger, streamingInput streamer.InputStream, streamingOutput streamer.OutputStream, notifier notifier.AlertSender, objectDetector inference.ObjectDetector, semanticEncoder inference.SemanticEncoder, trackerFactory tracking.TrackerFactory) (UseCases, error) {

	if ctx == nil {
		return nil, domain.ErrNilContext
	}

	if logger == nil {
		return nil, domain.ErrNilLogger
	}

	if streamingInput == nil {
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

	return &UseCase{
		logger:          logger,
		streamingInput:  streamingInput,
		streamingOutput: streamingOutput,
		notifier:        notifier,
		objectDetector:  objectDetector,
		semanticEncoder: semanticEncoder,
		trackerFactory:  trackerFactory,
	}, nil
}
