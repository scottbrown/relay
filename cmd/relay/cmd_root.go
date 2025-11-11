package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/scottbrown/relay"
	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/circuitbreaker"
	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/healthcheck"
	"github.com/scottbrown/relay/internal/server"
	"github.com/scottbrown/relay/internal/storage"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     relay.AppName,
	Short:   "TCP relay service for Zscaler ZPA LSS data to Splunk HEC",
	Long:    "A TCP relay service that receives Zscaler ZPA LSS data and forwards it to Splunk HEC with local persistence.",
	Version: relay.Version(),
	Run:     handleRootCmd,
}

func handleRootCmd(cmd *cobra.Command, args []string) {
	// Initialize structured logging
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "invalid log level %q, using info\n", logLevel)
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Convert all timestamps to UTC
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.TimeValue(t.UTC())
				}
			}
			return a
		},
	})
	slog.SetDefault(slog.New(handler))

	// Load configuration (config file is now required)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize healthcheck server if enabled
	var healthSrv *healthcheck.Server
	if cfg.HealthCheckEnabled {
		healthSrv, err = healthcheck.New(cfg.HealthCheckAddr)
		if err != nil {
			slog.Error("failed to create healthcheck server", "error", err)
			os.Exit(1)
		}
		defer healthSrv.Stop()

		if err := healthSrv.Start(); err != nil {
			slog.Error("failed to start healthcheck server", "error", err)
			os.Exit(1)
		}
		slog.Info("healthcheck server listening", "addr", cfg.HealthCheckAddr)
	}

	// Create servers for each listener
	servers := make([]*server.Server, 0, len(cfg.Listeners))
	storageManagers := make([]*storage.Manager, 0, len(cfg.Listeners))

	for _, listenerCfg := range cfg.Listeners {
		// Merge global and per-listener HEC config
		hecCfg := mergeHECConfig(cfg.Splunk, listenerCfg.Splunk)

		// Initialize ACL
		aclList, err := acl.New(listenerCfg.AllowedCIDRs)
		if err != nil {
			slog.Error("failed to initialize ACL", "listener", listenerCfg.Name, "error", err)
			os.Exit(1)
		}

		// Initialize storage with file prefix
		storageMgr, err := storage.New(listenerCfg.OutputDir, listenerCfg.FilePrefix)
		if err != nil {
			slog.Error("failed to initialize storage", "listener", listenerCfg.Name, "error", err)
			os.Exit(1)
		}
		storageManagers = append(storageManagers, storageMgr)

		// Initialize HEC forwarder
		hecForwarder := forwarder.New(hecCfg)

		// Health check for HEC if configured
		if hecCfg.URL != "" && hecCfg.Token != "" {
			slog.Info("testing Splunk HEC connectivity", "listener", listenerCfg.Name)
			if err := hecForwarder.HealthCheck(); err != nil {
				slog.Error("Splunk HEC health check failed", "listener", listenerCfg.Name, "error", err)
				os.Exit(1)
			}
			slog.Info("Splunk HEC connectivity verified", "listener", listenerCfg.Name)
		}

		// Initialize server
		var tlsCertFile, tlsKeyFile string
		if listenerCfg.TLS != nil {
			tlsCertFile = listenerCfg.TLS.CertFile
			tlsKeyFile = listenerCfg.TLS.KeyFile
		}

		srv, err := server.New(server.Config{
			ListenAddr:   listenerCfg.ListenAddr,
			TLSCertFile:  tlsCertFile,
			TLSKeyFile:   tlsKeyFile,
			MaxLineBytes: listenerCfg.MaxLineBytes,
		}, aclList, storageMgr, hecForwarder)
		if err != nil {
			slog.Error("failed to create server", "listener", listenerCfg.Name, "error", err)
			os.Exit(1)
		}

		servers = append(servers, srv)
		slog.Info("initialized listener", "listener", listenerCfg.Name, "log_type", listenerCfg.LogType, "addr", listenerCfg.ListenAddr)
	}

	// Cleanup storage managers on exit
	defer func() {
		for _, mgr := range storageManagers {
			if err := mgr.Close(); err != nil {
				slog.Warn("failed to close storage manager", "error", err)
			}
		}
	}()

	// Start all servers
	serverErrCh := make(chan error, len(servers))
	for i, srv := range servers {
		name := cfg.Listeners[i].Name
		go func(s *server.Server, n string) {
			slog.Info("starting listener", "listener", n)
			if err := s.Start(); err != nil {
				serverErrCh <- fmt.Errorf("[%s] %w", n, err)
			} else {
				serverErrCh <- nil
			}
		}(srv, name)
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Error("server error", "error", err)
		}
		// If any server fails, stop all
		for _, srv := range servers {
			if err := srv.Stop(); err != nil {
				slog.Warn("failed to stop server", "error", err)
			}
		}
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig.String())
		for _, srv := range servers {
			if err := srv.Stop(); err != nil {
				slog.Warn("failed to stop server", "error", err)
			}
		}
	}
}

