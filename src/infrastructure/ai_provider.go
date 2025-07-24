package infrastructure

import "live-semantic/src/domain/models"

// AIProvider is the interface for any AI model that can provide embeddings.
type AIProvider interface {
	// EncodeText converts a text filter into a vector embedding.
	EncodeText(filter *models.Filter) error
	// EncodeImage converts an image frame into a vector embedding.
	EncodeImage(frame *models.Frame) ([]float32, error)
}
