package session

import (
	"context"
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/application/uc"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// --- minimal test doubles, satisfying the port interfaces this package
// depends on (not uc's own unexported test mocks — different package,
// can't reuse them). CreateSession wires up a *real* uc.UseCase
// internally (not an injectable mock), so exercising StartRecognition
// needs working fakes all the way down, same as uc package's own tests.

type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Close()               {}

var _ logger.Logger = noopLogger{}

func testFrame() *entities.Frame {
	return &entities.Frame{Image: image.NewRGBA(image.Rect(0, 0, 10, 10))}
}

// mockInput delivers one synthetic frame then blocks until Stop() closes
// its stop channel — mirrors implementation/streamer/input.BrowserInput's
// shape closely enough to exercise the real RecognitionUseCase loop this
// package wires up internally.
type mockInput struct {
	mu      sync.Mutex
	stop    chan struct{}
	stopped bool
}

func newMockInput() *mockInput { return &mockInput{stop: make(chan struct{})} }

var _ streamer.InputStream = (*mockInput)(nil)

func (m *mockInput) Initialize() error { return nil }

func (m *mockInput) Start(cb func(*entities.Frame) (*entities.Frame, error)) error {
	if _, err := cb(testFrame()); err != nil {
		return err
	}
	<-m.stop
	return nil
}

func (m *mockInput) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stop)
	}
}

func (m *mockInput) Cleanup() {}

type mockOutput struct{}

var _ streamer.OutputStream = mockOutput{}

func (mockOutput) Initialize() error            { return nil }
func (mockOutput) Render(*entities.Frame) error { return nil }
func (mockOutput) HandleKeyEvent() int          { return -1 }
func (mockOutput) Stop()                        {}
func (mockOutput) Cleanup()                     {}

type mockDetector struct{}

var _ inference.ObjectDetector = mockDetector{}

func (mockDetector) AnalyzeFrame(frame *entities.Frame) (*inference.DetectionResult, error) {
	return &inference.DetectionResult{Frame: frame}, nil
}
func (mockDetector) Cleanup() {}

type mockEncoder struct{}

func (mockEncoder) EncodeImage(*entities.Frame) (entities.Embedding, error) {
	return entities.Embedding{1, 0}, nil
}
func (mockEncoder) EncodeText(string) (entities.Embedding, error) {
	return entities.Embedding{1, 0}, nil
}
func (mockEncoder) Cleanup() {}

type mockNotifier struct{}

func (mockNotifier) Notify(entities.Message) error { return nil }
func (mockNotifier) Cleanup()                      {}

func mockTrackerFactory() (tracking.ObjectTracker, error) { return mockTracker{}, nil }

type mockTracker struct{}

func (mockTracker) Init(*entities.Frame, entities.BoundingBox) error { return nil }
func (mockTracker) Update(*entities.Frame) (entities.BoundingBox, bool) {
	return entities.BoundingBox{}, true
}
func (mockTracker) Cleanup() {}

// newTestManager wires a Manager whose InputFactory always returns the
// given *mockInput (single-session tests) or a fresh one each call
// (multi-session tests use newTestManagerFreshInputs instead).
func newTestManager(mi *mockInput) *Manager {
	return NewManager(
		func(Source) (streamer.InputStream, error) { return mi, nil },
		func() streamer.OutputStream { return mockOutput{} },
		noopLogger{},
		mockNotifier{},
		mockDetector{},
		mockEncoder{},
		mockTrackerFactory,
		nil, // gallery — nil is fine, uc.NewUseCase creates one per call in that case; TestGallerySharedAcrossSessions passes an explicit one instead
	)
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

// --- tests ---------------------------------------------------------------

func TestCreateSession_AssignsSequentialIDs(t *testing.T) {
	m := newTestManager(newMockInput())

	info1, err := m.CreateSession(context.Background(), Source{Kind: "local", Device: 0})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	info2, err := m.CreateSession(context.Background(), Source{Kind: "local", Device: 1})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if info1.ID != "session-1" || info2.ID != "session-2" {
		t.Fatalf("IDs = %q, %q, want session-1, session-2", info1.ID, info2.ID)
	}
}

func TestCreateSession_InputFactoryErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	m := NewManager(
		func(Source) (streamer.InputStream, error) { return nil, boom },
		func() streamer.OutputStream { return mockOutput{} },
		noopLogger{}, mockNotifier{}, mockDetector{}, mockEncoder{}, mockTrackerFactory, nil,
	)

	if _, err := m.CreateSession(context.Background(), Source{Kind: "local"}); !errors.Is(err, boom) {
		t.Fatalf("CreateSession() error = %v, want %v", err, boom)
	}
}

func TestStartRecognition_UnknownSessionErrors(t *testing.T) {
	m := newTestManager(newMockInput())
	if err := m.StartRecognition(context.Background(), "nope", dto.RecognitionRequest{Filter: "person"}); err == nil {
		t.Fatal("StartRecognition() on an unknown session should error")
	}
}

func TestStartRecognition_RunsAndReflectsInInfo(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, err := m.CreateSession(context.Background(), Source{Kind: "local"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := m.StartRecognition(context.Background(), info.ID, dto.RecognitionRequest{Filter: "person"}); err != nil {
		t.Fatalf("StartRecognition() error = %v", err)
	}

	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && got.Running
	})

	mi.Stop()
	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && !got.Running
	})
}

func TestStartRecognition_AlreadyRunningErrors(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	if err := m.StartRecognition(context.Background(), info.ID, dto.RecognitionRequest{Filter: "person"}); err != nil {
		t.Fatalf("first StartRecognition() error = %v", err)
	}
	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && got.Running
	})

	if err := m.StartRecognition(context.Background(), info.ID, dto.RecognitionRequest{Filter: "person"}); err == nil {
		t.Fatal("second concurrent StartRecognition() should error")
	}

	mi.Stop()
}

