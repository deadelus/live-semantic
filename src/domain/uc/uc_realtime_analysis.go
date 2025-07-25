package uc

import (
	"context"
	"io"
	"live-semantic/src/domain/dto"
	"live-semantic/src/domain/models"
)

const (
	SimilarityThreshold = 0.8
)

// Execute starts the continuous analysis of the video stream.
// It reads frames, gets embeddings, compares them to filters, and sends alerts.
func (uc *UseCase) RealtimeAnalysisUseCase(ctx context.Context, req dto.RealtimeAnalysisRequest) (dto.Result[dto.RealtimeAnalysisResponse], error) {
	select {
	case <-ctx.Done():
		return dto.Failure[dto.RealtimeAnalysisResponse]("context cancelled"), ctx.Err()
	default:
	}

	filter := models.Filter{Text: req.Filter}
	if err := uc.aiProvider.EncodeText(&filter); err != nil {
		return dto.Failure[dto.RealtimeAnalysisResponse](err.Error()), err
	}

	// 1. Continuously read frames from the VideoSource.
	for {
		frame, err := uc.videoSource.NextFrame()
		if err != nil {
			if err == io.EOF {
				uc.logger.Info("End of video stream.")
				break
			}
			return dto.Failure[dto.RealtimeAnalysisResponse](err.Error()), err
		}

		// 2. For each frame, get the image embedding from the AIProvider.
		imageEmbedding, err := uc.aiProvider.EncodeImage(frame)
		if err != nil {
			uc.logger.Error("Failed to get image embedding", "err", err)
			continue
		}

		// 3. Compare the image embedding to the text filter's embedding.
		confidence := uc.utils.CosineSimilarity(imageEmbedding, filter.Embedding)

		// 4. If they match, create a models.MatchEvent.
		if confidence > req.SimilarityThreshold {
			matchEvent := models.MatchEvent{
				MatchedFrame:  *frame,
				MatchedFilter: filter,
				Confidence:    confidence,
			}
			// 5. Send the MatchEvent to the Alerter.
			if err := uc.alerter.Alert(matchEvent); err != nil {
				uc.logger.Error("Failed to send alert", "err", err)
				continue
			}
		}
	}

	response := dto.RealtimeAnalysisResponse{}
	return dto.Success(response), nil
}
