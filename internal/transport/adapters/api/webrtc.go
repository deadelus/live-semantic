package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pion/webrtc/v4"
)

// WebRTCSignaler performs one SDP offer/answer exchange — narrow
// interface, same rationale as FrameReceiver/FrameBroadcaster just above:
// this package only depends on the method it needs, not the concrete
// *implementation/streamer/input.WebRTCInput (main.go wires that in,
// satisfying this interface structurally, dependency inversion).
type WebRTCSignaler interface {
	HandleOffer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error)
}

// handleSessionWebRTCOffer handles POST /api/v1/sessions/:id/webrtc/offer
// — JSON body {"type":"offer","sdp":"..."}, response
// {"type":"answer","sdp":"..."} (docs/gui/spec.md § 2, TODO.md § H1
// "Ingestion WebRTC navigateur"). Non-trickle: the response already
// contains every gathered ICE candidate (WebRTCInput.HandleOffer waits
// for gathering to complete before returning) — a client doesn't need a
// separate candidate-exchange call. Type asserts session.Manager.Input(id)
// to WebRTCSignaler, which only holds for a session created with
// Source.Kind == "webrtc" — any other kind is rejected with a clear
// error, same pattern as handleSessionWebSocketIngest's FrameReceiver
// assertion just above.
func (s *Server) handleSessionWebRTCOffer(c *gin.Context) {
	id := c.Param("id")
	in, ok := s.sessions.manager.Input(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	signaler, ok := in.(WebRTCSignaler)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session isn't a webrtc-kind source, nothing to negotiate with"})
		return
	}

	var offer webrtc.SessionDescription
	if err := c.ShouldBindJSON(&offer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	answer, err := signaler.HandleOffer(offer)
	if err != nil {
		s.logger.Error("WebRTC offer/answer negotiation failed", map[string]interface{}{"session": id, "error": err.Error()})
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, answer)
}
