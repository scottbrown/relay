package server

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/storage"
)

// Integration tests for acceptLoop and full server lifecycle

func TestServer_AcceptLoop_TCP(t *testing.T) {
	// Setup server
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0", // Use random available port
		MaxLineBytes: 1024,
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server in background
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual listening address
	actualAddr := server.listener.Addr().String()

	// Connect and send data
	conn, err := net.Dial("tcp", actualAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Send valid JSON
	testData := `{"test":"data","timestamp":"2024-01-01T00:00:00Z"}` + "\n"
	_, err = conn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Close connection
	conn.Close()

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify data was stored
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Fatal("expected data to be stored")
	}

	// Read stored data
	data, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}

	if !strings.Contains(string(data), `"test":"data"`) {
		t.Errorf("stored data should contain test data, got: %s", string(data))
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}

	// Verify server stopped
	select {
	case <-serverErrCh:
		// Expected - server stopped
	case <-time.After(2 * time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestServer_AcceptLoop_TLS(t *testing.T) {
	// Generate test TLS certificate
	certFile, keyFile := generateTestCert(t)

	// Setup server
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0",
		TLSCertFile:  certFile,
		TLSKeyFile:   keyFile,
		MaxLineBytes: 1024,
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server in background
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	actualAddr := server.listener.Addr().String()

	// Connect with TLS (skip verification for test)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	conn, err := tls.Dial("tcp", actualAddr, tlsConfig)
	if err != nil {
		t.Fatalf("failed to connect with TLS: %v", err)
	}

	// Send valid JSON
	testData := `{"tls":"test","secure":true}` + "\n"
	_, err = conn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	conn.Close()

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify data was stored
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Fatal("expected data to be stored")
	}

	data, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}

	if !strings.Contains(string(data), `"tls":"test"`) {
		t.Errorf("stored data should contain test data, got: %s", string(data))
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}

	select {
	case <-serverErrCh:
		// Expected
	case <-time.After(2 * time.Second):
		t.Error("server did not stop within timeout")
	}
}

func TestServer_AcceptLoop_ACLRejection(t *testing.T) {
	// Setup server with ACL that only allows specific IP
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	// Only allow 10.0.0.0/8 (not 127.0.0.1)
	aclList, err := acl.New("10.0.0.0/8")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server
	go server.Start()
	time.Sleep(100 * time.Millisecond)

	actualAddr := server.listener.Addr().String()

	// Try to connect (should be rejected by ACL)
	conn, err := net.Dial("tcp", actualAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Try to send data
	testData := `{"should":"be rejected"}` + "\n"
	_, err = conn.Write([]byte(testData))

	// Connection should be closed by server
	// Try to read - should get EOF or error
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Read(buf)

	// Should get error (connection closed)
	if err == nil {
		t.Error("expected connection to be closed by ACL")
	}

	conn.Close()

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify no data was stored
	currentFile := storageManager.CurrentFile()
	if currentFile != "" {
		data, _ := os.ReadFile(currentFile)
		if strings.Contains(string(data), "rejected") {
			t.Error("ACL-rejected data should not be stored")
		}
	}

	server.Stop()
}

func TestServer_AcceptLoop_ConcurrentConnections(t *testing.T) {
	// Setup server
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server
	go server.Start()
	time.Sleep(100 * time.Millisecond)

	actualAddr := server.listener.Addr().String()

	// Create multiple concurrent connections
	numConnections := 5
	errCh := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			conn, err := net.Dial("tcp", actualAddr)
			if err != nil {
				errCh <- err
				return
			}
			defer conn.Close()

			// Send multiple lines
			for j := 0; j < 3; j++ {
				data := `{"connection":` + string(rune(id+48)) + `,"line":` + string(rune(j+48)) + `}` + "\n"
				_, err = conn.Write([]byte(data))
				if err != nil {
					errCh <- err
					return
				}
				time.Sleep(10 * time.Millisecond)
			}

			errCh <- nil
		}(i)
	}

	// Wait for all connections to complete
	for i := 0; i < numConnections; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("connection error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for connections")
		}
	}

	// Give server time to process all data
	time.Sleep(200 * time.Millisecond)

	// Verify data was stored
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Fatal("expected data to be stored")
	}

	data, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}

	// Should have received data from multiple connections
	if !strings.Contains(string(data), `"connection"`) {
		t.Error("stored data should contain connection data")
	}

	server.Stop()
}

