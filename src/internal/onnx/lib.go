package onnx

import (
	"fmt"
	"runtime"
)

// GetOnnxLibrary returns the path to the shared library based on the current OS and architecture.
func GetOnnxLibrary() string {
	fmt.Printf("OS and architecture: %s %s\n", runtime.GOOS, runtime.GOARCH)

	if runtime.GOOS == "windows" {
		if runtime.GOARCH == "amd64" {
			return "src/internal/onnx/libraries/win/onnxruntime.dll"
		}
	}
	if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			return "src/internal/onnx/libraries/osx/onnxruntime_arm64.dylib"
		}
		if runtime.GOARCH == "amd64" {
			return "src/internal/onnx/libraries/osx/onnxruntime_amd64.dylib"
		}

	}
	if runtime.GOOS == "linux" {
		if runtime.GOARCH == "arm64" {
			return "src/internal/onnx/libraries/linux/onnxruntime_arm64.so"
		}
		return "src/internal/onnx/libraries/linux/onnxruntime.so"
	}
	panic("Unable to find a version of the onnxruntime library supporting this system.")
}
