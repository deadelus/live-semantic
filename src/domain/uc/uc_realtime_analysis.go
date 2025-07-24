package uc

import (
	"live-semantic/src/domain/models"
)

// Execute starts the continuous analysis of the video stream.
// It reads frames, gets embeddings, compares them to filters, and sends alerts.
func (uc *UseCase) RealtimeAnalysisUseCase(filter models.Filter) error {
	// Implementation to be added in the next steps.
	// 1. Continuously read frames from the VideoSource.
	// 2. For each frame, get the image embedding from the AIProvider.
	// 3. Compare the image embedding to the text filter's embedding.
	// 4. If they match, create a models.MatchEvent.
	// 5. Send the MatchEvent to the Alerter.
	return nil
}
