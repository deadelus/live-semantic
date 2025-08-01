// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"live-semantic/src/domain"
	"live-semantic/src/domain/dto"
	"live-semantic/src/infrastructure/ai"
	displayhandler "live-semantic/src/infrastructure/displayHandler"
	"live-semantic/src/infrastructure/notifier"
	sourcehandler "live-semantic/src/infrastructure/sourceHandler"

	"github.com/deadelus/go-clean-app/src/logger"
)

// UseCases defines the interface for the use cases in the application.
type UseCases interface {
	ObjectRecognitionUseCase(ctx context.Context, req dto.ObjectRecognitionRequest) (dto.Result[dto.ObjectRecognitionResponse], error)
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger             logger.Logger
	videoSourceHandler sourcehandler.VideoHandler
	displayhandler     displayhandler.DisplayHandler
	notifier           notifier.Notifier
	ai                 ai.AI
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(ctx context.Context, logger logger.Logger, videoSourceHandler sourcehandler.VideoHandler, displayhandler displayhandler.DisplayHandler, notifier notifier.Notifier, ai ai.AI) (UseCases, error) {

	if ctx == nil {
		return nil, domain.ErrNilContext
	}

	if logger == nil {
		return nil, domain.ErrNilLogger
	}

	if videoSourceHandler == nil {
		return nil, domain.ErrNilVideoSource
	}

	if displayhandler == nil {
		return nil, domain.ErrNilDisplayHandler
	}

	if notifier == nil {
		return nil, domain.ErrNilNotifier
	}

	if ai == nil {
		return nil, domain.ErrNilAI
	}

	return &UseCase{
		logger:             logger,
		videoSourceHandler: videoSourceHandler,
		displayhandler:     displayhandler,
		notifier:           notifier,
		ai:                 ai,
	}, nil
}
