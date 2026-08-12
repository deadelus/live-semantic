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
func captureLoop(capture *gocv.VideoCapture, mirror bool, isRunning func() bool, onFrame func(*entities.Frame) (*entities.Frame, error)) {
	imgMat := gocv.NewMat()
	defer imgMat.Close()

	var frameNumber uint64
	for isRunning() {
		ok := capture.Read(&imgMat)
		if !ok || imgMat.Empty() {
			return
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
		if err != nil || outFrame == nil {
			return
		}
	}
}
