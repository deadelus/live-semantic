package cmd

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"

	"github.com/spf13/cobra"
)

var recognitionCmd = &cobra.Command{
	Use:   "recognition",
	Short: "Run recognition with filter and similarity threshold",
	Long:  `Execute the recognition use case with a filter and similarity threshold.`,
	Run: func(cmd *cobra.Command, args []string) {
		filter, err := cmd.Flags().GetString("filter")

		if err != nil {
			fmt.Printf("❌ Error: %s\n", err.Error())
			return
		}

		threshold, err := cmd.Flags().GetFloat32("similarity-threshold")

		if err != nil {
			fmt.Printf("❌ Error: %s\n", err.Error())
			return
		}

		if threshold < 0.0 || threshold > 1.0 {
			fmt.Println("❌ Error: Similarity threshold must be between 0.0 and 1.0")
			return
		}

		req := dto.RecognitionRequest{
			Filter:              filter,
			SimilarityThreshold: threshold,
		}

		result, err := useCases.RecognitionUseCase(context.Background(), req)
		if err != nil {
			fmt.Printf("❌ Error: %s\n", err.Error())
			return
		}
		if result.Success {
			fmt.Println("✅ Realtime analysis completed successfully.")
		} else {
			fmt.Printf("❌ Error: %s\n", result.Error)
		}
	},
}

func init() {
	rootCmd.AddCommand(recognitionCmd)
	recognitionCmd.Flags().String("filter", "", "Text filter for semantic analysis")
	// 0.20, not 0.8 (and not 0.25 anymore either): this gates a CLIP cosine
	// similarity (crop vs filter text), not YOLO's old box.Confidence. Real
	// measured CLIP ViT-B/32 zero-shot scores on webcam crops sit around
	// 0.20-0.30 (2026-08-10 calibration, docs/adr/clip-backend.md § 7-10) —
	// 0.8 would silently reject every detection. Lowered 0.25 -> 0.20 on
	// 2026-08-11: real end-to-end runs (person.mp4, a real webcam session)
	// scored genuine "person" matches at 0.235-0.238, just under 0.25 — the
	// absolute threshold was rejecting correct detections, not just noise
	// (docs/adr/clip-backend.md § 10, TODO.md § A).
	recognitionCmd.Flags().Float32("similarity-threshold", 0.20, "Similarity threshold for match (CLIP cosine similarity, typically 0.20-0.30)")
	err := recognitionCmd.MarkFlagRequired("filter")
	if err != nil {
		fmt.Printf("❌ Error: %s\n", err.Error())
		return
	}
}
