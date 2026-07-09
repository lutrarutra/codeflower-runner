package cli

import (
	"fmt"
	"os"
	"strings"

	"codeflower-runner/internal/core/client"
	openapi "codeflower-runner/internal/core/client"
	"codeflower-runner/internal/core/storage"

	"github.com/spf13/cobra"
)

// resolveImageRef looks up the full image digest for a tool ref from the API.
func resolveImageRef(cmd *cobra.Command, ref string) (string, error) {
	sm, err := storage.NewManager()
	if err != nil {
		return "", fmt.Errorf("initializing storage: %w", err)
	}
	token, err := sm.GetAPIToken()
	if err != nil {
		return "", fmt.Errorf("retrieving API token: %w", err)
	}

	clientResp, err := client.GetAPIClient(token)
	if err != nil {
		return "", fmt.Errorf("creating API client: %w", err)
	}

	params := &openapi.PullImageApiRegistryPullGetParams{Ref: &ref}
	resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(cmd.Context(), params)
	if err != nil {
		return "", fmt.Errorf("contacting server: %w", err)
	}
	if resp.JSON200 == nil {
		status := "<nil>"
		body := ""
		if resp.HTTPResponse != nil {
			status = resp.HTTPResponse.Status
		}
		if len(resp.Body) > 0 {
			body = string(resp.Body)
		}
		if body != "" {
			return "", fmt.Errorf("server returned %s: %s", status, body)
		}
		return "", fmt.Errorf("server returned %s", status)
	}
	return resp.JSON200.ImageRef, nil
}

// extractRefFromArgs finds the first non-flag argument after the given subcommand in os.Args.
func extractRefFromArgs(subcmd string) string {
	for i, a := range os.Args {
		if a == subcmd && i+1 < len(os.Args) {
			candidate := os.Args[i+1]
			if !strings.HasPrefix(candidate, "-") && candidate != "help" {
				return candidate
			}
			break
		}
	}
	return ""
}

// printToolHelp fetches tool info from the API and prints it to stderr.
func printToolHelp(cmd *cobra.Command, ref string) {
	sm, err := storage.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing storage: %v\n", err)
		os.Exit(1)
	}
	token, err := sm.GetAPIToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error retrieving API token: %v\n", err)
		os.Exit(1)
	}
	clientResp, err := client.GetAPIClient(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	params := &openapi.PullImageApiRegistryPullGetParams{Ref: &ref}
	resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(cmd.Context(), params)
	if err != nil || resp.JSON200 == nil {
		fmt.Fprintf(os.Stderr, "Error getting info for %q\n", ref)
		os.Exit(1)
	}

	spec := resp.JSON200.ReleaseSpec
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  codeflower %s %s -- [docker-flags...] -- [args...]\n\n", cmd.Name(), ref)
	fmt.Fprintf(os.Stderr, "Available commands:\n")
	for _, c := range spec.Commands {
		desc := ""
		if c.Description != nil {
			desc = *c.Description
		}
		fmt.Fprintf(os.Stderr, "  %-20s %s\n", c.Name, desc)
		fmt.Fprintf(os.Stderr, "    Variants:\n")
		for _, v := range c.Variants {
			fmt.Fprintf(os.Stderr, "      %-16s invoke: %s\n", v.Name, v.InvokeCmd)
			if len(v.Parameters) > 0 {
				fmt.Fprintf(os.Stderr, "      Parameters:\n")
				for _, p := range v.Parameters {
					req := "optional"
					if p.IsRequired {
						req = "required"
					}
					prefix := ""
					if p.Prefix != nil {
						prefix = *p.Prefix + " "
					}
					d := ""
					if p.Description != nil {
						d = *p.Description
					}
					fmt.Fprintf(os.Stderr, "        %-12s %s%-20s [%s]  %s\n", p.Type, prefix, p.Name, req, d)
				}
			}
		}
	}
}