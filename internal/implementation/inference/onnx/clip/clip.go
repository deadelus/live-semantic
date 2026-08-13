// Package clip implements the inference.SemanticEncoder port on CLIP
// ViT-B/32, directly against github.com/yalue/onnxruntime_go (no
// intermediate wrapper), same as implementation/inference/onnx/yolo11s.
package clip

import (
	"fmt"
	"image"
	"math"

	"live-semantic/internal/domain"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/inference/onnx/runtime"
	"live-semantic/internal/infrastructure/inference"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

// Compile-time check: Encoder must keep satisfying the port.
var _ inference.SemanticEncoder = (*Encoder)(nil)

var (
	// visionModelPath / textModelPath: CLIP ViT-B/32, exported per-encoder
	// by Xenova (docs/adr/clip-backend.md) — NOT bundled in this repo
	// (large, like yolo11s.onnx: ~85+61.5 Mo quantized, or ~335+242 Mo
	// fp32 — see the ADR for that decision), must be placed manually in
	// assets/models/clip/ before this package can be used.
	visionModelPath = "assets/models/clip/vision_model.onnx"
	textModelPath   = "assets/models/clip/text_model.onnx"

	// minORTVersion matches yolo11s.go's — same bundled runtime.
	minORTVersion = "1.20.0"
)

const (
	// imageSize: CLIP ViT-B/32 expects 224x224 input — verified against
	// openai/clip-vit-base-patch32's config.json ("image_size": 224,
	// "patch_size": 32), not assumed.
	imageSize = 224

	// embeddingDim: shared image/text embedding space size — verified
	// against config.json ("projection_dim": 512).
	embeddingDim = 512
)

// imageMean/imageStd: CLIP's own normalization constants — verified
// against preprocessor_config.json (image_mean/image_std), NOT the more
// common ImageNet defaults. CLIP was trained with its own stats.
var (
	imageMean = [3]float32{0.48145466, 0.4578275, 0.40821073}
	imageStd  = [3]float32{0.26862954, 0.26130258, 0.27577711}
)

// Encoder implements infrastructure/inference.SemanticEncoder on CLIP
// ViT-B/32 (docs/adr/clip-backend.md), via two independent ONNX sessions —
// matches the port's EncodeImage/EncodeText split exactly, no combined
// graph to split ourselves (Xenova's export already ships vision_model.onnx
// and text_model.onnx separately).
//
// Verified 2026-08-10 against the real (quantized) weights: pixel_values ->
// image_embeds on the vision side; input_ids alone (no attention_mask —
// tried, ORT rejects it with "Invalid input name: attention_mask", the
// graph genuinely doesn't have one, confirming CLIP's original
// eot-position pooling design doesn't need one here) -> text_embeds on the
// text side. See docs/adr/clip-backend.md.
// Encoder's two sessions are DynamicAdvancedSession (docs/gui/spec.md
// § 1.2), same rationale as yolo11s.Detector's doc
// comment: EncodeImage/EncodeText allocate fresh tensors per call instead
// of reusing struct-held ones, which were confirmed unsafe under
// concurrent calls on a shared Encoder. Model weights still load once and
// stay shared; only the small per-call tensors are allocated fresh.
type Encoder struct {
	tokenizer *tokenizer

	visionSession *ort.DynamicAdvancedSession
	textSession   *ort.DynamicAdvancedSession
}

// New initializes both CLIP ONNX sessions and the tokenizer.
func New(opts ...runtime.Option) (*Encoder, error) {
	tok, err := newTokenizer()
	if err != nil {
		return nil, fmt.Errorf("clip: %w", err)
	}

	if err := runtime.InitEnvironment(runtime.LibraryPath()); err != nil {
		return nil, fmt.Errorf("%w: clip.Encoder: %v", domain.ErrNilRuntime, err)
	}
	if err := runtime.RequireMinVersion(minORTVersion); err != nil {
		return nil, fmt.Errorf("%w: clip.Encoder: %v", domain.ErrNilRuntime, err)
	}

	sessionOptions, err := runtime.NewSessionOptions(opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: clip.Encoder: %v", domain.ErrModelInitialization, err)
	}
	defer sessionOptions.Destroy()

	visionSession, err := ort.NewDynamicAdvancedSession(
		visionModelPath,
		[]string{"pixel_values"}, []string{"image_embeds"},
		sessionOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, visionModelPath, err)
	}

	textSession, err := ort.NewDynamicAdvancedSession(
		textModelPath,
		[]string{"input_ids"}, []string{"text_embeds"},
		sessionOptions,
	)
	if err != nil {
		visionSession.Destroy()
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}

	return &Encoder{
		tokenizer:     tok,
		visionSession: visionSession,
		textSession:   textSession,
	}, nil
}

// EncodeImage implements SemanticEncoder.EncodeImage for Encoder. frame is
// expected to already be a crop of the region of interest (a YOLO bounding
// box) — this does not detect or crop anything itself.
func (e *Encoder) EncodeImage(frame *entities.Frame) (entities.Embedding, error) {
	if frame == nil || frame.Image == nil {
		return nil, fmt.Errorf("clip: nil frame")
	}

	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, imageSize, imageSize))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, visionModelPath, err)
	}
	defer input.Destroy()

	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, embeddingDim))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, visionModelPath, err)
	}
	defer output.Destroy()

	if err := writeImage(input.GetData(), frame.Image); err != nil {
		return nil, err
	}

	if err := e.visionSession.Run([]ort.Value{input}, []ort.Value{output}); err != nil {
		return nil, fmt.Errorf("clip: vision session run: %w", err)
	}

	return normalize(output.GetData()), nil
}

