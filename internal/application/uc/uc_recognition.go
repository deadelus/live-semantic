package uc

import (
	"context"
	"fmt"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/drawer"
)

// RecognitionUseCase starts the continuous analysis of the video stream.
// It periodically re-detects objects (YOLO, every reanchorInterval frames)
// and tracks them in between via a per-object tracker (TODO.md § B),
// instead of running detection on every single frame. Alerts fire once per
// track lifecycle event (TrackEntered/TrackMatched, TODO.md § D) rather
// than once per frame.
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

	tracks := newTrackManager(uc)
	defer tracks.cleanup()

	frameCount := 0

	uc.streamingInput.Start(func(frame *entities.Frame) (*entities.Frame, error) {
		frameCount++
		isReanchorFrame := frameCount == 1 || frameCount%reanchorInterval == 0

		var err error
		if isReanchorFrame {
			err = tracks.reanchor(frame, req)
		} else {
			tracks.advance(frame, req)
		}
		if err != nil {
			uc.logger.Info("AI analysis error", map[string]interface{}{"error": err.Error()})
			return nil, err
		}

		outImage := frame.Image
		if boxes := tracks.boxes(); len(boxes) > 0 {
			drawBoxes := make([]drawer.Box, 0, len(boxes))
			for _, box := range boxes {
				id := drawer.BoxID(box.Label)
				drawBoxes = append(drawBoxes, drawer.Box{
					ID:          id,
					Description: fmt.Sprintf("%s (%.2f%%)", box.Label, box.Confidence*100),
					Color:       entities.BoxColor(id),
					Thickness:   5,
					X1:          box.X1,
					Y1:          box.Y1,
					X2:          box.X2,
					Y2:          box.Y2,
				})
			}

			if boxDrawer := drawer.NewBoxDrawer(frame.Image, drawBoxes); boxDrawer != nil {
				boxDrawer.Draw()
				outImage = boxDrawer.ToImage()
			} else {
				uc.logger.Info("BoxDrawer creation failed, rendering original frame")
			}
		}

		outFrame := &entities.Frame{
			Image:       outImage,
			Timestamp:   time.Now(),
			FrameNumber: frame.FrameNumber,
		}

		// Output the frame — always, even with zero active tracks, so the
		// window keeps refreshing and key events (below) keep being polled.
		uc.streamingOutput.Render(outFrame)

		// Handle key events (e.g., for stopping the stream)
		if key := uc.streamingOutput.HandleKeyEvent(); key == 27 { // 'q' or Escape to quit
			uc.logger.Info("Stopping stream due to key event")
			return nil, nil
		}

		return outFrame, nil
	})

	// Stop all processing before cleanup
	uc.streamingInput.Stop()
	uc.streamingOutput.Stop()

	fmt.Println("Recognition completed successfully.")
	return dto.Success(dto.RecognitionResponse{}), nil
}
