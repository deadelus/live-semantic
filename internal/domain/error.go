package domain

import "errors"

var (
	// Define custom errors for the application
	ErrNilRuntime                = errors.New("nil runtime error")
	ErrModelInitialization       = errors.New("model initialization failed")
	ErrModelOptionInitialization = errors.New("model option initialization failed")

	// Implementation errors
	ErrNilContext        = errors.New("context not initialized")
	ErrNilLogger         = errors.New("logger not initialized")
	ErrNilVideoSource    = errors.New("video source not initialized")
	ErrNilDisplayHandler = errors.New("display handler not initialized")
	ErrNilAI             = errors.New("AI not initialized")
	ErrNilNotifier       = errors.New("notifier not initialized")
	ErrNilUtils          = errors.New("utils not initialized")
	ErrNilTrackerFactory = errors.New("tracker factory not initialized")

	// Error for camera-related issues
	ErrNoCameraFound        = errors.New("no camera found")
	ErrCouldNotOpenCamera   = errors.New("could not open camera")
	ErrCameraNotInitialized = errors.New("camera not initialized")

	ErrCouldNotReadFrameFromCamera = errors.New("could not read frame from camera")
	ErrCouldNotConvertFrameToImage = errors.New("could not convert frame to image")
	ErrCouldNotEncodeFrameToJPEG   = errors.New("could not encode frame to JPEG")

	// Error for streaming processor issues
	ErrNilStreamingProcessor = errors.New("streaming processor not initialized")
)
