package forwarder

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	config := Config{
		URL:        "https://example.com",
		Token:      "test-token",
		SourceType: "test:type",
		UseGzip:    true,
	}

	hec := New(config)
	if hec == nil {
		t.Fatal("New should return non-nil HEC")
	}

	if hec.config != config {
		t.Error("config should be stored")
	}

	if hec.client == nil {
		t.Error("client should be created")
	}

	if hec.client.Timeout != 15*time.Second {
		t.Errorf("expected timeout 15s, got %v", hec.client.Timeout)
	}
}

func TestForward_Disabled(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "empty URL",
			config: Config{
				URL:   "",
				Token: "test-token",
			},
		},
		{
			name: "empty token",
			config: Config{
				URL:   "https://example.com",
				Token: "",
			},
		},
		{
			name: "both empty",
			config: Config{
				URL:   "",
				Token: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hec := New(tt.config)
			err := hec.Forward("test-conn-id", []byte("test data"))
			if err != nil {
				t.Errorf("Forward with disabled config should return nil, got %v", err)
			}
		})
	}
}

func TestForward_Success(t *testing.T) {
	testData := []byte(`{"test": "data"}`)
	testConnID := "test-conn-id-12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		expectedAuth := "Splunk test-token"
		if auth := r.Header.Get("Authorization"); auth != expectedAuth {
			t.Errorf("expected auth %q, got %q", expectedAuth, auth)
		}

		if contentType := r.Header.Get("Content-Type"); contentType != "text/plain" {
			t.Errorf("expected content type text/plain, got %q", contentType)
		}

		// Verify correlation ID header
		if corrID := r.Header.Get("X-Correlation-ID"); corrID != testConnID {
			t.Errorf("expected X-Correlation-ID %q, got %q", testConnID, corrID)
		}

		if sourcetype := r.URL.Query().Get("sourcetype"); sourcetype != "test:type" {
			t.Errorf("expected sourcetype test:type, got %q", sourcetype)
		}

		// Read and verify body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		if string(body) != string(testData) {
			t.Errorf("expected body %q, got %q", string(testData), string(body))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test:type",
		UseGzip:    false,
	}

	hec := New(config)
	err := hec.Forward(testConnID, testData)
	if err != nil {
		t.Fatalf("Forward should succeed: %v", err)
	}
}

