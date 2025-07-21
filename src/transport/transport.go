// transport package provides transport layer functionalities
package transport

import "context"

// TransportRequest agnostic request structure
type TransportRequest[T any] struct {
	Data    T               `json:"data"`
	Context context.Context `json:"-"`
	Source  string          `json:"source"` // "cli", "web", "websocket"
}

// TransportResponse agnostic response structure
type TransportResponse[T any] struct {
	Success bool   `json:"success"`
	Data    *T     `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Source  string `json:"source"`
}
