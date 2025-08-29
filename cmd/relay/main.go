package main

import (
	"fmt"
	"log"
	"os"

	"github.com/scottbrown/relay"
	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/server"
	"github.com/scottbrown/relay/internal/storage"
	"github.com/spf13/cobra"
)

var (
	configFile    string
	templateFlag  bool
	smokeTestFlag bool
	listenAddr    string
	tlsCertFile   string
	tlsKeyFile    string
	outDir        string
	hecURL        string
	hecToken      string
	hecSourcetype string
	allowedCIDRs  string
	gzipHEC       bool
	maxLineBytes  int

	rootCmd = &cobra.Command{
		Use:     "relay",
		Short:   "TCP relay service for Zscaler ZPA LSS data to Splunk HEC",
		Long:    "A TCP relay service that receives Zscaler ZPA LSS data and forwards it to Splunk HEC with local persistence.",
		Version: relay.Version(),
		Run:     runRelay,
	}

	templateCmd = &cobra.Command{
		Use:   "template",
		Short: "Output configuration template",
		Long:  "Output a YAML configuration template and exit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(config.GetTemplate())
		},
	}

	smokeTestCmd = &cobra.Command{
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
)

func init() {
	// Add subcommands
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(smokeTestCmd)

	// Root command flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "f", "", "Path to configuration file")
	rootCmd.Flags().StringVar(&listenAddr, "listen", "", "TCP listen address (e.g., :9015)")
	rootCmd.Flags().StringVar(&tlsCertFile, "tls-cert", "", "TLS cert file (optional)")
	rootCmd.Flags().StringVar(&tlsKeyFile, "tls-key", "", "TLS key file (optional)")
	rootCmd.Flags().StringVar(&outDir, "out", "", "Directory to persist NDJSON")
	rootCmd.Flags().StringVar(&hecURL, "hec-url", "", "Splunk HEC raw endpoint")
	rootCmd.Flags().StringVar(&hecToken, "hec-token", "", "Splunk HEC token")
	rootCmd.Flags().StringVar(&hecSourcetype, "hec-sourcetype", "", "Splunk sourcetype")
	rootCmd.Flags().StringVar(&allowedCIDRs, "allow-cidrs", "", "Comma-separated CIDRs allowed to connect")
	rootCmd.Flags().BoolVar(&gzipHEC, "hec-gzip", false, "Gzip compress payloads to HEC")
	rootCmd.Flags().IntVar(&maxLineBytes, "max-line-bytes", 0, "Max bytes per JSON line")

	// Smoke test command flags (inherits persistent flags)
	smokeTestCmd.Flags().StringVar(&listenAddr, "listen", "", "TCP listen address (e.g., :9015)")
	smokeTestCmd.Flags().StringVar(&tlsCertFile, "tls-cert", "", "TLS cert file (optional)")
	smokeTestCmd.Flags().StringVar(&tlsKeyFile, "tls-key", "", "TLS key file (optional)")
	smokeTestCmd.Flags().StringVar(&outDir, "out", "", "Directory to persist NDJSON")
	smokeTestCmd.Flags().StringVar(&hecURL, "hec-url", "", "Splunk HEC raw endpoint")
	smokeTestCmd.Flags().StringVar(&hecToken, "hec-token", "", "Splunk HEC token")
	smokeTestCmd.Flags().StringVar(&hecSourcetype, "hec-sourcetype", "", "Splunk sourcetype")
	smokeTestCmd.Flags().StringVar(&allowedCIDRs, "allow-cidrs", "", "Comma-separated CIDRs allowed to connect")
	smokeTestCmd.Flags().BoolVar(&gzipHEC, "hec-gzip", false, "Gzip compress payloads to HEC")
	smokeTestCmd.Flags().IntVar(&maxLineBytes, "max-line-bytes", 0, "Max bytes per JSON line")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runRelay(cmd *cobra.Command, args []string) {
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
