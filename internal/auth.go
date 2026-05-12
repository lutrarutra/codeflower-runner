package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

func init() {
    rootCmd.AddCommand(authCmd)
}

var authCmd = &cobra.Command{
    Use:   "auth",
    Short: "Verify authentication status using CODEFLOWER_API_TOKEN",
    Run: func(cmd *cobra.Command, args []string) {
        token := os.Getenv("CODEFLOWER_API_TOKEN")
        if token == "" {
            fmt.Println("Error: CODEFLOWER_API_TOKEN environment variable is not set")
            os.Exit(1)
        }

        url := "http://localhost:8000/api/auth/api-token-me"
        req, err := http.NewRequest("GET", url, nil)
        if err != nil {
            fmt.Printf("Error creating request: %v\n", err)
            os.Exit(1)
        }

        req.Header.Set("X-API-Token", token)

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil {
            fmt.Printf("Error connecting to backend: %v\n", err)
            os.Exit(1)
        }
        defer resp.Body.Close()

        if resp.StatusCode == http.StatusUnauthorized {
            var errResp struct {
                Detail string `json:"detail"`
            }
            if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
                fmt.Printf("Authentication failed: %s\n", errResp.Detail)
            } else {
                fmt.Println("Authentication failed: Invalid credentials")
            }
            os.Exit(1)
        }

        if resp.StatusCode != http.StatusOK {
            fmt.Printf("Backend returned error: %s\n", resp.Status)
            os.Exit(1)
        }

        var authInfo struct {
            ID       string `json:"id"`
            Username string `json:"username"`
            Type     int    `json:"type"`
        }

        if err := json.NewDecoder(resp.Body).Decode(&authInfo); err != nil {
            fmt.Printf("Error decoding response: %v\n", err)
            os.Exit(1)
        }

        fmt.Printf("Successfully authenticated as %s (ID: %s)\n", authInfo.Username, authInfo.ID)
    },
}