package server

import (
	"bufio"
	"crypto/tls"
	"log"
	"net"

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

		log.Printf("listening TLS on %s", s.config.ListenAddr)
	} else {
		s.listener, err = net.Listen("tcp", s.config.ListenAddr)
		if err != nil {
			return err
		}

		log.Printf("listening TCP on %s", s.config.ListenAddr)
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
			log.Printf("accept: %v", err)
			continue
		}

		// Check ACL
		ra, _ := net.ResolveTCPAddr("tcp", conn.RemoteAddr().String())
		if !s.acl.Allows(ra.IP) {
			log.Printf("deny %s", ra.IP)
			conn.Close()
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	br := bufio.NewReader(conn)

	for {
		line, err := processor.ReadLineLimited(br, s.config.MaxLineBytes)
		if err != nil {
			// Only exit on EOF - other errors (like oversized lines) should just skip the line
			if err.Error() == "EOF" {
				return
			}
			log.Printf("read: %v", err)
			continue
		}

		// Validate JSON
		if !processor.IsValidJSON(line) {
			log.Printf("invalid json from %s: %q", conn.RemoteAddr(),
				processor.Truncate(line, 200))
			continue
		}

		// Store locally
		if err := s.storage.Write(line); err != nil {
			log.Printf("write: %v", err)
		}

		// Forward to HEC asynchronously to avoid blocking the read loop
		// Make a copy of the line to avoid data races
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		go func(data []byte) {
			if err := s.forwarder.Forward(data); err != nil {
				log.Printf("hec: %v", err)
			}
		}(lineCopy)
	}
}
