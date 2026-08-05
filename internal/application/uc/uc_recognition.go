package uc

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/drawer"
	"time"
)

// Execute starts the continuous analysis of the video stream.
// It reads frames, gets embeddings, compares them to filters, and sends alerts.
func (uc *UseCase) RecognitionUseCase(ctx context.Context, req dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error) {
	defer func() {
		// Ensure cleanup is called only after loop exits
		uc.streamingOutput.Cleanup()
	}()

	select {
	case <-ctx.Done():
		return dto.Failure[dto.RecognitionResponse]("context cancelled"), ctx.Err()
	default:
	}

	uc.streamingInput.Initialize()
	uc.streamingOutput.Initialize()

	uc.streamingInput.Start(func(frame *entities.Frame) (*entities.Frame, error) {
		/*
			go func() {
				// Process each bounding box
				for _, box := range result.BoundingBoxes {
					hit := box.Label == req.Filter && box.Confidence >= req.SimilarityThreshold
					if hit {
						uc.logger.Info("Match found", "label", box.Label, "confidence", box.Confidence)
						uc.notifier.Notify(entities.Message{
							MatchedFilter: req.Filter,
							Confidence:    box.Confidence,
						})
						break // Only process the first match
					}
				}
			}()
		*/

		println("Processing frame number :", frame.FrameNumber)

		// Analyze the frame using the AI model
		result, err := uc.ai.AnalyzeFrame(frame)

		if err != nil {
			uc.logger.Info("AI analysis error", "error", err)
			return nil, err
		}

		uc.logger.Info("Frame processed", "frame_number", frame.FrameNumber)

		if result.BoundingBoxes != nil {
			// If bounding boxes are detected, draw them on the frame
			var boxes []drawer.Box
			for _, boundingBox := range result.BoundingBoxes {
				ID := drawer.BoxID(boundingBox.Label)
				boxes = append(boxes, drawer.Box{
					ID:          ID,
					Description: fmt.Sprintf("%s (%.2f%%)", boundingBox.Label, boundingBox.Confidence*100),
					Color:       entities.BoxColor(ID),
					Thickness:   5,
					X1:          boundingBox.X1,
					Y1:          boundingBox.Y1,
					X2:          boundingBox.X2,
					Y2:          boundingBox.Y2,
				})
			}

			boxDrawer := drawer.NewBoxDrawer(frame.Image, boxes)

			if boxDrawer == nil {
				uc.logger.Info("BoxDrawer creation failed, returning original frame")
				return frame, nil
			}

			boxDrawer.Draw()

			outFrame := &entities.Frame{
				Image:       boxDrawer.ToImage(),
				Timestamp:   time.Now(),
				FrameNumber: frame.FrameNumber,
			}

			// Output the frame
			uc.streamingOutput.Render(outFrame)

			// Handle key events (e.g., for stopping the stream)
			key := uc.streamingOutput.HandleKeyEvent()
			if key == 27 { // 'q' or Escape to quit
				uc.logger.Info("Stopping stream due to key event")
				return nil, nil
			}

			return outFrame, nil
		}

		// If no bounding boxes, return the original frame
		uc.logger.Info("No bounding boxes detected", "frame_number", frame.FrameNumber)
		return frame, nil
	})

	// Stop all processing before cleanup
	uc.streamingInput.Stop()
	uc.streamingOutput.Stop()

	fmt.Println("Recognition completed successfully.")
	return dto.Success(dto.RecognitionResponse{}), nil
}
