package output

import (
	"image"
	"testing"
	"time"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"
)

func ringBufferTestFrame() *entities.Frame {
	return &entities.Frame{Image: image.NewRGBA(image.Rect(0, 0, 4, 4))}
}

func TestNewRingBufferOutput_RejectsNonBoxAwareInner(t *testing.T) {
	if _, err := NewRingBufferOutput(NewWindowOutput(), time.Minute); err == nil {
		t.Fatal("NewRingBufferOutput() with a non-BoxAwareOutputStream inner should error")
	}
}

func TestNewRingBufferOutput_AcceptsBoxAwareInner(t *testing.T) {
	if _, err := NewRingBufferOutput(NewWebSocketOutput(), time.Minute); err != nil {
		t.Fatalf("NewRingBufferOutput() error = %v", err)
	}
}

// TestRingBufferOutput_ForwardsFrameBroadcaster is a regression test for
// a real bug found in a live run (2026-08-13, same day this file was
// added): wrapping WebSocketOutput in RingBufferOutput without
// forwarding AddClient/RemoveClient made
// transport/adapters/api.handleSessionWebSocket's own
// `out.(FrameBroadcaster)` type assertion fail — no WS client ever got
// registered, so a session processed frames normally server-side but the
// GUI showed no video at all. This package can't import
// transport/adapters/api's own FrameBroadcaster interface (wrong
// dependency direction) to assert against directly, so this test checks
// the same thing structurally: AddClient/RemoveClient on the wrapper
// must actually reach the inner WebSocketOutput's client set.
func TestRingBufferOutput_ForwardsFrameBroadcaster(t *testing.T) {
	ws := NewWebSocketOutput()
	rb, err := NewRingBufferOutput(ws, time.Minute)
	if err != nil {
		t.Fatalf("NewRingBufferOutput() error = %v", err)
	}

	serverConn, _, cleanup := newConnPair(t)
	defer cleanup()

	rb.AddClient(serverConn)
	if len(ws.clients) != 1 {
		t.Fatalf("inner WebSocketOutput has %d clients after RingBufferOutput.AddClient(), want 1", len(ws.clients))
	}

	rb.RemoveClient(serverConn)
	if len(ws.clients) != 0 {
		t.Fatalf("inner WebSocketOutput has %d clients after RingBufferOutput.RemoveClient(), want 0", len(ws.clients))
	}
}

func TestRingBufferOutput_RewindRange_EmptyIsZero(t *testing.T) {
	rb, err := NewRingBufferOutput(NewWebSocketOutput(), time.Minute)
	if err != nil {
		t.Fatalf("NewRingBufferOutput() error = %v", err)
	}
	if got := rb.RewindRange(); got != 0 {
		t.Fatalf("RewindRange() on an empty buffer = %v, want 0", got)
	}
	if _, ok := rb.RewindAt(time.Second); ok {
		t.Fatal("RewindAt() on an empty buffer should return ok=false")
	}
}

func TestRingBufferOutput_RenderThenRewindAt_ReturnsClosestEntry(t *testing.T) {
	rb, err := NewRingBufferOutput(NewWebSocketOutput(), time.Minute)
	if err != nil {
		t.Fatalf("NewRingBufferOutput() error = %v", err)
	}

	if err := rb.RenderBoxes([]streamer.BoxData{{ID: "person", Label: "person (90%)"}}); err != nil {
		t.Fatalf("RenderBoxes() error = %v", err)
	}
	if err := rb.Render(ringBufferTestFrame()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := rb.Render(ringBufferTestFrame()); err != nil {
		t.Fatalf("second Render() error = %v", err)
	}

	entry, ok := rb.RewindAt(0)
	if !ok {
		t.Fatal("RewindAt(0) ok = false, want true right after rendering")
	}
	if len(entry.JPEG) == 0 {
		t.Fatal("RewindAt(0) returned an empty JPEG")
	}
	if len(entry.Boxes) != 1 || entry.Boxes[0].ID != "person" {
		t.Fatalf("RewindAt(0).Boxes = %+v, want the boxes set via RenderBoxes", entry.Boxes)
	}

	rangeAvailable := rb.RewindRange()
	if rangeAvailable <= 0 {
		t.Fatalf("RewindRange() = %v, want > 0 after two renders", rangeAvailable)
	}
}

func TestRingBufferOutput_EvictsEntriesOlderThanMaxAge(t *testing.T) {
	rb, err := NewRingBufferOutput(NewWebSocketOutput(), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewRingBufferOutput() error = %v", err)
	}

	if err := rb.Render(ringBufferTestFrame()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	time.Sleep(30 * time.Millisecond) // well past maxAge

	// A second Render triggers eviction of the now-stale first entry —
	// evictLocked runs inline on every Render, not on a separate ticker.
	if err := rb.Render(ringBufferTestFrame()); err != nil {
		t.Fatalf("second Render() error = %v", err)
	}

	if got := rb.RewindRange(); got > 10*time.Millisecond {
		t.Fatalf("RewindRange() = %v, want <= maxAge (10ms) after the stale entry was evicted", got)
	}
}

func TestRingBufferOutput_Cleanup_ClearsBuffer(t *testing.T) {
	rb, err := NewRingBufferOutput(NewWebSocketOutput(), time.Minute)
	if err != nil {
		t.Fatalf("NewRingBufferOutput() error = %v", err)
	}
	if err := rb.Render(ringBufferTestFrame()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	rb.Cleanup()
	if got := rb.RewindRange(); got != 0 {
		t.Fatalf("RewindRange() after Cleanup() = %v, want 0", got)
	}
}
