package cli

import (
	"codeflower-runner/internal/core/storage"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	sm       *storage.Manager
	apiToken string
	Version  = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "codeflower",
	Short:   "CLI runner for Code Flower",
	Version: Version,
}

func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		sm, err = storage.NewManager()
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		apiToken, _ = sm.GetAPIToken()
		return nil
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
