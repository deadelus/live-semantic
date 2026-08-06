// Package consolemetrics provides a minimal implementation of the
// MetricsCollector port that logs metrics via the injected logger.Logger
// instead of collecting/exposing them to a real backend (no
// Prometheus/StatsD/... integration exists yet).
package consolemetrics

import (
	"time"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// ConsoleMetrics implements metrics.MetricsCollector by logging each
// recorded metric as a structured log line.
type ConsoleMetrics struct {
	logger logger.Logger
}

// NewConsoleMetrics creates a new ConsoleMetrics backed by the given logger.
func NewConsoleMetrics(l logger.Logger) *ConsoleMetrics {
	return &ConsoleMetrics{logger: l}
}

// RecordLatency implements MetricsCollector.RecordLatency for ConsoleMetrics.
func (c *ConsoleMetrics) RecordLatency(stage string, d time.Duration) {
	c.logger.Info("metric.latency", map[string]interface{}{
		"stage":       stage,
		"duration_ms": d.Milliseconds(),
	})
}

// IncrementCounter implements MetricsCollector.IncrementCounter for ConsoleMetrics.
func (c *ConsoleMetrics) IncrementCounter(name string) {
	c.logger.Info("metric.counter", map[string]interface{}{
		"name": name,
	})
}

// Cleanup implements MetricsCollector.Cleanup for ConsoleMetrics. No-op:
// nothing is held open (no file handle, no network connection to a real
// metrics backend).
func (c *ConsoleMetrics) Cleanup() {}
