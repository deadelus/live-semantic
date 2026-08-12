package uc

import (
	"context"
	"fmt"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/drawer"
	"live-semantic/internal/infrastructure/streamer"
)

// boxDescription formats a track's label the same way regardless of
// output mode (composited into pixels by drawer.BoxDrawer, or sent as
// streamer.BoxData.Label to a boxAware client) — tb.Score (non-zero only
// for a semantic-term match) is the CLIP cosine similarity that actually
// decided this match, shown instead of tb.Box.Confidence (YOLO's own
// detection confidence, unrelated to CLIP and typically much higher-
// looking, e.g. ~85% regardless of whether the semantic condition is
// really a close call at ~0.25-0.28). An exact term or no-filter track
// has no CLIP score (Score == 0), so it keeps showing YOLO's confidence,
// which *is* what decided that match. Found misleading in real use
// (docs/adr/clip-backend.md § 20-21): a semantic match looked just as
// "confident" on screen as an exact one — same format (a "%") both times
// too, per the user's request, rather than mixing "0.XX" and "XX.XX%" —
// still two very different-looking numbers (~25% vs ~85%), just
// consistently formatted.
func boxDescription(tb trackedBox) string {
	percent := tb.Box.Confidence * 100
	if tb.Score != 0 {
		percent = tb.Score * 100
	}
	return fmt.Sprintf("%s (%.2f%%)", tb.FilterKey, percent)
}

