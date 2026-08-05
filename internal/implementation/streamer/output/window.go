// Package output provides the implementation of the output stream for displaying frames in a window.
package output

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"live-semantic/internal/domain/entities"

	"gocv.io/x/gocv"
)

type WindowOutput struct {
	window *gocv.Window
}

func NewWindowOutput() *WindowOutput {
	return &WindowOutput{
		window: nil,
	}
}

// Initialize implements the Output.Initialize for WindowOutput.
func (wo *WindowOutput) Initialize() error {
	wo.window = gocv.NewWindow("AI Agent")
	return nil
}

// Render implements the Output.Render for WindowOutput.
func (wo *WindowOutput) Render(frame *entities.Frame) error {
	if wo.window == nil {
		fmt.Println("window closed")
		return nil
	}

	fmt.Println("Rendering frame in window:", frame.FrameNumber)

	buf := new(bytes.Buffer)
	err := jpeg.Encode(buf, frame.Image, &jpeg.Options{Quality: 90})
	if err != nil {
		return err
	}
	img := buf.Bytes()
	processedImgMat, err := gocv.IMDecode(img, gocv.IMReadColor)
	if err != nil || processedImgMat.Empty() {
		fmt.Printf("Erreur lors du décodage JPEG pour affichage: %v\n", err)
		return err
	}
	wo.window.IMShow(processedImgMat)
	processedImgMat.Close()
	return nil
}

// HandleKeyEvent implements the Output.HandleKeyEvent for WindowOutput.
func (wo *WindowOutput) HandleKeyEvent() int {
	key := wo.window.WaitKey(1)
	return key
}

// Stop implements the Output.Stop for WindowOutput.
func (wo *WindowOutput) Stop() {
	// No-op for window output
}

// Cleanup implements the Output.Cleanup for WindowOutput.
func (wo *WindowOutput) Cleanup() {
	if wo.window != nil {
		wo.window.Close()
		wo.window = nil
	}
	fmt.Println("WindowOutput resources cleaned up successfully.")
}
