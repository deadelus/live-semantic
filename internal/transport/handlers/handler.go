package handlers

import (
	"live-semantic/internal/application/uc"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// BaseHandler handler générique réutilisable — depends on uc.Recognition
// (not the wider uc.UseCases) since HandleRecognitionUseCase is the only
// use case this handler wraps (interface segregation, 2026-08-12 — see
// uc.Recognition's doc comment). A uc.UseCases value satisfies
// uc.Recognition structurally, so every existing caller (CLI, API) keeps
// compiling unchanged.
type BaseHandler struct {
	useCases uc.Recognition
	logger   logger.Logger
}

// NewBaseHandler crée un handler de base
func NewBaseHandler(useCases uc.Recognition, logger logger.Logger) *BaseHandler {
	return &BaseHandler{
		useCases: useCases,
		logger:   logger,
	}
}
