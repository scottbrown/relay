package server

import (
	"context"
	"net"
	"testing"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/storage"
)

// mockForwarder implements the forwarder.Forwarder interface for testing
type mockForwarder struct {
	lastConfig forwarder.ReloadableConfig
}

func (m *mockForwarder) Forward(connID string, data []byte) error {
	return nil
}

func (m *mockForwarder) HealthCheck() error {
	return nil
}

func (m *mockForwarder) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockForwarder) UpdateConfig(cfg forwarder.ReloadableConfig) {
	m.lastConfig = cfg
}

func TestServer_UpdateConfig_ACL(t *testing.T) {
	// Create server with initial ACL
	aclList, err := acl.New("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	storage, err := storage.New(t.TempDir(), "test")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storage.Close()

	mockFwd := &mockForwarder{}
	srv, err := New(Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}, aclList, storage, mockFwd, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Update ACL to new CIDR
	err = srv.UpdateConfig(ReloadableConfig{
		AllowedCIDRs: "10.0.0.0/8",
	})
	if err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Verify ACL was updated by checking if old IP is rejected
	srv.aclMu.RLock()
	oldAllowed := srv.acl.Allows(net.ParseIP("192.168.1.100"))
	newAllowed := srv.acl.Allows(net.ParseIP("10.0.0.1"))
	srv.aclMu.RUnlock()

	if oldAllowed {
		t.Error("old CIDR range should not be allowed after reload")
	}
	if !newAllowed {
		t.Error("new CIDR range should be allowed after reload")
	}
}

func TestServer_UpdateConfig_Forwarder(t *testing.T) {
	// Create server with mock forwarder
	aclList, err := acl.New("0.0.0.0/0")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	storage, err := storage.New(t.TempDir(), "test")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storage.Close()

	mockFwd := &mockForwarder{}
	srv, err := New(Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}, aclList, storage, mockFwd, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Update forwarder configuration
	newCfg := ReloadableConfig{
		ForwarderConfig: forwarder.ReloadableConfig{
			Token:      "new-token-123",
			SourceType: "new-sourcetype",
			UseGzip:    true,
		},
	}

	err = srv.UpdateConfig(newCfg)
	if err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Verify forwarder received the update
	if mockFwd.lastConfig.Token != "new-token-123" {
		t.Errorf("expected token 'new-token-123', got '%s'", mockFwd.lastConfig.Token)
	}
	if mockFwd.lastConfig.SourceType != "new-sourcetype" {
		t.Errorf("expected sourcetype 'new-sourcetype', got '%s'", mockFwd.lastConfig.SourceType)
	}
	if !mockFwd.lastConfig.UseGzip {
		t.Error("expected UseGzip to be true")
	}
}

func TestServer_UpdateConfig_InvalidACL(t *testing.T) {
	// Create server with initial ACL
	aclList, err := acl.New("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	storage, err := storage.New(t.TempDir(), "test")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storage.Close()

	mockFwd := &mockForwarder{}
	srv, err := New(Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}, aclList, storage, mockFwd, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Try to update with invalid CIDR
	err = srv.UpdateConfig(ReloadableConfig{
		AllowedCIDRs: "invalid-cidr",
	})
	if err == nil {
		t.Error("expected error for invalid CIDR, got nil")
	}

	// Verify ACL was not updated
	srv.aclMu.RLock()
	stillAllowed := srv.acl.Allows(net.ParseIP("192.168.1.100"))
	srv.aclMu.RUnlock()

	if !stillAllowed {
		t.Error("old ACL should still be active after failed update")
	}
}

func TestServer_UpdateConfig_EmptyACL(t *testing.T) {
	// Create server with initial ACL
	aclList, err := acl.New("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	storage, err := storage.New(t.TempDir(), "test")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storage.Close()

	mockFwd := &mockForwarder{}
	srv, err := New(Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}, aclList, storage, mockFwd, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Update with empty CIDR (should not change ACL)
	err = srv.UpdateConfig(ReloadableConfig{
		AllowedCIDRs: "",
		ForwarderConfig: forwarder.ReloadableConfig{
			Token: "new-token",
		},
	})
	if err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Verify ACL was not updated (empty string should skip ACL update)
	srv.aclMu.RLock()
	stillAllowed := srv.acl.Allows(net.ParseIP("192.168.1.100"))
	srv.aclMu.RUnlock()

	if !stillAllowed {
		t.Error("ACL should not change when empty CIDR is provided")
	}

	// But forwarder config should be updated
	if mockFwd.lastConfig.Token != "new-token" {
		t.Errorf("expected token 'new-token', got '%s'", mockFwd.lastConfig.Token)
	}
}

func TestServer_UpdateConfig_NilForwarder(t *testing.T) {
	// Create server without forwarder
	aclList, err := acl.New("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	storage, err := storage.New(t.TempDir(), "test")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storage.Close()

	srv, err := New(Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}, aclList, storage, nil, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Update with forwarder config (should not panic)
	err = srv.UpdateConfig(ReloadableConfig{
		ForwarderConfig: forwarder.ReloadableConfig{
			Token: "new-token",
		},
	})
	if err != nil {
		t.Fatalf("failed to update config: %v", err)
	}
}
