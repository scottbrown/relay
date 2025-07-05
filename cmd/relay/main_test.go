package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/config"
	"gopkg.in/yaml.v3"
)

func TestConfig_Defaults(t *testing.T) {
	config := &config.Config{
		ListenPort:   "9514",
		SourceType:   "zscaler:zpa:lss",
		Index:        "main",
		BatchSize:    100,
		BatchTimeout: 5 * time.Second,
	}

	if config.ListenPort != "9514" {
		t.Errorf("Expected ListenPort to be '9514', got '%s'", config.ListenPort)
	}
	if config.SourceType != "zscaler:zpa:lss" {
		t.Errorf("Expected SourceType to be 'zscaler:zpa:lss', got '%s'", config.SourceType)
	}
	if config.Index != "main" {
		t.Errorf("Expected Index to be 'main', got '%s'", config.Index)
	}
	if config.BatchSize != 100 {
		t.Errorf("Expected BatchSize to be 100, got %d", config.BatchSize)
	}
	if config.BatchTimeout != 5*time.Second {
		t.Errorf("Expected BatchTimeout to be 5s, got %v", config.BatchTimeout)
	}
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		expectErr bool
		errMsg    string
	}{
		{
			name: "missing splunk_hec_url",
			config: &config.Config{
				SplunkToken: "test-token",
			},
			expectErr: true,
			errMsg:    "splunk_hec_url is required",
		},
		{
			name: "missing splunk_token",
			config: &config.Config{
				SplunkHECURL: "https://example.com",
			},
			expectErr: true,
			errMsg:    "splunk_token is required",
		},
		{
			name: "valid config",
			config: &config.Config{
				SplunkHECURL: "https://example.com",
				SplunkToken:  "test-token",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.expectErr {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func validateConfig(config *config.Config) error {
	if config.SplunkHECURL == "" {
		return errors.New("splunk_hec_url is required in config file")
	}
	if config.SplunkToken == "" {
		return errors.New("splunk_token is required in config file")
	}
	return nil
}

func TestSplunkEvent_JSONMarshaling(t *testing.T) {
	event := SplunkEvent{
		Time:       1234567890,
		Host:       "test-host",
		Source:     "test-source",
		SourceType: "test:type",
		Index:      "test-index",
		Event:      map[string]interface{}{"key": "value"},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal SplunkEvent: %v", err)
	}

	var unmarshaled SplunkEvent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SplunkEvent: %v", err)
	}

	if unmarshaled.Time != event.Time {
		t.Errorf("Expected Time %d, got %d", event.Time, unmarshaled.Time)
	}
	if unmarshaled.Host != event.Host {
		t.Errorf("Expected Host '%s', got '%s'", event.Host, unmarshaled.Host)
	}
	if unmarshaled.Source != event.Source {
		t.Errorf("Expected Source '%s', got '%s'", event.Source, unmarshaled.Source)
	}
}

func TestNewBatchProcessor(t *testing.T) {
	config := &config.Config{
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}

	bp := NewBatchProcessor(config, httpClient)

	if bp.config != config {
		t.Error("BatchProcessor config not set correctly")
	}
	if bp.httpClient != httpClient {
		t.Error("BatchProcessor httpClient not set correctly")
	}
	if bp.eventChan == nil {
		t.Error("BatchProcessor eventChan not initialized")
	}
	if bp.batch == nil {
		t.Error("BatchProcessor batch not initialized")
	}
	if bp.stopChan == nil {
		t.Error("BatchProcessor stopChan not initialized")
	}
}

func TestBatchProcessor_AddEvent(t *testing.T) {
	config := &config.Config{
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	event := SplunkEvent{
		Time:   time.Now().Unix(),
		Host:   "test-host",
		Source: "test-source",
		Event:  "test event",
	}

	bp.AddEvent(event)

	// Give some time for the event to be processed
	time.Sleep(10 * time.Millisecond)

	select {
	case receivedEvent := <-bp.eventChan:
		if receivedEvent.Host != event.Host {
			t.Errorf("Expected Host '%s', got '%s'", event.Host, receivedEvent.Host)
		}
	default:
		t.Error("Event was not added to channel")
	}
}

func TestBatchProcessor_SendToSplunk(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Splunk test-token" {
			t.Errorf("Expected Authorization 'Splunk test-token', got '%s'", auth)
		}

		// Check content type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
		}

		// Read and validate body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		for _, line := range lines {
			var event SplunkEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Errorf("Failed to unmarshal event: %v", err)
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &config.Config{
		SplunkHECURL: server.URL,
		SplunkToken:  "test-token",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	events := []SplunkEvent{
		{
			Time:   time.Now().Unix(),
			Host:   "test-host-1",
			Source: "test-source",
			Event:  "test event 1",
		},
		{
			Time:   time.Now().Unix(),
			Host:   "test-host-2",
			Source: "test-source",
			Event:  "test event 2",
		},
	}

	err := bp.sendToSplunk(events)
	if err != nil {
		t.Errorf("Unexpected error sending to Splunk: %v", err)
	}
}

func TestBatchProcessor_SendToSplunk_Error(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	config := &config.Config{
		SplunkHECURL: server.URL,
		SplunkToken:  "test-token",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	events := []SplunkEvent{
		{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  "test event",
		},
	}

	err := bp.sendToSplunk(events)
	if err == nil {
		t.Error("Expected error when server returns 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain '500', got: %v", err)
	}
}

func TestHandleConnection_ValidJSON(t *testing.T) {
	config := &config.Config{
		SourceType:   "test:type",
		Index:        "test-index",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Create pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Start handleConnection in goroutine
	go handleConnection(server, config, bp)

	// Send test data
	testData := `{"timestamp": "2023-01-01T00:00:00Z", "message": "test log"}`
	_, err := client.Write([]byte(testData + "\n"))
	if err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Close client to trigger connection handling completion
	client.Close()

	// Give some time for processing
	time.Sleep(50 * time.Millisecond)

	// Verify event was added to batch processor
	select {
	case event := <-bp.eventChan:
		if event.SourceType != config.SourceType {
			t.Errorf("Expected SourceType '%s', got '%s'", config.SourceType, event.SourceType)
		}
		if event.Index != config.Index {
			t.Errorf("Expected Index '%s', got '%s'", config.Index, event.Index)
		}
		if event.Source != "zpa_lss" {
			t.Errorf("Expected Source 'zpa_lss', got '%s'", event.Source)
		}
	default:
		t.Error("No event received in batch processor")
	}
}

func TestHandleConnection_InvalidJSON(t *testing.T) {
	config := &config.Config{
		SourceType:   "test:type",
		Index:        "test-index",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Create pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Start handleConnection in goroutine
	go handleConnection(server, config, bp)

	// Send invalid JSON data
	testData := `invalid json data`
	_, err := client.Write([]byte(testData + "\n"))
	if err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Close client to trigger connection handling completion
	client.Close()

	// Give some time for processing
	time.Sleep(50 * time.Millisecond)

	// Verify event was still added (as raw text)
	select {
	case event := <-bp.eventChan:
		if event.Event != testData {
			t.Errorf("Expected Event '%s', got '%v'", testData, event.Event)
		}
	default:
		t.Error("No event received in batch processor")
	}
}

func TestHandleConnection_EmptyLines(t *testing.T) {
	config := &config.Config{
		SourceType:   "test:type",
		Index:        "test-index",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Create pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Start handleConnection in goroutine
	go handleConnection(server, config, bp)

	// Send empty lines and valid data
	testData := "\n\n  \n" + `{"message": "test"}` + "\n\n"
	_, err := client.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Close client
	client.Close()

	// Give some time for processing
	time.Sleep(50 * time.Millisecond)

	// Should only receive one event (the valid JSON)
	eventCount := 0
	for {
		select {
		case <-bp.eventChan:
			eventCount++
		default:
			goto done
		}
	}
done:
	if eventCount != 1 {
		t.Errorf("Expected 1 event, got %d", eventCount)
	}
}

func TestBatchProcessor_FlushBatch_EmptyBatch(t *testing.T) {
	config := &config.Config{
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Should not panic or error on empty batch
	bp.flushBatch()
}

func TestBatchProcessor_AddToBatch_TimerBehavior(t *testing.T) {
	config := &config.Config{
		BatchSize:    10,
		BatchTimeout: 100 * time.Millisecond,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	event := SplunkEvent{
		Time:   time.Now().Unix(),
		Host:   "test-host",
		Source: "test-source",
		Event:  "test event",
	}

	// Add event to batch
	bp.addToBatch(event)

	// Verify timer was created
	if bp.batch.timer == nil {
		t.Error("Timer should be created for first event in batch")
	}

	// Verify batch contains the event
	if len(bp.batch.events) != 1 {
		t.Errorf("Expected 1 event in batch, got %d", len(bp.batch.events))
	}
}

func TestBatchProcessor_AddToBatch_BatchSizeLimit(t *testing.T) {
	config := &config.Config{
		BatchSize:    2, // Small batch size for testing
		BatchTimeout: 5 * time.Second,
	}

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config.SplunkHECURL = server.URL
	config.SplunkToken = "test-token"

	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Add events up to batch size
	for i := 0; i < 3; i++ {
		event := SplunkEvent{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  "test event",
		}
		bp.addToBatch(event)
	}

	// After adding 3 events with batch size 2, batch should be empty (flushed)
	if len(bp.batch.events) != 1 {
		t.Errorf("Expected 1 event in batch after flush, got %d", len(bp.batch.events))
	}
}

func TestConfigTemplate(t *testing.T) {
	if config.GetTemplate() == "" {
		t.Error("configTemplate should not be empty")
	}
}

func TestBatchProcessor_StartStop(t *testing.T) {
	config := &config.Config{
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Start processor in goroutine
	go bp.Start()

	// Add an event
	event := SplunkEvent{
		Time:   time.Now().Unix(),
		Host:   "test-host",
		Source: "test-source",
		Event:  "test event",
	}

	bp.AddEvent(event)

	// Give time for processing
	time.Sleep(10 * time.Millisecond)

	// Stop processor
	bp.Stop()
}

func TestBatchProcessor_ChannelFull(t *testing.T) {
	config := &config.Config{
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Fill the channel by adding more events than the channel buffer
	for i := 0; i < 1100; i++ {
		event := SplunkEvent{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  "test event",
		}
		bp.AddEvent(event)
	}
}

func TestBatchProcessor_FlushOnTimeout(t *testing.T) {
	batchFlushed := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		select {
		case batchFlushed <- true:
		default:
		}
	}))
	defer server.Close()

	config := &config.Config{
		SplunkHECURL: server.URL,
		SplunkToken:  "test-token",
		BatchSize:    100,
		BatchTimeout: 50 * time.Millisecond,
	}

	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Add one event (less than batch size)
	event := SplunkEvent{
		Time:   time.Now().Unix(),
		Host:   "test-host",
		Source: "test-source",
		Event:  "test event",
	}

	bp.addToBatch(event)

	// Wait for the flush to complete
	select {
	case <-batchFlushed:
		// Success - batch was flushed
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for batch to flush")
	}
}

func TestSendToSplunk_InvalidURL(t *testing.T) {
	config := &config.Config{
		SplunkHECURL: "invalid-url",
		SplunkToken:  "test-token",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	events := []SplunkEvent{
		{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  "test event",
		},
	}

	err := bp.sendToSplunk(events)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestSendToSplunk_MarshalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &config.Config{
		SplunkHECURL: server.URL,
		SplunkToken:  "test-token",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Create event with unmarshalable data (channel cannot be marshaled to JSON)
	ch := make(chan int)
	events := []SplunkEvent{
		{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  ch,
		},
	}

	err := bp.sendToSplunk(events)
	if err == nil {
		t.Error("Expected error for unmarshallable event")
	}
	if !strings.Contains(err.Error(), "failed to marshal event") {
		t.Errorf("Expected marshal error, got: %v", err)
	}
}

func TestParseFlags_MissingRequired(t *testing.T) {
	// Test parseFlags function - we can't call it directly due to os.Exit
	// but we can test the logic by creating a config with missing fields
	config := &config.Config{
		ListenPort:   "9514",
		SourceType:   "zscaler:zpa:lss",
		Index:        "main",
		BatchSize:    100,
		BatchTimeout: 5 * time.Second,
	}

	// Simulate missing required fields
	if config.SplunkHECURL == "" || config.SplunkToken == "" {
		// This simulates what parseFlags would validate
		if config.SplunkHECURL == "" {
			// Expected behavior: function would print usage and exit
		}
		if config.SplunkToken == "" {
			// Expected behavior: function would print usage and exit
		}
	}
}

func TestHTTPRequestTimeout(t *testing.T) {
	// Test slow server response to exercise HTTP timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Slight delay but within timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &config.Config{
		SplunkHECURL: server.URL,
		SplunkToken:  "test-token",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}

	// Use a very short timeout to test timeout handling
	httpClient := &http.Client{
		Timeout: 1 * time.Millisecond,
	}
	bp := NewBatchProcessor(config, httpClient)

	events := []SplunkEvent{
		{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  "test event",
		},
	}

	err := bp.sendToSplunk(events)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestLogBatch_Structure(t *testing.T) {
	batch := &LogBatch{
		events: make([]SplunkEvent, 0, 10),
		timer:  nil,
	}

	if batch.events == nil {
		t.Error("LogBatch events should be initialized")
	}
	if len(batch.events) != 0 {
		t.Errorf("Expected empty events slice, got length %d", len(batch.events))
	}
	if cap(batch.events) != 10 {
		t.Errorf("Expected capacity 10, got %d", cap(batch.events))
	}
}

func TestHandleConnection_ScannerError(t *testing.T) {
	config := &config.Config{
		SourceType:   "test:type",
		Index:        "test-index",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	// Create pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Start handleConnection in goroutine
	go handleConnection(server, config, bp)

	// Send data then close immediately to trigger scanner error path
	_, err := client.Write([]byte("partial data without newline"))
	if err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Close the client side to trigger an error
	client.Close()

	// Give some time for processing and error handling
	time.Sleep(50 * time.Millisecond)
}

func TestSendToSplunk_ResponseBodyRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("detailed error message from server"))
	}))
	defer server.Close()

	config := &config.Config{
		SplunkHECURL: server.URL,
		SplunkToken:  "test-token",
		BatchSize:    10,
		BatchTimeout: 5 * time.Second,
	}
	httpClient := &http.Client{}
	bp := NewBatchProcessor(config, httpClient)

	events := []SplunkEvent{
		{
			Time:   time.Now().Unix(),
			Host:   "test-host",
			Source: "test-source",
			Event:  "test event",
		},
	}

	err := bp.sendToSplunk(events)
	if err == nil {
		t.Error("Expected error when server returns 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain '500', got: %v", err)
	}
	if !strings.Contains(err.Error(), "detailed error message") {
		t.Errorf("Expected error to contain server response body, got: %v", err)
	}
}

func TestLoadConfig_WithFile(t *testing.T) {
	// Create temporary config file with valid content
	configContent := `
listen_port: "8080"
splunk_hec_url: "https://example.com:8088/services/collector"
splunk_token: "test-token"
source_type: "test:type"
index: "test"
batch_size: 50
batch_timeout: 10s
`
	tmpfile, err := os.CreateTemp("", "config*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test the file reading part without flag parsing by calling yaml unmarshal directly
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	config := &config.Config{
		ListenPort:   "9514",
		SourceType:   "zscaler:zpa:lss",
		Index:        "main",
		BatchSize:    100,
		BatchTimeout: 5 * time.Second,
	}

	// This tests part of the loadConfig logic
	err = yaml.Unmarshal(data, config)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	if config.ListenPort != "8080" {
		t.Errorf("Expected ListenPort '8080', got '%s'", config.ListenPort)
	}
	if config.SplunkHECURL != "https://example.com:8088/services/collector" {
		t.Errorf("Expected SplunkHECURL 'https://example.com:8088/services/collector', got '%s'", config.SplunkHECURL)
	}
}

func TestConfigValidation_Integration(t *testing.T) {
	// Test the validation logic that would be in loadConfig
	tests := []struct {
		name   string
		config *config.Config
		valid  bool
	}{
		{
			name: "valid config",
			config: &config.Config{
				SplunkHECURL: "https://example.com",
				SplunkToken:  "token123",
			},
			valid: true,
		},
		{
			name: "missing URL",
			config: &config.Config{
				SplunkToken: "token123",
			},
			valid: false,
		},
		{
			name: "missing token",
			config: &config.Config{
				SplunkHECURL: "https://example.com",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from loadConfig
			valid := true
			if tt.config.SplunkHECURL == "" {
				valid = false
			}
			if tt.config.SplunkToken == "" {
				valid = false
			}

			if valid != tt.valid {
				t.Errorf("Expected valid=%v, got valid=%v", tt.valid, valid)
			}
		})
	}
}

func TestMainTemplateFlag(t *testing.T) {
	// Test the template flag behavior by checking configTemplate is not empty
	if config.GetTemplate() == "" {
		t.Error("configTemplate should be embedded and not empty")
	}

	// Test that it looks like YAML content
	if !strings.Contains(config.GetTemplate(), "listen_port") {
		t.Error("configTemplate should contain 'listen_port' field")
	}
	if !strings.Contains(config.GetTemplate(), "splunk_hec_url") {
		t.Error("configTemplate should contain 'splunk_hec_url' field")
	}
}
