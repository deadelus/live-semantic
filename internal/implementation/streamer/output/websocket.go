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
	"time"

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
//
// Per-client streamer.ClientOptions (added 2026-08-14, mosaic view) —
// each connection can ask for a capped FPS and/or no boxes, independent
// of every other connection on the same broadcaster (a mosaic tile and
// the full Vue live tab for the very same session, open at once, get
// different treatment from the same Render/RenderBoxes calls).
type WebSocketOutput struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*clientState
}

type clientState struct {
	opts        streamer.ClientOptions
	lastFrameAt time.Time // zero until the first frame actually sent — always sends the first one
}

// NewWebSocketOutput creates a WebSocketOutput with no clients connected
// yet — clients register via AddClient as they open a WebSocket
// connection (see transport/adapters/api's /ws handler).
func NewWebSocketOutput() *WebSocketOutput {
	return &WebSocketOutput{
		clients: make(map[*websocket.Conn]*clientState),
	}
}

// AddClient registers a connection to start receiving frames, subject to
// opts (streamer.ClientOptions's own doc comment) — pass
// streamer.DefaultClientOptions() for the pre-2026-08-14 behavior
// (unlimited FPS, boxes included).
func (w *WebSocketOutput) AddClient(conn *websocket.Conn, opts streamer.ClientOptions) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clients[conn] = &clientState{opts: opts}
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
// broadcasts it to every connected client whose own FPS cap allows a
// frame right now (streamer.ClientOptions.FPS — 0 means every call). A
// client whose write fails (slow consumer, closed connection) is dropped
// rather than allowed to block the video loop — one bad GUI client must
// never stall inference.
func (w *WebSocketOutput) Render(frame *entities.Frame) error {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, frame.Image, &jpeg.Options{Quality: 80}); err != nil {
		return err
	}
	now := time.Now()
	return w.broadcast(websocket.BinaryMessage, buf.Bytes(), func(st *clientState) bool {
		if st.opts.FPS <= 0 {
			return true
		}
		minInterval := time.Duration(float64(time.Second) / st.opts.FPS)
		if st.lastFrameAt.IsZero() || now.Sub(st.lastFrameAt) >= minInterval {
			st.lastFrameAt = now
			return true
		}
		return false
	})
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
// as a JSON text message to every client whose streamer.ClientOptions.
// Boxes is true (a mosaic tile's own aperture léger explicitly excludes
// them), same client list/dead-client handling as Render's binary frames
// (see broadcast). boxes is marshaled even when empty (an explicit "no
// detections this cycle" message, not silence — lets a client clear
// stale overlays instead of guessing why nothing arrived).
func (w *WebSocketOutput) RenderBoxes(boxes []streamer.BoxData) error {
	payload, err := json.Marshal(boxesMessage{Boxes: boxes})
	if err != nil {
		return err
	}
	return w.broadcast(websocket.TextMessage, payload, func(st *clientState) bool {
		return st.opts.Boxes
	})
}

// broadcast writes payload to every connected client for which send
// returns true, dropping (and closing) any whose write fails — a
// slow/dead consumer must never block the video loop. Shared by Render
// (binary JPEG) and RenderBoxes (JSON text) — same delivery semantics,
// only the message type and per-client gating predicate differ.
func (w *WebSocketOutput) broadcast(messageType int, payload []byte, send func(*clientState) bool) error {
	w.mu.Lock() // write lock, not read: send() may mutate st.lastFrameAt
	var dead []*websocket.Conn
	for conn, st := range w.clients {
		if !send(st) {
			continue
		}
		if err := conn.WriteMessage(messageType, payload); err != nil {
			dead = append(dead, conn)
		}
	}
	for _, conn := range dead {
		delete(w.clients, conn)
		conn.Close()
	}
	w.mu.Unlock()

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
