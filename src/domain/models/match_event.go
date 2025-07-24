package models

// MatchEvent represents a successful match between a Frame and a Filter.
type MatchEvent struct {
	// MatchedFrame is a reference to the frame where the match occurred.
	MatchedFrame Frame
	// MatchedFilter is a reference to the filter that was matched.
	MatchedFilter Filter
	// Confidence is the similarity score (e.g., cosine similarity) of the match.
	Confidence float32
}
