// Package runtime wraps process-wide ONNX Runtime setup shared by every
// ONNX-backed adapter (yolo11s.Detector, clip.Encoder): resolving the
// platform-specific shared library (LibraryPath), environment/session
// lifecycle (InitEnvironment/DestroyEnvironment, session.go), minimum
// version enforcement (RequireMinVersion, version.go), and Execution
// Provider configuration (Option, options.go) — so those packages don't
// each reimplement the same ORT bootstrapping.
package runtime

import (
	"path/filepath"
	"runtime"
)

// LibraryPath returns the absolute path to the ONNX Runtime shared library
// for the current OS/architecture.
func LibraryPath() string {
	var absPath string

	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			absPath, _ = filepath.Abs("assets/libraries/win/onnxruntime.dll")
		}
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			absPath, _ = filepath.Abs("assets/libraries/osx/onnxruntime_arm64.dylib")
		case "amd64":
			absPath, _ = filepath.Abs("assets/libraries/osx/onnxruntime_amd64.dylib")
		}
	case "linux":
		switch runtime.GOARCH {
		case "arm64":
			absPath, _ = filepath.Abs("assets/libraries/linux/onnxruntime_arm64.so")
		default:
			absPath, _ = filepath.Abs("assets/libraries/linux/onnxruntime.so")
		}
	}

	if absPath == "" {
		panic("Unable to find a version of the onnxruntime library supporting this system.")
	}
	return absPath
}
