package main

import (
	"context"
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
	"github.com/scottbrown/relay/internal/metrics"
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

	// Initialize metrics
	metrics.Init(relay.Version())

	// Start metrics server
	if err := metrics.StartServer(metricsAddr); err != nil {
		slog.Error("failed to start metrics server", "error", err)
		os.Exit(1)
	}

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
	forwarders := make([]forwarder.Forwarder, 0, len(cfg.Listeners))

	for _, listenerCfg := range cfg.Listeners {
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

		// Initialize HEC forwarder (single or multi-target)
		var fwd forwarder.Forwarder
		hasMultiTarget := false

		// Check for multi-target configuration
		if cfg.Splunk != nil && len(cfg.Splunk.HECTargets) > 0 {
			hasMultiTarget = true
		}
		if listenerCfg.Splunk != nil && len(listenerCfg.Splunk.HECTargets) > 0 {
			hasMultiTarget = true
		}

		if hasMultiTarget {
			// Initialize multi-target forwarder
			targets, routingMode := getHECTargetsAndRouting(cfg.Splunk, listenerCfg.Splunk)
			multiFwd, err := forwarder.NewMulti(targets, routingMode)
			if err != nil {
				slog.Error("failed to initialize multi-target HEC forwarder", "listener", listenerCfg.Name, "error", err)
				os.Exit(1)
			}
			fwd = multiFwd
			slog.Info("initialized multi-target HEC forwarder",
				"listener", listenerCfg.Name,
				"targets", len(targets),
				"mode", routingMode)
		} else {
			// Initialize single-target forwarder (legacy mode)
			hecCfg := mergeHECConfig(cfg.Splunk, listenerCfg.Splunk)
			fwd = forwarder.New(hecCfg)
			if hecCfg.URL != "" {
				slog.Info("initialized single-target HEC forwarder", "listener", listenerCfg.Name)
			}
		}

		forwarders = append(forwarders, fwd)

		// Health check for HEC if configured
		if fwd != nil {
			slog.Info("testing Splunk HEC connectivity", "listener", listenerCfg.Name)
			if err := fwd.HealthCheck(); err != nil {
				// Only fail if HEC is actually configured (URL not empty)
				if hasMultiTarget || (listenerCfg.Splunk != nil && listenerCfg.Splunk.HECURL != "") || (cfg.Splunk != nil && cfg.Splunk.HECURL != "") {
					slog.Error("Splunk HEC health check failed", "listener", listenerCfg.Name, "error", err)
					os.Exit(1)
				}
			} else {
				slog.Info("Splunk HEC connectivity verified", "listener", listenerCfg.Name)
			}
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
		}, aclList, storageMgr, fwd)
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
		// If any server fails, gracefully shutdown all
		slog.Info("initiating graceful shutdown of all servers")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for i, srv := range servers {
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Warn("server shutdown error", "listener", cfg.Listeners[i].Name, "error", err)
			}
		}
	case sig := <-sigCh:
		slog.Info("received signal, initiating graceful shutdown", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for i, srv := range servers {
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Warn("server shutdown error", "listener", cfg.Listeners[i].Name, "error", err)
			}
		}
	}

	// Shutdown forwarders with timeout
	slog.Info("shutting down forwarders")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, fwd := range forwarders {
		if err := fwd.Shutdown(ctx); err != nil {
			slog.Warn("failed to shutdown forwarder", "error", err)
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
		cfg.Batch = mergeBatchConfig(global.Batch, nil)
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
			cfg.Batch = mergeBatchConfig(global.Batch, perListener.Batch)
		} else {
			cfg.CircuitBreaker = mergeCircuitBreakerConfig(nil, perListener.CircuitBreaker)
			cfg.Batch = mergeBatchConfig(nil, perListener.Batch)
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

func mergeBatchConfig(global, perListener *config.BatchConfig) forwarder.BatchConfig {
	// Start with defaults
	batchCfg := forwarder.BatchConfig{
		Enabled:       false,
		MaxSize:       100,
		MaxBytes:      1 << 20, // 1 MiB
		FlushInterval: 1 * time.Second,
	}

	// Apply global settings
	if global != nil {
		if global.Enabled != nil {
			batchCfg.Enabled = *global.Enabled
		}
		if global.MaxSize > 0 {
			batchCfg.MaxSize = global.MaxSize
		}
		if global.MaxBytes > 0 {
			batchCfg.MaxBytes = global.MaxBytes
		}
		if global.FlushInterval > 0 {
			batchCfg.FlushInterval = time.Duration(global.FlushInterval) * time.Second
		}
	}

	// Override with per-listener settings
	if perListener != nil {
		if perListener.Enabled != nil {
			batchCfg.Enabled = *perListener.Enabled
		}
		if perListener.MaxSize > 0 {
			batchCfg.MaxSize = perListener.MaxSize
		}
		if perListener.MaxBytes > 0 {
			batchCfg.MaxBytes = perListener.MaxBytes
		}
		if perListener.FlushInterval > 0 {
			batchCfg.FlushInterval = time.Duration(perListener.FlushInterval) * time.Second
		}
	}

	return batchCfg
}

func getHECTargetsAndRouting(global, perListener *config.SplunkConfig) ([]config.HECTarget, config.RoutingMode) {
	var targets []config.HECTarget
	var routingMode config.RoutingMode

	// Per-listener targets override global targets
	if perListener != nil && len(perListener.HECTargets) > 0 {
		targets = perListener.HECTargets
	} else if global != nil && len(global.HECTargets) > 0 {
		targets = global.HECTargets
	}

	// Get routing mode (per-listener overrides global)
	if perListener != nil && perListener.Routing != nil {
		routingMode = perListener.Routing.Mode
	} else if global != nil && global.Routing != nil {
		routingMode = global.Routing.Mode
	} else {
		// Default to "all" mode
		routingMode = config.RoutingModeAll
	}

	return targets, routingMode
}
