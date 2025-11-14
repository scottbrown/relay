package forwarder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scottbrown/relay/internal/circuitbreaker"
	"github.com/scottbrown/relay/internal/config"
)

// MultiHEC manages multiple HEC forwarders with configurable routing.
// It implements the Forwarder interface and supports multiple routing modes:
// - All (broadcast): sends to all targets
// - Primary-Failover: tries primary first, fails over to secondary
// - Round-Robin: distributes logs across targets
type MultiHEC struct {
	targets     []*HEC
	targetNames []string
	mode        config.RoutingMode
	mu          sync.RWMutex
	rrCounter   uint64 // atomic counter for round-robin
}

// NewMulti creates a new multi-target HEC forwarder with the given targets and routing mode.
// Each target is initialized as a separate HEC forwarder instance.
func NewMulti(targets []config.HECTarget, mode config.RoutingMode) (*MultiHEC, error) {
	if len(targets) == 0 {
		return nil, errors.New("at least one HEC target is required")
	}

	// Default to "all" mode if not specified
	if mode == "" {
		mode = config.RoutingModeAll
	}

	hecInstances := make([]*HEC, 0, len(targets))
	targetNames := make([]string, 0, len(targets))

	for _, target := range targets {
		// Convert target config to HEC config
		hecConfig := Config{
			URL:        target.HECURL,
			Token:      target.HECToken,
			SourceType: target.SourceType,
		}

		// Apply gzip setting
		if target.Gzip != nil {
			hecConfig.UseGzip = *target.Gzip
		}

		// Apply client timeout
		if target.ClientTimeout > 0 {
			hecConfig.ClientTimeout = time.Duration(target.ClientTimeout) * time.Second
		}

		// Convert batch config
		batchConfig := BatchConfig{
			Enabled:       false,
			MaxSize:       100,
			MaxBytes:      1 << 20,
			FlushInterval: 1 * time.Second,
		}
		if target.Batch != nil {
			if target.Batch.Enabled != nil {
				batchConfig.Enabled = *target.Batch.Enabled
			}
			if target.Batch.MaxSize > 0 {
				batchConfig.MaxSize = target.Batch.MaxSize
			}
			if target.Batch.MaxBytes > 0 {
				batchConfig.MaxBytes = target.Batch.MaxBytes
			}
			if target.Batch.FlushInterval > 0 {
				batchConfig.FlushInterval = time.Duration(target.Batch.FlushInterval) * time.Second
			}
		}
		hecConfig.Batch = batchConfig

		// Convert circuit breaker config
		cbConfig := circuitbreaker.DefaultConfig()
		if target.CircuitBreaker != nil {
			if target.CircuitBreaker.Enabled != nil && !*target.CircuitBreaker.Enabled {
				cbConfig.FailureThreshold = 0
			}
			if target.CircuitBreaker.FailureThreshold > 0 {
				cbConfig.FailureThreshold = target.CircuitBreaker.FailureThreshold
			}
			if target.CircuitBreaker.SuccessThreshold > 0 {
				cbConfig.SuccessThreshold = target.CircuitBreaker.SuccessThreshold
			}
			if target.CircuitBreaker.Timeout > 0 {
				cbConfig.Timeout = time.Duration(target.CircuitBreaker.Timeout) * time.Second
			}
			if target.CircuitBreaker.HalfOpenMaxCalls > 0 {
				cbConfig.HalfOpenMaxCalls = target.CircuitBreaker.HalfOpenMaxCalls
			}
		}
		hecConfig.CircuitBreaker = cbConfig

		hec := New(hecConfig)
		hecInstances = append(hecInstances, hec)
		targetNames = append(targetNames, target.Name)
	}

	return &MultiHEC{
		targets:     hecInstances,
		targetNames: targetNames,
		mode:        mode,
	}, nil
}

// Forward sends data to one or more HEC targets based on the configured routing mode.
func (m *MultiHEC) Forward(connID string, data []byte) error {
	switch m.mode {
	case config.RoutingModeAll:
		return m.forwardAll(connID, data)
	case config.RoutingModePrimaryFailover:
		return m.forwardPrimaryFailover(connID, data)
	case config.RoutingModeRoundRobin:
		return m.forwardRoundRobin(connID, data)
	default:
		return fmt.Errorf("unknown routing mode: %s", m.mode)
	}
}

