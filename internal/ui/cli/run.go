package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeflower-runner/internal/core/client"
	openapi "codeflower-runner/internal/core/client"
	"codeflower-runner/internal/core/engine"
	"codeflower-runner/internal/core/storage"

	"github.com/spf13/cobra"
)

var (
	runRef string
	force  bool
	noPull bool
)

func init() {
	runCmd.Flags().BoolVarP(&force, "force", "f", false, "Ignore validation errors and force execution")
	runCmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip pulling the image, use local copy only")
	rootCmd.AddCommand(runCmd)
	runCmd.SetHelpFunc(runHelpFunc)
}

func runHelpFunc(cmd *cobra.Command, args []string) {
	// Parse os.Args to find the ref — it's the first non-flag arg after "run"
	var ref string
	for i, a := range os.Args {
		if a == "run" && i+1 < len(os.Args) {
			candidate := os.Args[i+1]
			if !strings.HasPrefix(candidate, "-") && candidate != "help" {
				ref = candidate
			}
			break
		}
	}
	if ref == "" {
		// No ref — show generic run help
		cmd.Root().HelpFunc()(cmd, args)
		return
	}

	// Fetch release spec from the API to show tool info
	if apiToken == "" {
		fmt.Fprintf(os.Stderr, "Error: not authenticated. Run 'codeflower init' first.\n")
		os.Exit(1)
	}

	clientResp, err := client.GetAPIClient(apiToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	params := &openapi.PullImageApiRegistryPullGetParams{Ref: &ref}
	resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(cmd.Context(), params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting info for %q: %v\n", ref, err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Error getting info for %q: %s\n", ref, status)
		if body != "" {
			fmt.Fprintf(os.Stderr, "  Body: %s\n", body)
		}
		os.Exit(1)
	}

	spec := resp.JSON200.ReleaseSpec

	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s [command] [variant] -- [args...]\n\n", ref)
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  -f, --force   Ignore validation errors and force execution\n")
	fmt.Fprintf(os.Stderr, "      --no-pull Skip pulling the image, use local copy only\n\n")
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
					desc = ""
					if p.Description != nil {
						desc = *p.Description
					}
					fmt.Fprintf(os.Stderr, "        %-12s %s%-20s [%s]  %s\n", p.Type, prefix, p.Name, req, desc)
				}
			}
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	fmt.Fprintf(os.Stderr, "Examples:\n")
	// Show examples for each command's default variant
	for _, c := range spec.Commands {
		for _, v := range c.Variants {
			if v.Name == "default" || len(c.Variants) == 1 {
				args := ""
				for _, p := range v.Parameters {
					if p.IsRequired {
						prefix := ""
						if p.Prefix != nil {
							prefix = *p.Prefix + " "
						}
						args += prefix + "<" + p.Name + "> "
					}
				}
				variant := ""
				if v.Name != "default" {
					variant = " " + v.Name
				}
				fmt.Fprintf(os.Stderr, "  %s %s%s -- %s\n", ref, c.Name, variant, strings.TrimSpace(args))
				break
			}
		}
	}
	fmt.Fprintf(os.Stderr, "\n")
}

