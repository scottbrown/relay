package main

import (
	"fmt"

	"github.com/scottbrown/relay/internal/config"
	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Output configuration template",
	Long:  "Output a YAML configuration template and exit",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(config.GetTemplate())
	},
}
