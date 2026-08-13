package input

import (
	"live-semantic/internal/domain/entities"
	"testing"
)

// sampleVideo is a small real asset already shipped in the repo
// (originally added for cmd/tracking-drift-bench, tracking-by-detection
// drift testing) — reused here rather than requiring a network stream or
// hardware camera, neither of
// which are available in a test environment. Path is relative to this
// package's directory, which is `go test`'s working directory.
const sampleVideo = "../../../../assets/videos/car.mp4"

func TestNewFileInput_StoresURI(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	if fi.uri != sampleVideo {
		t.Fatalf("uri = %q, want %q", fi.uri, sampleVideo)
	}
}

func TestFileInput_Initialize_InvalidPathErrors(t *testing.T) {
	fi := NewFileInput("this/path/does/not/exist.mp4")
	if err := fi.Initialize(); err == nil {
		t.Fatal("Initialize() error = nil, want an error for a nonexistent file")
	}
}

func TestFileInput_Cleanup_WithoutInitializeIsSafe(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	// Must not panic even though Initialize was never called.
	fi.Cleanup()
	fi.Cleanup()
}

func TestFileInput_Start_WithoutInitializeErrors(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	if err := fi.Start(func(f *entities.Frame) (*entities.Frame, error) { return f, nil }); err == nil {
		t.Fatal("Start() error = nil, want an error when Initialize was never called")
	}
}

func TestFileInput_Start_ReadsRealFrames(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	if err := fi.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v (sample asset missing? %s)", err, sampleVideo)
	}

	var frameCount int
	var lastFrame *entities.Frame
	err := fi.Start(func(f *entities.Frame) (*entities.Frame, error) {
		frameCount++
		lastFrame = f
		return f, nil
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if frameCount == 0 {
		t.Fatal("read 0 frames from a real video file")
	}
	if lastFrame == nil || lastFrame.Image == nil {
		t.Fatal("callback received a frame with a nil Image")
	}
	if lastFrame.FrameNumber != uint64(frameCount) {
		t.Fatalf("last FrameNumber = %d, want %d (should match total frames read)", lastFrame.FrameNumber, frameCount)
	}
}

func TestFileInput_Stop_HaltsLoopEarly(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	if err := fi.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	var frameCount int
	err := fi.Start(func(f *entities.Frame) (*entities.Frame, error) {
		frameCount++
		fi.Stop() // stop after the very first frame
		return f, nil
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// captureLoop checks isRunning() before each read, so Stop() called
	// during frame 1's callback must prevent frame 2 from ever being read.
	if frameCount != 1 {
		t.Fatalf("frameCount = %d, want 1 (Stop() during callback should halt the loop immediately after)", frameCount)
	}
}

func TestFileInput_Start_CallbackErrorHaltsLoop(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	if err := fi.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	var frameCount int
	err := fi.Start(func(f *entities.Frame) (*entities.Frame, error) {
		frameCount++
		return nil, nil // signal stop without an error, same as a nil frame
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if frameCount != 1 {
		t.Fatalf("frameCount = %d, want 1 (a nil returned frame should halt the loop)", frameCount)
	}
}

func TestFileInput_Cleanup_AfterStartIsSafe(t *testing.T) {
	fi := NewFileInput(sampleVideo)
	if err := fi.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	_ = fi.Start(func(f *entities.Frame) (*entities.Frame, error) { return nil, nil })

	// Start's own deferred Cleanup already ran — calling it again directly
	// must still not panic (mirrors CameraInput's Cleanup-after-Cleanup
	// idempotency expectation from tracker_test.go's style).
	fi.Cleanup()
}
