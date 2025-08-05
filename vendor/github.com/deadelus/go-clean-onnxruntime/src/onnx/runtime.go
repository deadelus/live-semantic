// Package onnx provides structures and methods for handling ONNX model runtime configurations.
package onnx

// OnnxRuntime holds the model path and library paths for neural inference.
type OnnxRuntime struct {
	// modelPath is the path to the ML model file.
	modelPath string
	// libraryPath is the path to the OS library.
	libraryPath string
	// TensorInputShape
	tensorInputShape TensorInputShape
	// TensorOutputShape
	tensorOutputShape TensorOutputShape
}

// TensorInputShape defines the expected shape of input tensors for the ONNX model.
type TensorInputShape struct {
	BatchSize int64
	Channels  int64
	Height    int64
	Width     int64
}

// TensorOutputShape defines the expected shape of output tensors for the ONNX model.
type TensorOutputShape struct {
	BatchSize  int64
	Classes    int64
	Detections int64
}

// NewOnnxRuntime creates a new OnnxRuntime instance with the given model and library paths and tensor shapes.
func NewOnnxRuntime(modelPath, libraryPath string, inputShape TensorInputShape, outputShape TensorOutputShape) *OnnxRuntime {
	return &OnnxRuntime{
		modelPath:         modelPath,
		libraryPath:       libraryPath,
		tensorInputShape:  inputShape,
		tensorOutputShape: outputShape,
	}
}

// GetModelPath returns the path to the ONNX model file.
func (o *OnnxRuntime) GetModelPath() string {
	return o.modelPath
}

// GetLibraryPath returns the path to the ONNX runtime library.
func (o *OnnxRuntime) GetLibraryPath() string {
	return o.libraryPath
}

// GetTensorInputShape returns the expected input tensor shape for the ONNX model.
func (o *OnnxRuntime) GetTensorInputShape() TensorInputShape {
	return o.tensorInputShape
}

// GetTensorOutputShape returns the expected output tensor shape for the ONNX model.
func (o *OnnxRuntime) GetTensorOutputShape() TensorOutputShape {
	return o.tensorOutputShape
}
