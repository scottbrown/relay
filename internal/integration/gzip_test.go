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

// TestGzipCompression verifies that Relay properly compresses data sent to HEC when gzip is enabled.
func TestGzipCompression(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-gzip")
	defer hec.Close()

	// Create and start Relay instance with gzip enabled
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL, "test-token-gzip", "zscaler:zpa:lss", true), // gzip=true
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

	t.Logf("Sent %d lines", len(lines))

	// Give relay time to process
	time.Sleep(500 * time.Millisecond)

	// Close client
	client.Close()

	// Give relay time to flush to HEC
	time.Sleep(1 * time.Second)

	// Verify HEC received compressed data
	hecRequests := hec.GetRequests()
	if len(hecRequests) == 0 {
		t.Fatal("No requests received by HEC")
	}

	t.Logf("HEC received %d requests", len(hecRequests))

	// Verify all requests have gzip compression
	for i, req := range hecRequests {
		if !req.Compressed {
			t.Errorf("Request %d: expected gzip compression, but was not compressed", i)
		}

		// Verify Content-Encoding header
		if encoding := req.Headers.Get("Content-Encoding"); encoding != "gzip" {
			t.Errorf("Request %d: expected Content-Encoding: gzip, got %s", i, encoding)
		}
	}

	// Verify decompressed content matches original lines
	totalHECLines := 0
	for _, req := range hecRequests {
		totalHECLines += len(req.BodyLines)
	}

	if totalHECLines != len(lines) {
		t.Errorf("Expected %d decompressed lines in HEC, got %d", len(lines), totalHECLines)
	}

	// Verify storage contains uncompressed data
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
		t.Errorf("Expected %d lines in storage, got %d", len(lines), len(storedLines))
	}

	t.Logf("Gzip compression test completed successfully")
}

// TestNoGzipCompression verifies that Relay does not compress when gzip is disabled.
func TestNoGzipCompression(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-nogzip")
	defer hec.Close()

	// Create and start Relay instance with gzip disabled
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL, "test-token-nogzip", "zscaler:zpa:lss", false), // gzip=false
		relaytest.WithLogType("user-activity"),
	)
	defer relay.Stop()

	relay.MustStart(ctx)

	// Load fixture data
	lines := fixtures.LoadFixture(t, "valid-user-activity.ndjson")
	if len(lines) == 0 {
		t.Fatal("No fixture lines loaded")
	}

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

	// Give relay time to process and flush
	time.Sleep(1500 * time.Millisecond)

	client.Close()

	// Verify HEC received uncompressed data
	hecRequests := hec.GetRequests()
	if len(hecRequests) == 0 {
		t.Fatal("No requests received by HEC")
	}

	// Verify no requests have gzip compression
	for i, req := range hecRequests {
		if req.Compressed {
			t.Errorf("Request %d: expected no gzip compression, but was compressed", i)
		}

		// Verify no Content-Encoding header (or it's not gzip)
		if encoding := req.Headers.Get("Content-Encoding"); encoding == "gzip" {
			t.Errorf("Request %d: unexpected Content-Encoding: gzip", i)
		}
	}

	t.Logf("No gzip compression test completed successfully")
}
