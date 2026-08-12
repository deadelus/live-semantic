// Package output provides concrete streamer.OutputStream adapters:
// WindowOutput (native gocv window, CLI/desktop) and WebSocketOutput
// (browser-facing, this file).
package output

import (
	"bytes"
	"encoding/json"
	"image/jpeg"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"
	"sync"

	"github.com/gorilla/websocket"
)

var _ streamer.BoxAwareOutputStream = (*WebSocketOutput)(nil)

// WebSocketOutput broadcasts rendered frames to every connected client as
// binary JPEG messages, plus box/label/score data as a separate JSON text
// message (RenderBoxes, streamer.BoxAwareOutputStream, added 2026-08-12)
// so a client draws its own overlays instead of receiving
// them pre-composited into the image's pixels. History: the GUI backend
// prerequisites' minimal scope originally burned boxes into frame.Image
// server-side (the "MJPEG-like" fallback described in docs/gui/spec.md
// § 2) — good enough to validate the transport end-to-end, but
// click-to-select (reference gallery, docs/gui/mockups/ screen 1d) needs
// box geometry as data, not pixels.
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
	return w.broadcast(websocket.BinaryMessage, buf.Bytes())
}

// boxesMessage envelopes RenderBoxes' payload — a wrapper object rather
// than a bare JSON array, so a future second message type (e.g. per-
// stream metadata) can be added on this same channel without an
// ambiguous "is this array a boxes list or something else" client-side
// guess. Field is exported (encoding/json needs that) despite the type
// itself being unexported — only used internally to this file.
type boxesMessage struct {
	Boxes []streamer.BoxData `json:"boxes"`
}

// RenderBoxes implements streamer.BoxAwareOutputStream — broadcasts boxes
// as a JSON text message, same client list/dead-client handling as
// Render's binary frames (see broadcast). boxes is marshaled even when
// empty (an explicit "no detections this cycle" message, not silence —
// lets a client clear stale overlays instead of guessing why nothing
// arrived).
func (w *WebSocketOutput) RenderBoxes(boxes []streamer.BoxData) error {
	payload, err := json.Marshal(boxesMessage{Boxes: boxes})
	if err != nil {
		return err
	}
	return w.broadcast(websocket.TextMessage, payload)
}

// broadcast writes payload to every connected client, dropping (and
// closing) any whose write fails — a slow/dead consumer must never block
// the video loop. Shared by Render (binary JPEG) and RenderBoxes (JSON
// text) — same delivery semantics, only the message type differs.
func (w *WebSocketOutput) broadcast(messageType int, payload []byte) error {
	w.mu.RLock()
	var dead []*websocket.Conn
	for conn := range w.clients {
		if err := conn.WriteMessage(messageType, payload); err != nil {
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
// POST /api/v1/recognition/stop endpoint (uc.Recognition.StopRecognition()),
// never through this. Always returns -1 (no key), so Recognize's
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
