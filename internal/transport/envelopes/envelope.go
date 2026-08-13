// Package envelopes provides transport-agnostic request/response wrappers
// — the common shape every transport adapter (CLI, API, WebSocket) uses
// to call into transport/handlers, so a handler doesn't need to know
// which transport is calling it.
package envelopes

import "context"

// TransportRequest wraps use-case input data T with transport-agnostic
// metadata (a cancellable Context, and Source identifying the caller for
// logging).
type TransportRequest[T any] struct {
	Data    T               `json:"data"`
	Context context.Context `json:"-"`
	Source  string          `json:"source"` // "cli", "web", "websocket"
}

// TransportResponse wraps a use case's dto.Result[T] into the uniform
// success/data-or-error shape every transport adapter returns.
type TransportResponse[T any] struct {
	Success bool   `json:"success"`
	Data    *T     `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Source  string `json:"source"`
}
