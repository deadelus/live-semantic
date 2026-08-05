// Package runtime resolves the platform-specific ONNX Runtime shared library,
// shared by every ONNX-backed model implementation (yolo11s, and future models).
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
