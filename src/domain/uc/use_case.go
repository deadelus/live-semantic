// Package domain contains the business logic and use cases for the application.
package uc

import (
	"context"
	"live-semantic/src/domain/dto"
	"live-semantic/src/domain/errors"
	"live-semantic/src/infrastructure"
	"live-semantic/src/internal/utils"

	"github.com/deadelus/go-clean-app/src/logger"
)

// UseCases defines the interface for the use cases in the application.
type UseCases interface {
	RealtimeAnalysisUseCase(ctx context.Context, req dto.RealtimeAnalysisRequest) (dto.Result[dto.RealtimeAnalysisResponse], error)
}

// useCase implements the UseCases interface.
type UseCase struct {
	logger      logger.Logger
	videoSource infrastructure.VideoSource
	aiProvider  infrastructure.AIProvider
	alerter     infrastructure.Alerter
	utils       utils.Utils
}

// NewUseCase initializes your use cases with all the necessary dependencies
func NewUseCase(ctx context.Context, logger logger.Logger, videoSource infrastructure.VideoSource, aiProvider infrastructure.AIProvider, alerter infrastructure.Alerter, utils utils.Utils) (UseCases, error) {

	if ctx == nil {
		return nil, errors.ErrNilContext
	}

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

	if utils == nil {
		return nil, errors.ErrNilUtils
	}

	return &UseCase{
		logger:      logger,
		videoSource: videoSource,
		aiProvider:  aiProvider,
		alerter:     alerter,
		utils:       utils,
	}, nil
}
