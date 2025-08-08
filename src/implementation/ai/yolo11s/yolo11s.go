// Package yolo11s implements the YOLOv11s neural network for object detection.
package yolo11s

import (
	"fmt"
	"live-semantic/src/domain"
	"live-semantic/src/domain/model"
	"live-semantic/src/infrastructure/ai"
	"path/filepath"
	"runtime"

	"github.com/deadelus/go-clean-onnxruntime/src/onnx"
)

var (
	// yolo11sModelPath is the path to the YOLOv11s ONNX model file.
	yolo11sModelPath = "src/implementation/ai/yolo11s/yolo11s.onnx"
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
var yoloClasses = model.Yolo11sClasses()

// Yolo11sNeuralNetwork represents the YOLOv11s neural implementation.
type Yolo11sNeuralNetwork struct {
	Session *onnx.ONNXSession
}

func getOnnxLibrary() string {
	var absPath string

	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			absPath, _ = filepath.Abs("src/libraries/win/onnxruntime.dll")
		}
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			absPath, _ = filepath.Abs("src/libraries/osx/onnxruntime_arm64.dylib")
		case "amd64":
			absPath, _ = filepath.Abs("src/libraries/osx/onnxruntime_amd64.dylib")
		}
	case "linux":
		switch runtime.GOARCH {
		case "arm64":
			absPath, _ = filepath.Abs("src/libraries/linux/onnxruntime_arm64.so")
		default:
			absPath, _ = filepath.Abs("src/libraries/linux/onnxruntime.so")
		}
	}

	if absPath == "" {
		panic("Unable to find a version of the onnxruntime library supporting this system.")
	}
	return absPath
}

// NewNeuralNetwork initializes the ONNX runtime for YOLOv11s model.
func NewNeuralNetwork() *Yolo11sNeuralNetwork {
	// Initialize the ONNX runtime with the model path and input/output shapes
	onnxRuntime := onnx.NewOnnxRuntime(
		yolo11sModelPath,
		getOnnxLibrary(),
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
		fmt.Println(domain.ErrNilRuntime.Error(), "Yolo11sNeuralNetwork", yolo11sModelPath)
	}

	// Create a new ONNX session
	session, err := onnx.NewONNXSession(onnxRuntime)
	if err != nil {
		fmt.Println(domain.ErrModelInitialization.Error(), "Yolo11sNeuralNetwork", yolo11sModelPath, err)
	}
	return &Yolo11sNeuralNetwork{
		Session: session,
	}
}

// AnalyzeFrame implements the AI.AnalyzeFrame for Yolo11sNeuralNetwork.
func (m *Yolo11sNeuralNetwork) AnalyzeFrame(frame *model.Frame) (*ai.DetectionResult, error) {
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

	return &ai.DetectionResult{
		Frame:         frame,
		BoundingBoxes: boxes,
	}, nil
}

// Cleanup implements the AI.Cleanup for Yolo11sNeuralNetwork.
func (m *Yolo11sNeuralNetwork) Cleanup() {
	if m.Session != nil {
		m.Session.Close()
	}
	fmt.Println("Yolo11sNeuralNetwork resources cleaned up successfully.")
}
