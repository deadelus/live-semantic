package infrastructure

import "live-semantic/src/domain/models"

// VideoSource is the interface for any video input, live or from a file.
type VideoSource interface {
	// NextFrame reads the next available frame from the source.
	// It should return an error if the stream ends or a read error occurs.
	NextFrame() (*models.Frame, error)
	// Close releases any resources used by the video source.
	Close() error
}
