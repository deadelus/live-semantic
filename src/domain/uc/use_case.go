// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"live-semantic/src/domain"
	"live-semantic/src/domain/dto"
	"live-semantic/src/infrastructure/ai"
	"live-semantic/src/infrastructure/notifier"
	"live-semantic/src/infrastructure/streamer"

	"github.com/deadelus/go-clean-app/src/logger"
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
	ai              ai.AI
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(ctx context.Context, logger logger.Logger, streamingInput streamer.InputStream, streamingOutput streamer.OutputStream, notifier notifier.Notifier, ai ai.AI) (UseCases, error) {

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
