package handler

import (
	"live-semantic/internal/application/dto"
	"live-semantic/internal/transport/envelope"
)

// HandleRecognition handles a Recognition request
// It takes a envelope.TransportRequest with dto.RecognitionRequest data and returns a envelope.TransportResponse with dto.RecognitionResponse
func (h *BaseHandler) HandleRecognitionUseCase(req envelope.TransportRequest[dto.RecognitionRequest]) envelope.TransportResponse[dto.RecognitionResponse] {
	// Log the request details
	// This is where you would typically log the request for debugging or monitoring purposes
	h.logger.Info("Handling Recognition request", map[string]interface{}{
		"filter":               req.Data.Filter,
		"similarity_threshold": req.Data.SimilarityThreshold,
	})

	// Call the use case with the request data
	result, err := h.useCases.RecognitionUseCase(req.Context, req.Data)

	// Handle errors and convert to envelope.TransportResponse
	if err != nil {
		return envelope.TransportResponse[dto.RecognitionResponse]{
			Success: false,
			Error:   err.Error(),
			Source:  req.Source,
		}
	}

	// Check if the result is successful and return the appropriate envelope.TransportResponse
	if result.Success {
		return envelope.TransportResponse[dto.RecognitionResponse]{
			Success: true,
			Data:    result.Data,
			Source:  req.Source,
		}
	}

	// If the result is not successful, return an error response
	return envelope.TransportResponse[dto.RecognitionResponse]{
		Success: false,
		Error:   result.Error,
		Source:  req.Source,
	}
}
