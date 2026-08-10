package output

import (
	"image"
	"live-semantic/internal/domain/entities"
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

	wo.AddClient(serverConn)

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

	wo.AddClient(serverConn)
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
	wo.AddClient(other)

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

	wo.AddClient(server1)
	wo.AddClient(server2)

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

func TestRender_DropsClientOnWriteFailure(t *testing.T) {
	wo := NewWebSocketOutput()
	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	wo.AddClient(serverConn)
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

	wo.AddClient(serverConn)
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

	wo.AddClient(server1)
	wo.AddClient(server2)

	wo.Cleanup()

	if len(wo.clients) != 0 {
		t.Fatalf("clients = %d, want 0 after Cleanup", len(wo.clients))
	}
	if err := server1.WriteMessage(websocket.BinaryMessage, []byte("x")); err == nil {
		t.Fatal("expected write to closed connection to fail after Cleanup")
	}
}
