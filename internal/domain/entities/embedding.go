package entities

// Embedding is a semantic feature vector produced by a SemanticEncoder
// (e.g. CLIP image/text embeddings). Dimensionality depends on the model
// backend chosen for TODO.md § A — no encoder implementation exists yet,
// this type only exists to let the SemanticEncoder port compile.
type Embedding []float32
