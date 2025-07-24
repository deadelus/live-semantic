package infrastructure

import "live-semantic/src/domain/models"

// Alerter is the interface for sending notifications about match events.
type Alerter interface {
	// Alert sends a notification for a given match event.
	Alert(event models.MatchEvent) error
}
