package notifier

import (
	"live-semantic/internal/domain/entities"
)

// AlertSender is the port for sending alerts about matched filters. Formerly
// named Notifier — renamed as part of the ports/dependency-inversion
// cleanup; the contract itself (Notify/Cleanup) was already adequate, no
// shape change.
type AlertSender interface {
	// Notify sends a notification for a given message.
	Notify(msg entities.Message) error
	// Cleanup performs any necessary cleanup operations.
	Cleanup()
}
