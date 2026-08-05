package api

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// setupRoutes configure les routes
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.healthCheck)

	// API routes
	api := s.router.Group("/api/v1")
	{
		api.GET("/realtime-analysis", func(c *gin.Context) {
			// Handle realtime analysis request
			// This should call the RealtimeAnalysisUseCase method from the useCases
			c.JSON(http.StatusOK, gin.H{"message": "Realtime analysis endpoint"})
		})
		// Add more routes as needed
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
