package errors

import "errors"

var (
	// Define custom errors for the application
	ErrNilContext     = errors.New("context not initialized")
	ErrNilLogger      = errors.New("logger not initialized")
	ErrNilVideoSource = errors.New("video source not initialized")
	ErrNilAIProvider  = errors.New("AI provider not initialized")
	ErrNilAlerter     = errors.New("alerter not initialized")
	ErrNilUtils       = errors.New("utils not initialized")
)
