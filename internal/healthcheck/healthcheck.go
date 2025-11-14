package healthcheck

import (
	"log"
	"net"
)

// Server represents a simple TCP healthcheck server
type Server struct {
	addr     string
	listener net.Listener
	stopChan chan struct{}
}

// New creates a new healthcheck server with the given address
func New(addr string) (*Server, error) {
	return &Server{
		addr:     addr,
		stopChan: make(chan struct{}),
	}, nil
}

// Start starts the healthcheck server in the background
func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	log.Printf("healthcheck listening on %s", s.addr)

	go s.acceptLoop()
	return nil
}

// Stop stops the healthcheck server
func (s *Server) Stop() error {
	close(s.stopChan)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) acceptLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.stopChan:
					// Server is stopping, this is expected
					return
				default:
					log.Printf("healthcheck accept error: %v", err)
					continue
				}
			}

			// Immediately close the connection (complete handshake only)
			_ = conn.Close() // #nosec G104 - Error on close for health check connection is non-critical
		}
	}
}
