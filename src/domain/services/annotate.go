package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"live-semantic/src/domain/model"
	"math/rand"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// AnnotateImageWithBoundingBoxes draws bounding boxes and labels on the image.
func AnnotateImageWithBoundingBoxes(f *model.Frame, boxes []model.BoundingBox, thickness int, fontFace font.Face) *model.Frame {
	img, _, err := image.Decode(bytes.NewReader(f.ImageData))
	if err != nil {
		return nil // Handle error (e.g., log it)
	}

	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, img.Bounds(), img, image.Point{}, draw.Src)

	labelColor := map[string]color.Color{}
	for _, box := range boxes {
		if _, exists := labelColor[box.Label]; !exists {
			labelColor[box.Label] = color.RGBA{uint8(rand.Intn(256)), uint8(rand.Intn(256)), uint8(rand.Intn(256)), 255}
		}
		rect := box.ToRect()
		drawRect(out, rect, labelColor[box.Label], thickness)

		label := fmt.Sprintf("%s %.2f", box.Label, box.Confidence)
		drawLabel(out, rect.Min.X, rect.Min.Y-10, label, fontFace)
	}

	var buf bytes.Buffer

	switch f.ImageType {
	case model.ImageTypePNG:
		if err := png.Encode(&buf, out); err != nil {
			return nil // Handle error (e.g., log it)
		}
	default:
		if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 90}); err != nil {
			return nil // Handle error (e.g., log it)
		}
	}

	return &model.Frame{
		ImageData:   buf.Bytes(),
		ImageType:   f.ImageType,
		Width:       out.Bounds().Dx(),
		Height:      out.Bounds().Dy(),
		Timestamp:   f.Timestamp,
		FrameNumber: f.FrameNumber,
	}
}

// drawRect dessine un rectangle avec une épaisseur donnée.
func drawRect(img *image.RGBA, rect image.Rectangle, col color.Color, thickness int) {
	for t := 0; t < thickness; t++ {
		// Top
		for x := rect.Min.X + t; x < rect.Max.X-t; x++ {
			img.Set(x, rect.Min.Y+t, col)
		}
		// Bottom
		for x := rect.Min.X + t; x < rect.Max.X-t; x++ {
			img.Set(x, rect.Max.Y-1-t, col)
		}
		// Left
		for y := rect.Min.Y + t; y < rect.Max.Y-t; y++ {
			img.Set(rect.Min.X+t, y, col)
		}
		// Right
		for y := rect.Min.Y + t; y < rect.Max.Y-t; y++ {
			img.Set(rect.Max.X-1-t, y, col)
		}
	}
}

// drawLabel dessine un texte avec la police donnée.
func drawLabel(img *image.RGBA, x, y int, label string, face font.Face) {
	col := color.RGBA{255, 255, 0, 255} // Jaune
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  point,
	}
	d.DrawString(label)
}
