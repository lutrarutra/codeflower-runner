package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/moby/term"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(loginCmd)
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Prompt for an API token and overwrite the stored one",
	Run: func(cmd *cobra.Command, args []string) {
		token, err := promptToken()
		if err != nil {
			fmt.Printf("Error reading token: %v\n", err)
			return
		}

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

		fmt.Println("Login successful! Your token has been updated.")
	},
}

// promptToken reads the API token from stdin, hiding input when attached to a
// terminal and falling back to a plain line read otherwise (e.g. piped input).
func promptToken() (string, error) {
	fmt.Print("Enter your API token: ")

	isTerm := term.IsTerminal(os.Stdin.Fd())
	if isTerm {
		if state, err := term.SaveState(os.Stdin.Fd()); err == nil {
			_ = term.DisableEcho(os.Stdin.Fd(), state)
			defer term.RestoreTerminal(os.Stdin.Fd(), state)
		} else {
			isTerm = false
		}
	}

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	token := strings.TrimRight(line, "\r\n")

	if isTerm {
		fmt.Println() // move to a new line after hidden input
	}

	return token, nil
}
