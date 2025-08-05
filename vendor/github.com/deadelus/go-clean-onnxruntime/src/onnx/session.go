package onnx

import (
	ort "github.com/yalue/onnxruntime_go"
)

// ONNXSession is the structure that holds the ONNX session and tensors for inference.
type ONNXSession struct {
	Session      *ort.AdvancedSession
	TensorInput  *ort.Tensor[float32]
	TensorOutput *ort.Tensor[float32]
	Options      *ort.SessionOptions
}

// InputTensor represents the input tensor for the ONNX model.
// It contains the data and dimensions of the input image.
type InputTensor struct {
	Data   []float32
	Width  int64
	Height int64
}

// OutputTensor represents the output tensor from the ONNX model.
// It contains the data and dimensions of the output predictions.
type OutputTensor struct {
	Data []float32
}

// NewONNXSession initializes the ONNX model and session.
func NewONNXSession(nr *OnnxRuntime) (*ONNXSession, error) {
	onnxSession := &ONNXSession{}

	ort.SetSharedLibraryPath(nr.libraryPath)

	err := ort.InitializeEnvironment()
	if err != nil {
		return nil, err
	}

	onnxSession.SetInputTensor(nr.tensorInputShape)
	onnxSession.SetOutputTensor(nr.tensorOutputShape)

	// Create the ONNX session with the model path and input/output tensors
	session, err := ort.NewAdvancedSession(nr.modelPath,
		[]string{"images"}, []string{"output0"},
		[]ort.ArbitraryTensor{onnxSession.TensorInput},
		[]ort.ArbitraryTensor{onnxSession.TensorOutput},
		onnxSession.Options)

	if err != nil {
		onnxSession.Close()
		return nil, err
	}

	onnxSession.Session = session

	return onnxSession, nil
}

// Close releases resources associated with the ONNX model session.
// This method is essential for preventing memory leaks and ensuring that the ONNX session is properly cleaned
func (onnxSession *ONNXSession) Close() {
	if onnxSession.Session != nil {
		onnxSession.Session.Destroy()
	}
	if onnxSession.TensorInput != nil {
		onnxSession.TensorInput.Destroy()
	}
	if onnxSession.TensorOutput != nil {
		onnxSession.TensorOutput.Destroy()
	}
}

// SetInputTensor defines the expected input tensor shape for the ONNX model.
// Assuming the model expects an input shape of (1, 3, 640, 640)
// Adjust these shapes based on your specific model requirements
func (onnxSession *ONNXSession) SetInputTensor(shape TensorInputShape) {
	inputShape := ort.NewShape(shape.BatchSize, shape.Channels, shape.Height, shape.Width)
	inputTensor, err := ort.NewEmptyTensor[float32](inputShape)

	if err != nil {
		onnxSession.Close()
	}

	onnxSession.TensorInput = inputTensor
}

// SetOutputTensor defines the expected output tensor shape for the ONNX model.
// Assuming the model outputs a tensor with shape (1, 84, 8400)
// Adjust this shape based on your specific model requirements
// For example, if the model outputs bounding boxes, you might have a different shape
// Here we assume the output is a tensor with 84 classes and 8400 detections
func (onnxSession *ONNXSession) SetOutputTensor(shape TensorOutputShape) {
	outputShape := ort.NewShape(shape.BatchSize, shape.Classes, shape.Detections)
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		onnxSession.Close()
	}
	onnxSession.TensorOutput = outputTensor
}
