package api

import (
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"live-semantic/internal/application/session"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/storage"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"

	"github.com/gin-gonic/gin"
)

// Test doubles below mirror internal/application/session's own (can't
// import unexported types across packages) — just enough to exercise
// sessionController's REST handlers against a *real* session.Manager,
// same rationale as recognition_test.go/gallery_test.go using a real
// gin.Engine rather than calling handler methods directly.

func sessionTestFrame() *entities.Frame {
	return &entities.Frame{Image: image.NewRGBA(image.Rect(0, 0, 4, 4))}
}

type sessionMockInput struct {
	mu      sync.Mutex
	stop    chan struct{}
	stopped bool
}

func newSessionMockInput() *sessionMockInput { return &sessionMockInput{stop: make(chan struct{})} }

var _ streamer.InputStream = (*sessionMockInput)(nil)

func (m *sessionMockInput) Initialize() error { return nil }
func (m *sessionMockInput) Start(cb func(*entities.Frame) (*entities.Frame, error)) error {
	if _, err := cb(sessionTestFrame()); err != nil {
		return err
	}
	<-m.stop
	return nil
}
func (m *sessionMockInput) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stop)
	}
}
func (m *sessionMockInput) Cleanup() {}

type sessionMockOutput struct{}

var _ streamer.OutputStream = sessionMockOutput{}

func (sessionMockOutput) Initialize() error            { return nil }
func (sessionMockOutput) Render(*entities.Frame) error { return nil }
func (sessionMockOutput) HandleKeyEvent() int          { return -1 }
func (sessionMockOutput) Stop()                        {}
func (sessionMockOutput) Cleanup()                     {}

type sessionMockDetector struct{}

var _ inference.ObjectDetector = sessionMockDetector{}

func (sessionMockDetector) AnalyzeFrame(frame *entities.Frame) (*inference.DetectionResult, error) {
	return &inference.DetectionResult{Frame: frame}, nil
}
func (sessionMockDetector) Cleanup() {}

type sessionMockEncoder struct{}

func (sessionMockEncoder) EncodeImage(*entities.Frame) (entities.Embedding, error) {
	return entities.Embedding{1, 0}, nil
}
func (sessionMockEncoder) EncodeText(string) (entities.Embedding, error) {
	return entities.Embedding{1, 0}, nil
}
func (sessionMockEncoder) Cleanup() {}

type sessionMockNotifier struct{}

func (sessionMockNotifier) Notify(entities.Message) error { return nil }
func (sessionMockNotifier) Cleanup()                      {}

// sessionMockTrackerFactory is a tracking.TrackerFactoryFunc (not a bare
// func — TrackerFactory is a real interface since 2026-08-12, see its own
// doc comment) so it can be passed directly wherever a
// tracking.TrackerFactory is expected.
var sessionMockTrackerFactory tracking.TrackerFactoryFunc = func() (tracking.ObjectTracker, error) {
	return sessionMockTracker{}, nil
}

type sessionMockTracker struct{}

func (sessionMockTracker) Init(*entities.Frame, entities.BoundingBox) error { return nil }
func (sessionMockTracker) Update(*entities.Frame) (entities.BoundingBox, bool) {
	return entities.BoundingBox{}, true
}
func (sessionMockTracker) Cleanup() {}

// sessionMockGalleryRepo is a minimal storage.GalleryStorage test double —
// not implementation/storage/inmemory.Gallery, which this package's own
// tests must not import any more than production code may (dependency
// inversion, same rationale as application/session's own copy of this
// idea in session_test.go).
type sessionMockGalleryRepo struct {
	entries map[string]*storage.Gallery
}

func newSessionMockGalleryRepo() *sessionMockGalleryRepo {
	return &sessionMockGalleryRepo{entries: map[string]*storage.Gallery{}}
}

var _ storage.GalleryStorage = (*sessionMockGalleryRepo)(nil)

func (g *sessionMockGalleryRepo) Add(name string, embedding entities.Embedding) error {
	if _, exists := g.entries[name]; exists {
		return fmt.Errorf("gallery entry %q already exists", name)
	}
	g.entries[name] = &storage.Gallery{Name: name, Embedding: embedding, Enabled: true}
	return nil
}
func (g *sessionMockGalleryRepo) Remove(name string) { delete(g.entries, name) }
func (g *sessionMockGalleryRepo) Rename(oldName, newName string) error {
	e, ok := g.entries[oldName]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", oldName)
	}
	delete(g.entries, oldName)
	e.Name = newName
	g.entries[newName] = e
	return nil
}
func (g *sessionMockGalleryRepo) SetEnabled(name string, enabled bool) error {
	e, ok := g.entries[name]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	e.Enabled = enabled
	return nil
}
func (g *sessionMockGalleryRepo) Get(name string) (entities.Embedding, bool) {
	e, ok := g.entries[name]
	if !ok || !e.Enabled {
		return nil, false
	}
	return e.Embedding, true
}
func (g *sessionMockGalleryRepo) List() []storage.Gallery {
	out := make([]storage.Gallery, 0, len(g.entries))
	for _, e := range g.entries {
		out = append(out, *e)
	}
	return out
}

