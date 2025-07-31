package onnx

import (
	"fmt"
	"image"
	"live-semantic/src/domain"
	"live-semantic/src/domain/model"
	"live-semantic/src/implementation/ai/onnx"
	"live-semantic/src/infrastructure/ai"
	"sort"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	// Yolo11sModelPath is the path to the YOLOv11s ONNX model file.
	Yolo11sModelPath = "src/implementation/ai/onnx/models/yolo11s.onnx"
)

// Array of YOLOv8 class labels
// This is the list of classes that the YOLOv8 model can detect.
var yoloClasses = []string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
	"traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat", "dog", "horse",
	"sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack", "umbrella", "handbag", "tie",
	"suitcase", "frisbee", "skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
	"skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup", "fork", "knife", "spoon",
	"bowl", "banana", "apple", "sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut",
	"cake", "chair", "couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
	"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink", "refrigerator", "book",
	"clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
}

// Yolo11sNeuralNetwork represents the YOLOv11s neural implementation.
type Yolo11sNeuralNetwork struct {
	Session *onnx.ONNXSession
}

// NewYolo11sNeuralNetwork initializes the ONNX runtime for YOLOv11s model.
func NewYolo11sNeuralNetwork() (*Yolo11sNeuralNetwork, error) {
	// Initialize the ONNX runtime with the model path and input/output shapes
	onnxRuntime := onnx.NewOnnxRuntime(
		Yolo11sModelPath,
		"", // No specific library path needed for this model
		onnx.TensorInputShape{
			BatchSize: 1,
			Channels:  3,
			Height:    640,
			Width:     640,
		},
		onnx.TensorOutputShape{
			BatchSize:  1,
			Classes:    84,   // Adjust based on the number of classes in your model
			Detections: 8400, // Adjust based on the model's output shape
		},
	)
	if onnxRuntime == nil {
		return nil, domain.ErrModelInitialization
	}

	// Create a new ONNX session
	session, err := onnx.NewONNXSession(onnxRuntime)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}
	return &Yolo11sNeuralNetwork{
		Session: session,
	}, nil
}

// AnalyzeFrame implements the AI interface for Yolo11sNeuralNetwork.
func (m *Yolo11sNeuralNetwork) AnalyzeFrame(frame *model.Frame) (*ai.ObjectDetectionResult, error) {

	img, err := frame.Image()
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame image: %w", err)
	}

	err = preProcessFrame(img, m.Session.TensorInput)
	if err != nil {
		return nil, err
	}

	err = m.Session.Session.Run()
	if err != nil {
		return nil, err
	}

	output := m.Session.TensorOutput.GetData()
	boxes := postProcessOutput(output, frame.Width, frame.Height)

	return &ai.ObjectDetectionResult{
		Frame:         frame,
		BoundingBoxes: boxes,
	}, nil
}

// preProcessFrame prepares the input frame for the model.
// It resizes the image to 640x640 and normalizes the pixel values.
func preProcessFrame(img image.Image, tensor *ort.Tensor[float32]) error {
	data := tensor.GetData()
	channelSize := 640 * 640
	if len(data) < (channelSize * 3) {
		return fmt.Errorf("destination tensor only holds %d floats, needs %d (make sure it's the right shape!)", len(data), channelSize*3)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	// Resize the image to 640x640 using Lanczos3 algorithm
	img = resize.Resize(640, 640, img, resize.Lanczos3)
	i := 0
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			redChannel[i] = float32(r>>8) / 255.0
			greenChannel[i] = float32(g>>8) / 255.0
			blueChannel[i] = float32(b>>8) / 255.0
			i++
		}
	}

	return nil
}

// postProcessOutput processes the output of the YOLOv8 model and returns a slice of bounding boxes.
// It iterates through the output array, finds the class with the highest probability for each index
func postProcessOutput(output []float32, originalWidth, originalHeight int) *[]model.BoundingBox {
	boundingBoxes := make([]model.BoundingBox, 0, 8400)

	var classID int
	var probability float32

	// Iterate through the output array, considering 8400 indices
	for idx := 0; idx < 8400; idx++ {
		// Iterate through 80 classes and find the class with the highest probability
		probability = -1e9
		for col := 0; col < 80; col++ {
			currentProb := output[8400*(col+4)+idx]
			if currentProb > probability {
				probability = currentProb
				classID = col
			}
		}

		// If the probability is less than 0.5, continue to the next index
		if probability < 0.5 {
			continue
		}

		// Extract the coordinates and dimensions of the bounding box
		xc, yc := output[idx], output[8400+idx]
		w, h := output[2*8400+idx], output[3*8400+idx]
		x1 := (xc - w/2) / 640 * float32(originalWidth)
		y1 := (yc - h/2) / 640 * float32(originalHeight)
		x2 := (xc + w/2) / 640 * float32(originalWidth)
		y2 := (yc + h/2) / 640 * float32(originalHeight)

		// Append the bounding box to the result
		boundingBoxes = append(boundingBoxes, model.BoundingBox{
			Label:      yoloClasses[classID],
			Confidence: probability,
			X1:         x1,
			Y1:         y1,
			X2:         x2,
			Y2:         y2,
		})
	}

	// Sort the bounding boxes by probability
	sort.Slice(boundingBoxes, func(i, j int) bool {
		return boundingBoxes[i].Confidence < boundingBoxes[j].Confidence
	})

	// Define a slice to hold the final result
	mergedResults := make([]model.BoundingBox, 0, len(boundingBoxes))

	// Iterate through sorted bounding boxes, removing overlaps
	for _, candidateBox := range boundingBoxes {
		overlapsExistingBox := false
		for _, existingBox := range mergedResults {
			if (&candidateBox).IoU(&existingBox) > 0.7 {
				overlapsExistingBox = true
				break
			}
		}
		if !overlapsExistingBox {
			mergedResults = append(mergedResults, candidateBox)
		}
	}

	// This will still be in sorted order by confidence
	return &mergedResults
}
