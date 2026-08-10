// Package input provides concrete streamer.InputStream adapters.
package input

import (
	"fmt"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"

	"gocv.io/x/gocv"
)

var _ streamer.InputStream = (*CameraInput)(nil)

// CameraInput reads frames from a local USB/built-in webcam via OpenCV.
type CameraInput struct {
	device  int
	camera  *gocv.VideoCapture
	running bool
}

// NewCameraInput creates a CameraInput bound to the given device index
// (0 is usually the default/built-in camera on most systems). Configurable
// per TODO.md § H1 — previously hardcoded to 0 in this constructor itself.
func NewCameraInput(device int) *CameraInput {
	return &CameraInput{device: device}
}

// Initialize implements the Input.Initialize for CameraInput.
func (ci *CameraInput) Initialize() error {
	var err error
	ci.camera, err = gocv.OpenVideoCapture(ci.device)
	if err != nil {
		return err
	}
	return nil
}

// Start implements the Input.Start for CameraInput. Frames are mirrored
// horizontally (raw webcam feed reads as "backwards" to the person facing
// the camera) — see captureLoop's doc comment for why FileInput doesn't do
// this.
func (ci *CameraInput) Start(frameActionCallback func(*entities.Frame) (*entities.Frame, error)) error {
	defer ci.Cleanup()

	if ci.camera == nil {
		return fmt.Errorf("camera not initialized")
	}
	ci.running = true
	captureLoop(ci.camera, true, func() bool { return ci.running }, frameActionCallback)
	return nil
}

// Stop implements the Input.Stop for CameraInput.
func (ci *CameraInput) Stop() {
	ci.running = false
}

// Cleanup implements the Input.Cleanup for CameraInput.
func (ci *CameraInput) Cleanup() {
	if ci.camera != nil {
		ci.camera.Close()
	}
	fmt.Println("CameraInput resources cleaned up successfully.")
}
