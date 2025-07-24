// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"live-semantic/src/domain/dto"
	"live-semantic/src/domain/errors"
	"live-semantic/src/domain/models"
	"live-semantic/src/infrastructure"

	"github.com/deadelus/go-clean-app/src/logger"
)

// UseCases defines the interface for the use cases in the application.
type UseCases interface {
	CreateTask(context.Context, dto.TaskRequest) (dto.Result[dto.TaskResponse], error)
	RealtimeAnalysisUseCase(filter models.Filter) error
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger      logger.Logger
	videoSource infrastructure.VideoSource
	aiProvider  infrastructure.AIProvider
	alerter     infrastructure.Alerter
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(logger logger.Logger, videoSource infrastructure.VideoSource, aiProvider infrastructure.AIProvider, alerter infrastructure.Alerter) (UseCases, error) {

	if logger == nil {
		return nil, errors.ErrNilLogger
	}

	if videoSource == nil {
		return nil, errors.ErrNilVideoSource
	}

	if aiProvider == nil {
		return nil, errors.ErrNilAIProvider
	}

	if alerter == nil {
		return nil, errors.ErrNilAlerter
	}

	return &UseCase{
		logger:      logger,
		videoSource: videoSource,
		aiProvider:  aiProvider,
		alerter:     alerter,
	}, nil
}