func TestForward_WithGzip(t *testing.T) {
	testData := []byte(`{"test": "data"}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify gzip encoding
		if encoding := r.Header.Get("Content-Encoding"); encoding != "gzip" {
			t.Errorf("expected Content-Encoding gzip, got %q", encoding)
		}

		// Decompress and verify body
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("failed to create gzip reader: %v", err)
		}
		defer gz.Close()

		body, err := io.ReadAll(gz)
		if err != nil {
			t.Fatalf("failed to read gzip body: %v", err)
		}

		if string(body) != string(testData) {
			t.Errorf("expected decompressed body %q, got %q", string(testData), string(body))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:     server.URL,
		Token:   "test-token",
		UseGzip: true,
	}

	hec := New(config)
	err := hec.Forward("test-conn-id", testData)
	if err != nil {
		t.Fatalf("Forward with gzip should succeed: %v", err)
	}
}

func TestForward_Retry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:   server.URL,
		Token: "test-token",
	}

	hec := New(config)
	err := hec.Forward("test-conn-id", []byte("test data"))
	if err != nil {
		t.Fatalf("Forward should succeed after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestForward_RetryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := Config{
		URL:   server.URL,
		Token: "test-token",
	}

	hec := New(config)
	err := hec.Forward("test-conn-id", []byte("test data"))
	if err == nil {
		t.Fatal("expected error after max retries")
	}

	expectedMsg := "hec send failed after retries"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestHealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET method, got %s", r.Method)
		}

		expectedAuth := "Splunk test-token"
		if auth := r.Header.Get("Authorization"); auth != expectedAuth {
			t.Errorf("expected auth %q, got %q", expectedAuth, auth)
		}

		if !strings.Contains(r.URL.Path, "/services/collector/health") {
			t.Errorf("expected health endpoint in path, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:   server.URL + "/services/collector/raw",
		Token: "test-token",
	}

	hec := New(config)
	err := hec.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck should succeed: %v", err)
	}
}

func TestHealthCheck_Disabled(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "empty URL",
			config: Config{
				URL:   "",
				Token: "test-token",
			},
		},
		{
			name: "empty token",
			config: Config{
				URL:   "https://example.com",
				Token: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hec := New(tt.config)
			err := hec.HealthCheck()
			if err == nil {
				t.Fatal("expected error for disabled HEC")
			}

			expectedMsg := "HEC URL or token not configured"
			if err.Error() != expectedMsg {
				t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
			}
		})
	}
}

func TestHealthCheck_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	config := Config{
		URL:   server.URL,
		Token: "invalid-token",
	}

	hec := New(config)
	err := hec.HealthCheck()
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}

	expectedMsg := "invalid Splunk HEC token (403 Forbidden)"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestHealthCheck_OtherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	config := Config{
		URL:   server.URL,
		Token: "test-token",
	}

	hec := New(config)
	err := hec.HealthCheck()
	if err == nil {
		t.Fatal("expected error for bad request")
	}

	if !strings.Contains(err.Error(), "HEC health check failed with status:") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestGetHealthURL(t *testing.T) {
	tests := []struct {
		name        string
		inputURL    string
		expectedURL string
	}{
		{
			name:        "raw collector endpoint",
			inputURL:    "https://example.com:8088/services/collector/raw",
			expectedURL: "https://example.com:8088/services/collector/health",
		},
		{
			name:        "event collector endpoint",
			inputURL:    "https://example.com:8088/services/collector/event",
			expectedURL: "https://example.com:8088/services/collector/health",
		},
		{
			name:        "generic collector endpoint",
			inputURL:    "https://example.com:8088/services/collector",
			expectedURL: "https://example.com:8088/services/collector/health",
		},
		{
			name:        "already health endpoint",
			inputURL:    "https://example.com:8088/services/collector/health",
			expectedURL: "https://example.com:8088/services/collector/health",
		},
		{
			name:        "base URL with services",
			inputURL:    "https://example.com:8088/services",
			expectedURL: "https://example.com:8088/services/collector/health",
		},
		{
			name:        "base URL without services",
			inputURL:    "https://example.com:8088",
			expectedURL: "https://example.com:8088/services/collector/health",
		},
		{
			name:        "with trailing slash",
			inputURL:    "https://example.com:8088/services/collector/raw/",
			expectedURL: "https://example.com:8088/services/collector/health/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{URL: tt.inputURL}
			hec := New(config)
			result := hec.getHealthURL()
			if result != tt.expectedURL {
				t.Errorf("expected URL %q, got %q", tt.expectedURL, result)
			}
		})
	}
}

func TestSendWithRetry_RequestError(t *testing.T) {
	config := Config{
		URL:   "http://invalid-url-that-does-not-exist.invalid",
		Token: "test-token",
	}

	hec := New(config)
	err := hec.sendWithRetry("test-conn-id", []byte("test data"))
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}

	expectedMsg := "hec send failed after retries"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestBatch_FlushOnSize(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		body, _ := io.ReadAll(r.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		if len(lines) != 5 {
			t.Errorf("expected 5 lines in batch, got %d", len(lines))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled:       true,
			MaxSize:       5,
			MaxBytes:      1 << 20,
			FlushInterval: 1 * time.Second,
		},
	}

	hec := New(config)
	defer hec.Shutdown(context.Background())

	// Send 5 lines to trigger size-based flush
	for i := 0; i < 5; i++ {
		if err := hec.Forward("test-conn", []byte(`{"test":"data"}`)); err != nil {
			t.Fatalf("forward failed: %v", err)
		}
	}

	// Wait for flush to complete
	time.Sleep(100 * time.Millisecond)

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

func TestBatch_FlushOnBytes(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled:       true,
			MaxSize:       100,
			MaxBytes:      50, // Small byte limit
			FlushInterval: 1 * time.Second,
		},
	}

	hec := New(config)
	defer hec.Shutdown(context.Background())

	// Send data that exceeds byte limit
	if err := hec.Forward("test-conn", []byte(`{"test":"this is more than 50 bytes of data for sure"}`)); err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	// Wait for flush to complete
	time.Sleep(100 * time.Millisecond)

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

func TestBatch_FlushOnTimer(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled:       true,
			MaxSize:       100,
			MaxBytes:      1 << 20,
			FlushInterval: 100 * time.Millisecond,
		},
	}

	hec := New(config)
	defer hec.Shutdown(context.Background())

	// Send one line
	if err := hec.Forward("test-conn", []byte(`{"test":"data"}`)); err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	// Wait for timer-based flush
	time.Sleep(200 * time.Millisecond)

	if requestCount != 1 {
		t.Errorf("expected 1 request from timer flush, got %d", requestCount)
	}
}

func TestBatch_FlushOnShutdown(t *testing.T) {
	requestCount := 0
	receivedLines := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		body, _ := io.ReadAll(r.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		receivedLines = len(lines)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled:       true,
			MaxSize:       100,
			MaxBytes:      1 << 20,
			FlushInterval: 10 * time.Second, // Long interval
		},
	}

	hec := New(config)

	// Send 3 lines (not enough to trigger size flush)
	for i := 0; i < 3; i++ {
		if err := hec.Forward("test-conn", []byte(`{"test":"data"}`)); err != nil {
			t.Fatalf("forward failed: %v", err)
		}
	}

	// Shutdown should flush remaining data
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := hec.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("expected 1 request from shutdown flush, got %d", requestCount)
	}

	if receivedLines != 3 {
		t.Errorf("expected 3 lines in shutdown flush, got %d", receivedLines)
	}
}

func TestBatch_Disabled(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled: false, // Batching disabled
		},
	}

	hec := New(config)

	// Send 3 lines - should result in 3 separate requests
	for i := 0; i < 3; i++ {
		if err := hec.Forward("test-conn", []byte(`{"test":"data"}`)); err != nil {
			t.Fatalf("forward failed: %v", err)
		}
	}

	// Wait for requests to complete
	time.Sleep(100 * time.Millisecond)

	if requestCount != 3 {
		t.Errorf("expected 3 requests (batching disabled), got %d", requestCount)
	}
}

func TestBatch_MultipleBatches(t *testing.T) {
	var requestCount int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled:       true,
			MaxSize:       2,
			MaxBytes:      1 << 20,
			FlushInterval: 1 * time.Second,
		},
	}

	hec := New(config)

	// Send 7 lines - should trigger 3 batches (2+2+2) with 1 remaining
	for i := 0; i < 7; i++ {
		if err := hec.Forward("test-conn", []byte(`{"test":"data"}`)); err != nil {
			t.Fatalf("forward failed: %v", err)
		}
		// Small delay between sends to ensure proper batch separation
		time.Sleep(10 * time.Millisecond)
	}

	// Shutdown will flush the remaining 1 line
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := hec.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	mu.Lock()
	count := requestCount
	mu.Unlock()

	// Should have 3 full batches + 1 partial batch on shutdown = 4 total requests
	if count != 4 {
		t.Errorf("expected 4 requests (3 full batches + 1 shutdown), got %d", count)
	}
}

func TestBatch_ShutdownTimeout(t *testing.T) {
	// Server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		URL:        server.URL,
		Token:      "test-token",
		SourceType: "test",
		Batch: BatchConfig{
			Enabled:       true,
			MaxSize:       100,
			MaxBytes:      1 << 20,
			FlushInterval: 1 * time.Second,
		},
	}

	hec := New(config)

	// Send one line
	if err := hec.Forward("test-conn", []byte(`{"test":"data"}`)); err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	// Shutdown with short timeout should return context error
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := hec.Shutdown(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}
