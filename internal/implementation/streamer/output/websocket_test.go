package output

import (
	"encoding/json"
	"image"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newConnPair spins up a throwaway httptest.Server that upgrades exactly
// one connection, dials it, and hands back both ends — the server-side
// *websocket.Conn (what AddClient/Render operate on) and the client-side
// one (what a GUI would hold), plus a cleanup func.
func newConnPair(t *testing.T) (serverConn, clientConn *websocket.Conn, cleanup func()) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade failed: %v", err)
			return
		}
		connCh <- c
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("client dial failed: %v", err)
	}

	select {
	case serverConn = <-connCh:
	case <-time.After(2 * time.Second):
		clientConn.Close()
		srv.Close()
		t.Fatal("timed out waiting for server-side connection")
	}

	return serverConn, clientConn, func() {
		clientConn.Close()
		serverConn.Close()
		srv.Close()
	}
}

// testFrame is a tiny valid image, cheap to JPEG-encode.
func testFrame() *entities.Frame {
	return &entities.Frame{Image: image.NewRGBA(image.Rect(0, 0, 4, 4))}
}

func TestNewWebSocketOutput_StartsEmpty(t *testing.T) {
	wo := NewWebSocketOutput()
	if len(wo.clients) != 0 {
		t.Fatalf("clients = %d, want 0", len(wo.clients))
	}
	if err := wo.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
}

func TestAddClient_RegistersConnection(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn, streamer.DefaultClientOptions())

	if len(wo.clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(wo.clients))
	}
	if _, ok := wo.clients[serverConn]; !ok {
		t.Fatal("serverConn not registered in clients map")
	}
}

func TestRemoveClient_UnregistersAndCloses(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn, streamer.DefaultClientOptions())
	wo.RemoveClient(serverConn)

	if len(wo.clients) != 0 {
		t.Fatalf("clients = %d, want 0 after RemoveClient", len(wo.clients))
	}
	// The connection was closed by RemoveClient — writing to it now must fail.
	if err := serverConn.WriteMessage(websocket.BinaryMessage, []byte("x")); err == nil {
		t.Fatal("expected write to closed connection to fail")
	}
}

func TestRemoveClient_UnknownConnectionIsNoop(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	// Never added — must not panic, must not affect an unrelated client.
	other, _, otherCleanup := newConnPair(t)
	defer otherCleanup()
	wo.AddClient(other, streamer.DefaultClientOptions())

	wo.RemoveClient(serverConn)

	if len(wo.clients) != 1 {
		t.Fatalf("clients = %d, want 1 (unaffected)", len(wo.clients))
	}
}

func TestRender_BroadcastsToAllClients(t *testing.T) {
	wo := NewWebSocketOutput()

	server1, client1, cleanup1 := newConnPair(t)
	defer cleanup1()
	server2, client2, cleanup2 := newConnPair(t)
	defer cleanup2()

	wo.AddClient(server1, streamer.DefaultClientOptions())
	wo.AddClient(server2, streamer.DefaultClientOptions())

	if err := wo.Render(testFrame()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	for i, client := range []*websocket.Conn{client1, client2} {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		msgType, payload, err := client.ReadMessage()
		if err != nil {
			t.Fatalf("client %d: ReadMessage() error = %v", i, err)
		}
		if msgType != websocket.BinaryMessage {
			t.Fatalf("client %d: message type = %d, want BinaryMessage", i, msgType)
		}
		if len(payload) < 2 || payload[0] != 0xFF || payload[1] != 0xD8 {
			t.Fatalf("client %d: payload doesn't look like a JPEG (missing SOI marker)", i)
		}
	}
}

// TestRender_RespectsPerClientFPSCap — mosaic view (docs/gui/spec.md §
// 3.1, added 2026-08-14): a client subscribed with a low FPS cap must
// not receive every single Render call, only as often as its cap allows.
func TestRender_RespectsPerClientFPSCap(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, clientConn, cleanup := newConnPair(t)
	defer cleanup()

	// A very low cap (1 frame per 10 minutes) makes the test
	// deterministic: the first Render always sends (lastFrameAt is
	// zero), every subsequent one within the same test run must not.
	wo.AddClient(serverConn, streamer.ClientOptions{FPS: 1.0 / 600, Boxes: true})

	if err := wo.Render(testFrame()); err != nil {
		t.Fatalf("first Render() error = %v", err)
	}
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := clientConn.ReadMessage(); err != nil {
		t.Fatalf("first frame: ReadMessage() error = %v, want the first frame to always be sent", err)
	}

	if err := wo.Render(testFrame()); err != nil {
		t.Fatalf("second Render() error = %v", err)
	}
	clientConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, _, err := clientConn.ReadMessage(); err == nil {
		t.Fatal("second frame arrived despite the FPS cap — throttling isn't working")
	}
}

// TestRenderBoxes_SkipsClientsWithBoxesDisabled — mosaic tiles ask for
// Boxes:false (docs/gui/spec.md § 3.1's "aperture léger... sans boxes ni
// overlay") and must never receive a boxes message at all.
func TestRenderBoxes_SkipsClientsWithBoxesDisabled(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, clientConn, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn, streamer.ClientOptions{FPS: 0, Boxes: false})

	if err := wo.RenderBoxes([]streamer.BoxData{{ID: "person"}}); err != nil {
		t.Fatalf("RenderBoxes() error = %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, _, err := clientConn.ReadMessage(); err == nil {
		t.Fatal("client with Boxes:false received a boxes message")
	}
}

func TestRender_DropsClientOnWriteFailure(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn, streamer.DefaultClientOptions())
	// Close the connection directly (bypassing RemoveClient) to simulate a
	// client that disappeared without a clean unregister — Render must
	// detect the write failure and drop it on its own.
	serverConn.Close()

	if err := wo.Render(testFrame()); err != nil {
		t.Fatalf("Render() error = %v, want nil (per-client failures aren't fatal)", err)
	}

	if len(wo.clients) != 0 {
		t.Fatalf("clients = %d, want 0 (dead client should have been dropped)", len(wo.clients))
	}
}

