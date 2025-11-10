package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/scottbrown/relay"
	"github.com/scottbrown/relay/internal/acl"
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
	// Load configuration (config file is now required)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Initialize healthcheck server if enabled
	var healthSrv *healthcheck.Server
	if cfg.HealthCheckEnabled {
		healthSrv, err = healthcheck.New(cfg.HealthCheckAddr)
		if err != nil {
			log.Fatalf("healthcheck: %v", err)
		}
		defer healthSrv.Stop()

		if err := healthSrv.Start(); err != nil {
			log.Fatalf("healthcheck start: %v", err)
		}
		log.Printf("Healthcheck server listening on %s", cfg.HealthCheckAddr)
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
			log.Fatalf("[%s] allow-cidrs: %v", listenerCfg.Name, err)
		}

		// Initialize storage with file prefix
		storageMgr, err := storage.New(listenerCfg.OutputDir, listenerCfg.FilePrefix)
		if err != nil {
			log.Fatalf("[%s] storage: %v", listenerCfg.Name, err)
		}
		storageManagers = append(storageManagers, storageMgr)

		// Initialize HEC forwarder
		hecForwarder := forwarder.New(hecCfg)

		// Health check for HEC if configured
		if hecCfg.URL != "" && hecCfg.Token != "" {
			log.Printf("[%s] Testing Splunk HEC connectivity...", listenerCfg.Name)
			if err := hecForwarder.HealthCheck(); err != nil {
				log.Fatalf("[%s] Splunk HEC health check failed: %v", listenerCfg.Name, err)
			}
			log.Printf("[%s] Splunk HEC connectivity verified", listenerCfg.Name)
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
			log.Fatalf("[%s] server: %v", listenerCfg.Name, err)
		}

		servers = append(servers, srv)
		log.Printf("[%s] Initialized listener for %s on %s", listenerCfg.Name, listenerCfg.LogType, listenerCfg.ListenAddr)
	}

	// Cleanup storage managers on exit
	defer func() {
		for _, mgr := range storageManagers {
			if err := mgr.Close(); err != nil {
				log.Printf("warning: failed to close storage manager: %v", err)
			}
		}
	}()

	// Start all servers
	serverErrCh := make(chan error, len(servers))
	for i, srv := range servers {
		name := cfg.Listeners[i].Name
		go func(s *server.Server, n string) {
			log.Printf("[%s] Starting listener...", n)
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
			log.Printf("server error: %v", err)
		}
		// If any server fails, stop all
		for _, srv := range servers {
			if err := srv.Stop(); err != nil {
				log.Printf("warning: failed to stop server: %v", err)
			}
		}
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
		for _, srv := range servers {
			if err := srv.Stop(); err != nil {
				log.Printf("warning: failed to stop server: %v", err)
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
	}

	return cfg
}
