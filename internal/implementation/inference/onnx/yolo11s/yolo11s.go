// Package yolo11s implements the YOLOv11s neural network for object detection,
// directly against github.com/yalue/onnxruntime_go (no intermediate wrapper).
package yolo11s

import (
	"fmt"
	"image"
	"live-semantic/internal/domain"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/inference/onnx/runtime"
	"live-semantic/internal/infrastructure/inference"
	"sort"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

var (
	// yolo11sModelPath is the path to the YOLOv11s ONNX model file.
	// Exported from PyTorch 2.7.0, opset 19, IR version 9 (checked 2026-08-06
	// via the model's own protobuf header — see docs/adr/inference-runtimes.md
	// §3.1). Re-verify this comment if the .onnx file is ever re-exported.
	yolo11sModelPath = "assets/models/yolo11s.onnx"
	// minORTVersion is the lowest onnxruntime shared library version this
	// detector is known to work with (linked libs were 1.22.0 as of
	// 2026-08-06). New() refuses to start below this.
	minORTVersion = "1.20.0"
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
	thresholdConfidence float32 = 0.5
	// nmsIoUThreshold is the overlap above which two detections are considered
	// duplicates and merged by non-max suppression. Lowered 0.7 -> 0.45
	// (YOLO's usual default) on 2026-08-09: found in real usage — one
	// person produced 2 surviving "person" boxes at very different scales
	// (a tight face-only box + a wider torso box), IoU between them well
	// under 0.7 so both survived NMS untouched. 0.45 catches nested/
	// partially-overlapping same-object detections much more reliably.
	nmsIoUThreshold float32 = 0.45
)

// Array of YOLOv8 class labels
// This is the list of classes that the YOLOv8 model can detect.
var yoloClasses = entities.Yolo11sClasses()

// Detector represents the YOLOv11s neural implementation.
//
// session is a DynamicAdvancedSession (needed once concurrent sessions
// became a goal, docs/gui/spec.md § 1.2) rather than an AdvancedSession
// with tensors fixed at construction:
// AnalyzeFrame allocates fresh input/output tensors per call instead of
// reusing struct-held ones. That fixed-tensor pattern was confirmed unsafe
// under concurrent calls on a shared Detector (3/3 data races reproduced
// with `go build -race`, docs/gui/spec.md § 1.2) — AnalyzeFrame writes
// directly into the tensor's backing buffer (writeInput), so two
// goroutines calling it on the same *Detector at once would corrupt each
// other's input. DynamicAdvancedSession.Run(inputs, outputs []Value)
// takes tensors per call instead, which is the thread-safety contract ORT
// itself documents (microsoft/onnxruntime#114): Run() is safe on a shared
// session as long as each call uses its own buffers. The session/model
// weights are still loaded once and shared; only the small per-call
// tensors (~7.5 Mo combined for YOLO at 640x640) are now allocated fresh.
// Allocation cost per call not benchmarked yet (perf follow-up).
type Detector struct {
	session *ort.DynamicAdvancedSession
}

// New initializes the ONNX runtime for the YOLOv11s detector. By default it
// runs on CPU; pass runtime.Option (e.g. runtime.WithCUDA()) to select a
// different Execution Provider — see docs/adr/inference-runtimes.md §5.
//
// Returns a non-nil error on any initialization failure (missing runtime
// library, missing/invalid model file, ORT session setup) instead of a
// zero-value *Detector — callers must check the error and fail fast rather
// than proceed with a Detector whose tensors are nil (AnalyzeFrame would
// otherwise panic on first use, deep in the video loop).
func New(opts ...runtime.Option) (*Detector, error) {
	if err := runtime.InitEnvironment(runtime.LibraryPath()); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNilRuntime, err)
	}

	if err := runtime.RequireMinVersion(minORTVersion); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNilRuntime, err)
	}

	sessionOptions, err := runtime.NewSessionOptions(opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrModelInitialization, err)
	}
	defer sessionOptions.Destroy()

	session, err := ort.NewDynamicAdvancedSession(
		yolo11sModelPath,
		[]string{"images"}, []string{"output0"},
		sessionOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, yolo11sModelPath, err)
	}

	return &Detector{
		session: session,
	}, nil
}

// AnalyzeFrame implements the ObjectDetector.AnalyzeFrame for Detector.
// Allocates fresh input/output tensors for this call and destroys them
// before returning — see Detector's doc comment for why (thread-safety
// under concurrent calls).
func (d *Detector) AnalyzeFrame(frame *entities.Frame) (*inference.DetectionResult, error) {
	originalBounds := frame.Image.Bounds()

	inputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(
		int64(batchSize), int64(modelInputChannels), int64(modelHeight), int64(modelWidth),
	))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, yolo11sModelPath, err)
	}
	defer inputTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(
		int64(batchSize), int64(modelClasses+4), int64(modelDetections), // 4 for bounding box coordinates
	))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, yolo11sModelPath, err)
	}
	defer outputTensor.Destroy()

	if err := writeInput(inputTensor.GetData(), frame.Image); err != nil {
		return nil, err
	}

	if err := d.session.Run([]ort.Value{inputTensor}, []ort.Value{outputTensor}); err != nil {
		return nil, err
	}

	boxes := decodeDetections(outputTensor.GetData(), originalBounds)

	return &inference.DetectionResult{
		Frame:         frame,
		BoundingBoxes: boxes,
	}, nil
}

