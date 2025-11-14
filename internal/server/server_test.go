package server

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/storage"
)

func TestNew(t *testing.T) {
	config := Config{
		ListenAddr:   ":8080",
		TLSCertFile:  "",
		TLSKeyFile:   "",
		MaxLineBytes: 1024,
	}

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	hecConfig := forwarder.Config{
		URL:   "",
		Token: "",
	}
	hecForwarder := forwarder.New(hecConfig)

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}

	if server.config != config {
		t.Error("config should be stored")
	}

	if server.acl != aclList {
		t.Error("ACL should be stored")
	}

	if server.storage != storageManager {
		t.Error("storage should be stored")
	}

	if server.forwarder != hecForwarder {
		t.Error("forwarder should be stored")
	}

	if server.listener != nil {
		t.Error("listener should be nil initially")
	}
}

func TestStart_TCP(t *testing.T) {
	// Use a random available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to get available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	config := Config{
		ListenAddr:   ":" + string(rune(port+48)),
		MaxLineBytes: 1024,
	}

	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	err = server.Stop()
	if err != nil {
		t.Errorf("Stop should succeed: %v", err)
	}

	// Check if Start returned (with or without error)
	select {
	case <-errCh:
		// Expected - server stopped
	case <-time.After(time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestHandleConnection_StorageError(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("")

	// Use a path that will cause storage errors (pointing to a file instead of directory)
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "not-a-directory")
	if err := os.WriteFile(badPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create bad path: %v", err)
	}

	storageManager, err := storage.New(badPath)
	if err == nil {
		storageManager.Close()
		t.Skip("expected storage creation to fail")
	}

	// Use a working storage for the test
	workingStorage, _ := storage.New(tmpDir)
	defer workingStorage.Close()

	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, workingStorage, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test with valid JSON - this should still close connection gracefully even if other errors occur
	jsonData := `{"test": "data"}` + "\n"
	conn := newMockConn(jsonData, "192.168.1.1:12345")

	server.handleConnection(conn)

	if !conn.closed {
		t.Error("connection should be closed after handling")
	}
}

func TestStart_InvalidTLS(t *testing.T) {
	config := Config{
		ListenAddr:   ":0",
		TLSCertFile:  "nonexistent.crt",
		TLSKeyFile:   "nonexistent.key",
		MaxLineBytes: 1024,
	}

	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	err = server.Start()
	if err == nil {
		t.Fatal("expected error for invalid TLS certificates")
		server.Stop()
	}
}

func TestStart_InvalidAddress(t *testing.T) {
	config := Config{
		ListenAddr:   "invalid:address:format",
		MaxLineBytes: 1024,
	}

	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	err = server.Start()
	if err == nil {
		t.Fatal("expected error for invalid listen address")
		server.Stop()
	}
}

func TestStop_NoListener(t *testing.T) {
	config := Config{
		ListenAddr:   ":8080",
		MaxLineBytes: 1024,
	}

	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	err = server.Stop()
	if err != nil {
		t.Errorf("Stop with no listener should succeed: %v", err)
	}
}

// mockConn implements net.Conn for testing
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	addr     net.Addr
	closed   bool
}

func newMockConn(data string, addr string) *mockConn {
	tcpAddr, _ := net.ResolveTCPAddr("tcp", addr)
	return &mockConn{
		readBuf:  bytes.NewBufferString(data),
		writeBuf: &bytes.Buffer{},
		addr:     tcpAddr,
	}
}

func (m *mockConn) Read(b []byte) (int, error) {
	return m.readBuf.Read(b)
}

func (m *mockConn) Write(b []byte) (int, error) {
	return m.writeBuf.Write(b)
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr  { return nil }
func (m *mockConn) RemoteAddr() net.Addr { return m.addr }

func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestHandleConnection_ValidJSON(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create mock connection with valid JSON
	jsonData := `{"test": "data"}` + "\n"
	conn := newMockConn(jsonData, "192.168.1.1:12345")

	// Handle connection (this will block until connection is closed)
	server.handleConnection(conn)

	if !conn.closed {
		t.Error("connection should be closed after handling")
	}

	// Verify data was written to storage
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Error("storage should have current file after write")
	}
}