func mergeHECConfig(global, perListener *config.SplunkConfig) forwarder.Config {
	cfg := forwarder.Config{}

	// Start with global settings
	if global != nil {
		cfg.URL = global.HECURL
		cfg.Token = global.HECToken
		if global.Gzip != nil {
			cfg.UseGzip = *global.Gzip
		}
		cfg.CircuitBreaker = mergeCircuitBreakerConfig(global.CircuitBreaker, nil)
	}

	// Override with per-listener settings
	if perListener != nil {
		if perListener.HECURL != "" {
			cfg.URL = perListener.HECURL
		}
		if perListener.HECToken != "" {
			cfg.Token = perListener.HECToken
		}
		if perListener.SourceType != "" {
			cfg.SourceType = perListener.SourceType
		}
		if perListener.Gzip != nil {
			cfg.UseGzip = *perListener.Gzip
		}
		// Merge circuit breaker config (per-listener can override global)
		if global != nil {
			cfg.CircuitBreaker = mergeCircuitBreakerConfig(global.CircuitBreaker, perListener.CircuitBreaker)
		} else {
			cfg.CircuitBreaker = mergeCircuitBreakerConfig(nil, perListener.CircuitBreaker)
		}
	}

	return cfg
}

func mergeCircuitBreakerConfig(global, perListener *config.CircuitBreakerConfig) circuitbreaker.Config {
	// Start with defaults
	cbCfg := circuitbreaker.DefaultConfig()

	// Apply global settings
	if global != nil {
		if global.Enabled != nil && !*global.Enabled {
			// Disable circuit breaker by setting threshold to 0 (effectively infinite)
			cbCfg.FailureThreshold = 0
		}
		if global.FailureThreshold > 0 {
			cbCfg.FailureThreshold = global.FailureThreshold
		}
		if global.SuccessThreshold > 0 {
			cbCfg.SuccessThreshold = global.SuccessThreshold
		}
		if global.Timeout > 0 {
			cbCfg.Timeout = time.Duration(global.Timeout) * time.Second
		}
		if global.HalfOpenMaxCalls > 0 {
			cbCfg.HalfOpenMaxCalls = global.HalfOpenMaxCalls
		}
	}

	// Override with per-listener settings
	if perListener != nil {
		if perListener.Enabled != nil && !*perListener.Enabled {
			cbCfg.FailureThreshold = 0
		}
		if perListener.FailureThreshold > 0 {
			cbCfg.FailureThreshold = perListener.FailureThreshold
		}
		if perListener.SuccessThreshold > 0 {
			cbCfg.SuccessThreshold = perListener.SuccessThreshold
		}
		if perListener.Timeout > 0 {
			cbCfg.Timeout = time.Duration(perListener.Timeout) * time.Second
		}
		if perListener.HalfOpenMaxCalls > 0 {
			cbCfg.HalfOpenMaxCalls = perListener.HalfOpenMaxCalls
		}
	}

	return cbCfg
}
