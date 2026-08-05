// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"live-semantic/internal/domain"
	"live-semantic/internal/application/dto"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/notifier"
	"live-semantic/internal/infrastructure/streamer"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// UseCases defines the interface for the use cases in the application.
type UseCases interface {
	RecognitionUseCase(ctx context.Context, req dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error)
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger          logger.Logger
	streamingInput  streamer.InputStream
	streamingOutput streamer.OutputStream
	notifier        notifier.Notifier
	ai              inference.ObjectDetector
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(ctx context.Context, logger logger.Logger, streamingInput streamer.InputStream, streamingOutput streamer.OutputStream, notifier notifier.Notifier, ai inference.ObjectDetector) (UseCases, error) {

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

	if ai == nil {
		return nil, domain.ErrNilAI
	}

	return &UseCase{
		logger:          logger,
		streamingInput:  streamingInput,
		streamingOutput: streamingOutput,
		notifier:        notifier,
		ai:              ai,
	}, nil
}
