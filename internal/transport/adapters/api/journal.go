package api

import (
	"net/http"

	"live-semantic/internal/application/session"
	"live-semantic/internal/infrastructure/journal"

	"github.com/gin-gonic/gin"
)

// journalEntry is journal.Entry's REST shape — explicit JSON tags (not a
// bare re-export) so the wire format doesn't silently change if the
// domain type's field names ever do, same convention as every other
// REST-facing type in this package.
type journalEntry struct {
	Timestamp int64   `json:"timestampMs"`
	SessionID string  `json:"sessionId"`
	Type      string  `json:"type"`
	TrackID   string  `json:"trackId"`
	Class     string  `json:"class"`
	Score     float32 `json:"score,omitempty"`
}

// journalController exposes the aggregated multi-flux event log
// (docs/gui/mockups/ screen 1b "Journal des événements", TODO.md § H1,
// added 2026-08-14) — GET-only, there's nothing to create/update/delete
// here, entries are written exclusively by application/uc/tracking.go's
// emit(). Depends on *session.Manager directly (not a narrow interface)
// since Journal() is its only relevant method and every other controller
// in this file already depends on the concrete Manager the same way
// (sessionController) — no port to invert here, session.Manager already
// is the composition root's own type for this concern.
type journalController struct {
	manager *session.Manager
}

func newJournalController(manager *session.Manager) *journalController {
	return &journalController{manager: manager}
}

// list handles GET /api/v1/journal — every currently-retained entry,
// newest first (journal.Journal.List's own contract), across every
// session sharing this backend's journal.Journal instance. A client
// filters by sessionId itself (docs/gui/mockups/ screen 1b's own "Toutes
// les sources / Entrée principale / ..." pills) — no server-side
// filtering parameter yet, the whole list is small (maxEntries = 500,
// implementation/journal/inmemory) and cheap to filter client-side.
func (jc *journalController) list(c *gin.Context) {
	j := jc.manager.Journal()
	if j == nil {
		c.JSON(http.StatusOK, gin.H{"entries": []journalEntry{}})
		return
	}

	entries := j.List()
	out := make([]journalEntry, len(entries))
	for i, e := range entries {
		out[i] = toJournalEntry(e)
	}
	c.JSON(http.StatusOK, gin.H{"entries": out})
}

func toJournalEntry(e journal.Entry) journalEntry {
	return journalEntry{
		Timestamp: e.Timestamp.UnixMilli(),
		SessionID: e.SessionID,
		Type:      e.Type,
		TrackID:   e.TrackID,
		Class:     e.Class,
		Score:     e.Score,
	}
}
