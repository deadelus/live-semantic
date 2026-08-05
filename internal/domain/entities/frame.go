package entities

import (
	"image"
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
