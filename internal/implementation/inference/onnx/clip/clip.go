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
// UNVERIFIED as of 2026-08-10: written against the documented Optimum
// export contract (pixel_values -> image_embeds, input_ids+attention_mask
// -> text_embeds — researched, see docs/adr/clip-backend.md), but never
// run against the actual model files (not downloaded yet, fp32 vs
// quantized decision pending). Treat the exact input/output tensor names
// and whether attention_mask is truly required as unconfirmed until a real
// run against the weights succeeds — this is not "done", it's "ready to
// verify".
type Encoder struct {
	tokenizer *tokenizer

	visionSession *ort.AdvancedSession
	visionInput   *ort.Tensor[float32]
	visionOutput  *ort.Tensor[float32]

	textSession       *ort.AdvancedSession
	textInputIDs      *ort.Tensor[int64]
	textAttentionMask *ort.Tensor[int64]
	textOutput        *ort.Tensor[float32]
}

// New initializes both CLIP ONNX sessions and the tokenizer. See Encoder's
// doc comment re: unverified status.
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

	visionSession, visionInput, visionOutput, err := newVisionSession(sessionOptions)
	if err != nil {
		return nil, err
	}

	textSession, textInputIDs, textAttentionMask, textOutput, err := newTextSession(sessionOptions)
	if err != nil {
		visionSession.Destroy()
		visionInput.Destroy()
		visionOutput.Destroy()
		return nil, err
	}

	return &Encoder{
		tokenizer:         tok,
		visionSession:     visionSession,
		visionInput:       visionInput,
		visionOutput:      visionOutput,
		textSession:       textSession,
		textInputIDs:      textInputIDs,
		textAttentionMask: textAttentionMask,
		textOutput:        textOutput,
	}, nil
}

func newVisionSession(sessionOptions *ort.SessionOptions) (*ort.AdvancedSession, *ort.Tensor[float32], *ort.Tensor[float32], error) {
	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, imageSize, imageSize))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, visionModelPath, err)
	}

	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, embeddingDim))
	if err != nil {
		input.Destroy()
		return nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, visionModelPath, err)
	}

	session, err := ort.NewAdvancedSession(
		visionModelPath,
		[]string{"pixel_values"}, []string{"image_embeds"},
		[]ort.Value{input}, []ort.Value{output},
		sessionOptions,
	)
	if err != nil {
		input.Destroy()
		output.Destroy()
		return nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, visionModelPath, err)
	}

	return session, input, output, nil
}

func newTextSession(sessionOptions *ort.SessionOptions) (*ort.AdvancedSession, *ort.Tensor[int64], *ort.Tensor[int64], *ort.Tensor[float32], error) {
	inputIDs, err := ort.NewEmptyTensor[int64](ort.NewShape(1, contextLength))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}

	attentionMask, err := ort.NewEmptyTensor[int64](ort.NewShape(1, contextLength))
	if err != nil {
		inputIDs.Destroy()
		return nil, nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}

	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, embeddingDim))
	if err != nil {
		inputIDs.Destroy()
		attentionMask.Destroy()
		return nil, nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}

	session, err := ort.NewAdvancedSession(
		textModelPath,
		[]string{"input_ids", "attention_mask"}, []string{"text_embeds"},
		[]ort.Value{inputIDs, attentionMask}, []ort.Value{output},
		sessionOptions,
	)
	if err != nil {
		inputIDs.Destroy()
		attentionMask.Destroy()
		output.Destroy()
		return nil, nil, nil, nil, fmt.Errorf("%w: %s: %v", domain.ErrModelInitialization, textModelPath, err)
	}

	return session, inputIDs, attentionMask, output, nil
}

// EncodeImage implements SemanticEncoder.EncodeImage for Encoder. frame is
// expected to already be a crop of the region of interest (a YOLO bounding
// box, TODO.md § A) — this does not detect or crop anything itself.
func (e *Encoder) EncodeImage(frame *entities.Frame) (entities.Embedding, error) {
	if frame == nil || frame.Image == nil {
		return nil, fmt.Errorf("clip: nil frame")
	}

	if err := e.writeImage(frame.Image); err != nil {
		return nil, err
	}

	if err := e.visionSession.Run(); err != nil {
		return nil, fmt.Errorf("clip: vision session run: %w", err)
	}

	return normalize(e.visionOutput.GetData()), nil
}

// EncodeText implements SemanticEncoder.EncodeText for Encoder. Callers
// should call this once per filter at startup, not per frame (TODO.md § A).
func (e *Encoder) EncodeText(text string) (entities.Embedding, error) {
	ids := e.tokenizer.Encode(text)

	inputData := e.textInputIDs.GetData()
	maskData := e.textAttentionMask.GetData()
	for i, id := range ids {
		inputData[i] = int64(id)
		// All-ones: see Encoder's doc comment — whether the exported graph
		// actually needs a padding-aware mask isn't confirmed yet. CLIP's
		// original design pools via the eot token position rather than an
		// explicit mask, so all-ones is the safe default either way (a
		// real padding mask would only matter if the model applies it to
		// attention internally, which an all-ones mask trivially satisfies
		// without masking anything out).
		maskData[i] = 1
	}

	if err := e.textSession.Run(); err != nil {
		return nil, fmt.Errorf("clip: text session run: %w", err)
	}

	return normalize(e.textOutput.GetData()), nil
}

// writeImage resizes img the way CLIP's reference preprocessing does:
// shortest side to imageSize, then center-crop to imageSize x imageSize
// (preprocessor_config.json: do_resize + do_center_crop, both at 224) —
// deliberately NOT a plain squash-resize to 224x224 (yolo11s.go's simpler
// approach), which would distort aspect ratio. Fills the vision input
// tensor with normalized (CLIP's own mean/std, not plain /255),
// channel-split (NCHW) RGB data.
func (e *Encoder) writeImage(img image.Image) error {
	data := e.visionInput.GetData()

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
	if e.visionInput != nil {
		e.visionInput.Destroy()
	}
	if e.visionOutput != nil {
		e.visionOutput.Destroy()
	}
	if e.textSession != nil {
		e.textSession.Destroy()
	}
	if e.textInputIDs != nil {
		e.textInputIDs.Destroy()
	}
	if e.textAttentionMask != nil {
		e.textAttentionMask.Destroy()
	}
	if e.textOutput != nil {
		e.textOutput.Destroy()
	}
	fmt.Println("CLIP encoder resources cleaned up successfully.")
}
