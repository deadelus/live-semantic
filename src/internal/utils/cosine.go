package utils

import "math"

// CosineSimilarity calculates the cosine similarity between two vectors.
// It returns a value between -1 and 1, where 1 means the vectors are identical,
// 0 means they are orthogonal, and -1 means they are opposite.
// If the vectors are of different lengths or either is a zero vector, it returns 0
func (u *Toolbox) CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
