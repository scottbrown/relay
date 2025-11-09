package main

func init() {
	// Add subcommands
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(smokeTestCmd)

	// Root command flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "f", "", "Path to configuration file (required)")
	rootCmd.MarkPersistentFlagRequired("config")
}
