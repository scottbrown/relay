// go:build integration
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/testutil/fixtures"
	"github.com/scottbrown/relay/internal/testutil/hecmock"
	"github.com/scottbrown/relay/internal/testutil/relaytest"
	"github.com/scottbrown/relay/internal/testutil/zpamock"
)

// TestHECRetry verifies that Relay retries on HEC failures.
func TestHECRetry(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server in error mode
	hec := hecmock.NewMockHECServer("test-token-retry")
	defer hec.Close()

	// Start in error mode
	hec.SetResponse(hecmock.ResponseServiceUnavailable)

	// Create and start Relay instance
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL+"/services/collector/raw", "test-token-retry", "zscaler:zpa:lss", false),
		relaytest.WithLogType("user-activity"),
	)
	defer relay.Stop()

	relay.MustStart(ctx)

	// Load fixture data
	lines := fixtures.LoadFixture(t, "valid-user-activity.ndjson")
	if len(lines) == 0 {
		t.Fatal("No fixture lines loaded")
	}

	t.Logf("Loaded %d fixture lines", len(lines))

	// Create ZPA mock client and connect
	client := zpamock.New(relay.ListenAddr)
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect mock client: %v", err)
	}

	// Send fixture lines
	if err := client.SendLines(lines); err != nil {
		t.Fatalf("Failed to send lines: %v", err)
	}

	t.Logf("Sent %d lines to relay", len(lines))

	// Give relay time to attempt HEC forwarding (will fail)
	time.Sleep(500 * time.Millisecond)

	// Verify HEC received failed requests
	initialRequests := hec.RequestCount()
	t.Logf("HEC received %d failed requests", initialRequests)

	if initialRequests == 0 {
		t.Error("Expected at least one failed HEC request")
	}

	// Now switch HEC to success mode
	hec.SetResponse(hecmock.ResponseOK)
	t.Log("Switched HEC to success mode")

	// Wait for relay to retry (give it enough time for retry backoff)
	time.Sleep(5 * time.Second)

	// Verify HEC eventually received successful requests
	finalRequests := hec.RequestCount()
	t.Logf("HEC total requests after recovery: %d", finalRequests)

	if finalRequests <= initialRequests {
		t.Error("Expected retry attempts after HEC recovery")
	}

	// Count successful requests (last N requests should be successful)
	requests := hec.GetRequests()
	successfulLines := 0
	for _, req := range requests {
		successfulLines += len(req.BodyLines)
	}

	t.Logf("Total lines received by HEC: %d", successfulLines)

	// Verify all lines were eventually delivered
	if successfulLines < len(lines) {
		t.Errorf("Expected at least %d lines delivered to HEC after retry, got %d", len(lines), successfulLines)
	}

	// Verify local storage still has all lines regardless of HEC status
	files, err := relay.StorageFiles()
	if err != nil {
		t.Fatalf("Failed to list storage files: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("Expected at least one storage file")
	}

	storedLines, err := relay.ReadStorageFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read storage file: %v", err)
	}

	if len(storedLines) != len(lines) {
		t.Errorf("Expected %d lines in storage (regardless of HEC status), got %d", len(lines), len(storedLines))
	}

	t.Logf("HEC retry test completed successfully")
}
