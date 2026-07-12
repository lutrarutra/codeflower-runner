package cli

import (
	"fmt"
	"os"

	"codeflower-runner/internal/core/client"
	openapi "codeflower-runner/internal/core/client"
	"codeflower-runner/internal/core/engine"
	"codeflower-runner/internal/core/storage"

	"github.com/google/uuid"
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

		clientResp, err := client.GetAPIClient(token)
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
			toolID, err := uuid.Parse(pullToolID)
			if err != nil {
				fmt.Printf("Error: invalid tool UUID: %v\n", err)
				os.Exit(1)
			}
			params.ToolId = &toolID
		}
		if pullReleaseID != "" {
			releaseID, err := uuid.Parse(pullReleaseID)
			if err != nil {
				fmt.Printf("Error: invalid release UUID: %v\n", err)
				os.Exit(1)
			}
			params.ReleaseId = &releaseID
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
			ref := pullRef
			if ref == "" {
				ref = "tool/release ID"
			}
			fmt.Printf("Error: %v\n", pullImageError(ref, resp))
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
