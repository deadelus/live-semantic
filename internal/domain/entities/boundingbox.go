package entities

import (
	"fmt"
	"image"
)

// BoundingBox represents a detected object's location, label and confidence.
type BoundingBox struct {
	Label      string
	Confidence float32
	X1         float32
	Y1         float32
	X2         float32
	Y2         float32
}

// RectArea returns the area of b in pixels, after converting to an image.Rectangle.
func (b *BoundingBox) RectArea() int {
	size := b.ToRect().Size()
	return size.X * size.Y
}

// Intersection returns the intersection area of this bounding box with another.
// Returns 0 if the boxes do not overlap.
func (b *BoundingBox) Intersection(other *BoundingBox) float32 {
	r1 := b.ToRect()
	r2 := other.ToRect()
	intersected := r1.Intersect(r2).Canon().Size()
	return float32(intersected.X * intersected.Y)
}

// Union returns the union area of this bounding box with another.
func (b *BoundingBox) Union(other *BoundingBox) float32 {
	intersectArea := b.Intersection(other)
	totalArea := float32(b.RectArea() + other.RectArea())
	return totalArea - intersectArea
}

// IoU returns the Intersection over Union of this bounding box with another,
// in [0, 1]. Used to measure overlap, e.g. for non-max suppression or
// track-to-detection association.
func (b *BoundingBox) IoU(other *BoundingBox) float32 {
	return b.Intersection(other) / b.Union(other)
}

// ToString returns a human-readable representation, useful for logging.
func (b *BoundingBox) ToString() string {
	return fmt.Sprintf("Object %s (confidence %f): (%f, %f), (%f, %f)",
		b.Label, b.Confidence, b.X1, b.Y1, b.X2, b.Y2)
}

// ToRect converts the bounding box to an image.Rectangle. Loses fractional
// precision around the edges, which is acceptable for overlap estimation.
func (b *BoundingBox) ToRect() image.Rectangle {
	return image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)).Canon()
}
