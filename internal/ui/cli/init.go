package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the application",
	Run: func(cmd *cobra.Command, args []string) {
		if apiToken != "" {
			fmt.Println("API token is already set. To replace it, run: codeflower login")
			return
		}

		fmt.Print("Enter your API token: ")
		var token string
		fmt.Scanln(&token)

		if token == "" {
			fmt.Println("Error: API token cannot be empty")
			return
		}

		if status, err := getAuthStatus(token); err != nil {
			fmt.Printf("Authentication failed: %v\n", err)
			return
		} else {
			fmt.Println(status)
		}

		if err := sm.SaveAPIToken(token); err != nil {
			fmt.Printf("Error saving token: %v\n", err)
			return
		}

		fmt.Println("Initialization successful! Your token is saved in your config directory.")
	},
}
