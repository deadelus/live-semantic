// Package dto holds the data-transfer objects use cases return to
// transport adapters (CLI, API, WebSocket), keeping application logic
// decoupled from any one transport's serialization concerns.
package dto

// Result is a generic success/data-or-error envelope returned by use
// cases, mirrored 1:1 as the JSON shape transport adapters send back.
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
