package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"live-semantic/internal/application/session"
	"live-semantic/internal/infrastructure/journal"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"

	"github.com/gin-gonic/gin"
)

// journalMock is a package-local journal.Journal test double — not
// implementation/journal/inmemory, which this package's own tests must
// not import any more than production code may (dependency inversion,
// same rationale as every other mock in this package).
type journalMock struct {
	entries []journal.Entry
}

var _ journal.Journal = (*journalMock)(nil)

func (jm *journalMock) Record(e journal.Entry) { jm.entries = append(jm.entries, e) }
func (jm *journalMock) List() []journal.Entry  { return jm.entries }

func newJournalTestServer(j journal.Journal) *gin.Engine {
	mgr := session.NewManager(
		func(session.Source) (streamer.InputStream, error) { return newSessionMockInput(), nil },
		func() streamer.OutputStream { return sessionMockOutput{} },
		noopLogger{},
		sessionMockNotifier{},
		sessionMockDetector{},
		sessionMockEncoder{},
		func() tracking.TrackerFactory { return sessionMockTrackerFactory },
		newSessionMockGalleryRepo(),
		sessionMockCollectionRepo{},
		j,
	)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	jc := newJournalController(mgr)
	router.GET("/api/v1/journal", jc.list)
	return router
}

func TestJournalList_ReturnsEntries(t *testing.T) {
	jm := &journalMock{}
	jm.Record(journal.Entry{
		Timestamp: time.UnixMilli(1000),
		SessionID: "session-1",
		Type:      "TrackEntered",
		TrackID:   "track-1",
		Class:     "person",
		Score:     0.87,
	})
	router := newJournalTestServer(jm)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/journal", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var body struct {
		Entries []struct {
			SessionID string  `json:"sessionId"`
			Type      string  `json:"type"`
			TrackID   string  `json:"trackId"`
			Class     string  `json:"class"`
			Score     float32 `json:"score"`
			Timestamp int64   `json:"timestampMs"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, w.Body.String())
	}
	if len(body.Entries) != 1 {
		t.Fatalf("entries = %+v, want 1", body.Entries)
	}
	e := body.Entries[0]
	if e.SessionID != "session-1" || e.Type != "TrackEntered" || e.TrackID != "track-1" || e.Class != "person" {
		t.Fatalf("entry = %+v, want the recorded fields", e)
	}
	if e.Timestamp != 1000 {
		t.Fatalf("timestampMs = %d, want 1000", e.Timestamp)
	}
}

func TestJournalList_NilJournalReturnsEmptyList(t *testing.T) {
	router := newJournalTestServer(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/journal", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Entries []struct{} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Entries) != 0 {
		t.Fatalf("entries = %+v, want empty", body.Entries)
	}
}
