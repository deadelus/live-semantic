package dto

// RecognitionRequest DTO pour créer une tâche de reconnaissance.
//
// Filter is a comma-separated spec of terms, each optionally capped with
// "*N": "person" (up to 1, implicit), "person*2" (up to 2), "person*2,car"
// (two independent terms — a term can only appear once, overlapping terms
// are rejected). A term that's one of the 80 COCO classes YOLO11s can
// detect matches exactly (no score involved); a term that isn't matches
// semantically via CLIP against a fixed default threshold (not exposed
// here — application/uc.defaultSimilarityThreshold, meant to become a GUI
// control, TODO.md § H). Empty string means "track everything, no filter".
// Parsed by application/uc.parseFilterSpec.
//
// History (TODO.md § A, docs/adr/clip-backend.md § 12-13): CLIP-only
// (2026-08-10) → exact-label-only, SimilarityThreshold field removed
// (2026-08-11 morning — CLIP's absolute threshold rejected real matches at
// every value tried) → this hybrid (2026-08-11 afternoon, at the user's
// request — free-text/CLIP matching is useful for anything outside the 80
// COCO classes, just not as the *only* mechanism anymore).
type RecognitionRequest struct {
	Filter string `json:"filter"`
}

// RecognitionResponse DTO pour la réponse de reconnaissance
type RecognitionResponse struct{}
