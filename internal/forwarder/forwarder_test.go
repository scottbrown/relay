package forwarder

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
			err := hec.Forward([]byte("test data"))
			if err != nil {
				t.Errorf("Forward with disabled config should return nil, got %v", err)
			}
		})
	}
}

func TestForward_Success(t *testing.T) {
	testData := []byte(`{"test": "data"}`)

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
	err := hec.Forward(testData)
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
	err := hec.Forward(testData)
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
	err := hec.Forward([]byte("test data"))
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
	err := hec.Forward([]byte("test data"))
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
			result := hec.getHealthURL(config)
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
	err := hec.sendWithRetry([]byte("test data"), config)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}

	expectedMsg := "hec send failed after retries"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}
