// Package inference defines the port for inference (object detection, and
// future semantic encoding) functionalities.
package inference

import (
	"live-semantic/internal/domain/entities"
)

// DetectionResult represents the result of an object detection inference.
type DetectionResult struct {
	// Frame is the original image frame that was analyzed.
	Frame *entities.Frame
	// BoundingBoxes contains the detected bounding boxes in the frame.
	BoundingBoxes []entities.BoundingBox
}

// ObjectDetector defines all the methods required to run object detection inference.
type ObjectDetector interface {
	// AnalyzeFrame runs inference on a single frame and returns detected bounding boxes.
	AnalyzeFrame(frame *entities.Frame) (*DetectionResult, error)
	// Cleanup performs any necessary cleanup operations.
	Cleanup()
}
