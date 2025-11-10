package zpamock

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestMockZPAClient_Connect(t *testing.T) {
	// Start a simple TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Create client and connect
	client := New(listener.Addr().String())
	defer client.Close()

	ctx := context.Background()
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if client.conn == nil {
		t.Error("Expected connection to be established")
	}
}

func TestMockZPAClient_SendLine(t *testing.T) {
	// Start a simple TCP server that reads data
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	received := make(chan string, 10)

	// Accept and read in background
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			received <- scanner.Text()
		}
	}()

	// Create client and send lines
	client := New(listener.Addr().String())
	defer client.Close()

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	testLine := `{"test":"data"}`
	if err := client.SendLine(testLine); err != nil {
		t.Fatalf("Failed to send line: %v", err)
	}

	// Wait for received data
	select {
	case line := <-received:
		if line != testLine {
			t.Errorf("Expected %q, got %q", testLine, line)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for data")
	}

	if client.LinesSent != 1 {
		t.Errorf("Expected 1 line sent, got %d", client.LinesSent)
	}
}

func TestMockZPAClient_SendLines(t *testing.T) {
	// Start a simple TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	received := make(chan string, 10)

	// Accept and read in background
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			received <- scanner.Text()
		}
	}()

	// Create client and send lines
	client := New(listener.Addr().String())
	defer client.Close()

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	testLines := []string{
		`{"line":1}`,
		`{"line":2}`,
		`{"line":3}`,
	}

	if err := client.SendLines(testLines); err != nil {
		t.Fatalf("Failed to send lines: %v", err)
	}

	// Wait for all lines
	for i := 0; i < len(testLines); i++ {
		select {
		case line := <-received:
			if line != testLines[i] {
				t.Errorf("Line %d: expected %q, got %q", i, testLines[i], line)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Timeout waiting for line %d", i)
		}
	}

	if client.LinesSent != 3 {
		t.Errorf("Expected 3 lines sent, got %d", client.LinesSent)
	}
}

func TestMockZPAClient_LineDelay(t *testing.T) {
	// Start a simple TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Just keep connection open
		buf := make([]byte, 1024)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Create client with delay
	client := New(listener.Addr().String(), WithLineDelay(100*time.Millisecond))
	defer client.Close()

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	start := time.Now()

	// Send 2 lines (should take at least 200ms with delay)
	client.SendLine("line1")
	client.SendLine("line2")

	elapsed := time.Since(start)

	if elapsed < 200*time.Millisecond {
		t.Errorf("Expected at least 200ms delay, got %v", elapsed)
	}
}

func TestTruncatedJSON(t *testing.T) {
	input := `{"test":"value"}`
	truncated := TruncatedJSON(input)

	if truncated == input {
		t.Error("Expected truncated JSON to be different from input")
	}

	if strings.HasSuffix(truncated, "}") {
		t.Error("Expected truncated JSON to not end with '}'")
	}
}

func TestOversizedLine(t *testing.T) {
	size := 2048
	line := OversizedLine(size)

	if len(line) < size-100 { // Allow some tolerance for JSON structure
		t.Errorf("Expected line of at least %d bytes, got %d", size-100, len(line))
	}

	// Should be valid JSON structure (even if oversized)
	if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
		t.Error("Expected valid JSON structure")
	}
}

func TestBlankLine(t *testing.T) {
	line := BlankLine()

	if line != "" {
		t.Errorf("Expected empty string, got %q", line)
	}
}

func TestInvalidJSON(t *testing.T) {
	line := InvalidJSON()

	// Should not be valid JSON
	if strings.HasPrefix(line, `"`) {
		t.Error("Expected unquoted keys (invalid JSON)")
	}
}

func TestMissingClosingBrace(t *testing.T) {
	line := MissingClosingBrace()

	if strings.HasSuffix(line, "}") {
		t.Error("Expected line to not end with '}'")
	}

	if !strings.HasPrefix(line, "{") {
		t.Error("Expected line to start with '{'")
	}
}

func TestMockZPAClient_Close(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			time.Sleep(100 * time.Millisecond)
		}
	}()

	client := New(listener.Addr().String())

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	if client.conn != nil {
		t.Error("Expected connection to be nil after close")
	}
}

func TestWithVerbose(t *testing.T) {
	client := New("127.0.0.1:9999", WithVerbose(true))

	if !client.verbose {
		t.Error("Expected verbose to be true with WithVerbose option")
	}
}

func TestInvalidUTF8(t *testing.T) {
	line := InvalidUTF8()

	// Should contain invalid UTF-8 bytes
	if len(line) == 0 {
		t.Error("Expected non-empty line with invalid UTF-8")
	}

	// Should have JSON-like structure but with invalid UTF-8
	if !strings.Contains(line, "{") {
		t.Error("Expected line to contain '{'")
	}
}

func TestMockZPAClient_SendLineWithoutConnection(t *testing.T) {
	client := New("127.0.0.1:9999")

	// Try to send line without connecting
	err := client.SendLine("test")
	if err == nil {
		t.Error("Expected error when sending line without connection")
	}
}

func TestMockZPAClient_ConnectTimeout(t *testing.T) {
	// Use an address that will timeout
	client := New("192.0.2.1:9999") // TEST-NET-1, should not be routable
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Error("Expected error when connecting to unreachable address")
	}
}