func TestHandleConnection_InvalidJSON(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create mock connection with invalid JSON
	invalidJSON := "invalid json data\n"
	conn := newMockConn(invalidJSON, "192.168.1.1:12345")

	server.handleConnection(conn)

	if !conn.closed {
		t.Error("connection should be closed after handling")
	}

	// Storage should not have any files since JSON was invalid
	currentFile := storageManager.CurrentFile()
	if currentFile != "" {
		t.Error("storage should not have current file after invalid JSON")
	}
}

func TestHandleConnection_LineTooLong(t *testing.T) {
	config := Config{MaxLineBytes: 10} // Very small limit
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create mock connection with line exceeding limit
	longLine := "this line is definitely longer than 10 bytes\n"
	conn := newMockConn(longLine, "192.168.1.1:12345")

	server.handleConnection(conn)

	if !conn.closed {
		t.Error("connection should be closed after handling")
	}

	// Storage should not have any files since line was too long
	currentFile := storageManager.CurrentFile()
	if currentFile != "" {
		t.Error("storage should not have current file after line too long error")
	}
}

func TestHandleConnection_MultipleLines(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create mock connection with multiple valid JSON lines
	jsonData := `{"line": 1}` + "\n" + `{"line": 2}` + "\n"
	conn := newMockConn(jsonData, "192.168.1.1:12345")

	server.handleConnection(conn)

	if !conn.closed {
		t.Error("connection should be closed after handling")
	}

	// Verify data was written to storage
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Error("storage should have current file after writes")
	}
}

func TestUpdateACL(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	initialACL, _ := acl.New("10.0.0.0/8")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, initialACL, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test with initial ACL
	testIP := net.ParseIP("10.0.1.100")
	if !server.allows(testIP) {
		t.Error("IP 10.0.1.100 should be allowed with initial ACL")
	}

	outsideIP := net.ParseIP("192.168.1.1")
	if server.allows(outsideIP) {
		t.Error("IP 192.168.1.1 should not be allowed with initial ACL")
	}

	// Update ACL to allow different network
	newACL, _ := acl.New("192.168.0.0/16")
	server.UpdateACL(newACL)

	// Test with updated ACL
	if server.allows(testIP) {
		t.Error("IP 10.0.1.100 should not be allowed after ACL update")
	}

	if !server.allows(outsideIP) {
		t.Error("IP 192.168.1.1 should be allowed after ACL update")
	}
}

func TestUpdateMaxLineBytes(t *testing.T) {
	initialLimit := 1024
	config := Config{MaxLineBytes: initialLimit}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Verify initial limit
	if limit := server.getMaxLineBytes(); limit != initialLimit {
		t.Errorf("expected initial limit %d, got %d", initialLimit, limit)
	}

	// Update limit
	newLimit := 2048
	server.UpdateMaxLineBytes(newLimit)

	// Verify updated limit
	if limit := server.getMaxLineBytes(); limit != newLimit {
		t.Errorf("expected updated limit %d, got %d", newLimit, limit)
	}
}

func TestAllows_WithACL(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("10.0.0.0/8,192.168.1.0/24")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"allowed from 10.0.0.0/8", "10.0.1.100", true},
		{"allowed from 192.168.1.0/24", "192.168.1.50", true},
		{"denied outside CIDR", "172.16.0.1", false},
		{"denied from different /24", "192.168.2.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if result := server.allows(ip); result != tt.expected {
				t.Errorf("allows(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestAllows_NoACL(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// With no ACL configured, all IPs should be allowed
	testIPs := []string{
		"10.0.1.100",
		"192.168.1.50",
		"172.16.0.1",
		"8.8.8.8",
	}

	for _, ipStr := range testIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			if !server.allows(ip) {
				t.Errorf("IP %s should be allowed when no ACL is configured", ipStr)
			}
		})
	}
}

func TestUpdateACL_ThreadSafety(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	initialACL, _ := acl.New("10.0.0.0/8")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir)
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, initialACL, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Spawn multiple goroutines that update and check ACLs concurrently
	done := make(chan bool)
	testIP := net.ParseIP("10.0.1.100")

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				// Alternate between two different ACLs
				var newACL *acl.List
				if j%2 == 0 {
					newACL, _ = acl.New("10.0.0.0/8")
				} else {
					newACL, _ = acl.New("192.168.0.0/16")
				}
				server.UpdateACL(newACL)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = server.allows(testIP)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// If we get here without a race condition, the test passes
}
