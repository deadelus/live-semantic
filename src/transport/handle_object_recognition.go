package transport

import "live-semantic/src/domain/dto"

// HandleObjectRecognition handles an Object Recognition request
// It takes a TransportRequest with dto.ObjectRecognitionRequest data and returns a TransportResponse with dto.ObjectRecognitionResponse
func (h *BaseHandler) HandleObjectRecognition(req TransportRequest[dto.ObjectRecognitionRequest]) TransportResponse[dto.ObjectRecognitionResponse] {
	// Log the request details
	// This is where you would typically log the request for debugging or monitoring purposes
	h.logger.Info("Handling Object Recognition request", map[string]interface{}{
		"filter":               req.Data.Filter,
		"similarity_threshold": req.Data.SimilarityThreshold,
	})

	// Call the use case with the request data
	result, err := h.useCases.ObjectRecognitionUseCase(req.Context, req.Data)

	// Handle errors and convert to TransportResponse
	if err != nil {
		return TransportResponse[dto.ObjectRecognitionResponse]{
			Success: false,
			Error:   err.Error(),
			Source:  req.Source,
		}
	}

	// Check if the result is successful and return the appropriate TransportResponse
	if result.Success {
		return TransportResponse[dto.ObjectRecognitionResponse]{
			Success: true,
			Data:    result.Data,
			Source:  req.Source,
		}
	}

	// If the result is not successful, return an error respons
	return TransportResponse[dto.ObjectRecognitionResponse]{
		Success: false,
		Error:   result.Error,
		Source:  req.Source,
	}
}
