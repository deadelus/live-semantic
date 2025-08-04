// Package yolo11s implements the YOLOv11s neural network for object detection.
package yolo11s

import (
	"fmt"
	"live-semantic/src/domain"
	"live-semantic/src/domain/model"
	"live-semantic/src/infrastructure/ai"
	"live-semantic/src/internal/onnx"
	"sort"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	// yolo11sModelPath is the path to the YOLOv11s ONNX model file.
	yolo11sModelPath = "src/implementation/ai/yolo11s/yolo11s.onnx"
	fontsPath        = "src/assets/fonts/Roboto-Regular.ttf"
	fontSize         = 30
	boxThickness     = 5
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

// NewNeuralNetwork initializes the ONNX runtime for YOLOv11s model.
func NewNeuralNetwork() *Yolo11sNeuralNetwork {
	// Initialize the ONNX runtime with the model path and input/output shapes
	onnxRuntime := onnx.NewOnnxRuntime(
		yolo11sModelPath,
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

// AnalyzeFrame implements the AI interface for Yolo11sNeuralNetwork.
func (m *Yolo11sNeuralNetwork) AnalyzeFrame(frame *model.Frame) (*ai.DetectionResult, error) {
	err := preProcessFrame(m.Session.TensorInput, frame)

	if err != nil {
		return nil, err
	}

	err = m.Session.Session.Run()
	if err != nil {
		return nil, err
	}

	boxes := postProcessOutput(m.Session.TensorOutput, frame)

	return &ai.DetectionResult{
		Frame:         frame,
		BoundingBoxes: boxes,
	}, nil
}

// preProcessFrame prepares the input frame for the model.
// It resizes the image to 640x640 and normalizes the pixel values.
func preProcessFrame(tensor *ort.Tensor[float32], frame *model.Frame) error {
	data := tensor.GetData()
	channelSize := 640 * 640
	if len(data) < (channelSize * 3) {
		return fmt.Errorf("destination tensor only holds %d floats, needs %d (make sure it's the right shape!)", len(data), channelSize*3)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	// Resize the image to 640x640 using Lanczos3 algorithm
	originalBounds := frame.Image.Bounds()
	originalWidth := originalBounds.Dx()
	originalHeight := originalBounds.Dy()

	frame.Image = resize.Resize(640, 640, frame.Image, resize.Lanczos3)

	i := 0
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			r, g, b, _ := frame.Image.At(x, y).RGBA()
			redChannel[i] = float32(r>>8) / 255.0
			greenChannel[i] = float32(g>>8) / 255.0
			blueChannel[i] = float32(b>>8) / 255.0
			i++
		}
	}

	// Resize output image back to original dimensions and overwrite frame.Image
	frame.Image = resize.Resize(uint(originalWidth), uint(originalHeight), frame.Image, resize.Lanczos3)

	return nil
}

// postProcessOutput processes the output of the YOLOv8 model and returns a slice of bounding boxes.
// It iterates through the output array, finds the class with the highest probability for each index
func postProcessOutput(tensor *ort.Tensor[float32], frame *model.Frame) []model.BoundingBox {
	output := tensor.GetData()
	if len(output) < 8400*84 {
		fmt.Println("Output tensor does not have enough data for 8400 detections with 84 classes")
		return nil
	}

	// Initialize a slice to hold the bounding boxes
	// 8400 is the number of detections, 84 is the number of classes + 4 coordinates
	// Each detection has 4 coordinates and a class probability for each class
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
		x1 := (xc - w/2) / 640 * float32(frame.Image.Bounds().Max.X)
		y1 := (yc - h/2) / 640 * float32(frame.Image.Bounds().Max.Y)
		x2 := (xc + w/2) / 640 * float32(frame.Image.Bounds().Max.X)
		y2 := (yc + h/2) / 640 * float32(frame.Image.Bounds().Max.Y)

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
	return mergedResults
}
