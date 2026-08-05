package notifier

import (
	"live-semantic/internal/domain/entities"
)

// Notifier is the interface for sending notifications about messages.
type Notifier interface {
	// Notify sends a notification for a given message.
	Notify(msg entities.Message) error
	// Cleanup performs any necessary cleanup operations.
	Cleanup()
}
