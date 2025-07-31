package domain

import "errors"

var (
	// Define custom errors for the application
	ErrModelInitialization       = errors.New("model initialization failed")
	ErrModelOptionInitialization = errors.New("model option initialization failed")

	// Implementation errors
	ErrNilContext     = errors.New("context not initialized")
	ErrNilLogger      = errors.New("logger not initialized")
	ErrNilVideoSource = errors.New("video source not initialized")
	ErrNilAI          = errors.New("AI not initialized")
	ErrNilNotifier    = errors.New("notifier not initialized")
	ErrNilUtils       = errors.New("utils not initialized")

	// Error for camera-related issues
	ErrCouldNotOpenCamera          = errors.New("could not open camera")
	ErrCouldNotReadFrameFromCamera = errors.New("could not read frame from camera")
	ErrCouldNotConvertFrameToImage = errors.New("could not convert frame to image")
	ErrCouldNotEncodeFrameToJPEG   = errors.New("could not encode frame to JPEG")
)
