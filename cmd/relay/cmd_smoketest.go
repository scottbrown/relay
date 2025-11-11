package main

import (
	"fmt"
	"os"

	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/spf13/cobra"
)

var smokeTestCmd = &cobra.Command{
	Use:   "smoke-test",
	Short: "Test Splunk HEC connectivity for all configured listeners",
	Long:  "Test connectivity to Splunk HEC for all configured listeners and exit",
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		performSmokeTest(cfg)
	},
}

// performSmokeTest tests connectivity to Splunk HEC for all listeners
func performSmokeTest(cfg *config.Config) {
	fmt.Printf("🔍 Testing Splunk HEC connectivity for all listeners...\n\n")

	hasErrors := false

	for _, listenerCfg := range cfg.Listeners {
		fmt.Printf("Listener: %s (%s)\n", listenerCfg.Name, listenerCfg.LogType)

		// Merge global and per-listener HEC config
		hecCfg := mergeHECConfig(cfg.Splunk, listenerCfg.Splunk)

		if hecCfg.URL == "" {
			fmt.Printf("  ⚠️  Warning: Splunk HEC URL is not configured for this listener\n\n")
			continue
		}

		if hecCfg.Token == "" {
			fmt.Printf("  ⚠️  Warning: Splunk HEC token is not configured for this listener\n\n")
			continue
		}

		fmt.Printf("  URL: %s\n", hecCfg.URL)
		fmt.Printf("  Source Type: %s\n", hecCfg.SourceType)
		fmt.Printf("  Gzip: %v\n", hecCfg.UseGzip)

		// Create HEC forwarder for testing
		hecForwarder := forwarder.New(forwarder.Config{
			URL:        hecCfg.URL,
			Token:      hecCfg.Token,
			SourceType: hecCfg.SourceType,
			UseGzip:    hecCfg.UseGzip,
		})

		// Perform health check
		err := hecForwarder.HealthCheck()
		if err != nil {
			fmt.Printf("  ❌ Error: %v\n\n", err)
			hasErrors = true
			continue
		}

		fmt.Printf("  ✅ Success: Splunk HEC is reachable and token is valid\n\n")
	}

	if hasErrors {
		fmt.Printf("❌ Some listeners failed HEC connectivity test\n")
		os.Exit(1)
	}

	fmt.Printf("✅ All listeners passed HEC connectivity test\n")
}
