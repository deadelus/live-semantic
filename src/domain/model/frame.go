package model

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

const ImageTypeJPEG = "image/jpeg"
const ImageTypePNG = "image/png"

// Frame represents a single frame from a video source or camera.
type Frame struct {
	// ImageData contains the raw image data (e.g., in JPEG or PNG format).
	ImageData []byte
	// ImageType is the MIME type of the image data (e.g., "image/jpeg", "image/png").
	ImageType string // e.g., "image/jpeg", "image/png"
	// Width of the image
	Width int
	// Height of the image
	Height int
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

	switch f.ImageType {
	case ImageTypePNG:
		out, err := f.Image()
		if err != nil {
			return err
		}
		return png.Encode(outFile, out)
	default:
		out, err := f.Image()
		if err != nil {
			return err
		}
		return jpeg.Encode(outFile, out, &jpeg.Options{Quality: 90})
	}
}

// Image returns the image representation of the frame.
// It decodes the image data based on the image type.
func (f *Frame) Image() (image.Image, error) {
	switch f.ImageType {
	case ImageTypePNG:
		return png.Decode(bytes.NewReader(f.ImageData))
	default:
		return jpeg.Decode(bytes.NewReader(f.ImageData))
	}
}
