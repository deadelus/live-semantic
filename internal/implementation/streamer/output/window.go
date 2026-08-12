package output

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"live-semantic/internal/domain/entities"

	"gocv.io/x/gocv"
)

// WindowOutput implements streamer.OutputStream by displaying frames in a
// native gocv/OpenCV window (HighGUI) — the CLI/desktop path, as opposed
// to WebSocketOutput's browser-facing one.
type WindowOutput struct {
	window *gocv.Window
}

// NewWindowOutput constructs a WindowOutput with no window opened yet —
// see Initialize.
func NewWindowOutput() *WindowOutput {
	return &WindowOutput{
		window: nil,
	}
}

// Initialize implements streamer.OutputStream.Initialize — opens the
// display window.
func (wo *WindowOutput) Initialize() error {
	wo.window = gocv.NewWindow("AI Agent")
	return nil
}

// Render implements streamer.OutputStream.Render — JPEG round-trips the
// frame through gocv.IMDecode so it can be shown via HighGUI's IMShow.
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

// HandleKeyEvent implements streamer.OutputStream.HandleKeyEvent — polls
// for a keypress with a 1ms wait, non-blocking in practice for a video loop.
func (wo *WindowOutput) HandleKeyEvent() int {
	key := wo.window.WaitKey(1)
	return key
}

// Stop implements streamer.OutputStream.Stop — no-op, WindowOutput has
// nothing to halt independently of Cleanup.
func (wo *WindowOutput) Stop() {
}

// Cleanup implements streamer.OutputStream.Cleanup — closes the window.
func (wo *WindowOutput) Cleanup() {
	if wo.window != nil {
		wo.window.Close()
		wo.window = nil
	}
	fmt.Println("WindowOutput resources cleaned up successfully.")
}
