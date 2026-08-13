package domain

import "errors"

var (
	// ONNX runtime / model loading errors.
	ErrNilRuntime                = errors.New("nil runtime error")
	ErrModelInitialization       = errors.New("model initialization failed")
	ErrModelOptionInitialization = errors.New("model option initialization failed")

	// Nil-dependency errors, returned when a use case or adapter is wired
	// with a missing collaborator (dependency-injection guard clauses).
	ErrNilContext         = errors.New("context not initialized")
	ErrNilLogger          = errors.New("logger not initialized")
	ErrNilVideoSource     = errors.New("video source not initialized")
	ErrNilDisplayHandler  = errors.New("display handler not initialized")
	ErrNilObjectDetector  = errors.New("object detector not initialized")
	ErrNilSemanticEncoder = errors.New("semantic encoder not initialized")
	ErrNilNotifier        = errors.New("notifier not initialized")
	ErrNilUtils           = errors.New("utils not initialized")
	ErrNilTrackerFactory  = errors.New("tracker factory not initialized")
	ErrNilGalleryRepo     = errors.New("gallery repository not initialized")

	// Camera capture errors.
	ErrNoCameraFound        = errors.New("no camera found")
	ErrCouldNotOpenCamera   = errors.New("could not open camera")
	ErrCameraNotInitialized = errors.New("camera not initialized")

	// Frame acquisition/conversion errors, shared by camera and file input
	// streams.
	ErrCouldNotReadFrameFromCamera = errors.New("could not read frame from camera")
	ErrCouldNotConvertFrameToImage = errors.New("could not convert frame to image")
	ErrCouldNotEncodeFrameToJPEG   = errors.New("could not encode frame to JPEG")

	// Streaming processor errors.
	ErrNilStreamingProcessor = errors.New("streaming processor not initialized")
)
