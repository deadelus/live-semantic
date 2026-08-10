// Package output provides concrete streamer.OutputStream adapters.
package output

import (
	"bytes"
	"image/jpeg"
	"live-semantic/internal/domain/entities"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketOutput broadcasts rendered frames to every connected client as
// binary JPEG messages. Boxes/labels/scores are already burned into
// frame.Image by RecognitionUseCase (uc_recognition.go) before Render is
// called — this matches the "MJPEG-like" fallback described in
// docs/gui/spec.md § 2, not the richer JPEG+JSON-per-message format also
// described there (boxes as structured data). H1 minimal scope
// (TODO.md § H1): good enough to validate the transport end-to-end;
// revisit once click-to-select (galerie de références) needs box
// geometry as data rather than pixels.
type WebSocketOutput struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

// NewWebSocketOutput creates a WebSocketOutput with no clients connected
// yet — clients register via AddClient as they open a WebSocket
// connection (see transport/adapters/api's /ws handler).
func NewWebSocketOutput() *WebSocketOutput {
	return &WebSocketOutput{
		clients: make(map[*websocket.Conn]struct{}),
	}
}

// AddClient registers a connection to start receiving frames.
func (w *WebSocketOutput) AddClient(conn *websocket.Conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clients[conn] = struct{}{}
}

// RemoveClient unregisters and closes a connection. Safe to call more than
// once for the same connection.
func (w *WebSocketOutput) RemoveClient(conn *websocket.Conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.clients[conn]; ok {
		delete(w.clients, conn)
		conn.Close()
	}
}

// Initialize implements streamer.OutputStream. No setup needed — clients
// connect independently of the recognition session's lifecycle.
func (w *WebSocketOutput) Initialize() error {
	return nil
}

// Render implements streamer.OutputStream: JPEG-encodes the frame once and
// broadcasts it to every connected client. A client whose write fails
// (slow consumer, closed connection) is dropped rather than allowed to
// block the video loop — one bad GUI client must never stall inference.
func (w *WebSocketOutput) Render(frame *entities.Frame) error {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, frame.Image, &jpeg.Options{Quality: 80}); err != nil {
		return err
	}
	payload := buf.Bytes()

	w.mu.RLock()
	var dead []*websocket.Conn
	for conn := range w.clients {
		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			dead = append(dead, conn)
		}
	}
	w.mu.RUnlock()

	if len(dead) > 0 {
		w.mu.Lock()
		for _, conn := range dead {
			delete(w.clients, conn)
			conn.Close()
		}
		w.mu.Unlock()
	}

	return nil
}

// HandleKeyEvent implements streamer.OutputStream. There's no keyboard
// concept over WebSocket — stopping a session goes through the REST
// POST /api/v1/recognition/stop endpoint (uc.UseCases.Stop()), never
// through this. Always returns -1 (no key), so RecognitionUseCase's
// "Escape to quit" check never fires here.
func (w *WebSocketOutput) HandleKeyEvent() int {
	return -1
}

// Stop implements streamer.OutputStream. Deliberately a no-op: connected
// clients should keep receiving frames until Cleanup, not be cut off the
// moment the input stream stops (e.g. so the GUI can show the final
// frame/state before the connection closes).
func (w *WebSocketOutput) Stop() {}

// Cleanup implements streamer.OutputStream: closes every connected client.
func (w *WebSocketOutput) Cleanup() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for conn := range w.clients {
		conn.Close()
		delete(w.clients, conn)
	}
}
