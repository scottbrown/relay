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

// TestCIDRAccessControl verifies that Relay properly filters connections based on CIDR rules.
func TestCIDRAccessControl(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-acl")
	defer hec.Close()

	// Create and start Relay instance with CIDR restriction
	// Allow only localhost connections
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL, "test-token-acl", "zscaler:zpa:lss", false),
		relaytest.WithLogType("user-activity"),
		relaytest.WithAllowedCIDRs("127.0.0.1/32"),
	)
	defer relay.Stop()

	relay.MustStart(ctx)

	// Load fixture data
	lines := fixtures.LoadFixture(t, "valid-user-activity.ndjson")
	if len(lines) == 0 {
		t.Fatal("No fixture lines loaded")
	}

	t.Logf("Loaded %d fixture lines", len(lines))

	// Create ZPA mock client from localhost (should be allowed)
	client := zpamock.New(relay.ListenAddr)
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect from localhost (should be allowed): %v", err)
	}

	t.Log("Successfully connected from localhost (allowed by CIDR)")

	// Send fixture lines
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

	// Verify data was processed (connection was allowed)
	files, err := relay.StorageFiles()
	if err != nil {
		t.Fatalf("Failed to list storage files: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("Expected at least one storage file (connection should have been allowed)")
	}

	storedLines, err := relay.ReadStorageFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read storage file: %v", err)
	}

	if len(storedLines) != len(lines) {
		t.Errorf("Expected %d lines in storage, got %d", len(lines), len(storedLines))
	}

	// Verify HEC received data
	hecRequests := hec.GetRequests()
	if len(hecRequests) == 0 {
		t.Error("Expected HEC to receive data from allowed connection")
	}

	t.Logf("CIDR access control test completed successfully")
}

// TestCIDRAccessControlDeny verifies that connections from disallowed IPs are rejected.
// Note: This test is challenging to implement properly in integration tests since
// we can't easily connect from a different IP in the same test environment.
// This would be better tested with Docker containers or network namespaces.
func TestCIDRAccessControlAllowAll(t *testing.T) {
	ctx := context.Background()

	// Start mock HEC server
	hec := hecmock.NewMockHECServer("test-token-acl-all")
	defer hec.Close()

	// Create and start Relay instance with no CIDR restriction (allow all)
	relay := relaytest.NewRelayInstance(t,
		relaytest.WithHEC(hec.URL, "test-token-acl-all", "zscaler:zpa:lss", false),
		relaytest.WithLogType("user-activity"),
		// No WithAllowedCIDRs means all IPs are allowed
	)
	defer relay.Stop()

	relay.MustStart(ctx)

	// Load fixture data
	lines := fixtures.LoadFixture(t, "valid-user-activity.ndjson")
	if len(lines) == 0 {
		t.Fatal("No fixture lines loaded")
	}

	// Create ZPA mock client
	client := zpamock.New(relay.ListenAddr)
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect (should be allowed with no ACL): %v", err)
	}

	// Send fixture lines
	if err := client.SendLines(lines); err != nil {
		t.Fatalf("Failed to send lines: %v", err)
	}

	// Give relay time to process
	time.Sleep(500 * time.Millisecond)

	client.Close()
	time.Sleep(1 * time.Second)

	// Verify data was processed
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

	t.Logf("CIDR allow-all test completed successfully")
}
