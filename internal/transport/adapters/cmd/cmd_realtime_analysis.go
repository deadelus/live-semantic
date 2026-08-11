package cmd

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"

	"github.com/spf13/cobra"
)

var recognitionCmd = &cobra.Command{
	Use:   "recognition",
	Short: "Run recognition with a label filter",
	Long:  `Execute the recognition use case with a COCO label filter (e.g. "person", "person*2", "person*2,car").`,
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
	// Label-based filter (TODO.md § A, decision 2026-08-11 — replaced the
	// CLIP similarity-threshold flag that used to be here, see
	// docs/adr/clip-backend.md § 12): comma-separated COCO label(s), each
	// optionally capped with "*N" ("person" = up to 1, "person*2" = up to
	// 2, "person*2,car" = two independent terms). Validated against the 80
	// COCO classes by application/uc.parseFilterSpec once the request
	// reaches RecognitionUseCase.
	recognitionCmd.Flags().String("filter", "", `COCO label filter, e.g. "person", "person*2", "person*2,car"`)
	err := recognitionCmd.MarkFlagRequired("filter")
	if err != nil {
		fmt.Printf("❌ Error: %s\n", err.Error())
		return
	}
}
