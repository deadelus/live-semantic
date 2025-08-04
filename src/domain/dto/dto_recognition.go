package dto

// RecognitionRequest DTO pour créer une tâche de reconnaissance
type RecognitionRequest struct {
	Filter              string  `json:"filter"`
	SimilarityThreshold float32 `json:"similarity_threshold"`
}

// RecognitionResponse DTO pour la réponse de reconnaissance
type RecognitionResponse struct{}
