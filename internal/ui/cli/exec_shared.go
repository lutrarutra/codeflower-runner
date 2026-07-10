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

// printToolReleaseHelp prints a formatted help page for a tool release spec to stderr.
// The format matches compile_usage_string from the backend's cwl.py.
func printToolReleaseHelp(spec openapi.ToolReleaseSpecSchema) {
	for _, c := range spec.Commands {
		cmdDesc := ""
		if c.Description != nil {
			cmdDesc = *c.Description
		}
		fmt.Fprintf(os.Stderr, "Command: %s", c.Name)
		if cmdDesc != "" {
			fmt.Fprintf(os.Stderr, "  %s", cmdDesc)
		}
		fmt.Fprintf(os.Stderr, "\n")

		for _, v := range c.Variants {
			type paramRow struct {
				leftStr string
				desc    string
			}
			var positionals, options []paramRow
			maxLeft := 0
			for _, p := range v.Parameters {
				leftStr := ""
				if p.Prefix != nil {
					leftStr += *p.Prefix
					if p.Separate {
						leftStr += " "
					}
				}
				leftStr += "{" + p.Name + "}"
				if len(leftStr) > maxLeft {
					maxLeft = len(leftStr)
				}

				desc := ""
				if p.Description != nil {
					desc = *p.Description
				} else {
					desc = "No description provided."
				}

				constraints := []string{}
				if p.IsRequired {
					constraints = append(constraints, "Required")
				}
				constraints = append(constraints, fmt.Sprintf("Type: %s", p.Type))
				if constraints != nil {
					desc += fmt.Sprintf(" (%s)", strings.Join(constraints, ", "))
				}

				row := paramRow{leftStr: leftStr, desc: desc}
				if p.Prefix != nil {
					options = append(options, row)
				} else {
					positionals = append(positionals, row)
				}
			}

			leftColWidth := maxLeft + 4
			if leftColWidth > 40 {
				leftColWidth = 40
			}

			fmt.Fprintf(os.Stderr, "\n")
			if v.Name == "default" {
				fmt.Fprintf(os.Stderr, "  Usage: %s", v.InvokeCmd)
			} else {
				fmt.Fprintf(os.Stderr, "  Variant: %s  invoke: %s", v.Name, v.InvokeCmd)
			}
			if len(options) > 0 {
				fmt.Fprintf(os.Stderr, " [OPTIONS]")
			}
			for _, p := range v.Parameters {
				if p.Prefix == nil {
					val := "{" + p.Name + "}"
					if !p.IsRequired {
						val = "[" + val + "]"
					}
					fmt.Fprintf(os.Stderr, " %s", val)
				}
			}
			fmt.Fprintf(os.Stderr, "\n")

			if len(positionals) > 0 {
				fmt.Fprintf(os.Stderr, "\n")
				fmt.Fprintf(os.Stderr, "    Positional Arguments:\n")
				for _, row := range positionals {
					fmt.Fprintf(os.Stderr, "      %s%s%s\n", row.leftStr, strings.Repeat(" ", leftColWidth-len(row.leftStr)), row.desc)
				}
			}

			if len(options) > 0 {
				fmt.Fprintf(os.Stderr, "\n")
				fmt.Fprintf(os.Stderr, "    Options:\n")
				for _, row := range options {
					fmt.Fprintf(os.Stderr, "      %s%s%s\n", row.leftStr, strings.Repeat(" ", leftColWidth-len(row.leftStr)), row.desc)
				}
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
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

	printToolReleaseHelp(resp.JSON200.ReleaseSpec)
}