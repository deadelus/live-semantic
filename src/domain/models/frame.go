package models

import "time"

// Frame represents a single frame from a video source.
type Frame struct {
	// Timestamp is the time the frame was captured.
	Timestamp time.Time
	// ImageData contains the raw image data (e.g., in JPEG or PNG format).
	ImageData []byte
	// FrameNumber is the sequential number of the frame in the stream.
	FrameNumber int64
}
