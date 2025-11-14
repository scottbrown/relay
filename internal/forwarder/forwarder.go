// Package forwarder handles forwarding log data to Splunk HEC endpoints.
// It supports batching, gzip compression, retries with exponential backoff, and circuit breaker protection.
package forwarder

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/scottbrown/relay/internal/circuitbreaker"
	"github.com/scottbrown/relay/internal/metrics"
)

// BatchConfig holds configuration for batching multiple log lines before forwarding.
// When enabled, logs are accumulated and sent together to reduce network overhead.
type BatchConfig struct {
	Enabled       bool
	MaxSize       int           // Maximum lines per batch
	MaxBytes      int           // Maximum bytes per batch
	FlushInterval time.Duration // Maximum time before flushing
}

// RetryConfig holds configuration for retry behaviour with exponential backoff.
// These parameters control how many times the forwarder will retry failed HEC requests
// and how long it will wait between attempts.
type RetryConfig struct {
	MaxAttempts       int           // Maximum number of retry attempts (default: 5)
	InitialBackoff    time.Duration // Initial backoff duration (default: 250ms)
	BackoffMultiplier float64       // Backoff multiplier for exponential backoff (default: 2.0)
	MaxBackoff        time.Duration // Maximum backoff duration (default: 30s)
}

// Config holds configuration for the Splunk HEC forwarder.
type Config struct {
	URL            string
	Token          string
	SourceType     string
	UseGzip        bool
	ClientTimeout  time.Duration // HTTP client timeout for HEC requests
	Batch          BatchConfig
	CircuitBreaker circuitbreaker.Config
	Retry          RetryConfig
}

// batch holds the current batch state
type batch struct {
	lines [][]byte
	size  int
	timer *time.Timer
}

// HEC represents a Splunk HTTP Event Collector forwarder.
// It handles sending log data to Splunk HEC with retry logic and circuit breaker protection.
//
// HEC is safe for concurrent use by multiple goroutines.
type HEC struct {
	config         Config
	configMu       sync.RWMutex // Protects reloadable config fields
	client         *http.Client
	circuitBreaker *circuitbreaker.CircuitBreaker

	// Batch state (only used when batch.Enabled is true)
	mu       sync.Mutex
	batch    *batch
	flushCh  chan struct{}
	shutDown chan struct{}
	wg       sync.WaitGroup
}

// New creates a new HEC forwarder with the given configuration.
// If batching is enabled, a background worker is started to handle batch flushing.
func New(config Config) *HEC {
	// Use configured timeout, default to 15 seconds if not set
	clientTimeout := config.ClientTimeout
	if clientTimeout == 0 {
		clientTimeout = 15 * time.Second
	}

	h := &HEC{
		config:         config,
		client:         &http.Client{Timeout: clientTimeout},
		circuitBreaker: circuitbreaker.New(config.CircuitBreaker),
	}

	// Initialize batch mode if enabled
	if config.Batch.Enabled {
		h.batch = &batch{
			lines: make([][]byte, 0, config.Batch.MaxSize),
		}
		h.flushCh = make(chan struct{}, 1)
		h.shutDown = make(chan struct{})

		// Start flush worker
		h.wg.Add(1)
		go h.flushWorker()
	}

	return h
}

// Forward sends data to Splunk HEC with retry logic and circuit breaker protection.
// If batching is enabled, data is added to the current batch instead of being sent immediately.
// If HEC URL or token is empty, this method returns nil (forwarding disabled).
// The connID parameter is used for logging and correlation.
func (h *HEC) Forward(connID string, data []byte) error {
	h.configMu.RLock()
	url := h.config.URL
	token := h.config.Token
	batchEnabled := h.config.Batch.Enabled
	h.configMu.RUnlock()

	if url == "" || token == "" {
		return nil // HEC forwarding disabled
	}

	// If batching is enabled, add to batch
	if batchEnabled {
		return h.addToBatch(data)
	}

	// Otherwise send immediately
	slog.Debug("forwarding to HEC", "conn_id", connID, "hec_url", url)

	return h.circuitBreaker.Call(func() error {
		return h.sendWithRetry(connID, data)
	})
}

