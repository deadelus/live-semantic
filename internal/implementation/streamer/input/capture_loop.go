package input

import (
	"fmt"
	"live-semantic/internal/domain/entities"
	"time"

	"gocv.io/x/gocv"
)

// captureLoop drives frame reads from an already-open, non-nil
// *gocv.VideoCapture until either the read itself fails/reaches EOF, or
// onFrame signals stop (returns an error or a nil frame) — matching the
// streamer.InputStream.Start contract. isRunning is polled once per
// iteration so an external Stop() call (flips the caller's own bool) is
// noticed promptly.
//
// Shared by CameraInput and FileInput: the only real
// differences between a USB webcam and a local file/RTSP/HTTP stream are
// how the capture gets opened (device index vs URI) and whether the feed
// should be mirrored — everything past that point is the same OpenCV
// read/convert/callback loop, not worth duplicating twice.
//
// Return value — added 2026-08-13, found while diagnosing a real bug
// (two sessions pointed at the same physical webcam: one grabs it
// exclusively, the other's Read() fails forever and the session used to
// end in silent "success" with zero frames, no error anywhere, not even
// a log line). treatEndAsError distinguishes the two adapters' very
// different normal-termination semantics: a live camera that stops
// producing frames *while still supposed to be running* (isRunning()
// still true) is abnormal — device busy/unplugged/permission lost — and
// should surface as an error (CameraInput passes true). A file/RTSP
// stream reaching Read()==false while still "running" is completely
// normal (end of file, or an upstream FFmpeg-level disconnect already
// documented as a known gap, file.go's own doc comment) — FileInput
// passes false, keeping today's "ends cleanly" behavior. An
// intentional Stop() (isRunning() flips false first) is never an error
// either way. onFrame's own error is now returned too, instead of
// silently discarded — a detection-loop failure deserves the same
// visibility as a capture failure.
func captureLoop(capture *gocv.VideoCapture, mirror bool, treatEndAsError bool, isRunning func() bool, onFrame func(*entities.Frame) (*entities.Frame, error)) error {
	imgMat := gocv.NewMat()
	defer imgMat.Close()

	var frameNumber uint64
	for isRunning() {
		ok := capture.Read(&imgMat)
		if !ok || imgMat.Empty() {
			if treatEndAsError && isRunning() {
				return fmt.Errorf("capture stopped producing frames unexpectedly (device busy, disconnected, or permission lost?)")
			}
			return nil
		}

		if mirror {
			// Only a webcam facing the user needs this (mirror-effect UX
			// fix) — a recorded file or an IP camera must never be flipped.
			if err := gocv.Flip(imgMat, &imgMat, 1); err != nil {
				fmt.Println("Warning: could not mirror frame:", err)
			}
		}

		img, err := imgMat.ToImage()
		if err != nil {
			continue
		}

		frameNumber++
		frame := &entities.Frame{
			Image:       img,
			Timestamp:   time.Now(),
			FrameNumber: frameNumber,
		}

		outFrame, err := onFrame(frame)
		if err != nil {
			return err
		}
		if outFrame == nil {
			return nil
		}
	}
	return nil
}
