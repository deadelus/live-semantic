package cli

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"
	"live-semantic/internal/transport/envelopes"

	"github.com/AlecAivazis/survey/v2"
)

func (s *SurveyController) createRecognitionFlow() error {
	fmt.Println("\n📸 Recognition...")

	var qs = []*survey.Question{
		{
			Name:     "filter",
			Prompt:   &survey.Input{Message: "📝 What do you want to recognize?"},
			Validate: survey.Required,
		},
		{
			Name:     "similarity-threshold",
			Prompt:   &survey.Input{Message: "🔍 Similarity threshold between 0.0 and 1.0 (default: 0.25 — CLIP cosine similarity, not YOLO confidence: real scores sit around 0.20-0.30, see docs/adr/clip-backend.md § 7)", Default: "0.25"},
			Validate: survey.Required,
		},
	}

	answers := struct {
		Filter              string  `survey:"filter"`
		SimilarityThreshold float32 `survey:"similarity-threshold"`
	}{}

	if err := survey.Ask(qs, &answers); err != nil {
		return err
	}

	// Confirmer avant création
	confirm := false
	confirmPrompt := &survey.Confirm{
		Message: fmt.Sprintf("Recognize %s? with similarity threshold %.2f", answers.Filter, answers.SimilarityThreshold),
	}
	if err := survey.AskOne(confirmPrompt, &confirm); err != nil {
		return err
	}

	if !confirm {
		fmt.Println("⏹️ Action cancelled")
		return nil
	}

	// Créer via le handler
	req := envelopes.TransportRequest[dto.RecognitionRequest]{
		Data: dto.RecognitionRequest{
			Filter:              answers.Filter,
			SimilarityThreshold: answers.SimilarityThreshold,
		},
		Context: context.Background(),
		Source:  "interactive",
	}

	response := s.handler.HandleRecognitionUseCase(req)

	if response.Success {
		fmt.Printf("\n✅ Recognition request created successfully!\n")
	} else {
		fmt.Printf("\n❌ Error: %s\n\n", response.Error)
	}

	return nil
}
