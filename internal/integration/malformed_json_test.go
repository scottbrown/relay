// go:build integration
//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/testutil/fixtures"
	"github.com/scottbrown/relay/internal/testutil/hecmock"
	"github.com/scottbrown/relay/internal/testutil/relaytest"
	"github.com/scottbrown/relay/internal/testutil/zpamock"
)

// TestMalformedJSON verifies that Relay properly handles malformed JSON.
func TestMalformedJSON(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-456")
	defer hec.Close()

	// Create and start Relay instance
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL+"/services/collector/raw", "test-token-456", "zscaler:zpa:lss", false),
		relaytest.WithLogType("user-activity"),
	)
	defer relay.Stop()

	relay.MustStart(ctx)

	// Load malformed fixture data
	lines := fixtures.LoadFixture(t, "malformed-json.ndjson")
	if len(lines) == 0 {
		t.Fatal("No fixture lines loaded")
	}

	t.Logf("Loaded %d fixture lines (mix of valid and malformed)", len(lines))

	// Create ZPA mock client and connect
	client := zpamock.New(relay.ListenAddr)
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect mock client: %v", err)
	}

	// Send all lines (including malformed ones)
	if err := client.SendLines(lines); err != nil {
		t.Fatalf("Failed to send lines: %v", err)
	}

	t.Logf("Sent %d lines", len(lines))

	// Give relay time to process
	time.Sleep(500 * time.Millisecond)

	// Close client
	client.Close()

	// Give relay time to flush
	time.Sleep(1 * time.Second)

	// Read storage file
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

	// Count valid lines in fixture
	validLines := 0
	for _, line := range lines {
		// Simple heuristic: valid JSON lines start with { and end with }
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			// Further check: contains "SessionID" field (valid lines in our fixture)
			if strings.Contains(line, "SessionID") {
				validLines++
			}
		}
	}

	t.Logf("Valid lines in fixture: %d", validLines)
	t.Logf("Stored lines: %d", len(storedLines))

	// Storage should only contain valid lines
	if len(storedLines) != validLines {
		t.Errorf("Expected %d valid lines in storage, got %d", validLines, len(storedLines))
	}

	// Verify no malformed data in storage
	for i, line := range storedLines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
			t.Errorf("Stored line %d is not valid JSON: %s", i, line)
		}
	}

	// Verify HEC only received valid lines
	hecRequests := hec.GetRequests()
	totalHECLines := 0
	for _, req := range hecRequests {
		totalHECLines += len(req.BodyLines)
	}

	if totalHECLines != validLines {
		t.Errorf("Expected %d valid lines sent to HEC, got %d", validLines, totalHECLines)
	}

	// Check relay logs for error messages about malformed JSON
	stdout, stderr := relay.Logs()
	logs := stdout + stderr

	if !strings.Contains(logs, "invalid") && !strings.Contains(logs, "error") {
		t.Log("Note: Expected some error/warning logs about malformed JSON, but none found")
		t.Logf("Relay logs:\n%s", logs)
	}

	t.Logf("Malformed JSON test completed successfully")
}