func TestServer_AcceptLoop_InvalidJSONFiltering(t *testing.T) {
	// Setup server
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server
	go server.Start()
	time.Sleep(100 * time.Millisecond)

	actualAddr := server.listener.Addr().String()

	// Connect and send mixed valid/invalid JSON
	conn, err := net.Dial("tcp", actualAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Send invalid JSON first
	conn.Write([]byte("invalid json\n"))
	time.Sleep(50 * time.Millisecond)

	// Send valid JSON
	conn.Write([]byte(`{"valid":"json"}` + "\n"))
	time.Sleep(50 * time.Millisecond)

	// Send more invalid JSON
	conn.Write([]byte("more invalid\n"))

	conn.Close()

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify only valid JSON was stored
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Fatal("expected valid data to be stored")
	}

	data, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}

	dataStr := string(data)
	if !strings.Contains(dataStr, `"valid":"json"`) {
		t.Error("stored data should contain valid JSON")
	}

	if strings.Contains(dataStr, "invalid") {
		t.Error("stored data should not contain invalid JSON")
	}

	server.Stop()
}

func TestServer_AcceptLoop_OversizedLineRejection(t *testing.T) {
	// Setup server with small line limit
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 50, // Very small limit
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server
	go server.Start()
	time.Sleep(100 * time.Millisecond)

	actualAddr := server.listener.Addr().String()

	// Connect and send oversized line
	conn, err := net.Dial("tcp", actualAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Send line that exceeds limit
	oversizedLine := `{"data":"` + strings.Repeat("x", 100) + `"}` + "\n"
	conn.Write([]byte(oversizedLine))
	time.Sleep(50 * time.Millisecond)

	// Send valid small line
	conn.Write([]byte(`{"small":"ok"}` + "\n"))

	conn.Close()

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify only small line was stored
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Fatal("expected valid data to be stored")
	}

	data, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}

	dataStr := string(data)
	if !strings.Contains(dataStr, `"small":"ok"`) {
		t.Error("stored data should contain small valid line")
	}

	if strings.Contains(dataStr, strings.Repeat("x", 100)) {
		t.Error("stored data should not contain oversized line")
	}

	server.Stop()
}

func TestServer_AcceptLoop_ConnectionPersistence(t *testing.T) {
	// Test that connections can stay open and send multiple messages
	tmpDir := t.TempDir()
	storageManager, err := storage.New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageManager.Close()

	aclList, err := acl.New("")
	if err != nil {
		t.Fatalf("failed to create ACL: %v", err)
	}

	config := Config{
		ListenAddr:   "127.0.0.1:0",
		MaxLineBytes: 1024,
	}

	hecForwarder := forwarder.New(forwarder.Config{})
	server, err := New(config, aclList, storageManager, hecForwarder)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start server
	go server.Start()
	time.Sleep(100 * time.Millisecond)

	actualAddr := server.listener.Addr().String()

	// Open connection and keep it alive
	conn, err := net.Dial("tcp", actualAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send messages over time
	for i := 0; i < 5; i++ {
		data := `{"message":` + string(rune(i+48)) + `}` + "\n"
		_, err = conn.Write([]byte(data))
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify all messages were stored
	currentFile := storageManager.CurrentFile()
	if currentFile == "" {
		t.Fatal("expected data to be stored")
	}

	data, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read stored file: %v", err)
	}

	// Should have all 5 messages
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}

	if lineCount != 5 {
		t.Errorf("expected 5 lines stored, got %d", lineCount)
	}

	server.Stop()
}

// Helper function to generate a test TLS certificate
func generateTestCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	// Write certificate to file
	certFile = filepath.Join(t.TempDir(), "test.crt")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("failed to create cert file: %v", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	// Write key to file
	keyFile = filepath.Join(t.TempDir(), "test.key")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("failed to create key file: %v", err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()

	return certFile, keyFile
}