// Recognize starts the continuous analysis of the video stream, as
// two decoupled loops (the async 3-loop pipeline) sharing one trackManager:
//
//   - Video loop (this goroutine, driven by streamingInput.Start): per
//     frame, advances existing tracks via the cheap per-object
//     tracking-by-detection tracker and renders — never waits on detection.
//   - Detection loop (goroutine spawned below): re-detects via YOLO
//     (reanchor) whenever a frame is available, at its own pace. Frames are
//     handed off through a buffer-1, overwrite-on-full channel — the video
//     loop never blocks trying to hand off, and the detection loop always
//     works on the most recent frame rather than queueing up stale ones.
//     No explicit rate cap: reanchor's own cost (~150-270ms measured)
//     naturally self-paces it to roughly the 2-5 FPS this was meant to run
//     at, without needing a ticker.
//
// V1 "naive resync": a reanchored frame can be a few tens of
// ms stale by the time the detection loop gets to it — accepted, the
// tracker (running continuously on the video loop) catches up on its own.
// No ring buffer / replay (V2) yet.
//
// Alerts fire once per track lifecycle event (TrackEntered/TrackMatched)
// rather than once per frame.
func (uc *UseCase) Recognize(ctx context.Context, req dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error) {
	// Tracked end-to-end (every return path, including the early-exit ones
	// below) so WaitRecognition() can block until this call has genuinely
	// finished — see its doc comment for the SIGSEGV this prevents.
	uc.activeSessions.Add(1)
	defer uc.activeSessions.Done()

	defer func() {
		// Ensure cleanup is called only after loop exits
		uc.streamingOutput.Cleanup()
	}()

	select {
	case <-ctx.Done():
		return dto.Failure[dto.RecognitionResponse]("context cancelled"), ctx.Err()
	default:
	}

	// Parse/validate the filter before touching any I/O (camera, window):
	// a bad filter (unknown label, e.g. a typo) should fail fast without
	// opening the webcam first — found while testing (2026-08-11): the
	// camera was opening for ~800ms then immediately tearing down again on
	// every rejected filter, pure waste and a confusing brief flash for
	// nothing.
	tracks, err := newTrackManager(uc, req)
	if err != nil {
		return dto.Failure[dto.RecognitionResponse]("invalid filter"), err
	}
	defer tracks.cleanup()

	// Source picks which InputStream this session reads from (browser
	// camera capture) — "local" (default, empty string
	// included) is the backend's own camera, unchanged behavior for every
	// pre-existing CLI/API caller; "browser" is frames pushed over
	// /ws/ingest. Resolved and validated before touching any I/O, same
	// fail-fast rationale as the filter parse right above.
	streamingInput := uc.localInput
	if req.Source == "browser" {
		if uc.browserInput == nil {
			return dto.Failure[dto.RecognitionResponse]("browser source requested but not available (web mode only)"), fmt.Errorf("browser input not configured")
		}
		streamingInput = uc.browserInput
	}
	uc.mu.Lock()
	uc.activeInput = streamingInput
	uc.mu.Unlock()
	defer func() {
		uc.mu.Lock()
		uc.activeInput = nil
		uc.mu.Unlock()
	}()

	streamingInput.Initialize()
	uc.streamingOutput.Initialize()

	// ctx is watched for the *entire* call, not just checked once above —
	// found 2026-08-12 while auditing ctx usage across every uc method: it
	// used to be consulted only before I/O started, then silently ignored
	// for the rest of the (potentially indefinite) blocking
	// streamingInput.Start call below, making it purely decorative once a
	// session was actually running. streamingInput.Stop() is the same
	// mechanism StopRecognition() (uc_control.go) already uses to unstick
	// this exact call — converging both paths onto it rather than inventing a second
	// one. watchDone stops the watcher when Recognize returns on
	// its own (StopRecognition()/key event/stream end), so it doesn't leak for the
	// lifetime of a ctx that's never cancelled (e.g. context.Background(),
	// what every caller passes today — a per-session
	// cancellable context is a natural follow-up once a caller actually
	// wants to tie a session's lifetime to one).
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			streamingInput.Stop()
		case <-watchDone:
		}
	}()

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

	streamingInput.Start(func(frame *entities.Frame) (*entities.Frame, error) {
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
		boxes := tracks.boxes()

		// boxAware outputs (added 2026-08-12 — streamer.
		// BoxAwareOutputStream, in practice *output.WebSocketOutput)
		// receive the frame undrawn plus box data as structured JSON —
		// the client draws its own overlays (docs/gui/mockups/ 1c/1d/1h)
		// instead of getting them pre-composited into pixels. Every
		// other output (the gocv window, CLI) keeps the original
		// server-side drawer.BoxDrawer path unchanged.
		if boxAware, ok := uc.streamingOutput.(streamer.BoxAwareOutputStream); ok {
			bounds := frame.Image.Bounds()
			w, h := float32(bounds.Dx()), float32(bounds.Dy())
			boxData := make([]streamer.BoxData, 0, len(boxes))
			for _, tb := range boxes {
				boxData = append(boxData, streamer.BoxData{
					ID:      string(drawer.BoxID(tb.FilterKey)),
					Label:   boxDescription(tb),
					TrackID: tb.TrackID,
					// Normalized to [0,1] — see BoxData's doc comment for
					// why (client doesn't need the source resolution).
					X1: tb.Box.X1 / w, Y1: tb.Box.Y1 / h,
					X2: tb.Box.X2 / w, Y2: tb.Box.Y2 / h,
				})
			}
			if err := boxAware.RenderBoxes(boxData); err != nil {
				uc.logger.Info("RenderBoxes failed", map[string]interface{}{"error": err.Error()})
			}
		} else if len(boxes) > 0 {
			// cascadeOffsets stacks boxes that share (near-)identical
			// coordinates — e.g. an exact term + a "+overlap" semantic
			// term both anchored to the same physical object — so both
			// labels stay readable instead of one hiding the other
			// (docs/adr/clip-backend.md § 18-19). Only meaningful for
			// server-side drawing — a boxAware client can offset
			// overlapping labels itself with real DOM layout instead of
			// this pixel-shifting workaround.
			offsets := cascadeOffsets(boxes)

			drawBoxes := make([]drawer.Box, 0, len(boxes))
			for i, tb := range boxes {
				// Keyed on FilterKey (the filter term that matched this
				// track), not tb.Box.Label (YOLO's own class label) — two
				// tracks matched to the same physical object share the
				// same box.Label ("person" for both) but must be drawn as
				// two visually distinct boxes, not one. See
				// trackManager.boxes' doc comment / docs/adr/clip-backend.md
				// § 18.
				id := drawer.BoxID(tb.FilterKey)
				off := offsets[i]
				drawBoxes = append(drawBoxes, drawer.Box{
					ID:          id,
					Description: boxDescription(tb),
					Color:       entities.BoxColor(id),
					Thickness:   5,
					X1:          tb.Box.X1 + off,
					Y1:          tb.Box.Y1 + off,
					X2:          tb.Box.X2 + off,
					Y2:          tb.Box.Y2 + off,
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
	streamingInput.Stop()
	uc.streamingOutput.Stop()
	close(frameChan)
	<-detectionDone

	fmt.Println("Recognition completed successfully.")
	return dto.Success(dto.RecognitionResponse{}), nil
}
