// Package input provides the implementation of the camera input stream.
package input

import (
	"fmt"
	"live-semantic/src/domain/model"
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
func (ci *CameraInput) Start(frameActionCallback func(*model.Frame) (*model.Frame, error)) error {
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
		image, err := imgMat.ToImage()
		if err != nil {
			continue
		}
		frame := &model.Frame{
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
