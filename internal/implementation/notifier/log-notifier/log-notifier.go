// Package lognotifier implements the notifier.AlertSender port by logging
// matched-filter messages to stdout — the only AlertSender adapter today,
// a placeholder ahead of a real notification backend (webhook, desktop
// toast, etc.).
package lognotifier

import (
	"fmt"
	"live-semantic/internal/domain/entities"
)

// LogNotifier implements notifier.AlertSender by printing every
// notification to stdout.
type LogNotifier struct{}

// NewLogNotifier constructs a LogNotifier.
func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}

// Notify implements notifier.AlertSender.Notify — writes msg to stdout,
// never fails (no I/O that can error).
func (n *LogNotifier) Notify(msg entities.Message) error {
	fmt.Println("Notify:", msg)
	return nil
}

// Cleanup implements notifier.AlertSender.Cleanup — no resources held, so
// this only logs that shutdown happened.
func (n *LogNotifier) Cleanup() {
	fmt.Println("LogNotifier resources cleaned up successfully.")
}
