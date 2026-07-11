package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"codeflower-runner/internal/core/compiler"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	inspectAPIKey  string
	inspectAPIURL  string
	inspectModel   string
	inspectOutput  string
	inspectHeaders []string
	allowInternet  bool
)

func init() {
	inspectCmd.Flags().StringVar(&inspectAPIKey, "api-key", "", "LLM API key (or set LLM_API_KEY env var)")
	inspectCmd.Flags().StringVar(&inspectAPIURL, "api-url", "", "OpenAI-compatible chat completions endpoint URL (or set LLM_API_URL env var)")
	inspectCmd.Flags().StringVar(&inspectModel, "model", "deepseek/deepseek-v4-flash", "LLM model name")
	inspectCmd.Flags().StringSliceVar(&inspectHeaders, "header", nil, "Extra HTTP headers for the API call (key:value format, can be repeated)")
	inspectCmd.Flags().StringVarP(&inspectOutput, "output", "o", "", "Output file path (default: stdout)")
	inspectCmd.Flags().BoolVar(&allowInternet, "allow-internet", false, "Allow the analyzed tool to access the internet (unsafe)")
	rootCmd.AddCommand(inspectCmd)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <binary-path>",
	Short: "Reverse-engineer a CLI binary into a structured YAML spec",
	Long: `Analyze a command-line tool binary by running it in a sandboxed Docker container,
using --help to discover subcommands and parameters, then sending the output to an LLM
(any OpenAI-compatible API) to produce a structured YAML specification.

The output YAML describes all subcommands with their parameters, types, and descriptions.
This can be used to auto-generate ToolCommand entries in CodeFlower.

Example:
	codeflower inspect /usr/bin/samtools --api-url https://api.openai.com/v1/chat/completions --api-key sk-xxx
	codeflower inspect ./my-tool --api-url https://openrouter.ai/api/v1/chat/completions --api-key sk-or-xxx -o my-tool.yaml

Environment:
  LLM_API_URL    Default API endpoint (alternative to --api-url)
	LLM_API_KEY    API key (alternative to --api-key)

Security:
	The analyzed tool runs as a non-root user in a read-only container with no host
	mounts and no network access by default. --allow-internet enables container
	networking and prints a warning to stderr.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		binaryPath := args[0]

		// Resolve API key: flag > env var
		apiKey := inspectAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("LLM_API_KEY")
		}
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "Error: API key required. Set --api-key or LLM_API_KEY env var.")
			os.Exit(1)
		}

		// Resolve API URL: flag > env var
		apiURL := inspectAPIURL
		if apiURL == "" {
			apiURL = os.Getenv("LLM_API_URL")
		}
		if apiURL == "" {
			fmt.Fprintln(os.Stderr, "Error: API URL required. Set --api-url or LLM_API_URL env var.")
			os.Exit(1)
		}
		if allowInternet {
			fmt.Fprintln(os.Stderr, "Warning: --allow-internet enables network access inside the analyzed tool container.")
		}

		// Parse extra headers
		headers := make(map[string]string)
		for _, h := range inspectHeaders {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "Warning: invalid header format '%s', expected key:value\n", h)
				continue
			}
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}

		// Auto-add OpenRouter platform header (harmless for non-OpenRouter endpoints)
		if _, ok := headers["HTTP-Referer"]; !ok {
			headers["HTTP-Referer"] = "codeflower.app"
		}
		if _, ok := headers["X-OpenRouter-Title"]; !ok {
			headers["X-OpenRouter-Title"] = "Code Flower CLI"
		}

		// Verify binary exists
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: binary not found: %s\n", binaryPath)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "Inspecting %s (model: %s, endpoint: %s)...\n", binaryPath, inspectModel, apiURL)

		ctx := context.Background()
		result, err := compiler.Inspect(ctx, binaryPath, apiURL, apiKey, inspectModel, headers, allowInternet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Marshal to YAML
		output, err := yaml.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to marshal YAML: %v\n", err)
			os.Exit(1)
		}

		if inspectOutput != "" {
			if err := os.WriteFile(inspectOutput, output, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to write output file: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Output written to %s\n", inspectOutput)
		} else {
			fmt.Print(string(output))
		}
	},
}
