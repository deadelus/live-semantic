package uc

import (
	"context"
	"testing"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"
)

// mockInputStream delivers one frame then blocks until Stop() is called —
// enough to drive RecognitionUseCase's blocking Start() loop for exactly
// as long as a test needs it to run.
type mockInputStream struct {
	stop    chan struct{}
	stopped bool
}

func newMockInputStream() *mockInputStream { return &mockInputStream{stop: make(chan struct{})} }

var _ streamer.InputStream = (*mockInputStream)(nil)

func (m *mockInputStream) Initialize() error { return nil }
func (m *mockInputStream) Start(cb func(*entities.Frame) (*entities.Frame, error)) error {
	if _, err := cb(testFrame()); err != nil {
		return err
	}
	<-m.stop
	return nil
}
func (m *mockInputStream) Stop() {
	if !m.stopped {
		m.stopped = true
		close(m.stop)
	}
}
func (m *mockInputStream) Cleanup() {}

type mockOutputStream struct{}

var _ streamer.OutputStream = mockOutputStream{}

func (mockOutputStream) Initialize() error            { return nil }
func (mockOutputStream) Render(*entities.Frame) error { return nil }
func (mockOutputStream) HandleKeyEvent() int          { return -1 }
func (mockOutputStream) Stop()                        {}
func (mockOutputStream) Cleanup()                     {}

// TestRecognitionUseCase_ContextCancellation_StopsTheStream is the
// regression test for the 2026-08-12 fix (uc_recognition.go's watchDone
// goroutine): ctx used to be checked once before I/O started, then
// ignored for the rest of a potentially indefinite blocking call —
// cancelling it had no effect. mockInputStream here never stops on its
// own (no other caller ever closes its stop channel), so the only way
// this test passes is if ctx cancellation itself reaches
// streamingInput.Stop().
func TestRecognitionUseCase_ContextCancellation_StopsTheStream(t *testing.T) {
	input := newMockInputStream()
	u := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{}, &mockAlertSender{})
	u.localInput = input
	u.streamingOutput = mockOutputStream{}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = u.Recognize(ctx, dto.RecognitionRequest{})
	}()

	// Give RecognitionUseCase a moment to actually reach the blocking
	// streamingInput.Start() call before cancelling — otherwise this
	// would trivially pass via the early ctx.Done() check that already
	// existed before this fix, defeating the point of the test.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RecognitionUseCase did not return after ctx cancellation — watchDone goroutine regressed")
	}
}

// TestRecognitionUseCase_NeverCancelled_ReturnsNormallyOnStop confirms
// the fix above didn't break the pre-existing Stop()-driven shutdown
// path (uc_control.go) — ctx never cancelled here (context.Background()),
// only input.Stop() called directly, same as every real caller does
// today.
func TestRecognitionUseCase_NeverCancelled_ReturnsNormallyOnStop(t *testing.T) {
	input := newMockInputStream()
	u := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{}, &mockAlertSender{})
	u.localInput = input
	u.streamingOutput = mockOutputStream{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = u.Recognize(context.Background(), dto.RecognitionRequest{})
	}()

	time.Sleep(20 * time.Millisecond)
	u.StopRecognition()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RecognitionUseCase did not return after StopRecognition()")
	}
}
