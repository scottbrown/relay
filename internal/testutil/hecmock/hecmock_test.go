package hecmock

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMockHECServer_BasicRequest(t *testing.T) {
	server := NewMockHECServer("test-token-123")
	defer server.Close()

	// Send request
	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test log line 1\ntest log line 2"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk test-token-123")
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify recorded request
	requests := server.GetRequests()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 recorded request, got %d", len(requests))
	}

	if len(requests[0].BodyLines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(requests[0].BodyLines))
	}

	if requests[0].BodyLines[0] != "test log line 1" {
		t.Errorf("Unexpected first line: %s", requests[0].BodyLines[0])
	}
}

func TestMockHECServer_GzipCompression(t *testing.T) {
	server := NewMockHECServer("test-token-456")
	defer server.Close()

	// Create gzipped content
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte("compressed line 1\ncompressed line 2\ncompressed line 3"))
	if err != nil {
		t.Fatalf("Failed to write gzip: %v", err)
	}
	gzWriter.Close()

	// Send compressed request
	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", &buf)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk test-token-456")
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify recorded request
	requests := server.GetRequests()
	if len(requests) != 1 {
		t.Fatalf("Expected 1 recorded request, got %d", len(requests))
	}

	if !requests[0].Compressed {
		t.Error("Expected compressed flag to be true")
	}

	if len(requests[0].BodyLines) != 3 {
		t.Errorf("Expected 3 decompressed lines, got %d", len(requests[0].BodyLines))
	}

	if requests[0].BodyLines[0] != "compressed line 1" {
		t.Errorf("Unexpected first line: %s", requests[0].BodyLines[0])
	}
}

func TestMockHECServer_AuthenticationFailure(t *testing.T) {
	server := NewMockHECServer("correct-token")
	defer server.Close()

	// Send request with wrong token
	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk wrong-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 401
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Should not record the request (auth failed)
	if server.RequestCount() != 0 {
		t.Errorf("Expected 0 recorded requests (auth failed), got %d", server.RequestCount())
	}
}

func TestMockHECServer_ResponseModes(t *testing.T) {
	tests := []struct {
		mode           ResponseMode
		expectedStatus int
	}{
		{ResponseOK, http.StatusOK},
		{ResponseBadRequest, http.StatusBadRequest},
		{ResponseUnauthorised, http.StatusUnauthorized},
		{ResponseForbidden, http.StatusForbidden},
		{ResponseServerError, http.StatusInternalServerError},
		{ResponseServiceUnavailable, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			server := NewMockHECServer("test-token")
			defer server.Close()

			server.SetResponse(tt.mode)

			req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			req.Header.Set("Authorization", "Splunk test-token")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestMockHECServer_Delay(t *testing.T) {
	server := NewMockHECServer("test-token")
	defer server.Close()

	// Set 100ms delay
	server.SetDelay(100 * time.Millisecond)

	start := time.Now()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected delay of at least 100ms, got %v", elapsed)
	}
}

func TestMockHECServer_Reset(t *testing.T) {
	server := NewMockHECServer("test-token")
	defer server.Close()

	// Send some requests
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
		req.Header.Set("Authorization", "Splunk test-token")
		http.DefaultClient.Do(req)
	}

	if server.RequestCount() != 3 {
		t.Errorf("Expected 3 requests, got %d", server.RequestCount())
	}

	// Reset
	server.Reset()

	if server.RequestCount() != 0 {
		t.Errorf("Expected 0 requests after reset, got %d", server.RequestCount())
	}
}

func TestMockHECServer_ConcurrentRequests(t *testing.T) {
	server := NewMockHECServer("test-token")
	defer server.Close()

	// Send concurrent requests
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
			req.Header.Set("Authorization", "Splunk test-token")
			http.DefaultClient.Do(req)
			done <- true
		}()
	}

	// Wait for all requests
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should record all 10 requests
	if server.RequestCount() != 10 {
		t.Errorf("Expected 10 requests, got %d", server.RequestCount())
	}
}

func TestNewVerboseMockHECServer(t *testing.T) {
	server := NewVerboseMockHECServer("test-token-verbose")
	defer server.Close()

	if !server.verbose {
		t.Error("Expected verbose to be true for NewVerboseMockHECServer")
	}

	// Send a request to verify it works
	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test line"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk test-token-verbose")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMockHECServer_ResponseDrop(t *testing.T) {
	server := NewMockHECServer("test-token-drop")
	defer server.Close()

	server.SetResponse(ResponseDrop)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Splunk test-token-drop")

	// This should fail because the connection is dropped
	client := &http.Client{
		Timeout: 1 * time.Second,
	}
	_, err = client.Do(req)
	if err == nil {
		t.Error("Expected error when connection is dropped, got nil")
	}
}

func TestMockHECServer_MissingAuth(t *testing.T) {
	server := NewMockHECServer("test-token")
	defer server.Close()

	// Send request without authorisation header
	req, err := http.NewRequest(http.MethodPost, server.URL+"/services/collector/raw", strings.NewReader("test"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 401 for missing auth
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for missing auth, got %d", resp.StatusCode)
	}
}

// String returns the string representation of ResponseMode for testing
func (r ResponseMode) String() string {
	switch r {
	case ResponseOK:
		return "ResponseOK"
	case ResponseBadRequest:
		return "ResponseBadRequest"
	case ResponseUnauthorised:
		return "ResponseUnauthorised"
	case ResponseForbidden:
		return "ResponseForbidden"
	case ResponseServerError:
		return "ResponseServerError"
	case ResponseServiceUnavailable:
		return "ResponseServiceUnavailable"
	case ResponseDrop:
		return "ResponseDrop"
	default:
		return "Unknown"
	}
}
