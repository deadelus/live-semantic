package cmd

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"

	"github.com/spf13/cobra"
)

var recognitionCmd = &cobra.Command{
	Use:   "recognition",
	Short: "Run recognition with a hybrid label/semantic filter",
	Long:  `Execute the recognition use case with a filter: COCO labels match exactly ("person", "person*2"), free text matches semantically via CLIP ("person with a red hat*1"). Comma-separated for multiple terms.`,
	Run: func(cmd *cobra.Command, args []string) {
		filter, err := cmd.Flags().GetString("filter")

		if err != nil {
			fmt.Printf("❌ Error: %s\n", err.Error())
			return
		}

		req := dto.RecognitionRequest{
			Filter: filter,
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
	// Hybrid filter (TODO.md § A, docs/adr/clip-backend.md § 12-13):
	// comma-separated terms, each optionally capped with "*N" ("person" =
	// up to 1, "person*2" = up to 2). A COCO-class term matches exactly; a
	// free-text term matches semantically via CLIP against a fixed hidden
	// threshold (tracking.go's defaultSimilarityThreshold — not a flag on
	// purpose, meant to become a GUI control later).
	recognitionCmd.Flags().String("filter", "", `filter spec, e.g. "person", "person*2", "person*2,person with a red hat*1"`)
	err := recognitionCmd.MarkFlagRequired("filter")
	if err != nil {
		fmt.Printf("❌ Error: %s\n", err.Error())
		return
	}
}
