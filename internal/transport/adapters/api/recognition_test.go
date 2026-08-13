package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/application/uc"

	"github.com/gin-gonic/gin"
)

// noopLogger satisfies logger.Logger without printing anything — tests
// only care about behavior, not log output.
type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Close()               {}

// mockUseCases stands in for uc.UseCases. RecognitionUseCase blocks on
// proceed (if non-nil) so tests can control exactly when a "session"
// finishes — mirroring the real RecognitionUseCase, which blocks until the
// stream stops.
type mockUseCases struct {
	mu        sync.Mutex
	proceed   chan struct{}
	result    dto.Result[dto.RecognitionResponse]
	resultErr error
	stopCalls int

	// gallery is a minimal real in-memory store (name -> enabled), enough
	// to exercise galleryController's REST handlers without depending on
	// implementation/storage/inmemory (a different package's internals) —
	// this mock only needs to prove the HTTP layer wires calls through
	// correctly, not re-verify storage.GalleryStorage's own logic (already
	// covered by implementation/storage/inmemory's tests).
	galleryMu      sync.Mutex
	galleryEntries map[string]bool // name -> enabled

	// removedImage records the last RemoveGalleryImage call's arguments —
	// lets a test assert the REST layer forwarded name/imageID correctly
	// without this mock needing a real per-image store.
	removedImage struct{ name, imageID string }
	// thumbnail/thumbnailOK back GetGalleryThumbnail — set by a test to
	// control what the REST handler sees.
	thumbnail   []byte
	thumbnailOK bool

	// cocoClasses is a minimal real store (name -> linked COCO class),
	// same rationale as galleryEntries above — enough to exercise
	// galleryController.update's coco_class field without depending on a
	// different package's internals.
	cocoClasses map[string]string

	// collections is a minimal real Bibliothèque Collections store —
	// same rationale as galleryEntries/cocoClasses above, enough to
	// exercise collectionsController's REST handlers end to end.
	collections map[string]*uc.CollectionInfo
}

func (m *mockUseCases) AddGalleryReference(_ context.Context, name string, _ image.Image) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	if m.galleryEntries == nil {
		m.galleryEntries = map[string]bool{}
	}
	if _, exists := m.galleryEntries[name]; exists {
		return fmt.Errorf("gallery entry %q already exists", name)
	}
	m.galleryEntries[name] = true
	return nil
}

func (m *mockUseCases) RemoveGalleryReference(_ context.Context, name string) {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	delete(m.galleryEntries, name)
}

func (m *mockUseCases) RenameGalleryReference(_ context.Context, oldName, newName string) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	enabled, ok := m.galleryEntries[oldName]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", oldName)
	}
	delete(m.galleryEntries, oldName)
	m.galleryEntries[newName] = enabled
	return nil
}

func (m *mockUseCases) SetGalleryReferenceEnabled(_ context.Context, name string, enabled bool) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	if _, ok := m.galleryEntries[name]; !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	m.galleryEntries[name] = enabled
	return nil
}

// RemoveGalleryImage just records its arguments — this mock's simplified
// gallery (name -> enabled) has no real per-image store, so there's
// nothing to actually mutate, but a test can still assert the REST layer
// forwarded name/imageID correctly.
func (m *mockUseCases) RemoveGalleryImage(_ context.Context, name, imageID string) {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	m.removedImage = struct{ name, imageID string }{name, imageID}
}

// GetGalleryThumbnail returns whatever a test pre-configured via the
// thumbnail/thumbnailOK fields — defaults to not-found.
func (m *mockUseCases) GetGalleryThumbnail(_ context.Context, _, _ string) ([]byte, bool) {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	return m.thumbnail, m.thumbnailOK
}

func (m *mockUseCases) ListGalleryReferences(_ context.Context) []uc.GalleryEntryInfo {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	out := make([]uc.GalleryEntryInfo, 0, len(m.galleryEntries))
	for name, enabled := range m.galleryEntries {
		out = append(out, uc.GalleryEntryInfo{Name: name, Enabled: enabled, CocoClass: m.cocoClasses[name]})
	}
	return out
}

// SetGalleryCocoClass — real minimal behavior (name must exist), same
// rationale as SetGalleryReferenceEnabled above.
func (m *mockUseCases) SetGalleryCocoClass(_ context.Context, name, cocoClass string) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	if _, ok := m.galleryEntries[name]; !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	if m.cocoClasses == nil {
		m.cocoClasses = map[string]string{}
	}
	m.cocoClasses[name] = cocoClass
	return nil
}

