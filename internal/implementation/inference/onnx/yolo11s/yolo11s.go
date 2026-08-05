// Package yolo11s implements the YOLOv11s neural network for object detection.
package yolo11s

import (
	"fmt"
	"live-semantic/internal/domain"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/inference/onnx/runtime"
	"live-semantic/internal/infrastructure/inference"

	"github.com/deadelus/go-clean-onnxruntime/src/onnx"
)

var (
	// yolo11sModelPath is the path to the YOLOv11s ONNX model file.
	yolo11sModelPath = "assets/models/yolo11s.onnx"
	// modelHeight and modelWidth are the dimensions to which input images are resized.
	// The YOLOv11s model expects input images to be 640x640 pixels.
	// https://docs.ultralytics.com/fr/tasks/detect/
	modelHeight = 640
	modelWidth  = 640
	// The model processes one image at a time
	batchSize = 1
	// modelInputChannels is the number of channels in the input image (3 for RGB).
	modelInputChannels = 3
	// modelClasses is the number of classes the model can detect.
	modelClasses = 80
	// modelDetections is the number of detections the model can output.
	modelDetections = 8400
	// thresholdConfidence is the minimum confidence threshold for detections.
	thresholdConfidence = 0.5
)

// Array of YOLOv8 class labels
// This is the list of classes that the YOLOv8 model can detect.
var yoloClasses = entities.Yolo11sClasses()

// Detector represents the YOLOv11s neural implementation.
type Detector struct {
	Session *onnx.ONNXSession
}

// New initializes the ONNX runtime for the YOLOv11s detector.
func New() *Detector {
	// Initialize the ONNX runtime with the model path and input/output shapes
	onnxRuntime := onnx.NewOnnxRuntime(
		yolo11sModelPath,
		runtime.LibraryPath(),
		onnx.TensorInputShape{
			BatchSize: int64(batchSize),
			Channels:  int64(modelInputChannels),
			Height:    int64(modelHeight),
			Width:     int64(modelWidth),
		},
		onnx.TensorOutputShape{
			BatchSize:  int64(batchSize),
			Classes:    int64(modelClasses + 4), // 4 for bounding box coordinates
			Detections: int64(modelDetections),
		},
	)
	if onnxRuntime == nil {
		fmt.Println(domain.ErrNilRuntime.Error(), "Detector", yolo11sModelPath)
	}

	// Create a new ONNX session
	session, err := onnx.NewONNXSession(onnxRuntime)
	if err != nil {
		fmt.Println(domain.ErrModelInitialization.Error(), "Detector", yolo11sModelPath, err)
	}
	return &Detector{
		Session: session,
	}
}

// AnalyzeFrame implements the ObjectDetector.AnalyzeFrame for Detector.
func (m *Detector) AnalyzeFrame(frame *entities.Frame) (*inference.DetectionResult, error) {
	processor := &onnx.Processor{
		Image:               frame.Image,
		ModelClasses:        yoloClasses,
		ModelHeight:         uint(modelHeight),
		ModelWidth:          uint(modelWidth),
		ModelInputChannels:  uint(modelInputChannels),
		ModelOutputClasses:  uint(modelClasses),
		ModelDetections:     uint(modelDetections),
		ThresholdConfidence: float32(thresholdConfidence),
	}

	err := processor.Input(m.Session.TensorInput)

	if err != nil {
		return nil, err
	}

	err = m.Session.Session.Run()
	if err != nil {
		return nil, err
	}

	boxes := processor.Output(m.Session.TensorOutput)

	return &inference.DetectionResult{
		Frame:         frame,
		BoundingBoxes: boxes,
	}, nil
}

// Cleanup implements the ObjectDetector.Cleanup for Detector.
func (m *Detector) Cleanup() {
	if m.Session != nil {
		m.Session.Close()
	}
	fmt.Println("Detector resources cleaned up successfully.")
}
