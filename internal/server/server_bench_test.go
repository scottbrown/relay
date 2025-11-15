package server

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/storage"
)

// BenchmarkHandleConnection_Small benchmarks handling connections with small payloads
func BenchmarkHandleConnection_Small(b *testing.B) {
	// Setup mock HEC server
	hecServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer hecServer.Close()

	// Setup storage
	tmpDir := b.TempDir()
	storageManager, err := storage.New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer storageManager.Close()

	// Setup forwarder
	hecForwarder := forwarder.New(forwarder.Config{
		URL:     hecServer.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	// Setup ACL (allow all)
	aclList, err := acl.New("")
	if err != nil {
		b.Fatal(err)
	}

	// Create server
	srv, err := New(Config{
		ListenAddr:   ":0",
		MaxLineBytes: 1024 * 1024,
	}, aclList, storageManager, hecForwarder, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create test data (small JSON lines)
	line := `{"timestamp":"2024-01-01T00:00:00Z","event":"test"}` + "\n"
	data := bytes.Repeat([]byte(line), 10)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create mock connection
		client, server := net.Pipe()

		// Write data in background
		go func() {
			client.Write(data)
			client.Close()
		}()

		// Handle connection
		srv.handleConnection(server)
	}
}

// BenchmarkHandleConnection_Medium benchmarks handling connections with medium payloads
func BenchmarkHandleConnection_Medium(b *testing.B) {
	// Setup mock HEC server
	hecServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer hecServer.Close()

	// Setup storage
	tmpDir := b.TempDir()
	storageManager, err := storage.New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer storageManager.Close()

	// Setup forwarder
	hecForwarder := forwarder.New(forwarder.Config{
		URL:     hecServer.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	// Setup ACL (allow all)
	aclList, err := acl.New("")
	if err != nil {
		b.Fatal(err)
	}

	// Create server
	srv, err := New(Config{
		ListenAddr:   ":0",
		MaxLineBytes: 1024 * 1024,
	}, aclList, storageManager, hecForwarder, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create test data (medium JSON lines - 1KB each)
	var sb strings.Builder
	sb.WriteString(`{"timestamp":"2024-01-01T00:00:00Z","event":"test","data":"`)
	sb.WriteString(strings.Repeat("x", 900))
	sb.WriteString(`"}`)
	line := sb.String() + "\n"
	data := bytes.Repeat([]byte(line), 10)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create mock connection
		client, server := net.Pipe()

		// Write data in background
		go func() {
			client.Write(data)
			client.Close()
		}()

		// Handle connection
		srv.handleConnection(server)
	}
}

// BenchmarkHandleConnection_WithGzip benchmarks handling connections with gzip enabled
func BenchmarkHandleConnection_WithGzip(b *testing.B) {
	// Setup mock HEC server
	hecServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer hecServer.Close()

	// Setup storage
	tmpDir := b.TempDir()
	storageManager, err := storage.New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer storageManager.Close()

	// Setup forwarder with gzip
	hecForwarder := forwarder.New(forwarder.Config{
		URL:     hecServer.URL,
		Token:   "test-token",
		UseGzip: true,
	})

	// Setup ACL (allow all)
	aclList, err := acl.New("")
	if err != nil {
		b.Fatal(err)
	}

	// Create server
	srv, err := New(Config{
		ListenAddr:   ":0",
		MaxLineBytes: 1024 * 1024,
	}, aclList, storageManager, hecForwarder, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create test data
	line := `{"timestamp":"2024-01-01T00:00:00Z","event":"test"}` + "\n"
	data := bytes.Repeat([]byte(line), 10)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create mock connection
		client, server := net.Pipe()

		// Write data in background
		go func() {
			client.Write(data)
			client.Close()
		}()

		// Handle connection
		srv.handleConnection(server)
	}
}

// BenchmarkHandleConnection_MixedValid benchmarks handling connections with mixed valid/invalid JSON
func BenchmarkHandleConnection_MixedValid(b *testing.B) {
	// Setup mock HEC server
	hecServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer hecServer.Close()

	// Setup storage
	tmpDir := b.TempDir()
	storageManager, err := storage.New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer storageManager.Close()

	// Setup forwarder
	hecForwarder := forwarder.New(forwarder.Config{
		URL:     hecServer.URL,
		Token:   "test-token",
		UseGzip: false,
	})

	// Setup ACL (allow all)
	aclList, err := acl.New("")
	if err != nil {
		b.Fatal(err)
	}

	// Create server
	srv, err := New(Config{
		ListenAddr:   ":0",
		MaxLineBytes: 1024 * 1024,
	}, aclList, storageManager, hecForwarder, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create test data (mix of valid and invalid JSON)
	var buf bytes.Buffer
	buf.WriteString(`{"timestamp":"2024-01-01T00:00:00Z","event":"test"}` + "\n")
	buf.WriteString(`{"invalid json` + "\n") // Invalid
	buf.WriteString(`{"timestamp":"2024-01-01T00:00:00Z","event":"test2"}` + "\n")
	buf.WriteString(`not json at all` + "\n") // Invalid
	buf.WriteString(`{"timestamp":"2024-01-01T00:00:00Z","event":"test3"}` + "\n")
	data := buf.Bytes()

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create mock connection
		client, server := net.Pipe()

		// Write data in background
		go func() {
			client.Write(data)
			client.Close()
		}()

		// Handle connection
		srv.handleConnection(server)
	}
}

// BenchmarkHandleConnection_NoForwarding benchmarks handling connections without HEC forwarding
func BenchmarkHandleConnection_NoForwarding(b *testing.B) {
	// Setup storage
	tmpDir := b.TempDir()
	storageManager, err := storage.New(tmpDir, "bench")
	if err != nil {
		b.Fatal(err)
	}
	defer storageManager.Close()

	// Setup forwarder (disabled - no URL)
	hecForwarder := forwarder.New(forwarder.Config{})

	// Setup ACL (allow all)
	aclList, err := acl.New("")
	if err != nil {
		b.Fatal(err)
	}

	// Create server
	srv, err := New(Config{
		ListenAddr:   ":0",
		MaxLineBytes: 1024 * 1024,
	}, aclList, storageManager, hecForwarder, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create test data
	line := `{"timestamp":"2024-01-01T00:00:00Z","event":"test"}` + "\n"
	data := bytes.Repeat([]byte(line), 10)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create mock connection
		client, server := net.Pipe()

		// Write data in background
		go func() {
			client.Write(data)
			client.Close()
		}()

		// Handle connection
		srv.handleConnection(server)
	}
}
