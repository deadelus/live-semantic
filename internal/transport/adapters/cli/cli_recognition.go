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

	// Label-based filter (TODO.md § A, decision 2026-08-11 — replaced the
	// CLIP similarity-threshold prompt that used to be here, see
	// docs/adr/clip-backend.md § 12): "person" (up to 1), "person*2" (up to
	// 2), "person*2,car" (multiple independent terms, comma-separated).
	// Validated against the 80 COCO classes by application/uc.parseFilterSpec
	// once the request reaches RecognitionUseCase — a typo surfaces as a
	// clear error there, not a silent "detects nothing".
	var qs = []*survey.Question{
		{
			Name:     "filter",
			Prompt:   &survey.Input{Message: "📝 What do you want to recognize? (COCO label(s), e.g. \"person\", \"person*2\", \"person*2,car\")"},
			Validate: survey.Required,
		},
	}

	answers := struct {
		Filter string `survey:"filter"`
	}{}

	if err := survey.Ask(qs, &answers); err != nil {
		return err
	}

	// Confirmer avant création
	confirm := false
	confirmPrompt := &survey.Confirm{
		Message: fmt.Sprintf("Recognize %s?", answers.Filter),
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
			Filter: answers.Filter,
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
