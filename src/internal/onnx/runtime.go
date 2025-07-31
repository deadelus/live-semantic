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

func NewOnnxRuntime(modelPath, libraryPath string, inputShape TensorInputShape, outputShape TensorOutputShape) *OnnxRuntime {
	return &OnnxRuntime{
		modelPath:         modelPath,
		libraryPath:       libraryPath,
		tensorInputShape:  inputShape,
		tensorOutputShape: outputShape,
	}
}
