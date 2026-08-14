package session

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"
	"testing"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/storage"
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

// mockTrackerFactory is a tracking.TrackerFactoryFunc (not a bare func —
// TrackerFactory is a real interface since 2026-08-12, see its own doc
// comment) so it can be passed directly wherever a tracking.TrackerFactory
// is expected.
var mockTrackerFactory tracking.TrackerFactoryFunc = func() (tracking.ObjectTracker, error) {
	return mockTracker{}, nil
}

type mockTracker struct{}

func (mockTracker) Init(*entities.Frame, entities.BoundingBox) error { return nil }
func (mockTracker) Update(*entities.Frame) (entities.BoundingBox, bool) {
	return entities.BoundingBox{}, true
}
func (mockTracker) Cleanup() {}

// mockGalleryRepo is a minimal storage.GalleryStorage test double — not
// implementation/gallery/inmemory.Gallery, which this package must not
// import any more than session.go's own production code may (dependency
// inversion). Package-local, not shared with application/uc's own copy
// of the same idea (different package, same rationale — see that
// package's mockGalleryRepo doc comment).
type mockGalleryRepo struct {
	entries map[string]*storage.Gallery
	nextSeq int
}

func newMockGalleryRepo() *mockGalleryRepo {
	return &mockGalleryRepo{entries: map[string]*storage.Gallery{}}
}

var _ storage.GalleryStorage = (*mockGalleryRepo)(nil)

func (g *mockGalleryRepo) AddImage(name string, embedding entities.Embedding, thumbnail []byte) (string, error) {
	g.nextSeq++
	id := fmt.Sprintf("img-%d", g.nextSeq)
	e, ok := g.entries[name]
	if !ok {
		e = &storage.Gallery{Name: name, Enabled: true}
		g.entries[name] = e
	}
	e.Images = append(e.Images, storage.GalleryImage{ID: id, Embedding: embedding})
	return id, nil
}
func (g *mockGalleryRepo) RemoveImage(name, imageID string) {
	e, ok := g.entries[name]
	if !ok {
		return
	}
	for i, img := range e.Images {
		if img.ID == imageID {
			e.Images = append(e.Images[:i], e.Images[i+1:]...)
			break
		}
	}
	if len(e.Images) == 0 {
		delete(g.entries, name)
	}
}
func (g *mockGalleryRepo) Remove(name string) { delete(g.entries, name) }
func (g *mockGalleryRepo) Rename(oldName, newName string) error {
	e, ok := g.entries[oldName]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", oldName)
	}
	delete(g.entries, oldName)
	e.Name = newName
	g.entries[newName] = e
	return nil
}
func (g *mockGalleryRepo) SetEnabled(name string, enabled bool) error {
	e, ok := g.entries[name]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	e.Enabled = enabled
	return nil
}
func (g *mockGalleryRepo) SetCocoClass(name, cocoClass string) error {
	e, ok := g.entries[name]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	e.CocoClass = cocoClass
	return nil
}
func (g *mockGalleryRepo) Get(name string) ([]entities.Embedding, bool) {
	e, ok := g.entries[name]
	if !ok || !e.Enabled || len(e.Images) == 0 {
		return nil, false
	}
	out := make([]entities.Embedding, len(e.Images))
	for i, img := range e.Images {
		out[i] = img.Embedding
	}
	return out, true
}

// mockCollectionRepo is a minimal storage.CollectionStorage test double —
// same rationale as mockGalleryRepo just above (package-local, not
// implementation/*). Session tests don't exercise Collections behavior
// directly, just need a non-nil value to satisfy NewManager.
type mockCollectionRepo struct{}

var _ storage.CollectionStorage = (*mockCollectionRepo)(nil)

func (mockCollectionRepo) Create(name string, tags []string) error       { return nil }
func (mockCollectionRepo) Delete(name string)                            {}
func (mockCollectionRepo) Rename(oldName, newName string) error          { return nil }
func (mockCollectionRepo) SetTags(name string, tags []string) error      { return nil }
func (mockCollectionRepo) AddTerm(collectionName, termName string) error { return nil }
func (mockCollectionRepo) RemoveTerm(collectionName, termName string)    {}
func (mockCollectionRepo) Get(name string) (storage.Collection, bool) {
	return storage.Collection{}, false
}
func (mockCollectionRepo) List() []storage.Collection { return nil }
func (g *mockGalleryRepo) Thumbnail(name, imageID string) ([]byte, bool) {
	e, ok := g.entries[name]
	if !ok {
		return nil, false
	}
	for _, img := range e.Images {
		if img.ID == imageID {
			return []byte("thumb"), true
		}
	}
	return nil, false
}
func (g *mockGalleryRepo) List() []storage.Gallery {
	out := make([]storage.Gallery, 0, len(g.entries))
	for _, e := range g.entries {
		out = append(out, *e)
	}
	return out
}

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
		func() tracking.TrackerFactory { return mockTrackerFactory },
		// gallery — NewUseCase requires a non-nil Repository since the
		// 2026-08-12 port extraction (no more internal fallback, see its
		// doc comment); a fresh one per test is fine here, TestGallery
		// SharedAcrossSessions passes an explicit shared one instead.
		newMockGalleryRepo(),
		mockCollectionRepo{},
		nil,
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
		noopLogger{}, mockNotifier{}, mockDetector{}, mockEncoder{}, func() tracking.TrackerFactory { return mockTrackerFactory }, nil, nil, nil,
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
// the storage.GalleryStorage's own logic, already covered by
// implementation/gallery/inmemory's tests).
func TestGallerySharedAcrossSessions(t *testing.T) {
	galleryRepo := newMockGalleryRepo()
	if _, err := galleryRepo.AddImage("mon_sac", entities.Embedding{1, 0}, []byte("thumb")); err != nil {
		t.Fatalf("galleryRepo.AddImage() error = %v", err)
	}

	m := NewManager(
		func(Source) (streamer.InputStream, error) { return newMockInput(), nil },
		func() streamer.OutputStream { return mockOutput{} },
		noopLogger{}, mockNotifier{}, mockDetector{}, mockEncoder{}, func() tracking.TrackerFactory { return mockTrackerFactory }, galleryRepo, mockCollectionRepo{}, nil,
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
