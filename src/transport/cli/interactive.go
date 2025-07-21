// Package cli provides the CLI interface for the Live Semantic application.
package cli

import (
	"live-semantic/src/domain/uc"
	"live-semantic/src/transport"

	"github.com/deadelus/go-clean-app/src/logger"
)

type SurveyController struct {
	handler *transport.BaseHandler
	logger  logger.Logger
}

func NewSurveyController(useCases uc.UseCases, logger logger.Logger) *SurveyController {
	return &SurveyController{
		handler: transport.NewBaseHandler(useCases, logger),
		logger:  logger,
	}
}
