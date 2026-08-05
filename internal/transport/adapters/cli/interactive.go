// Package cli provides the CLI interface for the Live Semantic application.
package cli

import (
	"live-semantic/internal/application/uc"
	"live-semantic/internal/transport/handlers"

	"github.com/deadelus/go-clean-app/v2/logger"
)

type SurveyController struct {
	handler *handlers.BaseHandler
	logger  logger.Logger
}

func NewSurveyController(useCases uc.UseCases, logger logger.Logger) *SurveyController {
	return &SurveyController{
		handler: handlers.NewBaseHandler(useCases, logger),
		logger:  logger,
	}
}
