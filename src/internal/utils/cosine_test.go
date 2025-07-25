package utils_test

import (
	"live-semantic/src/provider"
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	// Define test cases for cosine similarity
	utils := provider.NewUtils()

	testCases := []struct {
		name     string
		a, b     []float32
		expected float32
	}{
		{
			name:     "Identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 1.0,
		},
		{
			name:     "Orthogonal vectors",
			a:        []float32{1, 0},
			b:        []float32{0, 1},
			expected: 0.0,
		},
		{
			name:     "Opposite vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{-1, -2, -3},
			expected: -1.0,
		},
		{
			name:     "Random vectors",
			a:        []float32{2, 3, 4, 5},
			b:        []float32{6, 7, 8, 9},
			expected: 0.9870316,
		},
		{
			name:     "Different length vectors",
			a:        []float32{1, 2},
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "Zero vector",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "Both zero vectors",
			a:        []float32{0, 0, 0},
			b:        []float32{0, 0, 0},
			expected: 0.0,
		},
		{
			name:     "Nil vector",
			a:        nil,
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.CosineSimilarity(tc.a, tc.b)
			if math.Abs(float64(result-tc.expected)) > 1e-5 {
				t.Errorf("Expected similarity of %f, but got %f", tc.expected, result)
			}
		})
	}
}
