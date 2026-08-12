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
// H1 minimal scope (TODO.md § H1): still one session for the whole
// process (the caller — today, transport/adapters/api's
// recognitionController — is responsible for not starting a second
// RecognitionUseCase concurrently). What that one session reads from can
// now vary per call (TODO.md § H2 "capture caméra navigateur": localInput
// or browserInput, dto.RecognitionRequest.Source) — Stop() reads
// activeInput (set by RecognitionUseCase at the start of its call) rather
// than assuming localInput, so it stops whichever is actually running.
// Revisit once multi-flux gives each session its own UseCase/InputStream
// pair, at which point Stop will need a session ID.
func (uc *UseCase) Stop() {
	uc.mu.Lock()
	active := uc.activeInput
	uc.mu.Unlock()
	if active != nil {
		active.Stop()
	}
}

// Wait blocks until any in-flight RecognitionUseCase call has fully
// returned — see the UseCases interface doc comment for why this exists
// (a real SIGSEGV, not a hypothetical). Safe to call even if nothing is
// currently running (sync.WaitGroup.Wait() on a zero counter returns
// immediately).
func (uc *UseCase) Wait() {
	uc.activeSessions.Wait()
}