// HealthCheck verifies that the HEC endpoint and token are valid.
// It sends a GET request to the HEC health endpoint and checks for a 200 OK response.
// Returns an error if the endpoint is unreachable, the token is invalid, or not configured.
func (h *HEC) HealthCheck() error {
	h.configMu.RLock()
	url := h.config.URL
	token := h.config.Token
	h.configMu.RUnlock()

	if url == "" || token == "" {
		return errors.New("HEC URL or token not configured")
	}

	// Convert the collector URL to health check URL
	healthURL := h.getHealthURL()

	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Splunk "+token)

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return errors.New("invalid Splunk HEC token (403 Forbidden)")
		}
		return errors.New("HEC health check failed with status: " + resp.Status)
	}

	return nil
}

// getHealthURL converts collector URL to health endpoint URL
func (h *HEC) getHealthURL() string {
	h.configMu.RLock()
	url := h.config.URL
	h.configMu.RUnlock()

	// Replace common collector endpoints with health endpoint
	if strings.Contains(url, "/services/collector/raw") {
		return strings.Replace(url, "/services/collector/raw", "/services/collector/health", 1)
	}
	if strings.Contains(url, "/services/collector/event") {
		return strings.Replace(url, "/services/collector/event", "/services/collector/health", 1)
	}
	if strings.Contains(url, "/services/collector") && !strings.Contains(url, "/services/collector/health") {
		return strings.Replace(url, "/services/collector", "/services/collector/health", 1)
	}

	// If URL doesn't match expected patterns, append health endpoint
	baseURL := strings.TrimSuffix(url, "/")
	if strings.Contains(baseURL, "/services") {
		// Extract base URL up to /services
		parts := strings.Split(baseURL, "/services")
		return parts[0] + "/services/collector/health"
	}

	// Fallback: append to base URL
	return baseURL + "/services/collector/health"
}

func (h *HEC) sendWithRetry(connID string, data []byte) error {
	// Read config with read lock
	h.configMu.RLock()
	useGzip := h.config.UseGzip
	url := h.config.URL
	token := h.config.Token
	sourceType := h.config.SourceType
	retryConfig := h.config.Retry
	h.configMu.RUnlock()

	// Pre-compress data if gzip is enabled
	var payloadData []byte
	var contentEnc string

	if useGzip {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		payloadData = buf.Bytes()
		contentEnc = "gzip"
	} else {
		payloadData = data
	}

	// Retry logic with exponential backoff
	maxAttempts := retryConfig.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5 // Default to 5 attempts
	}

	for i := 0; i < maxAttempts; i++ {
		// Create a fresh request for each attempt to avoid body reuse issues
		body := bytes.NewReader(payloadData)
		req, err := http.NewRequest("POST", url, body)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Splunk "+token)
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Correlation-ID", connID)
		if contentEnc != "" {
			req.Header.Set("Content-Encoding", contentEnc)
		}

		// Add sourcetype to query parameters if specified
		if sourceType != "" {
			q := req.URL.Query()
			q.Set("sourcetype", sourceType)
			req.URL.RawQuery = q.Encode()
		}

		resp, err := h.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := resp.Body.Close(); err != nil {
				// Log but don't fail on close error in success path
			}
			metrics.HecForwards.Add("success", 1)
			metrics.HecBytesForwarded.Add(int64(len(data)))
			slog.Debug("HEC forward succeeded", "conn_id", connID, "status", resp.StatusCode)
			return nil
		}
		if resp != nil {
			// Drain and close response body to enable connection reuse
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		// Track retry attempts (don't count initial attempt)
		if i > 0 {
			metrics.HecRetries.Add(1)
		}

		// Calculate backoff duration with exponential growth
		if i < maxAttempts-1 {
			backoff := h.calculateBackoff(i, retryConfig)
			time.Sleep(backoff)
		}
	}

	metrics.HecForwards.Add("failure", 1)
	return errors.New("hec send failed after retries")
}

