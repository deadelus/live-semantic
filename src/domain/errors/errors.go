package errors

import "errors"

var (
	ErrNilLogger      = errors.New("logger not initialized")
	ErrNilVideoSource = errors.New("video source not initialized")
	ErrNilAIProvider  = errors.New("AI provider not initialized")
	ErrNilAlerter     = errors.New("alerter not initialized")
)
