// Package handlers translates envelopes.TransportRequest/TransportResponse
// to and from application/uc calls — the shared logic every transport
// adapter (CLI, API, WebSocket) delegates to instead of calling uc
// directly.
package handlers

import (
	"live-semantic/internal/application/uc"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// BaseHandler is the reusable handler shared by every transport adapter
// — depends on uc.Recognition (not the wider uc.UseCases) since
// HandleRecognitionUseCase is the only use case this handler wraps
// (interface segregation, 2026-08-12 — see uc.Recognition's doc comment).
// A uc.UseCases value satisfies uc.Recognition structurally, so every
// existing caller (CLI, API) keeps compiling unchanged.
type BaseHandler struct {
	useCases uc.Recognition
	logger   logger.Logger
}

// NewBaseHandler constructs a BaseHandler.
func NewBaseHandler(useCases uc.Recognition, logger logger.Logger) *BaseHandler {
	return &BaseHandler{
		useCases: useCases,
		logger:   logger,
	}
}
