package uc

// Stop halts the currently running RecognitionUseCase loop by stopping the
// input stream. RecognitionUseCase's video loop is driven by
// streamingInput.Start's internal for-loop condition (see
// implementation/streamer/input.CameraInput.Start) — calling Stop() flips
// that condition, so the blocking RecognitionUseCase call returns shortly
// after (once the in-flight frame read/callback finishes), goes through
// its own cleanup, and returns normally.
//
// Safe to call even if nothing is currently running — streamer.InputStream
// implementations only flip an internal flag, they don't panic or block
// when called on an idle stream.
//
// H1 minimal scope (TODO.md § H1): uc.streamingInput is one shared field
// set once at construction (NewUseCase), not scoped per call — so this
// stops *the* session, there being only one. The caller (today,
// transport/adapters/api's recognitionController) is responsible for not
// starting a second RecognitionUseCase concurrently on the same UseCase.
// Revisit once multi-flux gives each session its own UseCase/InputStream
// pair, at which point Stop will need a session ID.
func (uc *UseCase) Stop() {
	uc.streamingInput.Stop()
}
