package main

func init() {
	// Add subcommands
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(smokeTestCmd)

	// Root command flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "f", "", "Path to configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&metricsAddr, "metrics-addr", ":9017", "Metrics server address (empty to disable)")
}
