package api

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// setupRoutes configure les routes REST et WebSocket.
func (s *Server) setupRoutes() {
	s.router.GET("/health", s.healthCheck)

	// Video stream: annotated JPEG frames, one binary WS message per
	// rendered frame (H1 minimal scope — see websocket.go/websocket_output.go).
	s.router.GET("/ws", s.handleWebSocket)
	// Reverse direction (TODO.md § H2 "capture caméra navigateur"): a
	// browser pushes its own camera feed here, one binary JPEG message per
	// frame — see handleWebSocketIngest.
	s.router.GET("/ws/ingest", s.handleWebSocketIngest)

	// Multi-flux (TODO.md § H1) — same two message shapes as /ws and
	// /ws/ingest above, scoped to one session.Manager-owned session.
	s.router.GET("/ws/sessions/:id", s.handleSessionWebSocket)
	s.router.GET("/ws/sessions/:id/ingest", s.handleSessionWebSocketIngest)

	v1 := s.router.Group("/api/v1")
	{
		v1.POST("/recognition/start", s.recognition.start)
		v1.POST("/recognition/stop", s.recognition.stop)
		v1.GET("/recognition/status", s.recognition.status)

		// Reference gallery CRUD (TODO.md § D/§ H1, docs/adr/
		// clip-backend.md § 24) — see gallery.go.
		v1.POST("/gallery", s.gallery.add)
		v1.GET("/gallery", s.gallery.list)
		v1.DELETE("/gallery/:name", s.gallery.remove)
		v1.PATCH("/gallery/:name", s.gallery.update)

		// Multi-flux session CRUD (TODO.md § H1) — see sessions.go.
		v1.POST("/sessions", s.sessions.create)
		v1.GET("/sessions", s.sessions.list)
		v1.GET("/sessions/:id", s.sessions.get)
		v1.DELETE("/sessions/:id", s.sessions.remove)
		v1.POST("/sessions/:id/recognition/start", s.sessions.startRecognition)
		v1.POST("/sessions/:id/recognition/stop", s.sessions.stopRecognition)
	}
}

// healthCheck endpoint de santé
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": os.Getenv("APP_NAME") + " API Server",
		"version": os.Getenv("APP_VERSION"),
	})
}
