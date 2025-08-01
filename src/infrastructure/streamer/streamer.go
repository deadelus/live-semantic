// Package camera defines the interface for camera processing.
package streamer

import "live-semantic/src/domain/model"

// StreamingProcessor defines the interface for streaming processing.
// It includes methods for initializing the stream, starting the frame processing,
// stopping the processing, and cleaning up resources.
type StreamingProcessor interface {
	// Initialize sets up the streaming processor, such as opening a video stream.
	Initialize() error
	// Start begins processing frames from the stream.
	Start(frameActionCallback func(*model.Frame) (*model.Frame, error)) error
	// Stop halts the processing of frames and releases any resources.
	Stop()
	// Cleanup performs any necessary cleanup before the processor is discarded.
	Cleanup()
}
