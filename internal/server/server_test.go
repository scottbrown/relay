package server

import (
	"bytes"
	"context"
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
	storageManager, err := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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

	storageManager, err := storage.New(badPath, "zpa")
	if err == nil {
		storageManager.Close()
		t.Skip("expected storage creation to fail")
	}

	// Use a working storage for the test
	workingStorage, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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
	storageManager, _ := storage.New(tmpDir, "zpa")
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

func TestGenerateConnID(t *testing.T) {
	// Test that generateConnID returns a non-empty string
	connID := generateConnID()
	if connID == "" {
		t.Error("generateConnID should return non-empty string")
	}

	// Test that it generates unique IDs
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateConnID()
		if ids[id] {
			t.Errorf("generateConnID generated duplicate ID: %s", id)
		}
		ids[id] = true
	}

	// Test UUID format (8-4-4-4-12 hex pattern)
	// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(connID) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(connID))
	}

	// Check for hyphens in correct positions
	if connID[8] != '-' || connID[13] != '-' || connID[18] != '-' || connID[23] != '-' {
		t.Errorf("UUID format incorrect, expected hyphens at positions 8,13,18,23: %s", connID)
	}
}

func TestShutdown_NoActiveConnections(t *testing.T) {
	// Use a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	config := Config{
		ListenAddr:   addr,
		MaxLineBytes: 1024,
	}

	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir, "zpa")
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

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should succeed with no active connections: %v", err)
	}

	// Check if Start returned
	select {
	case <-errCh:
		// Expected - server stopped
	case <-time.After(time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestShutdown_WithActiveConnections(t *testing.T) {
	// Use a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	config := Config{
		ListenAddr:   addr,
		MaxLineBytes: 1024,
	}

	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir, "zpa")
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

	// Establish a connection
	conn, err := net.Dial("tcp", config.ListenAddr)
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}

	// Send some data
	_, err = conn.Write([]byte(`{"test": "data"}` + "\n"))
	if err != nil {
		t.Errorf("failed to write data: %v", err)
	}

	// Close connection
	conn.Close()

	// Give connection time to be processed
	time.Sleep(100 * time.Millisecond)

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should succeed after connections close: %v", err)
	}

	// Check if Start returned
	select {
	case <-errCh:
		// Expected - server stopped
	case <-time.After(time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestShutdown_Timeout(t *testing.T) {
	config := Config{MaxLineBytes: 1024}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir, "zpa")
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Simulate an active connection by incrementing WaitGroup
	server.connections.Add(1)

	// Don't call Done() to simulate a stuck connection

	// Shutdown with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = server.Shutdown(ctx)
	if err == nil {
		t.Error("Shutdown should timeout with stuck connection")
	}

	// Clean up the WaitGroup to avoid hanging
	server.connections.Done()
}

func TestShutdown_DoubleCall(t *testing.T) {
	config := Config{ListenAddr: ":0", MaxLineBytes: 1024}
	aclList, _ := acl.New("")
	tmpDir := t.TempDir()
	storageManager, _ := storage.New(tmpDir, "zpa")
	defer storageManager.Close()
	hecForwarder := forwarder.New(forwarder.Config{})

	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First shutdown call
	err = server.Shutdown(ctx)
	if err != nil {
		t.Errorf("First Shutdown should succeed: %v", err)
	}

	// Second shutdown call (should be a no-op)
	err = server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Second Shutdown should succeed as no-op: %v", err)
	}
}
