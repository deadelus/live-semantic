// Package sourcehandler defines the interface for input sources.
package sourcehandler

import "live-semantic/src/domain/model"

// VideoHandler is the interface for any video input, live or from a file.
type VideoHandler interface {
	// Start initializes the video source.
	Start() error
	// NextFrame reads the next available frame from the source.
	NextFrame() (*model.Frame, error)
	// Close releases any resources used by the video source.
	Close() error
}
