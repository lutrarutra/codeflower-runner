package cmd

import (
	"fmt"
	"os"

	openapi "codeflower-runner/internal/backend"
	"codeflower-runner/internal/engine"

	"github.com/spf13/cobra"
)

var (
    pullRef       string
    pullToolID    string
    pullReleaseID string
)

func init() {
    pullCmd.Flags().StringVarP(&pullRef, "ref", "r", "", "Image ref to pull (e.g. 'tool:v1')")
    pullCmd.Flags().StringVarP(&pullToolID, "tool-id", "t", "", "Tool UUID")
    pullCmd.Flags().StringVarP(&pullReleaseID, "release-id", "s", "", "Release UUID")
    rootCmd.AddCommand(pullCmd)
}

var pullCmd = &cobra.Command{
    Use:   "pull",
    Short: "Pull the docker image for a tool release",
    Long: `Pull an image using named flags. 
Examples:
  pull --ref fastq2ubam:latest
  pull --tool-id <uuid> --release-id <uuid>`,
    Run: func(cmd *cobra.Command, args []string) {
        clientResp, err := GetAPIClient()
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            os.Exit(1)
        }

        if pullRef == "" && pullToolID == "" && pullReleaseID == "" {
            fmt.Println("Error: at least one of --ref, --tool-id, or --release-id must be provided")
            _ = cmd.Usage()
            os.Exit(1)
        }

        params := &openapi.PullImageApiRegistryPullGetParams{}
        if pullRef != "" {
            params.Ref = &pullRef
        }
        if pullToolID != "" {
            params.ToolId = &pullToolID
        }
        if pullReleaseID != "" {
            params.ReleaseId = &pullReleaseID
        }

        resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(
            cmd.Context(),
            params,
        )

        if err != nil {
            fmt.Printf("Error connecting to backend: %v\n", err)
            os.Exit(1)
        }

        if resp.JSON200 == nil {
            fmt.Printf("Error from backend: %s\n", resp.Status())
            if resp.JSON404 != nil {
                fmt.Printf("Detail: %v\n", resp.JSON404.Detail)
            } else if resp.JSON422 != nil {
                fmt.Printf("Validation Error: %v\n", resp.JSON422.Detail)
            }
            os.Exit(1)
        }

        imageRef := resp.JSON200.ImageRef
        fmt.Printf("Pulling image: %s\n", imageRef)

        var containerEngine engine.ContainerEngine
        containerEngine, err = engine.NewDockerEngine()
        if err != nil {
            fmt.Println(err)
            os.Exit(1)
        }

        err = containerEngine.PullImage(cmd.Context(), imageRef)
        if err != nil {
            fmt.Println(err)
            os.Exit(1)
        }

        fmt.Printf("\nSuccessfully pulled image: %s\n", imageRef)
    },
}