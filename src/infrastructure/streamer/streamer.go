// Package camera defines the interface for camera processing.
package streamer

import "live-semantic/src/domain/model"

// StreamingProcessor defines the interface for streaming processing.
// It includes methods for initializing the stream, starting the frame processing,
// stopping the processing, and cleaning up resources.
type InputStream interface {
	// Initialize sets up the input stream (camera, file, cctv, etc.).
	Initialize() error
	// Start begins reading frames from the input stream.
	Start(frameActionCallback func(*model.Frame) (*model.Frame, error)) error
	// Stop halts the input stream and releases any resources.
	Stop()
	// Cleanup performs any necessary cleanup before the processor is discarded.
	Cleanup()
}

type OutputStream interface {
	// Initialize sets up the output stream (websocket, stream server, window, etc.).
	Initialize() error
	// Render outputs a frame to the destination.
	Render(frame *model.Frame) error
	// HandleKeyEvent processes output-specific key events (optional).
	HandleKeyEvent() int
	// Stop halts the output stream and releases any resources.
	Stop()
	// Cleanup performs any necessary cleanup before the processor is discarded.
	Cleanup()
}
