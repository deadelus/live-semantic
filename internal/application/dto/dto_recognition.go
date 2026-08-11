package dto

// RecognitionRequest DTO pour créer une tâche de reconnaissance.
//
// Filter is a comma-separated spec of COCO labels YOLO11s can detect, each
// optionally capped with "*N": "person" (up to 1, implicit), "person*2"
// (up to 2), "person*2,car" (two independent terms — a label can only
// appear once, overlapping terms are rejected). Empty string means "track
// everything, no filter". Parsed by application/uc.parseFilterSpec.
//
// Replaced free-text CLIP semantic filtering (and the SimilarityThreshold
// field that gated it) on 2026-08-11 — decision reversal, TODO.md § A,
// docs/adr/clip-backend.md § 12: label matching is exact, doesn't share
// CLIP's absolute-threshold fragility, at the cost of only supporting the
// 80 COCO classes (no more open-vocabulary filters like "sac abandonné").
type RecognitionRequest struct {
	Filter string `json:"filter"`
}

// RecognitionResponse DTO pour la réponse de reconnaissance
type RecognitionResponse struct{}
