package transport

import "live-semantic/src/domain/dto"

// HandleRecognition handles a Recognition request
// It takes a TransportRequest with dto.RecognitionRequest data and returns a TransportResponse with dto.RecognitionResponse
func (h *BaseHandler) HandleRecognitionUseCase(req TransportRequest[dto.RecognitionRequest]) TransportResponse[dto.RecognitionResponse] {
	// Log the request details
	// This is where you would typically log the request for debugging or monitoring purposes
	h.logger.Info("Handling Recognition request", map[string]interface{}{
		"filter":               req.Data.Filter,
		"similarity_threshold": req.Data.SimilarityThreshold,
	})

	// Call the use case with the request data
	result, err := h.useCases.RecognitionUseCase(req.Context, req.Data)

	// Handle errors and convert to TransportResponse
	if err != nil {
		return TransportResponse[dto.RecognitionResponse]{
			Success: false,
			Error:   err.Error(),
			Source:  req.Source,
		}
	}

	// Check if the result is successful and return the appropriate TransportResponse
	if result.Success {
		return TransportResponse[dto.RecognitionResponse]{
			Success: true,
			Data:    result.Data,
			Source:  req.Source,
		}
	}

	// If the result is not successful, return an error response
	return TransportResponse[dto.RecognitionResponse]{
		Success: false,
		Error:   result.Error,
		Source:  req.Source,
	}
}