// forwardAll sends data to all targets concurrently (broadcast mode)
func (m *MultiHEC) forwardAll(connID string, data []byte) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(m.targets))

	for i, target := range m.targets {
		wg.Add(1)
		go func(hec *HEC, name string) {
			defer wg.Done()
			if err := hec.Forward(connID, data); err != nil {
				slog.Warn("HEC forward failed for target",
					"target", name,
					"conn_id", connID,
					"error", err)
				errCh <- fmt.Errorf("target %s: %w", name, err)
			} else {
				slog.Debug("HEC forward succeeded for target",
					"target", name,
					"conn_id", connID)
			}
		}(target, m.targetNames[i])
	}

	wg.Wait()
	close(errCh)

	// Collect all errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	// Return error if any target failed
	if len(errs) > 0 {
		return fmt.Errorf("forward to %d/%d targets failed: %v", len(errs), len(m.targets), errs)
	}

	return nil
}

// forwardPrimaryFailover tries primary target first, fails over to secondary targets on error
func (m *MultiHEC) forwardPrimaryFailover(connID string, data []byte) error {
	for i, target := range m.targets {
		err := target.Forward(connID, data)
		if err == nil {
			if i > 0 {
				slog.Info("failover successful",
					"target", m.targetNames[i],
					"conn_id", connID,
					"attempt", i+1)
			}
			return nil
		}

		slog.Warn("HEC forward failed, trying next target",
			"target", m.targetNames[i],
			"conn_id", connID,
			"error", err,
			"attempt", i+1,
			"remaining", len(m.targets)-i-1)
	}

	return fmt.Errorf("all %d targets failed for primary-failover", len(m.targets))
}

// forwardRoundRobin distributes logs across targets in round-robin fashion
func (m *MultiHEC) forwardRoundRobin(connID string, data []byte) error {
	// Atomically increment and get counter
	count := atomic.AddUint64(&m.rrCounter, 1)
	// Safe conversion: modulo ensures result fits in int since it's bounded by len(m.targets)
	numTargets := uint64(len(m.targets))
	// #nosec G115 -- Modulo operation guarantees result < numTargets, which is derived from len() (an int)
	idx := int((count - 1) % numTargets)

	target := m.targets[idx]
	targetName := m.targetNames[idx]

	err := target.Forward(connID, data)
	if err != nil {
		slog.Warn("HEC forward failed in round-robin",
			"target", targetName,
			"conn_id", connID,
			"error", err)
		return fmt.Errorf("target %s: %w", targetName, err)
	}

	slog.Debug("HEC forward succeeded in round-robin",
		"target", targetName,
		"conn_id", connID)
	return nil
}

// HealthCheck verifies connectivity to all configured HEC targets.
// Returns an error if any target fails the health check.
func (m *MultiHEC) HealthCheck() error {
	var errs []error

	for i, target := range m.targets {
		if err := target.HealthCheck(); err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", m.targetNames[i], err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("health check failed for %d/%d targets: %v", len(errs), len(m.targets), errs)
	}

	return nil
}

// Shutdown gracefully shuts down all HEC forwarders, flushing any remaining batched data.
func (m *MultiHEC) Shutdown(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(m.targets))

	for i, target := range m.targets {
		wg.Add(1)
		go func(hec *HEC, name string) {
			defer wg.Done()
			if err := hec.Shutdown(ctx); err != nil {
				errCh <- fmt.Errorf("target %s: %w", name, err)
			}
		}(target, m.targetNames[i])
	}

	wg.Wait()
	close(errCh)

	// Collect all errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown failed for %d/%d targets: %v", len(errs), len(m.targets), errs)
	}

	return nil
}

// UpdateConfig updates the reloadable configuration parameters for all targets in a thread-safe manner.
// Only safe parameters (token, sourcetype, gzip) are updated.
// Parameters that require restart (URL, batching, circuit breaker) are not affected.
func (m *MultiHEC) UpdateConfig(cfg ReloadableConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update all targets
	for i, target := range m.targets {
		target.UpdateConfig(cfg)
		slog.Debug("updated multi-target HEC configuration",
			"target", m.targetNames[i])
	}

	slog.Info("multi-target HEC configuration updated",
		"targets", len(m.targets))
}
