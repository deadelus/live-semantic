// Package inference defines the port for inference (object detection, and
// semantic encoding) functionalities.
//
// Fallback order (TODO.md § F, not implemented — only the first tier
// exists today): implementations are expected to be tried in this order
// of preference, each a fallback for when the previous one isn't
// available/fast enough on a given deployment target:
//
//  1. Native Go ONNX Runtime binding (github.com/yalue/onnxruntime_go,
//     direct CGo) — what yolo11s.Detector and clip.Encoder use today.
//     Fastest, no subprocess/network hop, but ties the binary to ORT's
//     native library being present for the target platform.
//  2. Embedded Python runtime (e.g. via a subprocess or CGo Python
//     binding) — for model formats/runtimes without a good Go story
//     (rare with ONNX, more relevant if a non-ONNX model becomes
//     necessary later).
//  3. Local REST inference server (e.g. a Triton/ORT-server sidecar) —
//     for splitting inference onto separate/more capable hardware while
//     staying on the same machine or LAN.
//  4. Cloud inference API — last resort, for deployment targets with no
//     usable local hardware at all. Adds network latency and an external
//     dependency, only worth it if 1-3 are all ruled out.
//
// This ordering is a stated intent for future adapters, not a promise —
// see AUDIT.md / readme.md's "État d'avancement" for what's real today.
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
