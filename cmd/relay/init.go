package main

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
	rootCmd.Flags().BoolVar(&healthCheckEnabled, "health-check-enabled", false, "Enable healthcheck server")
	rootCmd.Flags().StringVar(&healthCheckAddr, "health-check-addr", "", "Healthcheck listen address")

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
	smokeTestCmd.Flags().BoolVar(&healthCheckEnabled, "health-check-enabled", false, "Enable healthcheck server")
	smokeTestCmd.Flags().StringVar(&healthCheckAddr, "health-check-addr", "", "Healthcheck listen address")
}
