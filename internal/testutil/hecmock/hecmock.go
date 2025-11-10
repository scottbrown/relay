// Package hecmock provides a mock Splunk HEC server for integration testing.
package hecmock

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// ResponseMode defines the type of response the mock server should return.
type ResponseMode int

const (
	// ResponseOK returns 200 OK
	ResponseOK ResponseMode = iota
	// ResponseBadRequest returns 400 Bad Request
	ResponseBadRequest
	// ResponseUnauthorised returns 401 Unauthorised
	ResponseUnauthorised
	// ResponseForbidden returns 403 Forbidden
	ResponseForbidden
	// ResponseServerError returns 500 Internal Server Error
	ResponseServerError
	// ResponseServiceUnavailable returns 503 Service Unavailable
	ResponseServiceUnavailable
	// ResponseDrop drops the connection without responding
	ResponseDrop
)

// RecordedRequest represents a single HTTP request received by the mock HEC server.
type RecordedRequest struct {
	Timestamp  time.Time
	Headers    http.Header
	Body       []byte
	BodyLines  []string
	Compressed bool
}

// MockHECServer simulates a Splunk HEC endpoint.
type MockHECServer struct {
	// Server is the underlying HTTP test server
	Server *httptest.Server
	// URL is the base URL of the mock server
	URL string
	// Token is the expected authorisation token
	Token string

	// Response behaviour
	responseMode ResponseMode
	delay        time.Duration

	// Recorded data
	mu       sync.Mutex
	requests []RecordedRequest

	// Observability
	verbose bool
}

// NewMockHECServer creates a new mock HEC server with the specified authorisation token.
func NewMockHECServer(token string) *MockHECServer {
	m := &MockHECServer{
		Token:        token,
		responseMode: ResponseOK,
		requests:     make([]RecordedRequest, 0),
	}

	m.Server = httptest.NewServer(http.HandlerFunc(m.handler))
	m.URL = m.Server.URL

	return m
}

// NewVerboseMockHECServer creates a new mock HEC server with verbose logging enabled.
func NewVerboseMockHECServer(token string) *MockHECServer {
	m := NewMockHECServer(token)
	m.verbose = true
	return m
}

// handler processes incoming HTTP requests.
func (m *MockHECServer) handler(w http.ResponseWriter, r *http.Request) {
	m.logEvent("request_received", map[string]interface{}{
		"method": r.Method,
		"path":   r.URL.Path,
	})

	// Apply delay if configured
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	// Check response mode
	mode := m.getResponseMode()

	// Handle connection drop
	if mode == ResponseDrop {
		m.logEvent("connection_dropped", nil)
		// Close connection abruptly
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close() // Ignore close error in test mock
				return
			}
		}
		// Fallback to 500 if hijacking not supported
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Handle health check endpoint
	if r.URL.Path == "/services/collector/health" {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Validate authorisation for health check
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Splunk " + m.Token
		if authHeader != expectedAuth {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"text":"HEC is healthy","code":17}`)
		return
	}

	// Validate HTTP method for data ingestion
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate path
	if r.URL.Path != "/services/collector/raw" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Validate authorisation
	authHeader := r.Header.Get("Authorization")
	expectedAuth := "Splunk " + m.Token
	if authHeader != expectedAuth {
		m.logEvent("auth_failed", map[string]interface{}{
			"received": authHeader,
			"expected": expectedAuth,
		})
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Read body
	var bodyReader io.Reader = r.Body
	compressed := false

	// Check for gzip encoding
	if r.Header.Get("Content-Encoding") == "gzip" {
		compressed = true
		gzReader, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "Invalid gzip content", http.StatusBadRequest)
			return
		}
		defer gzReader.Close()
		bodyReader = gzReader
	}

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Split body into lines
	bodyStr := string(body)
	lines := strings.Split(bodyStr, "\n")
	// Remove empty trailing line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Record request
	m.mu.Lock()
	m.requests = append(m.requests, RecordedRequest{
		Timestamp:  time.Now(),
		Headers:    r.Header.Clone(),
		Body:       body,
		BodyLines:  lines,
		Compressed: compressed,
	})
	m.mu.Unlock()

	m.logEvent("request_recorded", map[string]interface{}{
		"lines":      len(lines),
		"compressed": compressed,
		"bytes":      len(body),
	})

	// Return response based on mode
	switch mode {
	case ResponseOK:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"text":"Success","code":0}`)
	case ResponseBadRequest:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"text":"Invalid request","code":5}`)
	case ResponseUnauthorised:
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"text":"Unauthorised","code":2}`)
	case ResponseForbidden:
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"text":"Forbidden","code":3}`)
	case ResponseServerError:
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"text":"Internal server error","code":8}`)
	case ResponseServiceUnavailable:
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"text":"Service unavailable","code":9}`)
	}
}

// SetResponse sets the response mode for subsequent requests.
func (m *MockHECServer) SetResponse(mode ResponseMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseMode = mode
	m.logEvent("response_mode_changed", map[string]interface{}{
		"mode": mode,
	})
}

// SetDelay sets a delay before responding to requests.
func (m *MockHECServer) SetDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = d
	m.logEvent("delay_changed", map[string]interface{}{
		"delay_ms": d.Milliseconds(),
	})
}

// GetRequests returns all recorded requests.
func (m *MockHECServer) GetRequests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to prevent concurrent modification
	result := make([]RecordedRequest, len(m.requests))
	copy(result, m.requests)
	return result
}

// RequestCount returns the number of requests received.
func (m *MockHECServer) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// Reset clears all recorded requests and resets the response mode.
func (m *MockHECServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = make([]RecordedRequest, 0)
	m.responseMode = ResponseOK
	m.delay = 0
	m.logEvent("reset", nil)
}

// Close shuts down the mock server.
func (m *MockHECServer) Close() {
	m.logEvent("shutdown", nil)
	m.Server.Close()
}

// getResponseMode safely gets the current response mode.
func (m *MockHECServer) getResponseMode() ResponseMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.responseMode
}

// logEvent logs structured events to stdout if verbose mode is enabled.
func (m *MockHECServer) logEvent(event string, data map[string]interface{}) {
	if !m.verbose {
		return
	}

	logEntry := map[string]interface{}{
		"level":     "info",
		"component": "hecmock",
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	for k, v := range data {
		logEntry[k] = v
	}

	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}

	fmt.Println(string(jsonBytes))
}
