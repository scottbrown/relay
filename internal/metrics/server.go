package metrics

import (
	"expvar"
	"log/slog"
	"net/http"
	"time"
)

// StartServer starts the metrics HTTP server on the specified address.
// It serves the standard expvar endpoint at /debug/vars.
// If addr is empty, the server is not started.
func StartServer(addr string) error {
	if addr == "" {
		slog.Info("Metrics server disabled")
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())

	// Create server with explicit timeouts to prevent resource exhaustion
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	slog.Info("Starting metrics server", "addr", addr)

	// Run in a goroutine so it doesn't block
	go func() {
		if err := server.ListenAndServe(); err != nil {
			slog.Error("Metrics server failed", "error", err)
		}
	}()

	return nil
}
