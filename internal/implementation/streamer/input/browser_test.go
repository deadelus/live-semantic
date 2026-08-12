package input

import (
	"errors"
	"image"
	"testing"
	"time"

	"live-semantic/internal/domain/entities"
)

func testFrame() *entities.Frame {
	return &entities.Frame{Image: image.NewRGBA(image.Rect(0, 0, 10, 10)), Timestamp: time.Now()}
}

func TestBrowserInput_Initialize_NoOp(t *testing.T) {
	bi := NewBrowserInput()
	if err := bi.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v, want nil (nothing to open)", err)
	}
}

func TestBrowserInput_PushFrame_ThenStart_DeliversIt(t *testing.T) {
	bi := NewBrowserInput()
	pushed := testFrame()
	bi.PushFrame(pushed) // before Start — must still be delivered

	received := make(chan *entities.Frame, 1)
	go func() {
		_ = bi.Start(func(f *entities.Frame) (*entities.Frame, error) {
			received <- f
			bi.Stop()
			return f, nil
		})
	}()

	select {
	case got := <-received:
		if got != pushed {
			t.Fatalf("callback received a different frame than pushed")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the pushed frame to reach the callback")
	}
}

func TestBrowserInput_PushFrame_OverwritesWhenFull(t *testing.T) {
	bi := NewBrowserInput()
	stale := testFrame()
	fresh := testFrame()
	bi.PushFrame(stale)
	bi.PushFrame(fresh) // channel is buffer-1: this must replace stale, not block/drop silently

	received := make(chan *entities.Frame, 1)
	go func() {
		_ = bi.Start(func(f *entities.Frame) (*entities.Frame, error) {
			received <- f
			bi.Stop()
			return f, nil
		})
	}()

	select {
	case got := <-received:
		if got != fresh {
			t.Fatalf("callback received the stale frame, want the overwriting fresh one")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestBrowserInput_Stop_UnblocksStart(t *testing.T) {
	bi := NewBrowserInput()
	done := make(chan error, 1)
	go func() {
		done <- bi.Start(func(f *entities.Frame) (*entities.Frame, error) { return f, nil })
	}()

	// Give Start a moment to actually enter its select loop before Stop.
	time.Sleep(10 * time.Millisecond)
	bi.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() error = %v, want nil on a clean Stop", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop() did not unblock Start()")
	}
}

func TestBrowserInput_CallbackErrorPropagates(t *testing.T) {
	bi := NewBrowserInput()
	bi.PushFrame(testFrame())
	boom := errors.New("boom")

	err := bi.Start(func(*entities.Frame) (*entities.Frame, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("Start() error = %v, want %v propagated from the callback", err, boom)
	}
}

func TestBrowserInput_CleanupThenStart_WorksAgain(t *testing.T) {
	// A single *BrowserInput instance is reused across sessions (wired
	// once in main.go, like CameraInput) — Start's deferred Cleanup()
	// recreates the stop channel so a second Start() after a Stop()
	// doesn't immediately return (a closed channel can't be reopened,
	// this is the regression that guard prevents).
	bi := NewBrowserInput()
	done := make(chan error, 1)
	go func() {
		done <- bi.Start(func(f *entities.Frame) (*entities.Frame, error) { return f, nil })
	}()
	time.Sleep(10 * time.Millisecond)
	bi.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first Start() never returned")
	}

	// Second session.
	bi.PushFrame(testFrame())
	received := make(chan *entities.Frame, 1)
	go func() {
		_ = bi.Start(func(f *entities.Frame) (*entities.Frame, error) {
			received <- f
			bi.Stop()
			return f, nil
		})
	}()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("second Start() after Cleanup never delivered a frame")
	}
}
