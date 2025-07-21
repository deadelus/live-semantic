package cli

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
)

func (s *SurveyController) showSettings() {
	var setting string
	prompt := &survey.Select{
		Message: "⚙️ Settings:",
		Options: []string{
			"🔊 Log Level",
			"🎨 Theme",
			"🔧 Advanced",
			"🔙 Back",
		},
	}

	if err := survey.AskOne(prompt, &setting); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	switch setting {
	case "🔊 Log Level":
		s.configureLogLevel()
	case "🎨 Theme":
		fmt.Println("🎨 Theme configuration coming soon...")
	case "🔧 Advanced":
		fmt.Println("🔧 Advanced settings coming soon...")
	}
}

func (s *SurveyController) configureLogLevel() {
	var level string
	prompt := &survey.Select{
		Message: "Select log level:",
		Options: []string{"DEBUG", "INFO", "WARN", "ERROR"},
		Default: "INFO",
	}

	if err := survey.AskOne(prompt, &level); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✅ Log level set to: %s\n", level)
}