// writeInput resizes img to the model's input dimensions and fills data
// (an input tensor's backing buffer) with normalized, channel-split
// (NCHW) RGB data. Free function (not a *Detector method) since it no
// longer touches any struct-held tensor — the caller owns the tensor's
// lifetime now.
func writeInput(data []float32, img image.Image) error {
	channelSize := modelHeight * modelWidth
	if len(data) < channelSize*modelInputChannels {
		return fmt.Errorf("input tensor only holds %d floats, needs %d (make sure it's the right shape!)", len(data), channelSize*modelInputChannels)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	resized := resize.Resize(uint(modelWidth), uint(modelHeight), img, resize.Lanczos3)

	i := 0
	for y := 0; y < modelHeight; y++ {
		for x := 0; x < modelWidth; x++ {
			r, g, b, _ := resized.At(x, y).RGBA()
			redChannel[i] = float32(r>>8) / 255.0
			greenChannel[i] = float32(g>>8) / 255.0
			blueChannel[i] = float32(b>>8) / 255.0
			i++
		}
	}
	return nil
}

// decodeDetections parses the raw YOLO output tensor into bounding boxes
// scaled back to originalBounds, applying a confidence threshold and
// non-max suppression on overlapping detections.
func decodeDetections(output []float32, originalBounds image.Rectangle) []entities.BoundingBox {
	if len(output) < modelDetections*(modelClasses+4) {
		fmt.Printf("Output tensor does not have enough data for %d detections with %d classes", modelDetections, modelClasses)
		return nil
	}

	boxes := make([]entities.BoundingBox, 0, modelDetections)
	var classID int
	var probability float32

	for idx := 0; idx < modelDetections; idx++ {
		probability = -1e9
		for col := 0; col < modelClasses; col++ {
			currentProb := output[(modelDetections*(col+4))+idx]
			if currentProb > probability {
				probability = currentProb
				classID = col
			}
		}
		if probability < thresholdConfidence {
			continue
		}
		xc, yc := output[idx], output[modelDetections+idx]
		w, h := output[2*modelDetections+idx], output[3*modelDetections+idx]
		x1 := (xc - w/2) / float32(modelWidth) * float32(originalBounds.Max.X)
		y1 := (yc - h/2) / float32(modelHeight) * float32(originalBounds.Max.Y)
		x2 := (xc + w/2) / float32(modelWidth) * float32(originalBounds.Max.X)
		y2 := (yc + h/2) / float32(modelHeight) * float32(originalBounds.Max.Y)
		boxes = append(boxes, entities.BoundingBox{
			Label:      yoloClasses[classID],
			Confidence: probability,
			X1:         x1,
			Y1:         y1,
			X2:         x2,
			Y2:         y2,
		})
	}

	// Highest confidence first: standard NMS keeps the best-scoring box in
	// each overlapping group and suppresses the rest. This used to sort
	// ascending, which kept whichever box happened to be *lowest*
	// confidence in a group instead — found alongside the threshold fix
	// above, same investigation (2026-08-09).
	sort.Slice(boxes, func(i, j int) bool {
		return boxes[i].Confidence > boxes[j].Confidence
	})

	merged := make([]entities.BoundingBox, 0, len(boxes))
	for _, candidate := range boxes {
		overlapsExisting := false
		for _, existing := range merged {
			if candidate.IoU(&existing) > nmsIoUThreshold {
				overlapsExisting = true
				break
			}
		}
		if !overlapsExisting {
			merged = append(merged, candidate)
		}
	}
	return merged
}

// Cleanup implements the ObjectDetector.Cleanup for Detector.
//
// Also explicitly releases the process-wide ORT environment (see
// runtime.DestroyEnvironment's doc comment) — without this, the app
// crashes with SIGABRT at exit (a documented ONNX Runtime bug on macOS —
// OrtEnv's static C++ destructor throws uncaught at process exit,
// microsoft/onnxruntime#24579 — fixed upstream for the Node.js binding
// only, not the C API this project's cgo binding calls). Safe: this
// project only ever constructs one
// Detector for the whole process lifetime (cmd/livesemantic/main.go).
func (d *Detector) Cleanup() {
	if d.session != nil {
		d.session.Destroy()
	}
	// No struct-held input/output tensors to destroy anymore — AnalyzeFrame
	// allocates and destroys its own per call (see Detector's doc comment).
	if err := runtime.DestroyEnvironment(); err != nil {
		fmt.Println("Warning: failed to destroy ORT environment:", err)
	}
	fmt.Println("Detector resources cleaned up successfully.")
}
