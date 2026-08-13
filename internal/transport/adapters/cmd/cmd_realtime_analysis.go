package cmd

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"

	"github.com/spf13/cobra"
)

// recognitionCmd runs one Recognize call synchronously (blocks until the
// video loop stops), same use case as the interactive CLI/REST paths, via
// the package-level useCases wired in by Execute (root.go).
var recognitionCmd = &cobra.Command{
	Use:   "recognition",
	Short: "Run recognition with a hybrid label/semantic filter",
	Long:  `Execute the recognition use case with a filter: "key[*cap][+option[=value]]...", comma-separated. COCO labels match exactly ("person", "person*2"), free text matches semantically via CLIP ("person with a red hat*1"). "+overlap" (default false) lets a term claim a box another term already claimed this cycle.`,
	Run: func(cmd *cobra.Command, args []string) {
		filter, err := cmd.Flags().GetString("filter")

		if err != nil {
			fmt.Printf("❌ Error: %s\n", err.Error())
			return
		}

		req := dto.RecognitionRequest{
			Filter: filter,
		}

		result, err := useCases.Recognize(context.Background(), req)
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
	// Hybrid filter (docs/adr/clip-backend.md § 12-13/16):
	// comma-separated terms, each optionally capped with "*N" ("person" =
	// up to 1, "person*2" = up to 2) and/or extended with "+option[=value]"
	// (currently just "+overlap", default false, parsed but not yet
	// consulted by reanchor). A COCO-class key matches exactly; a
	// free-text key matches semantically via CLIP against a fixed hidden
	// threshold (tracking.go's defaultSimilarityThreshold — not a flag on
	// purpose, meant to become a GUI control later).
	recognitionCmd.Flags().String("filter", "", `filter spec, e.g. "person", "person*2", "person*2,person with a red hat*1+overlap"`)
	err := recognitionCmd.MarkFlagRequired("filter")
	if err != nil {
		fmt.Printf("❌ Error: %s\n", err.Error())
		return
	}
}
