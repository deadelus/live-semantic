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
