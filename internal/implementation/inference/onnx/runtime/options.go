package runtime

import ort "github.com/yalue/onnxruntime_go"

// Option configures ONNX Runtime session options — in particular, which
// Execution Provider(s) the session should try, in order of preference.
// See docs/adr/inference-runtimes.md §5: switching hardware backend is a
// matter of which Options are passed here, nothing else in the pipeline
// changes.
type Option func(*ort.SessionOptions) error

// WithCUDA appends the CUDA Execution Provider (NVIDIA GPU).
func WithCUDA() Option {
	return func(so *ort.SessionOptions) error {
		cudaOpts, err := ort.NewCUDAProviderOptions()
		if err != nil {
			return err
		}
		defer cudaOpts.Destroy()
		return so.AppendExecutionProviderCUDA(cudaOpts)
	}
}

// WithTensorRT appends the TensorRT Execution Provider (NVIDIA GPU, AOT
// compiled). First session creation is slow (graph compilation); see
// docs/adr/inference-runtimes.md §5.
func WithTensorRT() Option {
	return func(so *ort.SessionOptions) error {
		trtOpts, err := ort.NewTensorRTProviderOptions()
		if err != nil {
			return err
		}
		defer trtOpts.Destroy()
		return so.AppendExecutionProviderTensorRT(trtOpts)
	}
}

// WithOpenVINO appends the OpenVINO Execution Provider. deviceType is e.g.
// "CPU", "GPU" (Intel iGPU), "NPU".
func WithOpenVINO(deviceType string) Option {
	return func(so *ort.SessionOptions) error {
		return so.AppendExecutionProviderOpenVINO(map[string]string{"device_type": deviceType})
	}
}

// NewSessionOptions builds an *ort.SessionOptions from the given Options.
// Called with no Option, it returns CPU-only default options — today's
// behavior, unchanged. The caller owns the returned options and must
// Destroy() it once the session has been created.
func NewSessionOptions(opts ...Option) (*ort.SessionOptions, error) {
	so, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if err := opt(so); err != nil {
			so.Destroy()
			return nil, err
		}
	}
	return so, nil
}
