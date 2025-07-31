package uc

import (
	"context"
	"io"
	"live-semantic/src/domain/dto"
	"live-semantic/src/domain/model"
)

// Execute starts the continuous analysis of the video stream.
// It reads frames, gets embeddings, compares them to filters, and sends alerts.
func (uc *UseCase) ObjectRecognitionUseCase(ctx context.Context, req dto.ObjectRecognitionRequest) (dto.Result[dto.ObjectRecognitionResponse], error) {
	select {
	case <-ctx.Done():
		return dto.Failure[dto.ObjectRecognitionResponse]("context cancelled"), ctx.Err()
	default:
	}

	// 1. Continuously read frames from the VideoSource.
	for {
		frame, err := uc.videoSource.NextFrame()
		if err != nil {
			if err == io.EOF {
				uc.logger.Info("End of video stream.")
				break
			}
			return dto.Failure[dto.ObjectRecognitionResponse](err.Error()), err
		}

		// 2. For each frame, get the image embedding from the AIProvider.

		result, err := uc.ai.AnalyzeFrame(frame)
		if err != nil {
			return dto.Failure[dto.ObjectRecognitionResponse](err.Error()), err
		}

		// 4. If they match, create a models.MatchEvent.
		if result.BoundingBoxes != nil {
			for _, box := range *result.BoundingBoxes {
				if box.Label == req.Filter && box.Confidence >= req.SimilarityThreshold {
					// 4. If it matches, send a notification.
					uc.notifier.Notify(model.Message{
						MatchedFilter: req.Filter,
						Confidence:    box.Confidence,
					})
				}
			}
		}
	}

	return dto.Success(dto.ObjectRecognitionResponse{}), nil
}
