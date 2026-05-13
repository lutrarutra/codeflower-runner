package cmd

import (
	"fmt"
	"os"

	openapi "codeflower-runner/internal/backend"
	"codeflower-runner/internal/engine"

	"github.com/spf13/cobra"
)

var (
    runRef string
)

func init() {
    runCmd.Flags().StringVarP(&runRef, "ref", "r", "", "Image ref to run (e.g. 'tool:v1')")
    rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
    Use:   "run",
    Short: "Pull (if needed) and run a tool release",
    Run: func(cmd *cobra.Command, args []string) {
        clientResp, err := GetAPIClient()
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            os.Exit(1)
        }

        params := &openapi.PullImageApiRegistryPullGetParams{Ref: &runRef}
        resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(cmd.Context(), params)
        if err != nil || resp.JSON200 == nil {
            fmt.Printf("Error getting image ref: %v\n", err)
            os.Exit(1)
        }

        imageRef := resp.JSON200.ImageRef
        
		var containerEngine engine.ContainerEngine
        containerEngine, err = engine.NewDockerEngine()
		if err != nil {
            fmt.Println(err)
            os.Exit(1)
        }
		
        exists, _ := containerEngine.ImageExists(cmd.Context(), imageRef)
        if !exists {
            fmt.Printf("Image not found locally, pulling %s...\n", imageRef)
            containerEngine.PullImage(cmd.Context(), imageRef)
        }

        fmt.Printf("Running %s...\n", imageRef)
        err = containerEngine.RunContainer(cmd.Context(), imageRef, nil, args)
        if err != nil {
            fmt.Printf("Error running container: %v\n", err)
            os.Exit(1)
        }
    },
}