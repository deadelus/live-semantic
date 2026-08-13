package handlers

import (
	"live-semantic/internal/application/dto"
	"live-semantic/internal/transport/envelopes"
)

// HandleRecognitionUseCase adapts a TransportRequest to a uc.Recognition.Recognize
// call and folds its (dto.Result, error) return into the uniform
// TransportResponse shape — the translation layer so every transport
// adapter deals in envelopes, never uc.Recognize's own signature directly.
func (h *BaseHandler) HandleRecognitionUseCase(req envelopes.TransportRequest[dto.RecognitionRequest]) envelopes.TransportResponse[dto.RecognitionResponse] {
	h.logger.Info("Handling Recognition request", map[string]interface{}{
		"filter": req.Data.Filter,
	})

	result, err := h.useCases.Recognize(req.Context, req.Data)
	if err != nil {
		return envelopes.TransportResponse[dto.RecognitionResponse]{
			Success: false,
			Error:   err.Error(),
			Source:  req.Source,
		}
	}

	if result.Success {
		return envelopes.TransportResponse[dto.RecognitionResponse]{
			Success: true,
			Data:    result.Data,
			Source:  req.Source,
		}
	}

	return envelopes.TransportResponse[dto.RecognitionResponse]{
		Success: false,
		Error:   result.Error,
		Source:  req.Source,
	}
}
