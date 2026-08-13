// Package metrics defines the port for operational metrics collection
// (latency, throughput, match rate...). No metrics existed before this
// port (ports/dependency-inversion cleanup work). First implementation is
// a console logger (implementation/metrics/console-metrics), consistent with Phase 1 of the
// roadmap in overview.md; a real backend (Prometheus/StatsD/...) is future
// work, not scoped here.
package metrics

import "time"

// MetricsCollector is the port for recording operational metrics.
type MetricsCollector interface {
	// RecordLatency records how long a named stage took (e.g. "detect",
	// "track", "encode").
	RecordLatency(stage string, d time.Duration)
	// IncrementCounter increments a named counter (e.g. "frames_processed",
	// "frames_dropped").
	IncrementCounter(name string)
	// Cleanup performs any necessary cleanup operations.
	Cleanup()
}
