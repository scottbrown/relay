// go:build integration
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/testutil/hecmock"
	"github.com/scottbrown/relay/internal/testutil/relaytest"
	"github.com/scottbrown/relay/internal/testutil/zpamock"
)

// TestOversizedLines verifies that Relay enforces maximum line size limits.
func TestOversizedLines(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-789")
	defer hec.Close()

	// Create and start Relay instance with small max line size
	maxLineBytes := 1024
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL+"/services/collector/raw", "test-token-789", "zscaler:zpa:lss", false),
		relaytest.WithLogType("user-activity"),
		relaytest.WithMaxLineBytes(maxLineBytes),
	)
	defer relay.Stop()

	relay.MustStart(ctx)

	// Create test data
	normalLine := `{"LogTimestamp":"test","Customer":"Normal","SessionID":"ABC123"}`
	oversizedLine := zpamock.OversizedLine(maxLineBytes + 500) // 500 bytes over limit

	t.Logf("Normal line size: %d bytes", len(normalLine))
	t.Logf("Oversized line size: %d bytes", len(oversizedLine))
	t.Logf("Max line bytes: %d", maxLineBytes)

	// Create ZPA mock client and connect
	client := zpamock.New(relay.ListenAddr)
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect mock client: %v", err)
	}

	// Send lines in order: normal, oversized, normal
	lines := []string{
		normalLine,
		oversizedLine,
		normalLine,
	}

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

	t.Logf("Stored lines: %d", len(storedLines))

	// Should only have 2 normal lines (oversized should be rejected)
	expectedLines := 2
	if len(storedLines) != expectedLines {
		t.Errorf("Expected %d lines in storage (oversized rejected), got %d", expectedLines, len(storedLines))
	}

	// Verify stored lines are the normal ones
	for i, line := range storedLines {
		if line != normalLine {
			t.Errorf("Stored line %d doesn't match normal line: %s", i, line)
		}
	}

	// Verify HEC only received normal lines
	hecRequests := hec.GetRequests()
	totalHECLines := 0
	for _, req := range hecRequests {
		totalHECLines += len(req.BodyLines)
		for _, line := range req.BodyLines {
			if len(line) > maxLineBytes {
				t.Errorf("HEC received oversized line: %d bytes", len(line))
			}
		}
	}

	if totalHECLines != expectedLines {
		t.Errorf("Expected %d lines sent to HEC, got %d", expectedLines, totalHECLines)
	}

	t.Logf("Oversized lines test completed successfully")
}
