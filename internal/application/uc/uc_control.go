package uc

// StopRecognition halts the currently running Recognize loop by stopping
// the input stream. Recognize's video loop is driven by
// streamingInput.Start's internal for-loop condition (see
// implementation/streamer/input.CameraInput.Start) — calling
// StopRecognition() flips that condition, so the blocking Recognize call
// returns shortly after (once the in-flight frame read/callback
// finishes), goes through its own cleanup, and returns normally.
//
// Safe to call even if nothing is currently running — streamer.InputStream
// implementations only flip an internal flag, they don't panic or block
// when called on an idle stream.
//
// Current minimal scope (GUI backend prerequisites): still one session
// for the whole process (the caller — today, transport/adapters/api's
// recognitionController — is responsible for not starting a second
// Recognize concurrently). What that one session reads from can now vary
// per call, since browser camera capture landed (localInput or
// browserInput, dto.RecognitionRequest.Source) — StopRecognition() reads
// activeInput (set by Recognize at the start of its call) rather than
// assuming localInput, so it stops whichever is actually running. Revisit
// once multi-flux gives each session its own UseCase/InputStream pair, at
// which point this will need a session ID.
func (uc *UseCase) StopRecognition() {
	uc.mu.Lock()
	active := uc.activeInput
	uc.mu.Unlock()
	if active != nil {
		active.Stop()
	}
}

// WaitRecognition blocks until any in-flight Recognize call has fully
// returned — see the Recognition interface doc comment for why this
// exists (a real SIGSEGV, not a hypothetical). Safe to call even if
// nothing is currently running (sync.WaitGroup.Wait() on a zero counter
// returns immediately).
func (uc *UseCase) WaitRecognition() {
	uc.activeSessions.Wait()
}
