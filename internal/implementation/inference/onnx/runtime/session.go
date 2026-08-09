package runtime

import ort "github.com/yalue/onnxruntime_go"

// InitEnvironment points the ORT bindings at the platform shared library and
// initializes the (process-wide, singleton) ONNX Runtime environment. Safe to
// call once per model: subsequent calls are no-ops once initialized.
func InitEnvironment(libraryPath string) error {
	if ort.IsInitialized() {
		return nil
	}
	ort.SetSharedLibraryPath(libraryPath)
	return ort.InitializeEnvironment()
}

// DestroyEnvironment explicitly releases the process-wide ORT environment.
//
// Must be called before process exit — found and confirmed via lldb
// (TODO.md, bug critique SIGABRT) on 2026-08-10: without this, ORT's own
// internal OrtEnv singleton gets torn down by a C++ static destructor at
// process exit instead, which throws an uncaught std::system_error ("mutex
// lock failed: Invalid argument") and crashes with SIGABRT. This is a
// documented upstream ONNX Runtime bug on macOS (1.21.x-1.22.x — the
// version bundled here is 1.22.0 — microsoft/onnxruntime#24579, fixed
// upstream via PR #26445 but only in the Node.js binding, not the core C
// API this project's binding calls directly). Calling ReleaseOrtEnv
// ourselves, explicitly, before exit avoids the buggy path entirely —
// confirmed 100% reproducible in both directions (3/3 crashes without
// this call, 3/3 clean exits with it, same binary).
//
// Safe to call even if the environment was never initialized (IsInitialized
// guards it) or already destroyed (Detector.Cleanup calling this twice
// across e.g. a failed re-init would just be a no-op the second time).
func DestroyEnvironment() error {
	if !ort.IsInitialized() {
		return nil
	}
	return ort.DestroyEnvironment()
}
