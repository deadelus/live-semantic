package dto

// ObjectRecognitionRequest DTO pour créer une tâche de reconnaissance d'objet
type ObjectRecognitionRequest struct {
	Filter              string  `json:"filter"`
	SimilarityThreshold float32 `json:"similarity_threshold"`
}

// ObjectRecognitionResponse DTO pour la réponse de reconnaissance d'objet
type ObjectRecognitionResponse struct{}