// calculateBackoff computes the backoff duration for a retry attempt using exponential backoff.
// The backoff is capped at the configured maximum.
func (h *HEC) calculateBackoff(attemptNumber int, cfg RetryConfig) time.Duration {
	initialBackoff := cfg.InitialBackoff
	if initialBackoff == 0 {
		initialBackoff = 250 * time.Millisecond // Default
	}

	multiplier := cfg.BackoffMultiplier
	if multiplier == 0 {
		multiplier = 2.0 // Default
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff == 0 {
		maxBackoff = 30 * time.Second // Default
	}

	// Calculate exponential backoff: initialBackoff * (multiplier ^ attemptNumber)
	backoff := float64(initialBackoff)
	for i := 0; i < attemptNumber; i++ {
		backoff *= multiplier
	}

	duration := time.Duration(backoff)
	if duration > maxBackoff {
		duration = maxBackoff
	}

	return duration
}

// addToBatch adds data to the current batch and triggers flush if needed
func (h *HEC) addToBatch(data []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Make a copy of the data to avoid data races
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	// Add to batch
	h.batch.lines = append(h.batch.lines, dataCopy)
	h.batch.size += len(dataCopy)

	// Check if we should flush
	shouldFlush := len(h.batch.lines) >= h.config.Batch.MaxSize ||
		h.batch.size >= h.config.Batch.MaxBytes

	if shouldFlush {
		// Stop timer if it exists
		if h.batch.timer != nil {
			h.batch.timer.Stop()
			h.batch.timer = nil
		}
		// Trigger immediate flush
		select {
		case h.flushCh <- struct{}{}:
		default:
			// Flush already pending
		}
	} else if h.batch.timer == nil {
		// Start flush timer for the first line in batch
		h.batch.timer = time.AfterFunc(h.config.Batch.FlushInterval, func() {
			h.mu.Lock()
			h.batch.timer = nil
			h.mu.Unlock()
			select {
			case h.flushCh <- struct{}{}:
			default:
			}
		})
	}

	return nil
}

// flushWorker runs in a goroutine and handles batch flushing
func (h *HEC) flushWorker() {
	defer h.wg.Done()

	for {
		select {
		case <-h.flushCh:
			h.doFlush()
		case <-h.shutDown:
			// Final flush before shutdown
			h.doFlush()
			return
		}
	}
}

// doFlush performs the actual flush operation
func (h *HEC) doFlush() {
	h.mu.Lock()

	if len(h.batch.lines) == 0 {
		h.mu.Unlock()
		return
	}

	// Collect lines to send
	lines := h.batch.lines
	batchSize := len(lines)
	batchBytes := h.batch.size

	// Reset batch
	h.batch.lines = make([][]byte, 0, h.config.Batch.MaxSize)
	h.batch.size = 0
	if h.batch.timer != nil {
		h.batch.timer.Stop()
		h.batch.timer = nil
	}

	h.mu.Unlock()

	// Combine lines with newlines
	payload := bytes.Join(lines, []byte("\n"))

	// Send batch
	err := h.circuitBreaker.Call(func() error {
		return h.sendWithRetry("batch", payload)
	})

	if err != nil {
		slog.Error("batch forward failed",
			"lines", batchSize,
			"bytes", batchBytes,
			"error", err)
	} else {
		slog.Debug("batch forwarded",
			"lines", batchSize,
			"bytes", batchBytes)
	}
}

// Shutdown gracefully shuts down the forwarder, flushing any remaining batched data.
// If batching is disabled, this method returns immediately.
// The provided context controls the shutdown timeout.
// Returns an error if the shutdown times out before flushing completes.
func (h *HEC) Shutdown(ctx context.Context) error {
	if !h.config.Batch.Enabled {
		return nil
	}

	// Signal shutdown
	close(h.shutDown)

	// Wait for flush worker to finish with timeout
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("forwarder shutdown complete")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UpdateConfig updates the reloadable configuration parameters in a thread-safe manner.
// Only safe parameters (token, sourcetype, gzip) are updated.
// Parameters that require restart (URL, batching, circuit breaker) are not affected.
func (h *HEC) UpdateConfig(cfg ReloadableConfig) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	h.config.Token = cfg.Token
	h.config.SourceType = cfg.SourceType
	h.config.UseGzip = cfg.UseGzip

	slog.Info("HEC configuration updated",
		"sourcetype", cfg.SourceType,
		"gzip", cfg.UseGzip)
}
