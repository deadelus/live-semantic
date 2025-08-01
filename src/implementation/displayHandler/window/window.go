// DisplayHandler .
package window

import (
	"errors"
	displayhandler "live-semantic/src/infrastructure/displayHandler"

	"gocv.io/x/gocv"
)

const (
	// windowName is the name of the display window for object recognition.
	windowName = "Output Stream"
)

const (
	StartCommand = "start"
	StopCommand  = "stop"
)

// Implementation for window display
type WindowDisplayHandler struct {
	windows  map[string]*gocv.Window
	commands chan DisplayCommand
	active   bool
}

type DisplayCommand struct {
	Action     string // "show", "close", "waitKey", "shutdown"
	WindowName string
	Frame      *gocv.Mat
	Delay      int
	Result     chan interface{}
}

// NewDisplayHandler creates a new display handler based on the useDisplay flag.
func NewDisplayHandler() displayhandler.DisplayHandler {
	return &WindowDisplayHandler{
		windows:  make(map[string]*gocv.Window),
		commands: make(chan DisplayCommand, 10), // Buffered channel to avoid blocking
		active:   true,
	}
}

func (w *WindowDisplayHandler) ProcessCommands() {
	defer func() {
		// Nettoyer toutes les fenêtres
		for _, window := range w.windows {
			window.Close()
		}
	}()

	for cmd := range w.commands {
		switch cmd.Action {
		case "show":
			// Créer la fenêtre si elle n'existe pas
			if _, exists := w.windows[cmd.WindowName]; !exists {
				w.windows[cmd.WindowName] = gocv.NewWindow(cmd.WindowName)
			}
			w.windows[cmd.WindowName].IMShow(*cmd.Frame)

		case "waitKey":
			key := gocv.WaitKey(cmd.Delay)
			cmd.Result <- key

		case "shutdown":
			close(w.commands)
			return
		}
	}
}

// ShowFrame implements the DisplayHandler.ShowFrame.
func (w *WindowDisplayHandler) ShowFrame(frame []byte) error {
	if !w.active {
		return errors.New("display handler is not active")
	}

	mat, err := gocv.IMDecode(frame, gocv.IMReadColor)
	if err != nil {
		return err
	}

	cmd := DisplayCommand{
		Action:     "show",
		WindowName: windowName,
		Frame:      &mat,
	}

	select {
	case w.commands <- cmd:
	default:
		// Drop frame if channel is full (avoid blocking)
	}

	return nil
}

func (w *WindowDisplayHandler) WaitKey(delay int) int {
	if !w.active {
		return -1
	}

	cmd := DisplayCommand{
		Action: "waitKey",
		Delay:  delay,
		Result: make(chan interface{}),
	}

	w.commands <- cmd
	result := <-cmd.Result
	return result.(int)
}

// IsActive implements the DisplayHandler.IsActive.
func (w *WindowDisplayHandler) IsActive() bool {
	return w.active
}

// Close implements the DisplayHandler.Close.
func (w *WindowDisplayHandler) Close() {
	if !w.active {
		return
	}

	w.active = false
	w.commands <- DisplayCommand{Action: "shutdown"}
}
