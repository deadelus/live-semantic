package dto

// RecognitionRequest DTO pour créer une tâche de reconnaissance.
//
// Filter is a comma-separated spec of terms: "key[*cap][+option[=value]]...".
// "person" (cap 1, implicit), "person*2" (cap 2), "person*2,car" (two
// independent terms — a key can only appear once, overlapping terms are
// rejected). A key that's one of the 80 COCO classes YOLO11s can detect
// matches exactly (no score involved); a key that isn't matches
// semantically via CLIP against a fixed default threshold (not exposed
// here — application/uc.defaultSimilarityThreshold, meant to become a GUI
// control, TODO.md § H). "+overlap" (default false, e.g.
// "person with a red hat*1+overlap") lets a term claim a candidate box
// another term already claimed this cycle — parsed by parseFilterSpec, not
// yet consulted by trackManager.reanchor (TODO.md § A). Empty string means
// "track everything, no filter". Parsed by application/uc.parseFilterSpec.
//
// History (TODO.md § A, docs/adr/clip-backend.md § 12-13/16): CLIP-only
// (2026-08-10) → exact-label-only, SimilarityThreshold field removed
// (2026-08-11 morning — CLIP's absolute threshold rejected real matches at
// every value tried) → hybrid (2026-08-11 afternoon, at the user's
// request — free-text/CLIP matching is useful for anything outside the 80
// COCO classes, just not as the *only* mechanism anymore) → "+option"
// grammar (2026-08-11 evening, for the "overlap" opt-in the user asked to
// have named without building the whole feature yet).
type RecognitionRequest struct {
	Filter string `json:"filter"`
}

// RecognitionResponse DTO pour la réponse de reconnaissance
type RecognitionResponse struct{}
