package notifier

import (
	"fmt"
	"live-semantic/src/domain/model"
)

// LogNotifier is a simple implementation of the Alerter interface that logs messages to the console.
type LogNotifier struct{}

// Notify implements the Notifier interface for LogNotifier.
func (n *LogNotifier) Notify(msg model.Message) error {
	// This is a placeholder for notification logic.
	// In a real implementation, you would format the message and send it to a notification service.
	fmt.Println("Notify:", msg)
	return nil
}

// NewLogNotifier creates a new instance of LogNotifier.
func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}
