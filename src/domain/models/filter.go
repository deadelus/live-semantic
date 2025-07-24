package models

// Filter represents a user-defined semantic filter.
type Filter struct {
	// Text is the natural language query (e.g., "a person walking").
	Text string
	// Embedding is the pre-computed vector representation of the text.
	Embedding []float32
}
