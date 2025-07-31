package model

import (
	"fmt"
	"image"
)

type BoundingBox struct {
	Label      string
	Confidence float32
	X1         float32
	Y1         float32
	X2         float32
	Y2         float32
}

// Returns the area of b in pixels, after converting to an image.Rectangle.
func (b *BoundingBox) RectArea() int {
	size := b.ToRect().Size()
	return size.X * size.Y
}

// Returns the intersection area of this bounding box with another bounding box.
// This is calculated by finding the intersection rectangle and returning its area.
// If the rectangles do not intersect, this will return 0.
// This is useful for determining how much two bounding boxes overlap.
func (b *BoundingBox) Intersection(other *BoundingBox) float32 {
	r1 := b.ToRect()
	r2 := other.ToRect()
	intersected := r1.Intersect(r2).Canon().Size()
	return float32(intersected.X * intersected.Y)
}

// Returns the union area of this bounding box with another bounding box.
// This is calculated by adding the areas of both rectangles and subtracting the intersection area.
// This is useful for determining how much two bounding boxes overlap in total.
func (b *BoundingBox) Union(other *BoundingBox) float32 {
	intersectArea := b.Intersection(other)
	totalArea := float32(b.RectArea() + other.RectArea())
	return totalArea - intersectArea
}

// Returns the Intersection over Union (IoU) of this bounding box with another bounding box.
// This is calculated as the intersection area divided by the union area.
// The IoU is a measure of how much two bounding boxes overlap.
// It ranges from 0 (no overlap) to 1 (perfect overlap).
// This is useful for determining how similar two bounding boxes are.
// This won't be entirely precise due to conversion to the integral rectangles
// from the image.Image library, but we're only using it to estimate which
// boxes are overlapping too much, so some imprecision should be OK.
func (b *BoundingBox) IoU(other *BoundingBox) float32 {
	return b.Intersection(other) / b.Union(other)
}

// String returns a string representation of the bounding box, including its label and coordinates.
// This is useful for debugging and logging purposes.
func (b *BoundingBox) ToString() string {
	return fmt.Sprintf("Object %s (confidence %f): (%f, %f), (%f, %f)",
		b.Label, b.Confidence, b.X1, b.Y1, b.X2, b.Y2)
}

// This loses precision, but recall that the boundingBox has already been
// scaled up to the original image's dimensions. So, it will only lose
// fractional pixels around the edges.
func (b *BoundingBox) ToRect() image.Rectangle {
	return image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)).Canon()
}
