package forwarder

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config contains configuration for the Splunk HEC forwarder
type Config struct {
	URL        string
	Token      string
	SourceType string
	UseGzip    bool
}

// HEC represents a Splunk HTTP Event Collector forwarder
type HEC struct {
	config Config
	client *http.Client
}

// New creates a new HEC forwarder with the given configuration
func New(config Config) *HEC {
	return &HEC{
		config: config,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Forward sends data to Splunk HEC with retry logic
func (h *HEC) Forward(data []byte) error {
	if h.config.URL == "" || h.config.Token == "" {
		return nil // HEC forwarding disabled
	}

	return h.sendWithRetry(data)
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

func (h *HEC) sendWithRetry(data []byte) error {
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
