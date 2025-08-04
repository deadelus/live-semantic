package uc

import (
	"context"
	"live-semantic/src/domain/dto"
	"live-semantic/src/domain/model"
	"live-semantic/src/internal/drawer"
	"time"
)

// Execute starts the continuous analysis of the video stream.
// It reads frames, gets embeddings, compares them to filters, and sends alerts.
func (uc *UseCase) RecognitionUseCase(ctx context.Context, req dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error) {
	select {
	case <-ctx.Done():
		return dto.Failure[dto.RecognitionResponse]("context cancelled"), ctx.Err()
	default:
	}

	uc.streamingProcessor.Initialize()

	uc.streamingProcessor.Start(func(frame *model.Frame) (*model.Frame, error) {
		println("Processing frame:", frame.FrameNumber)

		// Analyze the frame using the AI model
		result, err := uc.ai.AnalyzeFrame(frame)

		if err != nil {
			uc.logger.Info("AI analysis error", "error", err)
			return nil, err
		}

		uc.logger.Info("Frame processed", "frame_number", frame.FrameNumber)

		if result.BoundingBoxes != nil {
			go func() {
				// Process each bounding box
				for _, box := range result.BoundingBoxes {
					hit := box.Label == req.Filter && box.Confidence >= req.SimilarityThreshold
					if hit {
						uc.logger.Info("Match found", "label", box.Label, "confidence", box.Confidence)
						uc.notifier.Notify(model.Message{
							MatchedFilter: req.Filter,
							Confidence:    box.Confidence,
						})
						break // Only process the first match
					}
				}
			}()

			boxDrawer := drawer.NewBoxDrawer(frame.Image, result.BoundingBoxes)

			if boxDrawer == nil {
				uc.logger.Info("BoxDrawer creation failed, returning original frame")
				return frame, nil
			}

			boxDrawer.Draw()

			return &model.Frame{
				Image:       boxDrawer.ToImage(),
				Timestamp:   time.Now(),
				FrameNumber: frame.FrameNumber,
			}, nil
		}

		// If no bounding boxes, return the original frame
		uc.logger.Info("No bounding boxes detected", "frame_number", frame.FrameNumber)
		return frame, nil
	})

	return dto.Success(dto.RecognitionResponse{}), nil
}
