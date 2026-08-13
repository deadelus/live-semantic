package input

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"live-semantic/internal/domain/entities"
)

// encodeTestJPEG builds a tiny valid JPEG — real bytes from the standard
// library's own encoder, not a hand-rolled fixture, so readJPEGStream is
// exercised against exactly the byte-stuffing behavior a real encoder
// (including ffmpeg's) produces.
func encodeTestJPEG(t *testing.T, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("jpeg.Encode() error = %v", err)
	}
	return buf.Bytes()
}

func TestReadJPEGStream_SplitsConsecutiveFrames(t *testing.T) {
	frame1 := encodeTestJPEG(t, color.RGBA{R: 255, A: 255})
	frame2 := encodeTestJPEG(t, color.RGBA{B: 255, A: 255})
	stream := append(append([]byte{}, frame1...), frame2...)

	var pushed []*entities.Frame
	readJPEGStream(bytes.NewReader(stream), func(f *entities.Frame) { pushed = append(pushed, f) })

	if len(pushed) != 2 {
		t.Fatalf("pushed %d frames, want 2", len(pushed))
	}
	for i, f := range pushed {
		if f.Image == nil {
			t.Fatalf("frame %d has a nil Image", i)
		}
	}
}

func TestReadJPEGStream_TruncatedTrailingFrameDropped(t *testing.T) {
	frame1 := encodeTestJPEG(t, color.RGBA{G: 255, A: 255})
	frame2 := encodeTestJPEG(t, color.RGBA{R: 128, G: 128, A: 255})
	// Cut the second frame short, before its EOI marker — must never
	// reach jpeg.Decode with a complete-looking-but-corrupt buffer that
	// panics or hangs the reader; simply dropped, first frame unaffected.
	truncated := frame2[:len(frame2)-4]
	stream := append(append([]byte{}, frame1...), truncated...)

	var pushed []*entities.Frame
	readJPEGStream(bytes.NewReader(stream), func(f *entities.Frame) { pushed = append(pushed, f) })

	if len(pushed) != 1 {
		t.Fatalf("pushed %d frames, want exactly 1 (truncated trailing frame must be dropped)", len(pushed))
	}
}

func TestReadJPEGStream_EmptyStreamPushesNothing(t *testing.T) {
	var pushed []*entities.Frame
	readJPEGStream(bytes.NewReader(nil), func(f *entities.Frame) { pushed = append(pushed, f) })

	if len(pushed) != 0 {
		t.Fatalf("pushed %d frames from an empty stream, want 0", len(pushed))
	}
}

func TestReadJPEGStream_GarbagePrefixIgnored(t *testing.T) {
	frame := encodeTestJPEG(t, color.RGBA{R: 10, G: 200, B: 30, A: 255})
	stream := append([]byte{0x00, 0x01, 0xFF, 0xD9, 0xAB}, frame...)

	var pushed []*entities.Frame
	readJPEGStream(bytes.NewReader(stream), func(f *entities.Frame) { pushed = append(pushed, f) })

	if len(pushed) != 1 {
		t.Fatalf("pushed %d frames, want 1 (garbage prefix containing a stray FFD9 must not desync the real frame)", len(pushed))
	}
}

func TestNewWebRTCInput_ImplementsInputStream(t *testing.T) {
	wi := NewWebRTCInput()
	if wi.BrowserInput == nil {
		t.Fatal("NewWebRTCInput() has a nil embedded BrowserInput")
	}
}
