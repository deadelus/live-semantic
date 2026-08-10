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

	v1 := s.router.Group("/api/v1")
	{
		v1.POST("/recognition/start", s.recognition.start)
		v1.POST("/recognition/stop", s.recognition.stop)
		v1.GET("/recognition/status", s.recognition.status)
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
