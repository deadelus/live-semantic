package videofile

import (
	"live-semantic/src/domain/model"
)

// VideoSource implements the VideoSource interface for video sources.
type VideoFile struct{}

// NewVideoSource creates a new VideoSource instance with the given device ID.
func NewVideoSource() (*VideoFile, error) {
	return &VideoFile{}, nil
}

// NextFrame implements the VideoSource interface for VideoFile.
func (s *VideoFile) NextFrame() (*model.Frame, error) {
	// Simulate reading a frame from a video file
	return nil, nil
}

// Close implements the VideoSource interface for VideoFile.
func (s *VideoFile) Close() error {
	return nil
}
