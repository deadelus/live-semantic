package provider

import (
	"live-semantic/src/infrastructure"
	"live-semantic/src/internal/utils"
)

func NewVideoSource() infrastructure.VideoSource {
	return nil // Replace with actual implementation
}

func NewAIProvider() infrastructure.AIProvider {
	return nil // Replace with actual implementation
}

func NewAlerter() infrastructure.Alerter {
	return nil // Replace with actual implementation
}

func NewUtils() utils.Utils {
	return &utils.Toolbox{}
}
