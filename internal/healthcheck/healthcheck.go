// Package healthcheck provides a simple TCP health check endpoint.
// The server accepts connections and immediately closes them, allowing health monitors
// to verify the service is running.
package healthcheck

import (
	"log/slog"
	"net"
)

// Server represents a simple TCP health check server.
// It accepts connections and immediately closes them without reading or writing data.
type Server struct {
	addr     string
	listener net.Listener
	stopChan chan struct{}
}

// New creates a new health check server that will listen on the given address.
// The server is not started until Start is called.
func New(addr string) (*Server, error) {
	return &Server{
		addr:     addr,
		stopChan: make(chan struct{}),
	}, nil
}

// Start starts the health check server in a background goroutine.
// It returns immediately after starting the accept loop.
// Returns an error if the listener cannot be created.
func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	slog.Debug("healthcheck server started", "addr", s.addr)

	go s.acceptLoop()
	return nil
}

// Stop stops the health check server by closing the listener.
// It is safe to call Stop multiple times.
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
					slog.Debug("healthcheck accept error", "error", err)
					continue
				}
			}

			// Immediately close the connection (complete handshake only)
			slog.Debug("healthcheck request", "client_addr", conn.RemoteAddr().String())
			if err := conn.Close(); err != nil {
				// Ignore close errors for health check connections
			}
		}
	}
}
