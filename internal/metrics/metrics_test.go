package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	// Record start time before Init
	beforeInit := time.Now().Unix()

	// Call Init with a test version
	testVersion := "test-version-1.2.3"
	Init(testVersion)

	// Verify version is set correctly (expvar strings are JSON encoded, so it will have quotes)
	expectedVersionJSON := `"` + testVersion + `"`
	if Version.String() != expectedVersionJSON {
		t.Errorf("expected version %q, got %q", expectedVersionJSON, Version.String())
	}

	// Verify start time is set (should be recent)
	startTime := StartTime.Value()
	afterInit := time.Now().Unix()

	if startTime < beforeInit || startTime > afterInit {
		t.Errorf("start time %d is not within expected range [%d, %d]", startTime, beforeInit, afterInit)
	}
}

func TestMetricsIncrement(t *testing.T) {
	// Reset metrics to known state
	ConnectionsAccepted.Set(0)
	ConnectionsRejected.Set(0)
	BytesReceived.Set(0)

	// Increment metrics
	ConnectionsAccepted.Add(5)
	ConnectionsRejected.Add(2)
	BytesReceived.Add(1024)

	// Verify values
	if v := ConnectionsAccepted.Value(); v != 5 {
		t.Errorf("expected ConnectionsAccepted=5, got %d", v)
	}
	if v := ConnectionsRejected.Value(); v != 2 {
		t.Errorf("expected ConnectionsRejected=2, got %d", v)
	}
	if v := BytesReceived.Value(); v != 1024 {
		t.Errorf("expected BytesReceived=1024, got %d", v)
	}
}

func TestMetricsMap(t *testing.T) {
	// Test map metrics
	StorageWrites.Add("success", 10)
	StorageWrites.Add("failure", 3)

	HecForwards.Add("success", 8)
	HecForwards.Add("failure", 2)

	LinesProcessed.Add("valid", 100)
	LinesProcessed.Add("invalid", 5)

	// Maps don't have a direct API to read values in tests,
	// but we can verify they don't panic and work via HTTP endpoint
	// This is tested in TestStartServer
}

func TestStartServer(t *testing.T) {
	// Use a unique port for testing
	testAddr := ":19998"

	// Initialize metrics
	Init("test-server-1.0.0")

	// Start server
	if err := StartServer(testAddr); err != nil {
		t.Fatalf("failed to start metrics server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Record current values before adding test metrics
	beforeAccepted := ConnectionsAccepted.Value()
	beforeBytes := BytesReceived.Value()

	// Add some test metrics
	ConnectionsAccepted.Add(42)
	BytesReceived.Add(2048)
	StorageWrites.Add("test_success", 15)
	LinesProcessed.Add("test_valid", 20)
	LinesProcessed.Add("test_invalid", 1)

	// Fetch metrics via HTTP
	resp, err := http.Get("http://localhost" + testAddr + "/debug/vars")
	if err != nil {
		t.Fatalf("failed to fetch metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Parse JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(body, &metrics); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Verify metrics include our additions
	if v, ok := metrics["connections_accepted"].(float64); !ok || v != float64(beforeAccepted+42) {
		t.Errorf("expected connections_accepted=%d, got %v", beforeAccepted+42, metrics["connections_accepted"])
	}

	if v, ok := metrics["bytes_received_total"].(float64); !ok || v != float64(beforeBytes+2048) {
		t.Errorf("expected bytes_received_total=%d, got %v", beforeBytes+2048, metrics["bytes_received_total"])
	}

	if v, ok := metrics["version_info"].(string); !ok || v != "test-server-1.0.0" {
		t.Errorf("expected version_info=test-server-1.0.0, got %v", metrics["version_info"])
	}

	// Verify map metrics (using unique test keys)
	if storageWrites, ok := metrics["storage_writes"].(map[string]interface{}); ok {
		if v, ok := storageWrites["test_success"].(float64); !ok || v != 15 {
			t.Errorf("expected storage_writes.test_success=15, got %v", storageWrites["test_success"])
		}
	} else {
		t.Error("storage_writes not found or not a map")
	}

	if linesProcessed, ok := metrics["lines_processed"].(map[string]interface{}); ok {
		if v, ok := linesProcessed["test_valid"].(float64); !ok || v != 20 {
			t.Errorf("expected lines_processed.test_valid=20, got %v", linesProcessed["test_valid"])
		}
		if v, ok := linesProcessed["test_invalid"].(float64); !ok || v != 1 {
			t.Errorf("expected lines_processed.test_invalid=1, got %v", linesProcessed["test_invalid"])
		}
	} else {
		t.Error("lines_processed not found or not a map")
	}
}

func TestStartServerDisabled(t *testing.T) {
	// Starting server with empty address should not return error (disabled mode)
	if err := StartServer(""); err != nil {
		t.Errorf("StartServer with empty addr should not return error, got: %v", err)
	}
}