func TestStartRecognition_InvalidFilterSurfacesError(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	// "person%+%unicorn": relation attachment must be a COCO class —
	// parseFilterSpec rejects it, RecognitionUseCase returns a failure
	// Result immediately (never even calls Start on mi).
	if err := m.StartRecognition(context.Background(), info.ID, dto.RecognitionRequest{Filter: "person%+%unicorn"}); err != nil {
		t.Fatalf("StartRecognition() error = %v, want nil (the failure surfaces async via Info, not this call)", err)
	}

	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && got.Error != ""
	})
	got, _ := m.Info(info.ID)
	if got.Running {
		t.Fatal("session should not be marked running after an immediate filter-validation failure")
	}
}

func TestStopRecognition_UnknownSessionErrors(t *testing.T) {
	m := newTestManager(newMockInput())
	if err := m.StopRecognition("nope"); err == nil {
		t.Fatal("StopRecognition() on an unknown session should error")
	}
}

func TestStopRecognition_UnblocksStartedSession(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	if err := m.StartRecognition(context.Background(), info.ID, dto.RecognitionRequest{Filter: "person"}); err != nil {
		t.Fatalf("StartRecognition() error = %v", err)
	}
	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && got.Running
	})

	if err := m.StopRecognition(info.ID); err != nil {
		t.Fatalf("StopRecognition() error = %v", err)
	}
	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && !got.Running
	})
}

func TestRemoveSession_UnknownSessionErrors(t *testing.T) {
	m := newTestManager(newMockInput())
	if err := m.RemoveSession("nope"); err == nil {
		t.Fatal("RemoveSession() on an unknown session should error")
	}
}

func TestRemoveSession_ForgetsSession(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	if err := m.RemoveSession(info.ID); err != nil {
		t.Fatalf("RemoveSession() error = %v", err)
	}
	if _, ok := m.Info(info.ID); ok {
		t.Fatal("session still present after RemoveSession()")
	}
}

func TestRemoveSession_StopsARunningSessionFirst(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	if err := m.StartRecognition(context.Background(), info.ID, dto.RecognitionRequest{Filter: "person"}); err != nil {
		t.Fatalf("StartRecognition() error = %v", err)
	}
	waitUntil(t, time.Second, func() bool {
		got, ok := m.Info(info.ID)
		return ok && got.Running
	})

	done := make(chan error, 1)
	go func() { done <- m.RemoveSession(info.ID) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RemoveSession() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RemoveSession() on a running session hung — should Stop() + Wait() internally")
	}
}

func TestList_ReturnsAllSessions(t *testing.T) {
	m := newTestManager(newMockInput())
	id1, _ := m.CreateSession(context.Background(), Source{Kind: "local", Device: 0})
	id2, _ := m.CreateSession(context.Background(), Source{Kind: "local", Device: 1})

	list := m.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d entries, want 2", len(list))
	}
	got := map[string]bool{}
	for _, info := range list {
		got[info.ID] = true
	}
	if !got[id1.ID] || !got[id2.ID] {
		t.Fatalf("List() = %+v, missing one of the created sessions", list)
	}
}

func TestOutput_Input_ReturnTheActualInstances(t *testing.T) {
	mi := newMockInput()
	m := newTestManager(mi)
	info, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	gotInput, ok := m.Input(info.ID)
	if !ok || gotInput != streamer.InputStream(mi) {
		t.Fatalf("Input() = %v, %v, want the mockInput instance CreateSession used", gotInput, ok)
	}
	if _, ok := m.Output(info.ID); !ok {
		t.Fatal("Output() ok = false, want true")
	}
}

func TestOutput_Input_UnknownSession(t *testing.T) {
	m := newTestManager(newMockInput())
	if _, ok := m.Output("nope"); ok {
		t.Fatal("Output() ok = true for an unknown session, want false")
	}
	if _, ok := m.Input("nope"); ok {
		t.Fatal("Input() ok = true for an unknown session, want false")
	}
}

// TestGallerySharedAcrossSessions confirms every session created by one
// Manager sees the *same* reference gallery — white-box (same package),
// reaches into entry.useCases directly since Manager doesn't otherwise
// expose gallery contents itself (uc.UseCases does, via
// ListGalleryReferences — that's the real surface a REST caller would
// use; this test just needs to prove sharing, not re-test
// ReferenceGallery's own logic, already covered in application/uc).
func TestGallerySharedAcrossSessions(t *testing.T) {
	gallery := uc.NewReferenceGallery()
	if err := gallery.Add("mon_sac", entities.Embedding{1, 0}); err != nil {
		t.Fatalf("gallery.Add() error = %v", err)
	}

	m := NewManager(
		func(Source) (streamer.InputStream, error) { return newMockInput(), nil },
		func() streamer.OutputStream { return mockOutput{} },
		noopLogger{}, mockNotifier{}, mockDetector{}, mockEncoder{}, mockTrackerFactory, gallery,
	)

	infoA, _ := m.CreateSession(context.Background(), Source{Kind: "local"})
	infoB, _ := m.CreateSession(context.Background(), Source{Kind: "local"})

	for _, id := range []string{infoA.ID, infoB.ID} {
		e, ok := m.get(id)
		if !ok {
			t.Fatalf("session %q not found", id)
		}
		names := map[string]bool{}
		for _, entry := range e.useCases.ListGalleryReferences(context.Background()) {
			names[entry.Name] = true
		}
		if !names["mon_sac"] {
			t.Fatalf("session %q doesn't see the shared gallery entry — got %+v", id, e.useCases.ListGalleryReferences(context.Background()))
		}
	}
}
