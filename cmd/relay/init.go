package main

func init() {
	// Add subcommands
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(smokeTestCmd)

	// Root command flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "f", "", "Path to configuration file (required)")
	if err := rootCmd.MarkPersistentFlagRequired("config"); err != nil {
		panic("failed to mark config flag as required: " + err.Error())
	}
}
