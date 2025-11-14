// Package forwarder handles forwarding log data to Splunk HEC endpoints.
package forwarder

import (
	"context"
)

// ReloadableConfig holds configuration parameters that can be safely reloaded at runtime.
// These parameters do not require restarting connections or forwarders.
type ReloadableConfig struct {
	Token      string
	SourceType string
	UseGzip    bool
}

// Forwarder defines the interface for forwarding log data to one or more HEC endpoints.
// Implementations can be single-target (HEC) or multi-target (MultiHEC) forwarders.
type Forwarder interface {
	// Forward sends data to the configured HEC endpoint(s).
	// The connID parameter is used for logging and correlation.
	// Returns an error if forwarding fails.
	Forward(connID string, data []byte) error

	// HealthCheck verifies that the HEC endpoint(s) and token(s) are valid.
	// Returns an error if any endpoint is unreachable or has invalid credentials.
	HealthCheck() error

	// Shutdown gracefully shuts down the forwarder, flushing any remaining batched data.
	// The provided context controls the shutdown timeout.
	// Returns an error if the shutdown times out before flushing completes.
	Shutdown(ctx context.Context) error

	// UpdateConfig updates the reloadable configuration parameters in a thread-safe manner.
	// Only safe parameters (token, sourcetype, gzip) are updated.
	// Parameters that require restart (URL, batching, circuit breaker) are not affected.
	UpdateConfig(cfg ReloadableConfig)
}
