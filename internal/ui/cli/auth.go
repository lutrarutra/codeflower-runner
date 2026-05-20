package cli

import (
	"context"
	"fmt"

	"codeflower-runner/internal/core/client"
	openapi "codeflower-runner/internal/core/client"
	"codeflower-runner/internal/core/storage"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(authCmd)
}

func getAuthStatus(token string) (string, error) {
	client, err := client.GetAPIClient(token)
	if err != nil {
		return "", err
	}

	resp, err := client.ApiTokenAuthenticateApiAuthApiTokenMeGetWithResponse(context.Background(), &openapi.ApiTokenAuthenticateApiAuthApiTokenMeGetParams{})
	if err != nil {
		return "", fmt.Errorf("error connecting to backend: %v", err)
	}

	switch {
	case resp.JSON200 != nil:
		return fmt.Sprintf("Successfully authenticated as %s", resp.JSON200.Username), nil
	case resp.JSON422 != nil:
		return "", fmt.Errorf("validation error: %s", resp.JSON422.Detail)
	case resp.StatusCode() == 409:
		return "", fmt.Errorf("Authentication failed: %s", resp.Status())
	default:
		return "", fmt.Errorf("backend returned unexpected error: %s", resp.Status())
	}
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Verify authentication status with the backend",
	Run: func(cmd *cobra.Command, args []string) {
		sm, err := storage.NewManager()
		if err != nil {
			fmt.Printf("Error initializing storage: %v\n", err)
			return
		}

		token, err := sm.GetAPIToken()
		if err != nil {
			fmt.Printf("Error retrieving API token: %v\n", err)
			return
		}

		if status, err := getAuthStatus(token); err != nil {
			fmt.Printf("Authentication failed: %v\n", err)
		} else {
			fmt.Println(status)
		}
	},
}
