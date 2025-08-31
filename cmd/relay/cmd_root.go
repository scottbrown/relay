package main

import (
	"log"

	"github.com/scottbrown/relay"
	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/server"
	"github.com/scottbrown/relay/internal/storage"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "relay",
	Short:   "TCP relay service for Zscaler ZPA LSS data to Splunk HEC",
	Long:    "A TCP relay service that receives Zscaler ZPA LSS data and forwards it to Splunk HEC with local persistence.",
	Version: relay.Version(),
	Run:     handleRootCmd,
}

func handleRootCmd(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Override config with CLI flags if provided
	applyFlagOverrides(cfg, cmd)

	// Initialize ACL
	aclList, err := acl.New(cfg.AllowedCIDRs)
	if err != nil {
		log.Fatalf("allow-cidrs: %v", err)
	}

	// Initialize storage
	storageManager, err := storage.New(cfg.OutputDir)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer storageManager.Close()

	// Initialize HEC forwarder
	hecForwarder := forwarder.New(forwarder.Config{
		URL:        cfg.SplunkHECURL,
		Token:      cfg.SplunkToken,
		SourceType: cfg.SourceType,
		UseGzip:    cfg.GzipHEC,
	})

	// Perform startup health check for Splunk HEC (if configured)
	if cfg.SplunkHECURL != "" && cfg.SplunkToken != "" {
		log.Printf("Testing Splunk HEC connectivity...")
		if err := hecForwarder.HealthCheck(); err != nil {
			log.Fatalf("Splunk HEC health check failed: %v", err)
		}
		log.Printf("Splunk HEC connectivity verified successfully")
	}

	// Initialize and start server
	srv, err := server.New(server.Config{
		ListenAddr:   cfg.ListenAddr,
		TLSCertFile:  cfg.TLSCertFile,
		TLSKeyFile:   cfg.TLSKeyFile,
		MaxLineBytes: cfg.MaxLineBytes,
	}, aclList, storageManager, hecForwarder)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	log.Fatal(srv.Start())
}