// EncodeText implements SemanticEncoder.EncodeText for Encoder. Callers
// should call this once per filter at startup, not per frame.
func (e *Encoder) EncodeText(text string) (entities.Embedding, error) {
	inputIDs, err := ort.NewEmptyTensor[int64](ort.NewShape(1, contextLength))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}
	defer inputIDs.Destroy()

	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, embeddingDim))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}
	defer output.Destroy()

	ids := e.tokenizer.Encode(text)
	inputData := inputIDs.GetData()
	for i, id := range ids {
		inputData[i] = int64(id)
	}

	if err := e.textSession.Run([]ort.Value{inputIDs}, []ort.Value{output}); err != nil {
		return nil, fmt.Errorf("clip: text session run: %w", err)
	}

	return normalize(output.GetData()), nil
}

// writeImage resizes img the way CLIP's reference preprocessing does:
// shortest side to imageSize, then center-crop to imageSize x imageSize
// (preprocessor_config.json: do_resize + do_center_crop, both at 224) —
// deliberately NOT a plain squash-resize to 224x224 (yolo11s.go's simpler
// approach), which would distort aspect ratio. Fills data (a vision input
// tensor's backing buffer) with normalized (CLIP's own mean/std, not plain
// /255), channel-split (NCHW) RGB data. Free function — see
// yolo11s.writeInput's doc comment for why.
func writeImage(data []float32, img image.Image) error {
	channelSize := imageSize * imageSize
	if len(data) < channelSize*3 {
		return fmt.Errorf("clip: input tensor only holds %d floats, needs %d", len(data), channelSize*3)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	resized, offsetX, offsetY := resizeAndCenterCropOffsets(img)

	i := 0
	for y := 0; y < imageSize; y++ {
		for x := 0; x < imageSize; x++ {
			r, g, b, _ := resized.At(offsetX+x, offsetY+y).RGBA()
			redChannel[i] = (float32(r>>8)/255.0 - imageMean[0]) / imageStd[0]
			greenChannel[i] = (float32(g>>8)/255.0 - imageMean[1]) / imageStd[1]
			blueChannel[i] = (float32(b>>8)/255.0 - imageMean[2]) / imageStd[2]
			i++
		}
	}
	return nil
}

// resizeAndCenterCropOffsets resizes img so its shortest side becomes
// imageSize (preserving aspect ratio), and returns the (x, y) offset into
// that resized image where a centered imageSize x imageSize crop starts —
// callers read pixels via resized.At(offsetX+x, offsetY+y) rather than
// materializing a second cropped image.
func resizeAndCenterCropOffsets(img image.Image) (resized image.Image, offsetX, offsetY int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var newW, newH uint
	if w <= h {
		newW = uint(imageSize)
		newH = uint(float64(h) * float64(imageSize) / float64(w))
	} else {
		newH = uint(imageSize)
		newW = uint(float64(w) * float64(imageSize) / float64(h))
	}

	resized = resize.Resize(newW, newH, img, resize.Bicubic)
	b := resized.Bounds()
	offsetX = b.Min.X + (b.Dx()-imageSize)/2
	offsetY = b.Min.Y + (b.Dy()-imageSize)/2
	return resized, offsetX, offsetY
}

// normalize L2-normalizes an embedding — CLIP similarity is cosine
// similarity, computed as a plain dot product once both sides are unit
// vectors, so normalizing here means callers never have to.
func normalize(data []float32) entities.Embedding {
	var sumSquares float64
	for _, v := range data {
		sumSquares += float64(v) * float64(v)
	}
	if sumSquares == 0 {
		out := make(entities.Embedding, len(data))
		copy(out, data)
		return out
	}
	norm := float32(1 / math.Sqrt(sumSquares))
	out := make(entities.Embedding, len(data))
	for i, v := range data {
		out[i] = v * norm
	}
	return out
}

// Cleanup implements SemanticEncoder.Cleanup for Encoder.
func (e *Encoder) Cleanup() {
	if e.visionSession != nil {
		e.visionSession.Destroy()
	}
	if e.textSession != nil {
		e.textSession.Destroy()
	}
	// No struct-held input/output tensors to destroy anymore — EncodeImage/
	// EncodeText allocate and destroy their own per call (see Encoder's doc
	// comment).
	// Same fix as yolo11s.Detector.Cleanup (see its doc comment — the
	// 2026-08-10 SIGABRT-on-shutdown root cause) — required here too, independently: whichever
	// of the two Cleanup()s runs first actually destroys the shared ORT
	// environment, runtime.DestroyEnvironment() is a no-op if already
	// destroyed, so having both call it is safe, not redundant risk.
	if err := runtime.DestroyEnvironment(); err != nil {
		fmt.Println("Warning: failed to destroy ORT environment:", err)
	}
	fmt.Println("CLIP encoder resources cleaned up successfully.")
}
