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
	"live-semantic/internal/application/session"
	"live-semantic/internal/application/uc"

	"github.com/deadelus/go-clean-app/v2/logger"
	"github.com/gin-gonic/gin"
)

// Server is the unified REST + WebSocket server.
type Server struct {
	useCases      uc.UseCases
	broadcaster   FrameBroadcaster
	frameReceiver FrameReceiver
	logger        logger.Logger
	port          int
	router        *gin.Engine
	recognition   *recognitionController
	gallery       *galleryController
	sessions      *sessionController
}

// NewServer creates the unified server. broadcaster receives every
// upgraded WebSocket connection (see websocket.go) — in practice
// *implementation/streamer/output.WebSocketOutput, the same instance
// passed as uc.NewUseCase's streamingOutput, so frames rendered by the
// running RecognitionUseCase reach whoever is connected to /ws.
// frameReceiver is the reverse direction (TODO.md § H2 "capture caméra
// navigateur") — in practice *implementation/streamer/input.BrowserInput,
// the same instance passed as one of uc.NewUseCase's InputStream options.
// sessionManager backs the parallel, per-ID multi-flux surface (TODO.md
// § H1 "Multi-flux" — see sessions.go/session.go's package doc comment
// for why it coexists with the single-session useCases/broadcaster/
// frameReceiver above rather than replacing them).
func NewServer(useCases uc.UseCases, broadcaster FrameBroadcaster, frameReceiver FrameReceiver, sessionManager *session.Manager, logger logger.Logger, port int) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	server := &Server{
		useCases:      useCases,
		broadcaster:   broadcaster,
		frameReceiver: frameReceiver,
		logger:        logger,
		port:          port,
		router:        router,
		recognition:   newRecognitionController(useCases, logger),
		gallery:       newGalleryController(useCases, logger),
		sessions:      newSessionController(sessionManager, logger),
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
