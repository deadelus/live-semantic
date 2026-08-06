// Package lognotifier provides a simple implementation of the AlertSender interface that logs messages to the console.
package lognotifier

import (
	"fmt"
	"live-semantic/internal/domain/entities"
)

// LogNotifier is a simple implementation of the AlertSender interface that logs messages to the console.
type LogNotifier struct{}

// NewLogNotifier creates a new instance of LogNotifier.
func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}

// Notify implements the AlertSender interface for LogNotifier.
func (n *LogNotifier) Notify(msg entities.Message) error {
	// This is a placeholder for notification logic.
	// In a real implementation, you would format the message and send it to a notification service.
	fmt.Println("Notify:", msg)
	return nil
}

// Cleanup implements the AlertSender interface for LogNotifier.
func (n *LogNotifier) Cleanup() {
	fmt.Println("LogNotifier resources cleaned up successfully.")
}
