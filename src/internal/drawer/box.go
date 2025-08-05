package drawer

import (
	"image"
	"image/color"
)

type BoxID string

type Box struct {
	ID          BoxID
	Description string
	Color       color.Color
	Thickness   int
	X1          float32
	Y1          float32
	X2          float32
	Y2          float32
}

// Returns the area of b in pixels, after converting to an image.Rectangle.
func (b *Box) RectArea() int {
	size := b.ToRect().Size()
	return size.X * size.Y
}

// This loses precision, but recall that the boundingBox has already been
// scaled up to the original image's dimensions. So, it will only lose
// fractional pixels around the edges.
func (b *Box) ToRect() image.Rectangle {
	return image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)).Canon()
}