// newSessionTestServer wires a Server whose session.Manager's
// InputFactory always hands out a fresh *sessionMockInput — the caller
// gets the underlying manager back too, for tests that need to inspect
// state a REST response doesn't expose.
func newSessionTestServer() (*gin.Engine, *session.Manager) {
	mgr := session.NewManager(
		func(session.Source) (streamer.InputStream, error) { return newSessionMockInput(), nil },
		func() streamer.OutputStream { return sessionMockOutput{} },
		noopLogger{},
		sessionMockNotifier{},
		sessionMockDetector{},
		sessionMockEncoder{},
		sessionMockTrackerFactory,
		newSessionMockGalleryRepo(),
	)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	sc := newSessionController(mgr, noopLogger{})
	v1 := router.Group("/api/v1")
	v1.POST("/sessions", sc.create)
	v1.GET("/sessions", sc.list)
	v1.GET("/sessions/:id", sc.get)
	v1.DELETE("/sessions/:id", sc.remove)
	v1.POST("/sessions/:id/recognition/start", sc.startRecognition)
	v1.POST("/sessions/:id/recognition/stop", sc.stopRecognition)

	return router, mgr
}

func doJSON(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestSessionCreate_ReturnsInfo(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions", `{"source":{"kind":"local"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var info session.Info
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.ID == "" || info.Running {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestSessionCreate_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions", `{not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSessionList_ReturnsCreatedSessions(t *testing.T) {
	router, _ := newSessionTestServer()
	doJSON(router, http.MethodPost, "/api/v1/sessions", `{"source":{"kind":"local"}}`)
	doJSON(router, http.MethodPost, "/api/v1/sessions", `{"source":{"kind":"file","uri":"x.mp4"}}`)

	w := doJSON(router, http.MethodGet, "/api/v1/sessions", "")
	var body struct {
		Sessions []session.Info `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(body.Sessions))
	}
}

func TestSessionGet_UnknownID_ReturnsNotFound(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodGet, "/api/v1/sessions/nope", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionRemove_UnknownID_ReturnsNotFound(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodDelete, "/api/v1/sessions/nope", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionRemove_KnownID_ThenGetReturnsNotFound(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions", `{"source":{"kind":"local"}}`)
	var info session.Info
	json.Unmarshal(w.Body.Bytes(), &info)

	w = doJSON(router, http.MethodDelete, "/api/v1/sessions/"+info.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	w = doJSON(router, http.MethodGet, "/api/v1/sessions/"+info.ID, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after removal, got %d", w.Code)
	}
}

func TestSessionStart_UnknownID_ReturnsNotFound(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions/nope/recognition/start", `{"filter":"person"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionStart_ThenAlreadyRunning_ReturnsConflict(t *testing.T) {
	router, mgr := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions", `{"source":{"kind":"local"}}`)
	var info session.Info
	json.Unmarshal(w.Body.Bytes(), &info)

	w = doJSON(router, http.MethodPost, "/api/v1/sessions/"+info.ID+"/recognition/start", `{"filter":"person"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(router, http.MethodPost, "/api/v1/sessions/"+info.ID+"/recognition/start", `{"filter":"person"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 on second start, got %d", w.Code)
	}

	// Cleanup — stop so the goroutine doesn't leak past the test.
	mgr.StopRecognition(info.ID)
}

func TestSessionStop_UnknownID_ReturnsNotFound(t *testing.T) {
	router, _ := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions/nope/recognition/stop", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionStop_KnownID_UnblocksRunningSession(t *testing.T) {
	router, mgr := newSessionTestServer()
	w := doJSON(router, http.MethodPost, "/api/v1/sessions", `{"source":{"kind":"local"}}`)
	var info session.Info
	json.Unmarshal(w.Body.Bytes(), &info)

	doJSON(router, http.MethodPost, "/api/v1/sessions/"+info.ID+"/recognition/start", `{"filter":"person"}`)

	w = doJSON(router, http.MethodPost, "/api/v1/sessions/"+info.ID+"/recognition/stop", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	if err := mgr.RemoveSession(info.ID); err != nil {
		t.Fatalf("RemoveSession after stop should not block/error: %v", err)
	}
}
