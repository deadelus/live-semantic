package api

import (
	"context"
	"encoding/json"
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
	// uc.UseCase's own gallery.go (a different package's internals) —
	// this mock only needs to prove the HTTP layer wires calls through
	// correctly, not re-verify ReferenceGallery's own logic (already
	// covered by internal/application/uc's tests).
	galleryMu      sync.Mutex
	galleryEntries map[string]bool // name -> enabled
}

func (m *mockUseCases) AddGalleryReference(name string, _ image.Image) error {
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

func (m *mockUseCases) RemoveGalleryReference(name string) {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	delete(m.galleryEntries, name)
}

func (m *mockUseCases) RenameGalleryReference(oldName, newName string) error {
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

func (m *mockUseCases) SetGalleryReferenceEnabled(name string, enabled bool) error {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	if _, ok := m.galleryEntries[name]; !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	m.galleryEntries[name] = enabled
	return nil
}

func (m *mockUseCases) ListGalleryReferences() []uc.GalleryEntryInfo {
	m.galleryMu.Lock()
	defer m.galleryMu.Unlock()
	out := make([]uc.GalleryEntryInfo, 0, len(m.galleryEntries))
	for name, enabled := range m.galleryEntries {
		out = append(out, uc.GalleryEntryInfo{Name: name, Enabled: enabled})
	}
	return out
}

func (m *mockUseCases) RecognitionUseCase(_ context.Context, _ dto.RecognitionRequest) (dto.Result[dto.RecognitionResponse], error) {
	if m.proceed != nil {
		<-m.proceed
	}
	return m.result, m.resultErr
}

func (m *mockUseCases) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
}

// Wait is a no-op here: these tests don't exercise main.go's shutdown
// path, just recognitionController's REST handlers.
func (m *mockUseCases) Wait() {}

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
