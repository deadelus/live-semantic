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
// (a multi-source/GUI backend need) — previously hardcoded to 0 in this constructor itself.
func NewCameraInput(device int) *CameraInput {
	return &CameraInput{device: device}
}

// Initialize implements streamer.InputStream.Initialize for CameraInput.
// Checks IsOpened() explicitly, not just the error return — gocv/OpenCV's
// VideoCapture backends commonly return a non-nil capture with a nil
// error even when the device didn't actually open (e.g. already held
// exclusively by another process/browser tab, a real scenario: a "local"
// session and a "browser" session both pointed at the same physical
// webcam). Without this check, that case used to only surface later, and
// silently, as zero frames ever being read (see captureLoop's doc
// comment) — this turns it into a clear error at Initialize time
// instead.
func (ci *CameraInput) Initialize() error {
	var err error
	ci.camera, err = gocv.OpenVideoCapture(ci.device)
	if err != nil {
		return fmt.Errorf("open camera device %d: %w", ci.device, err)
	}
	if ci.camera == nil || !ci.camera.IsOpened() {
		return fmt.Errorf("camera device %d did not open — already in use by another process or browser tab?", ci.device)
	}
	return nil
}

// Start implements streamer.InputStream.Start for CameraInput. Frames are
// mirrored horizontally (raw webcam feed reads as "backwards" to the
// person facing the camera) — see captureLoop's doc comment for why
// FileInput doesn't do this. treatEndAsError=true: a live camera going
// silent while still running is abnormal (see captureLoop's doc comment).
func (ci *CameraInput) Start(frameActionCallback func(*entities.Frame) (*entities.Frame, error)) error {
	defer ci.Cleanup()

	if ci.camera == nil {
		return fmt.Errorf("camera not initialized")
	}
	ci.running = true
	return captureLoop(ci.camera, true, true, func() bool { return ci.running }, frameActionCallback)
}

// Stop implements streamer.InputStream.Stop for CameraInput.
func (ci *CameraInput) Stop() {
	ci.running = false
}

// Cleanup implements streamer.InputStream.Cleanup for CameraInput.
func (ci *CameraInput) Cleanup() {
	if ci.camera != nil {
		ci.camera.Close()
	}
	fmt.Println("CameraInput resources cleaned up successfully.")
}
