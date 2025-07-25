package dto

// RealtimeAnalysisRequest DTO pour créer une tâche d'analyse en temps réel
type RealtimeAnalysisRequest struct {
	Filter              string  `json:"filter"`
	SimilarityThreshold float32 `json:"similarity_threshold"`
}

// RealtimeAnalysisResponse DTO pour la réponse d'analyse en temps réel
type RealtimeAnalysisResponse struct{}
