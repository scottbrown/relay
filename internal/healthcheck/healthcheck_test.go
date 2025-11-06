package healthcheck

import (
	"net"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	srv, err := New(":0")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if srv == nil {
		t.Fatal("New() returned nil server")
	}
	if srv.addr != ":0" {
		t.Errorf("New() addr = %v, want :0", srv.addr)
	}
}

func TestServerStartStop(t *testing.T) {
	srv, err := New(":0")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start the server
	err = srv.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify the server is listening
	if srv.listener == nil {
		t.Fatal("Start() did not create listener")
	}

	// Stop the server
	err = srv.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestServerAcceptsConnections(t *testing.T) {
	srv, err := New(":0")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start the server
	err = srv.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer srv.Stop()

	// Get the actual listening address
	addr := srv.listener.Addr().String()

	// Give the server time to start accepting
	time.Sleep(100 * time.Millisecond)

	// Connect to the server
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	// The server should immediately close the connection
	// Try to read - should get EOF quickly
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("Expected connection to be closed, but read succeeded")
	}
}

func TestServerMultipleConnections(t *testing.T) {
	srv, err := New(":0")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start the server
	err = srv.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer srv.Stop()

	// Get the actual listening address
	addr := srv.listener.Addr().String()

	// Give the server time to start accepting
	time.Sleep(100 * time.Millisecond)

	// Connect multiple times
	for i := 0; i < 5; i++ {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("Dial() iteration %d error = %v", i, err)
		}
		conn.Close()
	}
}

func TestServerStopWithoutStart(t *testing.T) {
	srv, err := New(":0")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Stop without starting should not error
	err = srv.Stop()
	if err != nil {
		t.Errorf("Stop() without Start() error = %v", err)
	}
}