var runCmd = &cobra.Command{
	Use:   "run [ref] [command] [variant] -- [args...]",
	Short: "Pull (if needed) and run a tool release",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runRef = args[0]
		var pullResp *openapi.RegistryImagePullResponse

		sm, err := storage.NewManager()
		if err != nil {
			fmt.Printf("Error initializing storage: %v\n", err)
			os.Exit(1)
		}

		token, err := sm.GetAPIToken()
		if err != nil {
			fmt.Printf("Error retrieving API token: %v\n", err)
			os.Exit(1)
		}

		if err := sm.GetCache(runRef, pullResp, 5*time.Minute); err == nil && !force {
			fmt.Fprintf(os.Stderr, "Using cached image ref for %s: %s\n", runRef, pullResp.ImageRef)
		} else {
			clientResp, err := client.GetAPIClient(token)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			params := &openapi.PullImageApiRegistryPullGetParams{Ref: &runRef}
			resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(cmd.Context(), params)

			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting image ref from server: %v\n", err)
				os.Exit(1)
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
				fmt.Fprintf(os.Stderr, "Error getting image ref from server: %s\n", status)
				if body != "" {
					fmt.Fprintf(os.Stderr, "  Body: %s\n", body)
				}
				os.Exit(1)
			}

			pullResp = resp.JSON200
			if err := sm.SetCache(runRef, pullResp); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save cache: %v\n", err)
			}
		}

		imageRef := pullResp.ImageRef
		releaseSpec := pullResp.ReleaseSpec

		var selectedCommand *openapi.CommandSpecSchema
		var selectedVariant *openapi.CommandVariantSpecSchema
		var imageArgs []string

		if len(args) > 1 && !isSeparator(args[1]) {
			cmdName := args[1]
			for _, c := range releaseSpec.Commands {
				if c.Name == cmdName {
					selectedCommand = &c
					args = args[1:]
					break
				}
			}
		}
		if selectedCommand == nil {
			if len(releaseSpec.Commands) == 1 {
				selectedCommand = &releaseSpec.Commands[0]
			} else {
				fmt.Fprintf(os.Stderr, "Error: Multiple commands available. Please specify one: %v\n", getCommandNames(releaseSpec.Commands))
				os.Exit(1)
			}
		}

		if len(args) > 1 && !isSeparator(args[1]) {
			variantName := args[1]
			for _, v := range selectedCommand.Variants {
				if v.Name == variantName {
					selectedVariant = &v
					imageArgs = args[2:]
					break
				}
			}
		}

		if selectedVariant == nil {
			if len(selectedCommand.Variants) == 1 {
				selectedVariant = &selectedCommand.Variants[0]
				if len(args) > 1 && imageArgs == nil {
					imageArgs = args[1:]
				}
			} else {
				for _, v := range selectedCommand.Variants {
					if v.Name == "default" {
						selectedVariant = &v
						if len(args) > 1 && imageArgs == nil {
							imageArgs = args[1:]
						}
						fmt.Fprintf(os.Stderr, "Using default variant\n")
						break
					}
				}

				if selectedVariant == nil {
					fmt.Fprintf(os.Stderr, "Error: Multiple variants available for command %s and none are named 'default'. Please specify one.\n", selectedCommand.Name)
					os.Exit(1)
				}
			}
		}

		entrypoint := []string{filepath.Join("/app", selectedVariant.InvokeCmd)}
		binds, validatedArgs, err := validateAndResolveMounts(selectedVariant, imageArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Validation Error: %v\n", err)
			if !force {
				fmt.Fprintf(os.Stderr, "Use --force to ignore validation errors.\n")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Warning: Proceeding with --force despite validation errors.\n")
			validatedArgs = imageArgs
		}

		var containerEngine engine.ContainerEngine
		containerEngine, err = engine.NewDockerEngine()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		exists, _ := containerEngine.ImageExists(cmd.Context(), imageRef)
		if !exists {
			if noPull {
				fmt.Fprintf(os.Stderr, "Error: image %s not found locally and --no-pull is set\n", imageRef)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Image not found locally, pulling %s...\n", imageRef)
			if err := containerEngine.PullImage(cmd.Context(), imageRef); err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to pull image: %v\n", err)
				os.Exit(1)
			}
		}

		versionStr := releaseSpec.VersionTag
		if releaseSpec.IsLatest {
			versionStr = fmt.Sprintf("%s (latest)", releaseSpec.VersionTag)
		}

		variantStr := ""
		if selectedVariant.Name != "default" {
			variantStr = "@" + selectedVariant.Name
		}

		fmt.Fprintf(os.Stderr, "Executing: %s:%s %s%s\n", runRef, versionStr, selectedCommand.Name, variantStr)
		err = containerEngine.RunContainer(cmd.Context(), imageRef, entrypoint, validatedArgs, binds)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running container: %v\n", err)
			os.Exit(1)
		}
	},
}

func isSeparator(arg string) bool {
	return arg == "--"
}

func getCommandNames(cmds []openapi.CommandSpecSchema) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name
	}
	return names
}

func validateAndResolveMounts(variant *openapi.CommandVariantSpecSchema, args []string) ([]string, []string, error) {
	var binds []string
	var finalArgs []string

	paramMap := make(map[string]openapi.ParamSpecSchema)
	requiredParams := make(map[string]bool)

	for _, p := range variant.Parameters {
		if p.Prefix != nil {
			paramMap[*p.Prefix] = p
		}
		if p.IsRequired {
			requiredParams[p.Name] = true
		}
	}

	providedParams := make(map[string]bool)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		finalArgs = append(finalArgs, arg)

		param, isKnown := paramMap[arg]
		if isKnown {
			providedParams[param.Name] = true
			if i+1 < len(args) {
				val := args[i+1]
				if param.Type == openapi.Path {
					absPath, _ := filepath.Abs(val)
					subDir := "inputs"
					if _, err := os.Stat(absPath); os.IsNotExist(err) {
						isOutput := false
						if param.Prefix != nil && (len(*param.Prefix) >= 3 && (*param.Prefix)[:3] == "--o") {
							isOutput = true
						}
						if isOutput {
							subDir = "outputs"
							_ = os.MkdirAll(filepath.Dir(absPath), 0755)
						} else {
							return nil, nil, fmt.Errorf("input file does not exist: %s (parameter: %s)", val, param.Name)
						}
					}
					containerPath := filepath.Join("/cf", subDir, param.Name+filepath.Ext(val))
					binds = append(binds, fmt.Sprintf("%s:%s", absPath, containerPath))
					finalArgs = append(finalArgs, containerPath)
				} else {
					finalArgs = append(finalArgs, val)
				}
				i++
				continue
			}
		}
	}

	for name := range requiredParams {
		if !providedParams[name] {
			return nil, nil, fmt.Errorf("missing required parameter: %s", name)
		}
	}
	return binds, finalArgs, nil
}
