// Package server implements the TCP/TLS listener that accepts incoming log connections.
// It coordinates ACL validation, data processing, local storage, and HEC forwarding.
package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/processor"
	"github.com/scottbrown/relay/internal/storage"
)

// Config holds server configuration including listen address and TLS settings.
type Config struct {
	ListenAddr   string
	TLSCertFile  string
	TLSKeyFile   string
	MaxLineBytes int
}

// Server manages incoming TCP/TLS connections and coordinates log processing.
// It accepts connections, validates clients against ACLs, processes log lines,
// stores them locally, and forwards to Splunk HEC.
//
// Server is safe for concurrent use by multiple goroutines.
type Server struct {
	config      Config
	acl         *acl.List
	storage     *storage.Manager
	forwarder   forwarder.Forwarder
	listener    net.Listener
	connections sync.WaitGroup // Tracks active connections for graceful shutdown
	shutdown    chan struct{}  // Signals when shutdown is initiated
	shutdownMu  sync.Mutex     // Protects shutdown channel from double-close
}

// isTestMode checks if we're running in test or benchmark mode
func isTestMode() bool {
	return flag.Lookup("test.v") != nil || flag.Lookup("test.bench") != nil
}

// generateConnID generates a unique correlation ID (UUID v4) for a connection
func generateConnID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// New creates a new Server with the given configuration and dependencies.
// It initialises the server but does not start listening.
// The acl, storage, and forwarder parameters may be nil if those features are disabled.
func New(config Config, aclList *acl.List, storageManager *storage.Manager, fwd forwarder.Forwarder) (*Server, error) {
	return &Server{
		config:    config,
		acl:       aclList,
		storage:   storageManager,
		forwarder: fwd,
		shutdown:  make(chan struct{}),
	}, nil
}

// Start begins accepting connections on the configured listen address.
// It blocks until an error occurs or Stop is called.
//
// If TLS is configured (TLSCertFile and TLSKeyFile are set), connections are encrypted.
// Each accepted connection is handled in a separate goroutine.
//
// Returns an error if the listener cannot be created or TLS setup fails.
func (s *Server) Start() error {
	var err error

	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			return err
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		s.listener, err = tls.Listen("tcp", s.config.ListenAddr, tlsConfig)
		if err != nil {
			return err
		}

		slog.Info("server listening", "addr", s.config.ListenAddr, "tls_enabled", true)
	} else {
		s.listener, err = net.Listen("tcp", s.config.ListenAddr)
		if err != nil {
			return err
		}

		slog.Info("server listening", "addr", s.config.ListenAddr, "tls_enabled", false)
	}

	return s.acceptLoop()
}

// Stop stops the server by closing the listener.
// Active connections are not forcibly closed but will eventually terminate.
// Deprecated: Use Shutdown instead for graceful connection handling.
func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Shutdown gracefully shuts down the server by first stopping new connections,
// then waiting for active connections to complete or timeout.
// It returns nil if all connections closed gracefully, or an error if the context
// deadline is exceeded while waiting for connections to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	startTime := time.Now()
	slog.Info("initiating graceful shutdown")

	// Signal shutdown to acceptLoop
	s.shutdownMu.Lock()
	select {
	case <-s.shutdown:
		// Already shutting down
		s.shutdownMu.Unlock()
		return nil
	default:
		close(s.shutdown)
		s.shutdownMu.Unlock()
	}

	// Close listener to stop accepting new connections
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			slog.Warn("failed to close listener", "error", err)
		}
		slog.Info("stopped accepting new connections")
	}

	// Wait for existing connections with timeout
	done := make(chan struct{})
	go func() {
		s.connections.Wait()
		close(done)
	}()

	select {
	case <-done:
		duration := time.Since(startTime)
		slog.Info("all connections closed gracefully", "duration", duration.String())
		return nil
	case <-ctx.Done():
		duration := time.Since(startTime)
		slog.Warn("shutdown timeout: some connections still active", "duration", duration.String())
		return fmt.Errorf("shutdown timeout after %v: some connections still active", duration)
	}
}

func (s *Server) acceptLoop() error {
	for {
		// Set accept deadline to periodically check shutdown channel
		// This allows graceful shutdown without forcibly closing connections
		if tcpListener, ok := s.listener.(*net.TCPListener); ok {
			_ = tcpListener.SetDeadline(time.Now().Add(1 * time.Second)) // Ignore error - deadline is best effort
		}

		conn, err := s.listener.Accept()
		if err != nil {
			// Check if shutdown was initiated
			select {
			case <-s.shutdown:
				slog.Info("accept loop stopping, shutdown initiated")
				return nil
			default:
			}

			// Check for timeout (expected during normal operation due to deadline)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			// Check if listener was closed (expected during shutdown)
			if netErr, ok := err.(net.Error); ok && !netErr.Temporary() {
				// Listener closed, exit gracefully
				return nil
			}

			slog.Warn("accept error", "error", err)
			continue
		}

		// Check ACL
		ra, _ := net.ResolveTCPAddr("tcp", conn.RemoteAddr().String())
		if !s.acl.Allows(ra.IP) {
			slog.Warn("connection denied by ACL", "client_ip", ra.IP.String())
			if err := conn.Close(); err != nil {
				slog.Warn("failed to close denied connection", "error", err)
			}
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	// Track this connection for graceful shutdown
	s.connections.Add(1)
	defer s.connections.Done()
	defer conn.Close()

	// Check if shutdown is in progress
	select {
	case <-s.shutdown:
		slog.Info("rejecting new connection, shutdown in progress")
		return
	default:
	}

	connID := generateConnID()
	clientAddr := conn.RemoteAddr().String()
	connStartTime := time.Now()
	slog.Info("connection accepted", "conn_id", connID, "client_addr", clientAddr)

	br := bufio.NewReader(conn)

	defer func() {
		duration := time.Since(connStartTime)
		slog.Info("connection closed", "conn_id", connID, "client_addr", clientAddr, "duration", duration.String())
	}()

	for {
		line, err := processor.ReadLineLimited(br, s.config.MaxLineBytes)
		if err != nil {
			// Only exit on EOF - other errors (like oversized lines) should just skip the line
			if err.Error() == "EOF" {
				slog.Debug("connection EOF", "conn_id", connID, "client_addr", clientAddr)
				return
			}
			slog.Warn("read error", "conn_id", connID, "client_addr", clientAddr, "error", err)
			continue
		}

		// Validate JSON
		if !processor.IsValidJSON(line) {
			slog.Warn("invalid JSON", "conn_id", connID, "client_addr", clientAddr, "line", processor.Truncate(line, 200))
			continue
		}

		// Store locally
		if err := s.storage.Write(connID, line); err != nil {
			slog.Error("storage write failed", "conn_id", connID, "error", err)
		}

		// Forward to HEC asynchronously to avoid blocking the read loop
		// Make a copy of the line to avoid data races
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		go func(data []byte, id string) {
			if err := s.forwarder.Forward(id, data); err != nil {
				// Suppress HEC errors in test/benchmark mode to reduce noise
				if !isTestMode() {
					slog.Debug("HEC forward failed", "conn_id", id, "error", err)
				}
			}
		}(lineCopy, connID)
	}
}