// --- Bibliothèque — Collections mock. A minimal real store (name ->
// *uc.CollectionInfo), same rationale as galleryEntries/cocoClasses
// above — enough to exercise collectionsController's REST handlers end
// to end without depending on a different package's internals. ---

func (m *mockUseCases) CreateCollection(_ context.Context, name string, tags []string) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	if m.collections == nil {
		m.collections = map[string]*uc.CollectionInfo{}
	}
	if _, exists := m.collections[name]; exists {
		return fmt.Errorf("collection %q already exists", name)
	}
	m.collections[name] = &uc.CollectionInfo{Name: name, Tags: tags}
	return nil
}

func (m *mockUseCases) DeleteCollection(_ context.Context, name string) {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	delete(m.collections, name)
}

func (m *mockUseCases) RenameCollection(_ context.Context, oldName, newName string) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	e, ok := m.collections[oldName]
	if !ok {
		return fmt.Errorf("collection %q not found", oldName)
	}
	if _, exists := m.collections[newName]; exists {
		return fmt.Errorf("collection %q already exists", newName)
	}
	delete(m.collections, oldName)
	e.Name = newName
	m.collections[newName] = e
	return nil
}

func (m *mockUseCases) SetCollectionTags(_ context.Context, name string, tags []string) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	e, ok := m.collections[name]
	if !ok {
		return fmt.Errorf("collection %q not found", name)
	}
	e.Tags = tags
	return nil
}

func (m *mockUseCases) AddTermToCollection(_ context.Context, collectionName, termName string) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	e, ok := m.collections[collectionName]
	if !ok {
		return fmt.Errorf("collection %q not found", collectionName)
	}
	if _, exists := m.galleryEntries[termName]; !exists {
		return fmt.Errorf("term %q does not exist", termName)
	}
	for _, t := range e.Terms {
		if t == termName {
			return nil
		}
	}
	e.Terms = append(e.Terms, termName)
	return nil
}

func (m *mockUseCases) RemoveTermFromCollection(_ context.Context, collectionName, termName string) {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	e, ok := m.collections[collectionName]
	if !ok {
		return
	}
	for i, t := range e.Terms {
		if t == termName {
			e.Terms = append(e.Terms[:i], e.Terms[i+1:]...)
			return
		}
	}
}

func (m *mockUseCases) ListCollections(_ context.Context) []uc.CollectionInfo {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	out := make([]uc.CollectionInfo, 0, len(m.collections))
	for _, e := range m.collections {
		out = append(out, *e)
	}
	return out
}

func (m *mockUseCases) Recognize(_ context.Context, _ dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error) {
	if m.proceed != nil {
		<-m.proceed
	}
	return m.result, m.resultErr
}

func (m *mockUseCases) StopRecognition() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
}

// WaitRecognition is a no-op here: these tests don't exercise main.go's
// shutdown path, just recognitionController's REST handlers.
func (m *mockUseCases) WaitRecognition() {}

func (m *mockUseCases) stopCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCalls
}

// isRunningForTest reads recognitionController.running under its mutex —
// defined here (test-only) rather than adding a test-only accessor to the
// production file.
func (rc *recognitionController) isRunningForTest() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.running
}

func newTestContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, w
}

// waitUntil polls cond every few ms until it's true or timeout elapses,
// failing the test on timeout — used to observe the async flip of
// recognitionController.running once a blocked mock is released.
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

func TestStart_ValidRequest_ReturnsAcceptedAndRunsAsync(t *testing.T) {
	mock := &mockUseCases{proceed: make(chan struct{})}
	rc := newRecognitionController(mock, noopLogger{})

	c, w := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"person"}`)
	rc.start(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["status"] != "started" {
		t.Fatalf("status field = %v, want %q", body["status"], "started")
	}
	if body["filter"] != "person" {
		t.Fatalf("filter field = %v, want %q", body["filter"], "person")
	}

	waitUntil(t, time.Second, rc.isRunningForTest)

	close(mock.proceed) // let the mocked session "finish", avoid leaking the goroutine
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() })
}

func TestStart_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	mock := &mockUseCases{}
	rc := newRecognitionController(mock, noopLogger{})

	c, w := newTestContext(http.MethodPost, "/api/v1/recognition/start", `not json`)
	rc.start(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if rc.isRunningForTest() {
		t.Fatal("running should stay false when the request body is invalid")
	}
}

func TestStart_WhileAlreadyRunning_ReturnsConflict(t *testing.T) {
	mock := &mockUseCases{proceed: make(chan struct{})}
	rc := newRecognitionController(mock, noopLogger{})

	c1, w1 := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"person"}`)
	rc.start(c1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first start: status = %d, want %d", w1.Code, http.StatusAccepted)
	}

	c2, w2 := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"car"}`)
	rc.start(c2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("second start: status = %d, want %d, body = %s", w2.Code, http.StatusConflict, w2.Body.String())
	}

	close(mock.proceed)
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() })
}

func TestStop_WhenRunning_CallsUseCasesStopAndReturnsAccepted(t *testing.T) {
	mock := &mockUseCases{proceed: make(chan struct{})}
	rc := newRecognitionController(mock, noopLogger{})

	startCtx, _ := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"person"}`)
	rc.start(startCtx)
	waitUntil(t, time.Second, rc.isRunningForTest)

	stopCtx, w := newTestContext(http.MethodPost, "/api/v1/recognition/stop", "")
	rc.stop(stopCtx)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusAccepted, w.Body.String())
	}
	if got := mock.stopCallCount(); got != 1 {
		t.Fatalf("useCases.Stop() called %d times, want 1", got)
	}

	close(mock.proceed)
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() })
}

