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
// process-wide Recognize run. Current minimal scope: one webcam
// session, no session IDs yet — uc.UseCase's streamingInput/
// streamingOutput are shared fields, not per-call (see the multi-flux
// work for the follow-up that replaces this with per-ID sessions).
// The `running` guard here exists purely to reject a second concurrent
// start with a clear error instead of two goroutines fighting over the
// same InputStream. Depends on uc.Recognition, not the wider uc.UseCases
// (interface segregation, 2026-08-12) — this controller never touches
// the gallery.
type recognitionController struct {
	handler  *handlers.BaseHandler
	logger   logger.Logger
	useCases uc.Recognition

	mu        sync.Mutex
	running   bool
	lastError string
}

func newRecognitionController(useCases uc.Recognition, logger logger.Logger) *recognitionController {
	return &recognitionController{
		handler:  handlers.NewBaseHandler(useCases, logger),
		logger:   logger,
		useCases: useCases,
	}
}

// start validates the request body against dto.RecognitionRequest, then
// runs Recognize in a goroutine and returns immediately — it
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
			"error": "a recognition session is already running — stop it first (current minimal scope: one session at a time, see the multi-flux work)",
		})
		return
	}
	rc.running = true
	rc.lastError = "" // clear any error from a previous session — see status's doc comment
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
			rc.mu.Lock()
			rc.lastError = resp.Error
			rc.mu.Unlock()
			return
		}
		rc.logger.Info("Recognition session ended", nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status": "started",
		"filter": req.Filter,
	})
}

// stop signals the running session to halt (uc.Recognition.StopRecognition())
// and returns immediately — the session finishes shortly after (see
// uc.UseCase.StopRecognition's doc comment), the `running` flag flips
// back to false on its own once the start goroutine's deferred cleanup
// runs.
func (rc *recognitionController) stop(c *gin.Context) {
	rc.mu.Lock()
	running := rc.running
	rc.mu.Unlock()

	if !running {
		c.JSON(http.StatusConflict, gin.H{"error": "no recognition session running"})
		return
	}

	rc.useCases.StopRecognition()
	c.JSON(http.StatusAccepted, gin.H{"status": "stopping"})
}

// status reports whether a session is running and, if the most recent
// one ended in an error (e.g. an invalid filter), that error — fixed
// 2026-08-12 (the REST API didn't use to surface an invalid filter to the
// client at all), now that the web frontend gives a client a real reason
// to poll this (docs/adr/clip-backend.md § 26 onward): start() used to return 202
// immediately regardless of whether the filter was valid, and the
// failure only ever surfaced in server logs — a GUI had no way to show
// the user why nothing happened. lastError is cleared at the start of
// the *next* start() call, not here, so it survives across polls until
// superseded by a new attempt.
func (rc *recognitionController) status(c *gin.Context) {
	rc.mu.Lock()
	running := rc.running
	lastError := rc.lastError
	rc.mu.Unlock()

	body := gin.H{"running": running}
	if lastError != "" {
		body["error"] = lastError
	}
	c.JSON(http.StatusOK, body)
}
