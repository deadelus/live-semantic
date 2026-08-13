// Package cli provides the CLI interface for the Live Semantic application.
package cli

import (
	"live-semantic/internal/application/uc"
	"live-semantic/internal/transport/handlers"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// SurveyController drives the interactive CLI (AlecAivazis/survey prompts)
// — the only transport adapter actually wired to business logic today
// (see menu.go's Run for the main loop, cli_recognition.go/cli_settings.go
// for the individual flows).
type SurveyController struct {
	handler *handlers.BaseHandler
	logger  logger.Logger
}

// NewSurveyController constructs a SurveyController backed by useCases,
// via the same handlers.BaseHandler every other transport adapter uses.
func NewSurveyController(useCases uc.UseCases, logger logger.Logger) *SurveyController {
	return &SurveyController{
		handler: handlers.NewBaseHandler(useCases, logger),
		logger:  logger,
	}
}
