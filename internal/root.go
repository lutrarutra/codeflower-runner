package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
    Use:   "codeflower",
    Short: "CLI runner for Code Flower",
	Version: "0.1.0",
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}