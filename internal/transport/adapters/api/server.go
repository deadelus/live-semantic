// Package api is the unified HTTP + WebSocket backend for the GUI (H1
// minimal scope, TODO.md § H1): REST control (recognition start/stop/
// status) and a single /ws endpoint streaming annotated JPEG frames, on
// one gin.Engine/one port.
//
// Deliberately one server for both, not two independent processes: the
// GUI needs REST control and live video against the *same* running
// session at once, which the previous split (this package + a standalone
// transport/adapters/websocket package, each with its own gin.Engine on
// its own port, never sharing state) couldn't express. See
// docs/gui/spec.md § 0 ("un seul backend Go") and § 2 (transport table).
package api

import (
	"fmt"
	"live-semantic/internal/application/uc"

	"github.com/deadelus/go-clean-app/v2/logger"
	"github.com/gin-gonic/gin"
)

// Server is the unified REST + WebSocket server.
type Server struct {
	useCases    uc.UseCases
	broadcaster FrameBroadcaster
	logger      logger.Logger
	port        int
	router      *gin.Engine
	recognition *recognitionController
}

// NewServer creates the unified server. broadcaster receives every
// upgraded WebSocket connection (see websocket.go) — in practice
// *implementation/streamer/output.WebSocketOutput, the same instance
// passed as uc.NewUseCase's streamingOutput, so frames rendered by the
// running RecognitionUseCase reach whoever is connected to /ws.
func NewServer(useCases uc.UseCases, broadcaster FrameBroadcaster, logger logger.Logger, port int) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	server := &Server{
		useCases:    useCases,
		broadcaster: broadcaster,
		logger:      logger,
		port:        port,
		router:      router,
		recognition: newRecognitionController(useCases, logger),
	}

	server.setupRoutes()
	return server
}

// Start runs the server, blocking until it exits (error or process
// termination) — same contract as the CLI/interactive modes' entry points.
func (s *Server) Start() error {
	s.logger.Info("Starting web server", map[string]interface{}{
		"port": s.port,
	})

	return s.router.Run(fmt.Sprintf(":%d", s.port))
}
