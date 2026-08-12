package cli

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/AlecAivazis/survey/v2"
)

// Run drives the interactive CLI's main menu loop until the user exits or
// ctx is cancelled (e.g. process shutdown signal).
func (s *SurveyController) Run(ctx context.Context) error {
	fmt.Printf("🚀 Welcome to %s Interactive CLI!\n", os.Getenv("APP_NAME"))

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Context cancelled, exiting CLI for graceful shutdown.")
			return ctx.Err()
		default:
			var action string
			prompt := &survey.Select{
				Message: "What would you like to do?",
				Options: []string{
					"📸 Recognition",
					"⚙️ Settings",
					"❌ Exit",
				},
			}

			if err := survey.AskOne(prompt, &action); err != nil {
				return err
			}

			switch action {
			case "📸 Recognition":
				if err := s.createRecognitionFlow(); err != nil {
					fmt.Printf("❌ Error: %v\n", err)
				}
			case "⚙️ Settings":
				s.showSettings()
			case "❌ Exit":
				fmt.Println("👋 Goodbye!")
				// Send SIGTERM to self for graceful shutdown, then wait
				// briefly for it to be handled before this function returns.
				p, err := os.FindProcess(os.Getpid())
				if err == nil {
					_ = p.Signal(syscall.SIGTERM)
				}
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}
	}
}
