package cli

import (
	"context"
	"fmt"
	"live-semantic/internal/application/dto"
	"live-semantic/internal/transport/envelopes"

	"github.com/AlecAivazis/survey/v2"
)

// createRecognitionFlow prompts interactively for a filter spec, confirms,
// then submits it through the same handler/envelope path the REST API
// uses (envelopes.TransportRequest, Source: "interactive").
func (s *SurveyController) createRecognitionFlow() error {
	fmt.Println("\n📸 Recognition...")

	// Hybrid filter (docs/adr/clip-backend.md § 12-13/16):
	// "key[*cap][+option[=value]]...", comma-separated for multiple terms.
	// A key that's one of the 80 COCO classes matches exactly, no
	// similarity score involved ("person" up to 1, "person*2" up to 2). A
	// key that isn't a COCO class is matched semantically via CLIP against
	// a fixed, hidden default threshold ("person with a red hat*1" — see
	// tracking.go's defaultSimilarityThreshold; not a CLI knob on purpose,
	// meant to become a GUI control later). "+overlap" (default false)
	// lets a term claim a candidate box another term already claimed this
	// cycle — parsed, not yet consulted by reanchor.
	var qs = []*survey.Question{
		{
			Name:     "filter",
			Prompt:   &survey.Input{Message: "📝 What do you want to recognize? (COCO label(s) and/or free text, e.g. \"person\", \"person*2\", \"person*2,person with a red hat*1+overlap\")"},
			Validate: survey.Required,
		},
	}

	answers := struct {
		Filter string `survey:"filter"`
	}{}

	if err := survey.Ask(qs, &answers); err != nil {
		return err
	}

	// Confirm before submitting.
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

	// Submit via the handler.
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