func TestStop_WhenNotRunning_ReturnsConflict(t *testing.T) {
	mock := &mockUseCases{}
	rc := newRecognitionController(mock, noopLogger{})

	c, w := newTestContext(http.MethodPost, "/api/v1/recognition/stop", "")
	rc.stop(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusConflict, w.Body.String())
	}
	if got := mock.stopCallCount(); got != 0 {
		t.Fatalf("useCases.Stop() called %d times, want 0 (nothing running)", got)
	}
}

func TestStatus_ReflectsRunningState(t *testing.T) {
	mock := &mockUseCases{proceed: make(chan struct{})}
	rc := newRecognitionController(mock, noopLogger{})

	assertStatus := func(want bool) {
		t.Helper()
		c, w := newTestContext(http.MethodGet, "/api/v1/recognition/status", "")
		rc.status(c)
		var body map[string]bool
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if body["running"] != want {
			t.Fatalf("running = %v, want %v", body["running"], want)
		}
	}

	assertStatus(false)

	startCtx, _ := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"person"}`)
	rc.start(startCtx)
	waitUntil(t, time.Second, rc.isRunningForTest)
	assertStatus(true)

	close(mock.proceed)
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() })
	assertStatus(false)
}

// TestStatus_SurfacesLastErrorAfterFailedSession — fixed 2026-08-12 (the
// REST API didn't use to surface an invalid filter to the client):
// a session that ends in error (e.g. an invalid filter) must be visible
// via GET /recognition/status, not just server logs.
func TestStatus_SurfacesLastErrorAfterFailedSession(t *testing.T) {
	mock := &mockUseCases{resultErr: errors.New("invalid filter: unknown COCO label")}
	rc := newRecognitionController(mock, noopLogger{})

	startCtx, _ := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"bogus"}`)
	rc.start(startCtx)
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() }) // resultErr set -> RecognitionUseCase "returns" immediately

	c, w := newTestContext(http.MethodGet, "/api/v1/recognition/status", "")
	rc.status(c)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["running"] != false {
		t.Fatalf("running = %v, want false", body["running"])
	}
	if body["error"] != "invalid filter: unknown COCO label" {
		t.Fatalf("error field = %v, want the session's actual error", body["error"])
	}
}

// TestStatus_LastErrorClearedByNextStart confirms a stale error from a
// past failed session doesn't linger forever — a fresh, currently
// successful start() must clear it.
func TestStatus_LastErrorClearedByNextStart(t *testing.T) {
	mock := &mockUseCases{resultErr: errors.New("boom")}
	rc := newRecognitionController(mock, noopLogger{})

	startCtx, _ := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"bogus"}`)
	rc.start(startCtx)
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() })

	if got := rc.lastError; got == "" {
		t.Fatal("lastError should be set after the first failed session")
	}

	// Second start, this time the mock succeeds (no proceed channel ->
	// RecognitionUseCase "returns" immediately with the zero Result).
	mock.mu.Lock()
	mock.resultErr = nil
	mock.mu.Unlock()

	startCtx2, _ := newTestContext(http.MethodPost, "/api/v1/recognition/start", `{"filter":"person"}`)
	rc.start(startCtx2)
	waitUntil(t, time.Second, func() bool { return !rc.isRunningForTest() })

	c, w := newTestContext(http.MethodGet, "/api/v1/recognition/status", "")
	rc.status(c)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if _, hasError := body["error"]; hasError {
		t.Fatalf("error field = %v, want absent after a successful session superseded the failed one", body["error"])
	}
}
