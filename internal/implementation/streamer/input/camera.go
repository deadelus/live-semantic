// Package input provides the implementation of the camera input stream.
package input

import (
	"fmt"
	"live-semantic/internal/domain/entities"
	"time"

	"gocv.io/x/gocv"
)

type CameraInput struct {
	camera  *gocv.VideoCapture
	running bool
}

func NewCameraInput() *CameraInput {
	return &CameraInput{
		camera:  nil,
		running: false,
	}
}

// Initialize implements the Input.Initialize for CameraInput.
func (ci *CameraInput) Initialize() error {
	var err error
	ci.camera, err = gocv.OpenVideoCapture(0)
	if err != nil {
		return err
	}
	return nil
}

// Start implements the Input.Start for CameraInput.
func (ci *CameraInput) Start(frameActionCallback func(*entities.Frame) (*entities.Frame, error)) error {
	defer ci.Cleanup()

	if ci.camera == nil {
		return fmt.Errorf("camera not initialized")
	}
	ci.running = true
	imgMat := gocv.NewMat()
	defer imgMat.Close()
	for ci.running {
		ok := ci.camera.Read(&imgMat)
		if !ok || imgMat.Empty() {
			break
		}
		// Mirror horizontally: raw webcam feed isn't mirrored, which reads
		// as "backwards" to the person facing the camera (TODO.md bug UX).
		if err := gocv.Flip(imgMat, &imgMat, 1); err != nil {
			fmt.Println("Warning: could not mirror frame:", err)
		}
		image, err := imgMat.ToImage()
		if err != nil {
			continue
		}
		frame := &entities.Frame{
			Image:       image,
			Timestamp:   time.Now(),
			FrameNumber: 0, // Should increment if needed
		}
		outFrame, err := frameActionCallback(frame)
		if err != nil || outFrame == nil {
			ci.running = false
		}
	}
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
