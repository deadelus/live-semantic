// package displayhandler defines the interface for displaying frames in a video processing application.
package displayhandler

// Interface pour les différents types d'affichage
type DisplayHandler interface {
	// ProcessCommands processes display commands in a separate goroutine.
	ProcessCommands()
	// WaitKey waits for a key press and returns the key code.
	WaitKey(delay int) int
	// ShowFrame displays a frame.
	ShowFrame(frame []byte) error
	// IsActive returns true if the display handler is active.
	IsActive() bool
	// Close closes the display handler and releases resources.
	Close()
}
