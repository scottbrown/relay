package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/config"
	"github.com/scottbrown/relay/internal/forwarder"
	"github.com/scottbrown/relay/internal/server"
	"github.com/scottbrown/relay/internal/storage"
)

var (
	configFile    = flag.String("f", "", "Path to configuration file")
	templateFlag  = flag.Bool("t", false, "Output configuration template and exit")
	smokeTestFlag = flag.Bool("smoke-test", false, "Test Splunk HEC connectivity and exit")
	listenAddr    = flag.String("listen", "", "TCP listen address (e.g., :9015)")
	tlsCertFile   = flag.String("tls-cert", "", "TLS cert file (optional)")
	tlsKeyFile    = flag.String("tls-key", "", "TLS key file (optional)")
	outDir        = flag.String("out", "", "Directory to persist NDJSON")
	hecURL        = flag.String("hec-url", "", "Splunk HEC raw endpoint")
	hecToken      = flag.String("hec-token", "", "Splunk HEC token")
	hecSourcetype = flag.String("hec-sourcetype", "", "Splunk sourcetype")
	allowedCIDRs  = flag.String("allow-cidrs", "", "Comma-separated CIDRs allowed to connect")
	gzipHEC       = flag.Bool("hec-gzip", false, "Gzip compress payloads to HEC (use with explicit flag)")
	maxLineBytes  = flag.Int("max-line-bytes", 0, "Max bytes per JSON line")
)

func main() {
	flag.Parse()

	// Handle template output
	if *templateFlag {
		fmt.Print(config.GetTemplate())
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Override config with CLI flags if provided (non-empty string flags or explicitly set flags)
	if *listenAddr != "" {
		cfg.ListenAddr = *listenAddr
	}
	if *tlsCertFile != "" {
		cfg.TLSCertFile = *tlsCertFile
	}
	if *tlsKeyFile != "" {
		cfg.TLSKeyFile = *tlsKeyFile
	}
	if *outDir != "" {
		cfg.OutputDir = *outDir
	}
	if *hecURL != "" {
		cfg.SplunkHECURL = *hecURL
	}
	if *hecToken != "" {
		cfg.SplunkToken = *hecToken
	}
	if *hecSourcetype != "" {
		cfg.SourceType = *hecSourcetype
	}
	if *allowedCIDRs != "" {
		cfg.AllowedCIDRs = *allowedCIDRs
	}
	// For bool flags, check if they were explicitly set by examining flag.Args
	visited := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	if visited["hec-gzip"] {
		cfg.GzipHEC = *gzipHEC
	}
	if visited["max-line-bytes"] {
		cfg.MaxLineBytes = *maxLineBytes
	}

	// Handle smoke test
	if *smokeTestFlag {
		performSmokeTest(cfg)
		os.Exit(0)
	}

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