func TestRenderBoxes_BroadcastsJSONToAllClients(t *testing.T) {
	wo := NewWebSocketOutput()
	server1, client1, cleanup1 := newConnPair(t)
	defer cleanup1()
	server2, client2, cleanup2 := newConnPair(t)
	defer cleanup2()

	wo.AddClient(server1, streamer.DefaultClientOptions())
	wo.AddClient(server2, streamer.DefaultClientOptions())

	boxes := []streamer.BoxData{
		{ID: "person", Label: "person (89.97%)", TrackID: "track-1", X1: 0.1, Y1: 0.2, X2: 0.3, Y2: 0.4},
	}
	if err := wo.RenderBoxes(boxes); err != nil {
		t.Fatalf("RenderBoxes() error = %v", err)
	}

	for i, client := range []*websocket.Conn{client1, client2} {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		msgType, payload, err := client.ReadMessage()
		if err != nil {
			t.Fatalf("client %d: ReadMessage() error = %v", i, err)
		}
		if msgType != websocket.TextMessage {
			t.Fatalf("client %d: message type = %d, want TextMessage", i, msgType)
		}
		var body struct {
			Boxes []streamer.BoxData `json:"boxes"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			t.Fatalf("client %d: failed to decode JSON: %v (payload: %s)", i, err, payload)
		}
		if len(body.Boxes) != 1 || body.Boxes[0].ID != "person" || body.Boxes[0].TrackID != "track-1" {
			t.Fatalf("client %d: decoded boxes = %+v, want the box sent", i, body.Boxes)
		}
	}
}

func TestRenderBoxes_EmptySliceStillSendsAMessage(t *testing.T) {
	// A client needs an explicit "no detections this cycle" message to
	// clear stale overlays — silence would be indistinguishable from a
	// dropped/slow connection.
	wo := NewWebSocketOutput()
	serverConn, clientConn, cleanup := newConnPair(t)
	defer cleanup()
	wo.AddClient(serverConn, streamer.DefaultClientOptions())

	if err := wo.RenderBoxes(nil); err != nil {
		t.Fatalf("RenderBoxes(nil) error = %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	var body struct {
		Boxes []streamer.BoxData `json:"boxes"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("failed to decode JSON: %v (payload: %s)", err, payload)
	}
	if len(body.Boxes) != 0 {
		t.Fatalf("decoded boxes = %+v, want empty", body.Boxes)
	}
}

func TestRenderBoxes_DropsClientOnWriteFailure(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn, streamer.DefaultClientOptions())
	serverConn.Close()

	if err := wo.RenderBoxes([]streamer.BoxData{{ID: "person"}}); err != nil {
		t.Fatalf("RenderBoxes() error = %v, want nil (per-client failures aren't fatal)", err)
	}
	if len(wo.clients) != 0 {
		t.Fatalf("clients = %d, want 0 (dead client should have been dropped)", len(wo.clients))
	}
}

func TestHandleKeyEvent_AlwaysReturnsNoKey(t *testing.T) {
	wo := NewWebSocketOutput()
	if got := wo.HandleKeyEvent(); got != -1 {
		t.Fatalf("HandleKeyEvent() = %d, want -1", got)
	}
}

func TestStop_DoesNotDisconnectClients(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn, streamer.DefaultClientOptions())
	wo.Stop()

	if len(wo.clients) != 1 {
		t.Fatalf("clients = %d, want 1 (Stop must not drop clients)", len(wo.clients))
	}
}

func TestCleanup_ClosesAllClients(t *testing.T) {
	wo := NewWebSocketOutput()

	server1, _, cleanup1 := newConnPair(t)
	defer cleanup1()
	server2, _, cleanup2 := newConnPair(t)
	defer cleanup2()

	wo.AddClient(server1, streamer.DefaultClientOptions())
	wo.AddClient(server2, streamer.DefaultClientOptions())

	wo.Cleanup()

	if len(wo.clients) != 0 {
		t.Fatalf("clients = %d, want 0 after Cleanup", len(wo.clients))
	}
	if err := server1.WriteMessage(websocket.BinaryMessage, []byte("x")); err == nil {
		t.Fatal("expected write to closed connection to fail after Cleanup")
	}
}
