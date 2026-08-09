package entities

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"
)

// Frame represents a single frame from a video source or camera.
type Frame struct {
	// Image contains the raw image data (e.g., in JPEG or PNG format).
	Image image.Image
	// Timestamp is the time the frame was captured.
	Timestamp time.Time
	// FrameNumber is the sequential number of the frame in the stream.
	FrameNumber uint64
}

// ToFile saves the image data to a file with a specified prefix.
func (f *Frame) ToFile(path string, prefix string) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	newName := prefix + name + ext
	newPath := filepath.Join(dir, newName)

	outFile, err := os.Create(newPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	return jpeg.Encode(outFile, f.Image, &jpeg.Options{Quality: 90})
}

// Crop returns a new Frame restricted to box, clamped to f's own bounds (a
// box straddling the edge — common right after a fresh detection — is
// silently clipped rather than erroring). Returns false if the clamped
// region is empty (box entirely outside f, or zero-sized).
//
// Doesn't copy pixel data: croppedImage is a read-only view into the same
// underlying image.Image, offset by box's origin — cheap regardless of
// crop size, and doesn't depend on the source image's concrete type
// supporting SubImage (not all image.Image implementations do).
func (f *Frame) Crop(box BoundingBox) (*Frame, bool) {
	region := box.ToRect().Intersect(f.Image.Bounds())
	if region.Empty() {
		return nil, false
	}

	return &Frame{
		Image:       &croppedImage{src: f.Image, bounds: region},
		Timestamp:   f.Timestamp,
		FrameNumber: f.FrameNumber,
	}, true
}

// croppedImage is a read-only image.Image view into a region of another
// image.Image, without copying pixel data.
type croppedImage struct {
	src    image.Image
	bounds image.Rectangle
}

func (c *croppedImage) ColorModel() color.Model {
	return c.src.ColorModel()
}

func (c *croppedImage) Bounds() image.Rectangle {
	return image.Rect(0, 0, c.bounds.Dx(), c.bounds.Dy())
}

func (c *croppedImage) At(x, y int) color.Color {
	return c.src.At(c.bounds.Min.X+x, c.bounds.Min.Y+y)
}
