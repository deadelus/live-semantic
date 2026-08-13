package api

import (
	"context"
	"net/http"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/application/session"

	"github.com/deadelus/go-clean-app/v2/logger"
	"github.com/gin-gonic/gin"
)

// sessionController is the REST surface over session.Manager — the
// multi-flux counterpart to recognitionController, addressable by session
// ID rather than one process-wide session. Deliberately parallel to, not
// replacing, recognitionController:
// see session.go's package doc comment for why both currently coexist.
type sessionController struct {
	manager *session.Manager
	logger  logger.Logger
}

func newSessionController(manager *session.Manager, logger logger.Logger) *sessionController {
	return &sessionController{manager: manager, logger: logger}
}

// create builds a new session from the given Source (kind: "local" |
// "browser" | "file") and returns its Info — recognition doesn't start
// automatically, a client calls start next once it's ready (e.g. after
// opening the /ws/sessions/:id video connection so no frames are missed).
func (sc *sessionController) create(c *gin.Context) {
	var body struct {
		Source session.Source `json:"source"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := sc.manager.CreateSession(context.Background(), body.Source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, info)
}

// list returns every session's current state (docs/gui/mockups/ screens
// 1a/1b — the sources list/mosaic).
func (sc *sessionController) list(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"sessions": sc.manager.List()})
}

// get returns a single session's current state.
func (sc *sessionController) get(c *gin.Context) {
	id := c.Param("id")
	info, ok := sc.manager.Info(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, info)
}

// remove stops (if running) and forgets a session, releasing its
// input/output resources.
func (sc *sessionController) remove(c *gin.Context) {
	id := c.Param("id")
	if err := sc.manager.RemoveSession(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// startRecognition validates the request body against
// dto.RecognitionRequest and starts recognition on the given session —
// same "runs in a goroutine, returns immediately" contract as
// recognitionController.start.
func (sc *sessionController) startRecognition(c *gin.Context) {
	id := c.Param("id")

	var req dto.RecognitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Checked separately from StartRecognition's own error so "session
	// doesn't exist" (404) and "session exists but is already running"
	// (409) aren't collapsed into the same status code — found via manual
	// testing (2026-08-12): an unknown ID returned 409, misleading a
	// client into thinking the fix was to stop-then-retry.
	if _, ok := sc.manager.Info(id); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	if err := sc.manager.StartRecognition(context.Background(), id, req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "started", "filter": req.Filter})
}

// stopRecognition signals the session's running recognition to halt.
func (sc *sessionController) stopRecognition(c *gin.Context) {
	id := c.Param("id")
	if err := sc.manager.StopRecognition(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "stopping"})
}
