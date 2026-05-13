package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var Version = "dev" 

var rootCmd = &cobra.Command{
    Use:   "codeflower",
    Short: "CLI runner for Code Flower",
	Version: Version,
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}