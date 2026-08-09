package uc

import (
	"context"
	"fmt"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/drawer"
)

// RecognitionUseCase starts the continuous analysis of the video stream, as
// two decoupled loops (TODO.md § C) sharing one trackManager:
//
//   - Video loop (this goroutine, driven by streamingInput.Start): per
//     frame, advances existing tracks via the cheap per-object tracker
//     (TODO.md § B) and renders — never waits on detection.
//   - Detection loop (goroutine spawned below): re-detects via YOLO
//     (reanchor) whenever a frame is available, at its own pace. Frames are
//     handed off through a buffer-1, overwrite-on-full channel — the video
//     loop never blocks trying to hand off, and the detection loop always
//     works on the most recent frame rather than queueing up stale ones.
//     No explicit rate cap: reanchor's own cost (~150-270ms measured)
//     naturally self-paces it to roughly the 2-5 FPS this was meant to run
//     at, without needing a ticker.
//
// V1 "naive resync" (TODO.md § C): a reanchored frame can be a few tens of
// ms stale by the time the detection loop gets to it — accepted, the
// tracker (running continuously on the video loop) catches up on its own.
// No ring buffer / replay (V2) yet.
//
// Alerts fire once per track lifecycle event (TrackEntered/TrackMatched,
// TODO.md § D) rather than once per frame.
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

	frameChan := make(chan *entities.Frame, 1)
	detectionDone := make(chan struct{})

	go func() {
		defer close(detectionDone)
		for frame := range frameChan {
			start := time.Now()
			err := tracks.reanchor(frame, req)
			duration := time.Since(start)
			if err != nil {
				uc.logger.Info("Detection loop error", map[string]interface{}{"error": err.Error()})
				continue
			}
			uc.logger.Info("Frame timing", map[string]interface{}{
				"kind":               "reanchor",
				"detect_or_track_ms": duration.Milliseconds(),
				"active_tracks":      tracks.count(),
			})
		}
	}()

	frameCount := 0
	lastFrameAt := time.Now()

	uc.streamingInput.Start(func(frame *entities.Frame) (*entities.Frame, error) {
		frameStart := time.Now()
		// Proxy for real achieved FPS: includes camera capture wait, which
		// happens in the input stream's loop before this callback runs and
		// so isn't otherwise visible from in here.
		interFrameGap := frameStart.Sub(lastFrameAt)
		lastFrameAt = frameStart
		frameCount++

		advanceStart := time.Now()
		tracks.advance(frame, req)
		advanceDuration := time.Since(advanceStart)

		// Non-blocking hand-off to the detection loop — drop whatever's
		// pending (not yet picked up) and replace it, never wait.
		select {
		case frameChan <- frame:
		default:
			select {
			case <-frameChan:
			default:
			}
			select {
			case frameChan <- frame:
			default:
			}
		}

		renderStart := time.Now()
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
		renderDuration := time.Since(renderStart)

		uc.logger.Info("Frame timing", map[string]interface{}{
			"frame":              frameCount,
			"kind":               "advance",
			"inter_frame_gap_ms": interFrameGap.Milliseconds(),
			"detect_or_track_ms": advanceDuration.Milliseconds(),
			"draw_and_render_ms": renderDuration.Milliseconds(),
			"active_tracks":      tracks.count(),
		})

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
	close(frameChan)
	<-detectionDone

	fmt.Println("Recognition completed successfully.")
	return dto.Success(dto.RecognitionResponse{}), nil
}
