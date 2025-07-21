package dto

// Result pattern simple
type Result[T any] struct {
	Success bool   `json:"success"`
	Data    *T     `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Success creates a successful result with the provided data.
func Success[T any](data T) Result[T] {
	return Result[T]{Success: true, Data: &data}
}

// Failure creates a failed result with the provided error message.
func Failure[T any](err string) Result[T] {
	return Result[T]{Success: false, Error: err}
}
