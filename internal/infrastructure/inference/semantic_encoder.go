package inference

import "live-semantic/internal/domain/entities"

// SemanticEncoder is the port for semantic feature extraction (e.g. CLIP),
// the cascade step run after ObjectDetector crops a bounding box (TODO.md
// § A: YOLO → crop → CLIP). No implementation exists yet.
type SemanticEncoder interface {
	// EncodeImage returns the embedding for a frame region (typically a
	// crop of a detected bounding box, not the full frame).
	EncodeImage(frame *entities.Frame) (entities.Embedding, error)
	// EncodeText returns the embedding for a natural-language filter.
	// Callers should compute this once per filter at startup, not per
	// frame (TODO.md § A).
	EncodeText(text string) (entities.Embedding, error)
	// Cleanup performs any necessary cleanup operations.
	Cleanup()
}
