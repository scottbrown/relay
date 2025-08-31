package main

import (
	"fmt"
	"log"
	"os"

	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/spf13/cobra"
)

var smokeTestCmd = &cobra.Command{
	Use:   "smoke-test",
	Short: "Test Splunk HEC connectivity",
	Long:  "Test connectivity to Splunk HEC and exit",
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			log.Fatalf("config: %v", err)
		}

		// Override with CLI flags
		applyFlagOverrides(cfg, cmd)

		performSmokeTest(cfg)
	},
}

func applyFlagOverrides(cfg *config.Config, cmd *cobra.Command) {
	// Override config with CLI flags if provided (non-empty string flags or explicitly set flags)
	if cmd.Flags().Changed("listen") {
		cfg.ListenAddr = listenAddr
	}
	if cmd.Flags().Changed("tls-cert") {
		cfg.TLSCertFile = tlsCertFile
	}
	if cmd.Flags().Changed("tls-key") {
		cfg.TLSKeyFile = tlsKeyFile
	}
	if cmd.Flags().Changed("out") {
		cfg.OutputDir = outDir
	}
	if cmd.Flags().Changed("hec-url") {
		cfg.SplunkHECURL = hecURL
	}
	if cmd.Flags().Changed("hec-token") {
		cfg.SplunkToken = hecToken
	}
	if cmd.Flags().Changed("hec-sourcetype") {
		cfg.SourceType = hecSourcetype
	}
	if cmd.Flags().Changed("allow-cidrs") {
		cfg.AllowedCIDRs = allowedCIDRs
	}
	if cmd.Flags().Changed("hec-gzip") {
		cfg.GzipHEC = gzipHEC
	}
	if cmd.Flags().Changed("max-line-bytes") {
		cfg.MaxLineBytes = maxLineBytes
	}
}

// performSmokeTest tests connectivity to Splunk HEC
func performSmokeTest(cfg *config.Config) {
	fmt.Printf("🔍 Testing Splunk HEC connectivity...\n")
	fmt.Printf("URL: %s\n", cfg.SplunkHECURL)

	if cfg.SplunkHECURL == "" {
		fmt.Printf("❌ Error: Splunk HEC URL is not configured\n")
		fmt.Printf("Please set splunk_hec_url in config file or use -hec-url flag\n")
		os.Exit(1)
	}

	if cfg.SplunkToken == "" {
		fmt.Printf("❌ Error: Splunk HEC token is not configured\n")
		fmt.Printf("Please set splunk_token in config file or use -hec-token flag\n")
		os.Exit(1)
	}

	// Create HEC forwarder for testing
	hecForwarder := forwarder.New(forwarder.Config{
		URL:        cfg.SplunkHECURL,
		Token:      cfg.SplunkToken,
		SourceType: cfg.SourceType,
		UseGzip:    cfg.GzipHEC,
	})

	// Perform health check
	err := hecForwarder.HealthCheck()
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		fmt.Printf("Please verify your Splunk HEC URL and token are correct\n")
		os.Exit(1)
	}

	fmt.Printf("✅ Success: Splunk HEC is reachable and token is valid\n")
}
