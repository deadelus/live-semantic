package onnx

import (
	"fmt"
	"image"
)

// BoundingBox represents a rectangular region in an image, typically used for object detection results.
type BoundingBox struct {
	Label      string
	Confidence float32
	X1         float32
	Y1         float32
	X2         float32
	Y2         float32
}

// RectArea returns the area of the bounding box in pixels, after converting to an image.Rectangle.
func (b *BoundingBox) RectArea() int {
	size := b.ToRect().Size()
	return size.X * size.Y
}

// Intersection returns the intersection area of this bounding box with another bounding box.
// This is calculated by finding the intersection rectangle and returning its area.
// If the rectangles do not intersect, this will return 0.
func (b *BoundingBox) Intersection(other *BoundingBox) float32 {
	r1 := b.ToRect()
	r2 := other.ToRect()
	intersected := r1.Intersect(r2).Canon().Size()
	return float32(intersected.X * intersected.Y)
}

// Union returns the union area of this bounding box with another bounding box.
// This is calculated by adding the areas of both rectangles and subtracting the intersection area.
func (b *BoundingBox) Union(other *BoundingBox) float32 {
	intersectArea := b.Intersection(other)
	totalArea := float32(b.RectArea() + other.RectArea())
	return totalArea - intersectArea
}

// IoU returns the Intersection over Union (IoU) of this bounding box with another bounding box.
func (b *BoundingBox) IoU(other *BoundingBox) float32 {
	return b.Intersection(other) / b.Union(other)
}

// ToString returns a string representation of the BoundingBox.
func (b *BoundingBox) ToString() string {
	return fmt.Sprintf("Object %s (confidence %f): (%f, %f), (%f, %f)",
		b.Label, b.Confidence, b.X1, b.Y1, b.X2, b.Y2)
}

// ToRect converts the BoundingBox to an image.Rectangle.
func (b *BoundingBox) ToRect() image.Rectangle {
	return image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)).Canon()
}
