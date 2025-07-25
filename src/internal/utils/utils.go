// Package utils provides utility functions for the application.
package utils

// Toolbox is a struct that can hold utility methods.
type Toolbox struct{}

// Utils defines the interface for utility methods used in the application.
type Utils interface {
	CosineSimilarityCalculator
}

// CosineSimilarityCalculator defines the interface for calculating cosine similarity.
type CosineSimilarityCalculator interface {
	CosineSimilarity(a, b []float32) float32
}
