package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"live-semantic/internal/application/session"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"

	"github.com/gin-gonic/gin"
)

// rewindMockOutput is a package-local streamer.OutputStream +
// streamer.BoxAwareOutputStream + streamer.Rewindable test double — not
// implementation/streamer/output.RingBufferOutput, which this package's
// own tests must not import any more than production code may
// (dependency inversion, same rationale as every other mock in this
// package). preset lets a test control exactly what RewindAt/RewindRange
// return without needing a real ring buffer's timing behavior.
type rewindMockOutput struct {
	rangeAvailable time.Duration
	entry          streamer.RewindEntry
	hasEntry       bool
}

var (
	_ streamer.OutputStream         = (*rewindMockOutput)(nil)
	_ streamer.BoxAwareOutputStream = (*rewindMockOutput)(nil)
	_ streamer.Rewindable           = (*rewindMockOutput)(nil)
)

func (*rewindMockOutput) Initialize() error            { return nil }
func (*rewindMockOutput) Render(*entities.Frame) error { return nil }
func (*rewindMockOutput) RenderBoxes([]streamer.BoxData) error {
	return nil
}
func (*rewindMockOutput) HandleKeyEvent() int { return -1 }
func (*rewindMockOutput) Stop()               {}
func (*rewindMockOutput) Cleanup()            {}

func (m *rewindMockOutput) RewindAt(time.Duration) (streamer.RewindEntry, bool) {
	return m.entry, m.hasEntry
}
func (m *rewindMockOutput) RewindRange() time.Duration { return m.rangeAvailable }

// newRewindTestServer wires a Server whose session.Manager always hands
// out the same *rewindMockOutput for every created session, so a test can
// preset exactly what it returns before hitting the REST endpoints.
func newRewindTestServer(out *rewindMockOutput) (*gin.Engine, *session.Manager) {
	mgr := session.NewManager(
		func(session.Source) (streamer.InputStream, error) { return newSessionMockInput(), nil },
		func() streamer.OutputStream { return out },
		noopLogger{},
		sessionMockNotifier{},
		sessionMockDetector{},
		sessionMockEncoder{},
		func() tracking.TrackerFactory { return sessionMockTrackerFactory },
		newSessionMockGalleryRepo(),
		sessionMockCollectionRepo{},
	)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	s := &Server{sessions: newSessionController(mgr, noopLogger{}), logger: noopLogger{}}
	v1 := router.Group("/api/v1")
	v1.POST("/sessions", s.sessions.create)
	v1.GET("/sessions/:id/rewind/range", s.handleSessionRewindRange)
	v1.GET("/sessions/:id/rewind", s.handleSessionRewindBoxes)
	v1.GET("/sessions/:id/rewind/image", s.handleSessionRewindImage)

	return router, mgr
}

func createTestSession(t *testing.T, router *gin.Engine) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(`{"source":{"kind":"local"}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("create session status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal create response: %v, body = %s", err, w.Body.String())
	}
	if body.ID == "" {
		t.Fatalf("create session response has no id, body = %s", w.Body.String())
	}
	return body.ID
}

func TestHandleSessionRewindRange_UnknownSession404(t *testing.T) {
	router, _ := newRewindTestServer(&rewindMockOutput{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/nope/rewind/range", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleSessionRewindRange_ReturnsRangeMs(t *testing.T) {
	router, _ := newRewindTestServer(&rewindMockOutput{rangeAvailable: 12300 * time.Millisecond})
	id := createTestSession(t, router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id+"/rewind/range", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var body struct {
		RangeMs int64 `json:"rangeMs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.RangeMs != 12300 {
		t.Fatalf("rangeMs = %d, want 12300", body.RangeMs)
	}
}

func TestHandleSessionRewindBoxes_NothingBufferedYet404(t *testing.T) {
	router, _ := newRewindTestServer(&rewindMockOutput{hasEntry: false})
	id := createTestSession(t, router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id+"/rewind?offset_ms=5000", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleSessionRewindBoxes_ReturnsBoxes(t *testing.T) {
	out := &rewindMockOutput{
		hasEntry: true,
		entry: streamer.RewindEntry{
			JPEG:   []byte{0xFF, 0xD8, 0xFF, 0xD9},
			Boxes:  []streamer.BoxData{{ID: "person", Label: "person (90%)", TrackID: "t1", X1: 0.1, Y1: 0.2, X2: 0.3, Y2: 0.4}},
			AgeAgo: 4800 * time.Millisecond,
		},
	}
	router, _ := newRewindTestServer(out)
	id := createTestSession(t, router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id+"/rewind?offset_ms=5000", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Boxes []struct {
			ID    string  `json:"ID"`
			Label string  `json:"Label"`
			X1    float32 `json:"X1"`
		} `json:"boxes"`
		ActualAgeMs int64 `json:"actualAgeMs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, w.Body.String())
	}
	if len(body.Boxes) != 1 || body.Boxes[0].ID != "person" || body.Boxes[0].Label != "person (90%)" {
		t.Fatalf("boxes = %+v, want the preset box", body.Boxes)
	}
	if body.ActualAgeMs != 4800 {
		t.Fatalf("actualAgeMs = %d, want 4800", body.ActualAgeMs)
	}
}

func TestHandleSessionRewindImage_ServesRawJPEG(t *testing.T) {
	jpegBytes := []byte{0xFF, 0xD8, 0xAB, 0xCD, 0xFF, 0xD9}
	out := &rewindMockOutput{hasEntry: true, entry: streamer.RewindEntry{JPEG: jpegBytes}}
	router, _ := newRewindTestServer(out)
	id := createTestSession(t, router)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id+"/rewind/image?offset_ms=1000", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", ct)
	}
	if string(w.Body.Bytes()) != string(jpegBytes) {
		t.Fatalf("body = %v, want %v", w.Body.Bytes(), jpegBytes)
	}
}
