package api

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// setupRoutes registers every REST and WebSocket route on s.router.
func (s *Server) setupRoutes() {
	s.router.GET("/health", s.healthCheck)

	// Video stream: annotated JPEG frames, one binary WS message per
	// rendered frame (see websocket.go/websocket_output.go).
	s.router.GET("/ws", s.handleWebSocket)
	// Reverse direction (browser camera capture): a
	// browser pushes its own camera feed here, one binary JPEG message per
	// frame — see handleWebSocketIngest.
	s.router.GET("/ws/ingest", s.handleWebSocketIngest)

	// Multi-flux — same two message shapes as /ws and
	// /ws/ingest above, scoped to one session.Manager-owned session.
	s.router.GET("/ws/sessions/:id", s.handleSessionWebSocket)
	s.router.GET("/ws/sessions/:id/ingest", s.handleSessionWebSocketIngest)

	v1 := s.router.Group("/api/v1")
	{
		// Local camera device discovery (docs/gui/ device picker) — see
		// devices.go for why this is index-probing, not a true enumeration.
		v1.GET("/devices", s.devices.list)

		v1.POST("/recognition/start", s.recognition.start)
		v1.POST("/recognition/stop", s.recognition.stop)
		v1.GET("/recognition/status", s.recognition.status)

		// Reference gallery CRUD (docs/adr/
		// clip-backend.md § 24) — see gallery.go.
		v1.POST("/gallery", s.gallery.add)
		v1.GET("/gallery", s.gallery.list)
		v1.DELETE("/gallery/:name", s.gallery.remove)
		v1.PATCH("/gallery/:name", s.gallery.update)
		// Per-image routes (2026-08-13, multi-image entries) — see
		// gallery.go's removeImage/thumbnail doc comments.
		v1.DELETE("/gallery/:name/images/:imageID", s.gallery.removeImage)
		v1.GET("/gallery/:name/images/:imageID", s.gallery.thumbnail)

		// Bibliothèque — Collections CRUD (docs/gui/design-brief.md §
		// Bibliothèque, 2026-08-13) — see collections.go. Grouping only,
		// references Terms above by name (:term), never owns their data.
		v1.POST("/collections", s.collections.add)
		v1.GET("/collections", s.collections.list)
		v1.DELETE("/collections/:name", s.collections.remove)
		v1.PATCH("/collections/:name", s.collections.update)
		v1.POST("/collections/:name/terms/:term", s.collections.addTerm)
		v1.DELETE("/collections/:name/terms/:term", s.collections.removeTerm)

		// Multi-flux session CRUD — see sessions.go.
		v1.POST("/sessions", s.sessions.create)
		v1.GET("/sessions", s.sessions.list)
		v1.GET("/sessions/:id", s.sessions.get)
		v1.DELETE("/sessions/:id", s.sessions.remove)
		v1.POST("/sessions/:id/recognition/start", s.sessions.startRecognition)
		v1.POST("/sessions/:id/recognition/stop", s.sessions.stopRecognition)
	}
}

// healthCheck reports basic liveness/version info for monitoring.
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": os.Getenv("APP_NAME") + " API Server",
		"version": os.Getenv("APP_VERSION"),
	})
}
