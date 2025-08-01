package notifier

import (
	"live-semantic/src/domain/model"
)

// Notifier is the interface for sending notifications about messages.
type Notifier interface {
	// Notify sends a notification for a given message.
	Notify(msg model.Message) error
}
