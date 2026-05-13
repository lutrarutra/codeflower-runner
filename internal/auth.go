package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"

	openapi "codeflower-runner/internal/backend"

	"github.com/spf13/cobra"
)

func GetAPIClient() (*openapi.ClientWithResponses, error) {
    token := os.Getenv("CODEFLOWER_API_TOKEN")
    if token == "" {
        return nil, fmt.Errorf("CODEFLOWER_API_TOKEN environment variable is not set")
    }

    // RequestEditorFn to inject the API token into every request
    addToken := func(ctx context.Context, req *http.Request) error {
        req.Header.Set("X-API-Token", token)
        return nil
    }

    return openapi.NewClientWithResponses("http://localhost:8000", openapi.WithRequestEditorFn(addToken))
}

func init() {
    rootCmd.AddCommand(authCmd)
}

var authCmd = &cobra.Command{
    Use:   "auth",
    Short: "Verify authentication status using CODEFLOWER_API_TOKEN",
    Run: func(cmd *cobra.Command, args []string) {
        client, err := GetAPIClient()
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            os.Exit(1)
        }

        resp, err := client.ApiTokenAuthenticateApiAuthApiTokenMeGetWithResponse(cmd.Context(), &openapi.ApiTokenAuthenticateApiAuthApiTokenMeGetParams{})
        if err != nil {
            fmt.Printf("Error connecting to backend: %v\n", err)
            os.Exit(1)
        }

        switch {
        case resp.JSON200 != nil:
            fmt.Printf("Successfully authenticated as %s\n", resp.JSON200.Username)
        case resp.JSON422 != nil:
            fmt.Printf("Validation error: %s\n", resp.JSON422.Detail)
        default:
            fmt.Printf("Backend returned unexpected error: %s\n", resp.Status())
            os.Exit(1)
        }
    },
}