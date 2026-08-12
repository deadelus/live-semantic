package api

import (
	"bytes"
	"image/jpeg"
	"net/http"
	"time"

	"live-semantic/internal/domain/entities"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// FrameBroadcaster registers/unregisters an upgraded WebSocket connection
// to receive frames pushed by the currently running OutputStream. A narrow
// interface (not the concrete implementation/streamer/output.WebSocketOutput
// type) so this transport package doesn't depend on that implementation
// package directly — main.go wires the concrete type in, satisfying this
// interface structurally.
type FrameBroadcaster interface {
	AddClient(conn *websocket.Conn)
	RemoveClient(conn *websocket.Conn)
}

// FrameReceiver accepts frames decoded from a browser's own camera feed
// (TODO.md § H2 "capture caméra navigateur") — narrow interface, same
// rationale as FrameBroadcaster: main.go wires in the concrete
// *implementation/streamer/input.BrowserInput, this package only depends
// on the method it needs.
type FrameReceiver interface {
	PushFrame(frame *entities.Frame)
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Dev-only: no origin check yet. H1 minimal scope (TODO.md § H1)
		// doesn't cover auth/CORS — revisit before anything but local use.
		return true
	},
}

// handleWebSocket upgrades the connection and registers it with the frame
// broadcaster so it starts receiving frames rendered by the running
// RecognitionUseCase. H1 minimal scope: read-only from the client's side —
// no control messages over WS yet, all control is REST (docs/gui/spec.md
// § 2). The read loop below exists only to detect disconnects (gorilla
// requires reading to notice a closed connection).
func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade to WebSocket", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	s.broadcaster.AddClient(conn)
	defer s.broadcaster.RemoveClient(conn)

	s.logger.Info("WebSocket client connected", nil)

	for {
		if _, _, err := conn.NextReader(); err != nil {
			s.logger.Info("WebSocket client disconnected", map[string]interface{}{
				"reason": err.Error(),
			})
			return
		}
	}
}

// handleWebSocketIngest is the reverse direction of handleWebSocket: a
// browser pushes its own camera feed (TODO.md § H2 "capture caméra
// navigateur") as one binary JPEG message per frame, decoded here and
// handed to frameReceiver (in practice input.BrowserInput.PushFrame) for
// RecognitionUseCase to consume like any other InputStream. Non-binary
// messages (text/ping) are ignored rather than erroring — a client
// shouldn't be disconnected over a stray control frame. A frame that
// fails to decode as JPEG is dropped with a log line, not fatal to the
// connection either (a single corrupted frame from a lossy capture loop
// shouldn't kill the whole session).
func (s *Server) handleWebSocketIngest(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade ingest connection to WebSocket", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	defer conn.Close()

	s.logger.Info("Browser camera ingest client connected", nil)

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			s.logger.Info("Browser camera ingest client disconnected", map[string]interface{}{
				"reason": err.Error(),
			})
			return
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			s.logger.Info("Failed to decode ingested frame, dropping it", map[string]interface{}{
				"error": err.Error(),
			})
			continue
		}
		s.frameReceiver.PushFrame(&entities.Frame{Image: img, Timestamp: time.Now()})
	}
}
