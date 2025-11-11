package server

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/processor"
	"github.com/scottbrown/relay/internal/storage"
)

// Config contains server configuration
type Config struct {
	ListenAddr   string
	TLSCertFile  string
	TLSKeyFile   string
	MaxLineBytes int
}

// Server represents the TCP relay server
type Server struct {
	config    Config
	acl       *acl.List
	storage   *storage.Manager
	forwarder *forwarder.HEC
	listener  net.Listener
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

// New creates a new server with the given configuration
func New(config Config, aclList *acl.List, storageManager *storage.Manager, hecForwarder *forwarder.HEC) (*Server, error) {
	return &Server{
		config:    config,
		acl:       aclList,
		storage:   storageManager,
		forwarder: hecForwarder,
	}, nil
}

// Start starts the TCP/TLS server
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

// Stop stops the server
func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) acceptLoop() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
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
	defer conn.Close()

	connID := generateConnID()
	clientAddr := conn.RemoteAddr().String()
	slog.Info("connection accepted", "conn_id", connID, "client_addr", clientAddr)

	br := bufio.NewReader(conn)

	for {
		line, err := processor.ReadLineLimited(br, s.config.MaxLineBytes)
		if err != nil {
			// Only exit on EOF - other errors (like oversized lines) should just skip the line
			if err.Error() == "EOF" {
				slog.Debug("connection closed", "conn_id", connID, "client_addr", clientAddr)
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
