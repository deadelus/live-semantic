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

	defer uc.videoSourceHandler.Close()

	err := uc.videoSourceHandler.Start()
	if err != nil {
		return dto.Failure[dto.ObjectRecognitionResponse](err.Error()), err
	}

	// 1. Continuously read frames from the VideoSource.
	for {
		frame, err := uc.videoSourceHandler.NextFrame()
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
			defer uc.displayhandler.Close()

			for _, box := range *result.BoundingBoxes {

				// Check if the bounding box matches the filter and confidence threshold
				hit := box.Label == req.Filter && box.Confidence >= req.SimilarityThreshold

				// 5. Display the frame with bounding boxes.
				var img []byte

				if hit {
					img = result.Frame.ImageData
				} else {
					img = frame.ImageData
				}

				if uc.displayhandler.IsActive() {
					uc.logger.Warn("Display handler is active, displaying frame")
					uc.displayhandler.ShowFrame(img)
				} else {
					uc.logger.Warn("Display handler is not active, skipping frame display")
				}

				// Wait (30 FPS)
				if key := uc.displayhandler.WaitKey(33); key == 'q' || key == 27 {
					break
				}

				// 6. If it matches, send a notification.
				if hit {
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
