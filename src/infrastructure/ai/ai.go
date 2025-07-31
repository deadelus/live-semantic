// Package ai defines the interface for AI-related functionalities.
package ai

import (
	"live-semantic/src/domain/model"
)

// ObjectDetectionResult represents the result of an object detection inference.
type ObjectDetectionResult struct {
	// Frame is the original image frame that was analyzed.
	Frame *model.Frame
	// BoundingBoxes contains the detected bounding boxes in the frame.
	BoundingBoxes *[]model.BoundingBox
}

// AI defines all the methods required for neural network stuff.
type AI interface {
	// AnalyzeFrame runs inference on a single frame and returns detected bounding boxes.
	AnalyzeFrame(frame *model.Frame) (*ObjectDetectionResult, error)
}
