package api

import (
	"context"
	"net/http"
	"sync"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/application/uc"
	"live-semantic/internal/transport/envelopes"
	"live-semantic/internal/transport/handlers"

	"github.com/deadelus/go-clean-app/v2/logger"
	"github.com/gin-gonic/gin"
)

// recognitionController wires REST start/stop/status around the single,
// process-wide RecognitionUseCase run. H1 minimal scope (TODO.md § H1):
// one webcam session, no session IDs yet — uc.UseCase's streamingInput/
// streamingOutput are shared fields, not per-call (see TODO.md § H1
// "Multi-flux" for the follow-up that replaces this with per-ID sessions).
// The `running` guard here exists purely to reject a second concurrent
// start with a clear error instead of two goroutines fighting over the
// same InputStream.
type recognitionController struct {
	handler  *handlers.BaseHandler
	logger   logger.Logger
	useCases uc.UseCases

	mu      sync.Mutex
	running bool
}

func newRecognitionController(useCases uc.UseCases, logger logger.Logger) *recognitionController {
	return &recognitionController{
		handler:  handlers.NewBaseHandler(useCases, logger),
		logger:   logger,
		useCases: useCases,
	}
}

// start validates the request body against dto.RecognitionRequest, then
// runs RecognitionUseCase in a goroutine and returns immediately — it
// would otherwise block the HTTP response until the stream stops (webcam
// runs indefinitely), which isn't what a "start" endpoint should do.
func (rc *recognitionController) start(c *gin.Context) {
	var req dto.RecognitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rc.mu.Lock()
	if rc.running {
		rc.mu.Unlock()
		c.JSON(http.StatusConflict, gin.H{
			"error": "a recognition session is already running — stop it first (H1 minimal scope: one session at a time, see TODO.md § H1 Multi-flux)",
		})
		return
	}
	rc.running = true
	rc.mu.Unlock()

	go func() {
		defer func() {
			rc.mu.Lock()
			rc.running = false
			rc.mu.Unlock()
		}()

		resp := rc.handler.HandleRecognitionUseCase(envelopes.TransportRequest[dto.RecognitionRequest]{
			Data:    req,
			Context: context.Background(),
			Source:  "web",
		})
		if !resp.Success {
			rc.logger.Error("Recognition session ended with error", map[string]interface{}{"error": resp.Error})
			return
		}
		rc.logger.Info("Recognition session ended", nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status":               "started",
		"filter":               req.Filter,
		"similarity_threshold": req.SimilarityThreshold,
	})
}

// stop signals the running session to halt (uc.UseCases.Stop()) and
// returns immediately — the session finishes shortly after (see
// uc.UseCase.Stop's doc comment), the `running` flag flips back to false
// on its own once the start goroutine's deferred cleanup runs.
func (rc *recognitionController) stop(c *gin.Context) {
	rc.mu.Lock()
	running := rc.running
	rc.mu.Unlock()

	if !running {
		c.JSON(http.StatusConflict, gin.H{"error": "no recognition session running"})
		return
	}

	rc.useCases.Stop()
	c.JSON(http.StatusAccepted, gin.H{"status": "stopping"})
}

func (rc *recognitionController) status(c *gin.Context) {
	rc.mu.Lock()
	running := rc.running
	rc.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"running": running})
}
