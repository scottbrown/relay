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
)

// BatchConfig contains batch forwarding configuration
type BatchConfig struct {
	Enabled       bool
	MaxSize       int           // Maximum lines per batch
	MaxBytes      int           // Maximum bytes per batch
	FlushInterval time.Duration // Maximum time before flushing
}

// Config contains configuration for the Splunk HEC forwarder
type Config struct {
	URL            string
	Token          string
	SourceType     string
	UseGzip        bool
	Batch          BatchConfig
	CircuitBreaker circuitbreaker.Config
}

// batch holds the current batch state
type batch struct {
	lines [][]byte
	size  int
	timer *time.Timer
}

// HEC represents a Splunk HTTP Event Collector forwarder
type HEC struct {
	config         Config
	client         *http.Client
	circuitBreaker *circuitbreaker.CircuitBreaker

	// Batch state (only used when batch.Enabled is true)
	mu       sync.Mutex
	batch    *batch
	flushCh  chan struct{}
	shutDown chan struct{}
	wg       sync.WaitGroup
}

// New creates a new HEC forwarder with the given configuration
func New(config Config) *HEC {
	h := &HEC{
		config:         config,
		client:         &http.Client{Timeout: 15 * time.Second},
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

// Forward sends data to Splunk HEC with retry logic and circuit breaker protection
func (h *HEC) Forward(connID string, data []byte) error {
	if h.config.URL == "" || h.config.Token == "" {
		return nil // HEC forwarding disabled
	}

	// If batching is enabled, add to batch
	if h.config.Batch.Enabled {
		return h.addToBatch(data)
	}

	// Otherwise send immediately
	slog.Debug("forwarding to HEC", "conn_id", connID, "hec_url", h.config.URL)

	return h.circuitBreaker.Call(func() error {
		return h.sendWithRetry(connID, data)
	})
}

// HealthCheck verifies that the HEC endpoint and token are valid
func (h *HEC) HealthCheck() error {
	if h.config.URL == "" || h.config.Token == "" {
		return errors.New("HEC URL or token not configured")
	}

	// Convert the collector URL to health check URL
	healthURL := h.getHealthURL()

	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Splunk "+h.config.Token)

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
	url := h.config.URL

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
	// Pre-compress data if gzip is enabled
	var payloadData []byte
	var contentEnc string

	if h.config.UseGzip {
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
	for i := 0; i < 5; i++ {
		// Create a fresh request for each attempt to avoid body reuse issues
		body := bytes.NewReader(payloadData)
		req, err := http.NewRequest("POST", h.config.URL, body)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Splunk "+h.config.Token)
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Correlation-ID", connID)
		if contentEnc != "" {
			req.Header.Set("Content-Encoding", contentEnc)
		}

		// Add sourcetype to query parameters if specified
		if h.config.SourceType != "" {
			q := req.URL.Query()
			q.Set("sourcetype", h.config.SourceType)
			req.URL.RawQuery = q.Encode()
		}

		resp, err := h.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := resp.Body.Close(); err != nil {
				// Log but don't fail on close error in success path
			}
			slog.Debug("HEC forward succeeded", "conn_id", connID, "status", resp.StatusCode)
			return nil
		}
		if resp != nil {
			// Drain and close response body to enable connection reuse
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		// Don't sleep after the last attempt
		if i < 4 {
			time.Sleep(time.Duration(250*(1<<i)) * time.Millisecond)
		}
	}

	return errors.New("hec send failed after retries")
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

// Shutdown gracefully shuts down the forwarder, flushing any remaining batched data
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
