// Package ai defines the interface for AI-related functionalities.
package ai

import (
	"live-semantic/src/domain/model"

	"github.com/deadelus/go-clean-onnxruntime/src/onnx"
)

// DetectionResult represents the result of an object detection inference.
type DetectionResult struct {
	// Frame is the original image frame that was analyzed.
	Frame *model.Frame
	// BoundingBoxes contains the detected bounding boxes in the frame.
	BoundingBoxes []onnx.BoundingBox
}

// AI defines all the methods required for neural network stuff.
type AI interface {
	// AnalyzeFrame runs inference on a single frame and returns detected bounding boxes.
	AnalyzeFrame(frame *model.Frame) (*DetectionResult, error)
	// Cleanup performs any necessary cleanup operations.
	Cleanup()
}
