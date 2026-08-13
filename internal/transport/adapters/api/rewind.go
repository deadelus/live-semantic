package api

import (
	"net/http"
	"strconv"
	"time"

	"live-semantic/internal/infrastructure/streamer"

	"github.com/gin-gonic/gin"
)

// rewindBoxData mirrors websocket.go's own live-stream boxData shape
// (RawBoxData on the frontend, docs/adr/clip-backend.md § 32) — kept as
// its own local type rather than reusing streamer.BoxData directly so a
// JSON field rename on one side doesn't silently ripple into the other;
// same defensive posture already used for the live WS message.
type rewindBoxData struct {
	ID      string  `json:"ID"`
	Label   string  `json:"Label"`
	TrackID string  `json:"TrackID"`
	X1      float32 `json:"X1"`
	Y1      float32 `json:"Y1"`
	X2      float32 `json:"X2"`
	Y2      float32 `json:"Y2"`
}

// parseOffsetMs reads the offset_ms query parameter shared by every
// rewind endpoint below — missing/invalid defaults to 0 ("right now",
// the most recent buffered entry), not an error: a slightly-wrong rewind
// offset is a UX rounding concern, not something worth failing a request
// over.
func parseOffsetMs(c *gin.Context) time.Duration {
	ms, err := strconv.Atoi(c.Query("offset_ms"))
	if err != nil || ms < 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// rewindable looks up id's session output and type-asserts it to
// streamer.Rewindable — nil + false with the response already written if
// either step fails, same "session not found" / "wrong capability"
// two-step pattern as handleSessionWebSocketIngest's FrameReceiver
// assertion.
func (s *Server) rewindable(c *gin.Context, id string) (streamer.Rewindable, bool) {
	out, ok := s.sessions.manager.Output(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return nil, false
	}
	rw, ok := out.(streamer.Rewindable)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session output doesn't support rewind"})
		return nil, false
	}
	return rw, true
}

// handleSessionRewindRange handles GET /api/v1/sessions/:id/rewind/range
// — how far back (milliseconds) this session can currently rewind, grows
// from 0 right after the session starts up to the backend's configured
// retention window (cmd/livesemantic/main.go's rewindBufferDuration).
func (s *Server) handleSessionRewindRange(c *gin.Context) {
	rw, ok := s.rewindable(c, c.Param("id"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"rangeMs": rw.RewindRange().Milliseconds()})
}

// handleSessionRewindBoxes handles GET
// /api/v1/sessions/:id/rewind?offset_ms=N — the boxes that were current
// offset_ms ago, same JSON shape as the live WS boxes message so a client
// can reuse its existing parsing. See handleSessionRewindImage for the
// paired frame (two endpoints, not one — mirrors the live path's own
// binary-JPEG + separate-JSON split, docs/adr/clip-backend.md § 32).
func (s *Server) handleSessionRewindBoxes(c *gin.Context) {
	rw, ok := s.rewindable(c, c.Param("id"))
	if !ok {
		return
	}
	entry, found := rw.RewindAt(parseOffsetMs(c))
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "nothing buffered yet for this session"})
		return
	}

	boxes := make([]rewindBoxData, len(entry.Boxes))
	for i, b := range entry.Boxes {
		boxes[i] = rewindBoxData{ID: b.ID, Label: b.Label, TrackID: b.TrackID, X1: b.X1, Y1: b.Y1, X2: b.X2, Y2: b.Y2}
	}
	c.JSON(http.StatusOK, gin.H{"boxes": boxes, "actualAgeMs": entry.AgeAgo.Milliseconds()})
}

// handleSessionRewindImage handles GET
// /api/v1/sessions/:id/rewind/image?offset_ms=N — serves the buffered
// JPEG frame directly (real Content-Type, no base64/JSON wrapping), same
// convention as gallery.go's thumbnail endpoint, so a client can point a
// plain <img src="..."> at it.
func (s *Server) handleSessionRewindImage(c *gin.Context) {
	rw, ok := s.rewindable(c, c.Param("id"))
	if !ok {
		return
	}
	entry, found := rw.RewindAt(parseOffsetMs(c))
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "nothing buffered yet for this session"})
		return
	}
	c.Data(http.StatusOK, "image/jpeg", entry.JPEG)
}
