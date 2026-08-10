package input

import "testing"

func TestNewCameraInput_StoresDevice(t *testing.T) {
	ci := NewCameraInput(2)
	if ci.device != 2 {
		t.Fatalf("device = %d, want 2", ci.device)
	}
}

func TestCameraInput_Cleanup_WithoutInitializeIsSafe(t *testing.T) {
	ci := NewCameraInput(0)
	// Must not panic even though Initialize (which needs real hardware,
	// unavailable in a test environment) was never called.
	ci.Cleanup()
	ci.Cleanup()
}

func TestCameraInput_Start_WithoutInitializeErrors(t *testing.T) {
	ci := NewCameraInput(0)
	if err := ci.Start(nil); err == nil {
		t.Fatal("Start() error = nil, want an error when Initialize was never called")
	}
}
