// go:build integration
//go:build integration

package integration

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/testutil/fixtures"
	"github.com/scottbrown/relay/internal/testutil/hecmock"
	"github.com/scottbrown/relay/internal/testutil/relaytest"
	"github.com/scottbrown/relay/internal/testutil/zpamock"
)

// TestHappyPath validates end-to-end functionality with valid data.
func TestHappyPath(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-123")
	defer hec.Close()

	// Create and start Relay instance
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL+"/services/collector/raw", "test-token-123", "zscaler:zpa:lss", false),
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

	// Create ZPA mock client and connect with verbose logging
	// Add a small delay between lines to allow server to process each one
	client := zpamock.New(relay.ListenAddr, zpamock.WithVerbose(true), zpamock.WithLineDelay(100*time.Millisecond))
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect mock client: %v", err)
	}

	t.Logf("Client connected to %s", relay.ListenAddr)

	// Send fixture lines
	if err := client.SendLines(lines); err != nil {
		t.Fatalf("Failed to send lines: %v", err)
	}

	t.Logf("Sent %d lines", len(lines))

	// Keep connection open to allow relay to process all data
	// Don't close immediately, as that causes EOF before all lines are read
	time.Sleep(2 * time.Second)

	// Now close client
	client.Close()

	// Give relay a moment to finish processing
	time.Sleep(500 * time.Millisecond)

	// Stop relay to ensure all data is flushed
	if err := relay.Stop(); err != nil {
		t.Logf("Warning: relay stop returned error: %v", err)
	}

	// Give a moment for cleanup
	time.Sleep(500 * time.Millisecond)

	// Verify storage
	files, err := relay.StorageFiles()
	if err != nil {
		t.Fatalf("Failed to list storage files: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 storage file, got %d: %v", len(files), files)
	}

	t.Logf("Storage file: %s", files[0])

	// Read storage file
	storedLines, err := relay.ReadStorageFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read storage file: %v", err)
	}

	// Also read raw file for debugging
	rawPath := relay.StorageDir + "/" + files[0]
	rawBytes, _ := os.ReadFile(rawPath)
	t.Logf("Raw file size: %d bytes", len(rawBytes))
	t.Logf("Raw file newline count: %d", bytes.Count(rawBytes, []byte("\n")))

	t.Logf("Stored lines count: %d", len(storedLines))
	for i, line := range storedLines {
		preview := line
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		t.Logf("Stored line %d (len=%d): %s", i, len(line), preview)
	}

	if len(storedLines) != len(lines) {
		t.Errorf("Expected %d stored lines, got %d", len(lines), len(storedLines))
	}

	// Verify all lines are in storage
	for i, expectedLine := range lines {
		if i >= len(storedLines) {
			t.Errorf("Missing line %d in storage", i)
			continue
		}
		if storedLines[i] != expectedLine {
			t.Errorf("Line %d mismatch:\nExpected: %s\nGot: %s", i, expectedLine, storedLines[i])
		}
	}

	// Verify HEC received data
	hecRequests := hec.GetRequests()
	if len(hecRequests) == 0 {
		t.Fatal("No requests received by HEC")
	}

	t.Logf("HEC received %d requests", len(hecRequests))

	// Verify HEC headers
	for i, req := range hecRequests {
		if auth := req.Headers.Get("Authorization"); auth != "Splunk test-token-123" {
			t.Errorf("Request %d: incorrect Authorization header: %s", i, auth)
		}

		if contentType := req.Headers.Get("Content-Type"); contentType != "text/plain" {
			t.Errorf("Request %d: incorrect Content-Type header: %s", i, contentType)
		}

		if req.Compressed {
			t.Errorf("Request %d: unexpected gzip compression", i)
		}
	}

	// Count total lines sent to HEC
	totalHECLines := 0
	for _, req := range hecRequests {
		totalHECLines += len(req.BodyLines)
	}

	if totalHECLines != len(lines) {
		t.Errorf("Expected %d lines sent to HEC, got %d", len(lines), totalHECLines)
	}

	t.Logf("Happy path test completed successfully")
}
