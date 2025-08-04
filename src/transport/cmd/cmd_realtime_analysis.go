package cmd

import (
	"context"
	"fmt"
	"live-semantic/src/domain/dto"

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
	recognitionCmd.Flags().Float32("similarity-threshold", 0.8, "Similarity threshold for match")
	err := recognitionCmd.MarkFlagRequired("filter")
	if err != nil {
		fmt.Printf("❌ Error: %s\n", err.Error())
		return
	}
}
