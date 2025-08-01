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
	ObjectRecognitionUseCase(ctx context.Context, req dto.ObjectRecognitionRequest) (dto.Result[dto.ObjectRecognitionResponse], error)
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger             logger.Logger
	streamingProcessor streamer.StreamingProcessor
	notifier           notifier.Notifier
	ai                 ai.AI
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(ctx context.Context, logger logger.Logger, streamingProcessor streamer.StreamingProcessor, notifier notifier.Notifier, ai ai.AI) (UseCases, error) {

	if ctx == nil {
		return nil, domain.ErrNilContext
	}

	if logger == nil {
		return nil, domain.ErrNilLogger
	}

	if streamingProcessor == nil {
		return nil, domain.ErrNilStreamingProcessor
	}

	if notifier == nil {
		return nil, domain.ErrNilNotifier
	}

	if ai == nil {
		return nil, domain.ErrNilAI
	}

	return &UseCase{
		logger:             logger,
		streamingProcessor: streamingProcessor,
		notifier:           notifier,
		ai:                 ai,
	}, nil
}
