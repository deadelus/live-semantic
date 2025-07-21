package cli

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
)

func (s *SurveyController) Run() error {
	fmt.Println("🚀 Welcome to Live Semantic Interactive CLI!")

	for {
		var action string
		prompt := &survey.Select{
			Message: "What would you like to do?",
			Options: []string{
				"📝 Create Task",
				"📋 List Tasks",
				"⚙️ Settings",
				"❌ Exit",
			},
		}

		if err := survey.AskOne(prompt, &action); err != nil {
			return err
		}

		switch action {
		case "📝 Create Task":
			if err := s.createTaskFlow(); err != nil {
				fmt.Printf("❌ Error: %v\n", err)
			}
		case "📋 List Tasks":
			s.listTasks()
		case "⚙️ Settings":
			s.showSettings()
		case "❌ Exit":
			fmt.Println("👋 Goodbye!")
			return nil
		}
	}
}
