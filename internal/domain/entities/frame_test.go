package entities

import (
	"image"
	"image/color"
	"testing"
)

// solidImage is a minimal image.Image that returns a fixed color for every
// pixel within bounds — enough to test cropping geometry without needing a
// real decoded image.
type solidImage struct {
	bounds image.Rectangle
	c      color.Color
}

func (s solidImage) ColorModel() color.Model { return color.RGBAModel }
func (s solidImage) Bounds() image.Rectangle { return s.bounds }
func (s solidImage) At(x, y int) color.Color { return s.c }

func TestFrameCropWithinBounds(t *testing.T) {
	src := solidImage{bounds: image.Rect(0, 0, 100, 100), c: color.RGBA{255, 0, 0, 255}}
	f := &Frame{Image: src, FrameNumber: 7}

	cropped, ok := f.Crop(BoundingBox{X1: 10, Y1: 20, X2: 40, Y2: 60})
	if !ok {
		t.Fatal("Crop() returned ok=false for a box fully within bounds")
	}

	gotBounds := cropped.Image.Bounds()
	wantW, wantH := 30, 40
	if gotBounds.Dx() != wantW || gotBounds.Dy() != wantH {
		t.Errorf("cropped bounds = %v (%dx%d), want %dx%d", gotBounds, gotBounds.Dx(), gotBounds.Dy(), wantW, wantH)
	}
	if cropped.FrameNumber != f.FrameNumber {
		t.Errorf("FrameNumber = %d, want %d (should be preserved)", cropped.FrameNumber, f.FrameNumber)
	}
}

func TestFrameCropClampsToSourceBounds(t *testing.T) {
	src := solidImage{bounds: image.Rect(0, 0, 100, 100), c: color.RGBA{0, 255, 0, 255}}
	f := &Frame{Image: src}

	// Box straddles the right/bottom edge — common right after a fresh
	// detection whose box wasn't clamped.
	cropped, ok := f.Crop(BoundingBox{X1: 80, Y1: 80, X2: 150, Y2: 150})
	if !ok {
		t.Fatal("Crop() returned ok=false for a box overlapping bounds")
	}

	gotBounds := cropped.Image.Bounds()
	if gotBounds.Dx() != 20 || gotBounds.Dy() != 20 {
		t.Errorf("cropped bounds = %v, want 20x20 (clamped to source)", gotBounds)
	}
}

func TestFrameCropEntirelyOutsideBoundsFails(t *testing.T) {
	src := solidImage{bounds: image.Rect(0, 0, 100, 100), c: color.RGBA{0, 0, 255, 255}}
	f := &Frame{Image: src}

	_, ok := f.Crop(BoundingBox{X1: 200, Y1: 200, X2: 250, Y2: 250})
	if ok {
		t.Error("Crop() returned ok=true for a box entirely outside bounds, want false")
	}
}

func TestFrameCropReadsCorrectPixels(t *testing.T) {
	// A 4x4 image where pixel (x,y) has color value x+y*4, to verify the
	// crop reads from the right offset, not just the right size.
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(x + y*4)})
		}
	}
	f := &Frame{Image: img}

	cropped, ok := f.Crop(BoundingBox{X1: 2, Y1: 1, X2: 4, Y2: 3})
	if !ok {
		t.Fatal("Crop() returned ok=false")
	}

	// cropped(0,0) should be source(2,1) = 2+1*4 = 6.
	r, g, b, _ := cropped.Image.At(0, 0).RGBA()
	want, _, _, _ := color.Gray{Y: 6}.RGBA()
	if r != want || g != want || b != want {
		t.Errorf("cropped.At(0,0) = %v, want gray value 6", r)
	}
}
