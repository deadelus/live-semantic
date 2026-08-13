// Package drawer composites bounding boxes and labels directly onto image
// pixels (BoxDrawer) — the server-side rendering path used whenever the
// active OutputStream doesn't implement streamer.BoxAwareOutputStream
// (see that interface's doc comment for the alternative, client-side path).
package drawer

import (
	"image"
	"image/color"
)

// BoxID identifies a box's color/identity bucket — typically a filter
// term key (application/uc's trackedBox.FilterKey), used to look up a
// stable color via entities.BoxColor.
type BoxID string

// Box is one box to render: position, color, thickness and the label
// text already formatted by the caller (application/uc's boxDescription).
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

// RectArea returns the area of b in pixels, after converting to an
// image.Rectangle.
func (b *Box) RectArea() int {
	size := b.ToRect().Size()
	return size.X * size.Y
}

// ToRect converts b to an image.Rectangle. Loses fractional precision
// around the edges, which is acceptable since b's coordinates have
// already been scaled up to the original image's dimensions.
func (b *Box) ToRect() image.Rectangle {
	return image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)).Canon()
}
