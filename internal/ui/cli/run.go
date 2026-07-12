package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeflower-runner/internal/core/client"
	openapi "codeflower-runner/internal/core/client"
	"codeflower-runner/internal/core/engine"
	"codeflower-runner/internal/core/storage"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	runRef string
	force  bool
	noPull bool
)

func init() {
	runCmd.Flags().BoolVarP(&force, "force", "f", false, "Ignore validation errors and force execution")
	runCmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip pulling the image, use local copy only")
	runCmd.Flags().SetInterspersed(false)
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting info for %q: %v\n", ref, err)
		os.Exit(1)
	}
	if resp.JSON200 == nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", pullImageError(ref, resp))
		os.Exit(1)
	}

	printToolReleaseHelp(resp.JSON200.ReleaseSpec)
}

var runCmd = &cobra.Command{
	Use:   "run [flags] [ref] [command] [variant] [args...]",
	Short: "Pull (if needed) and run a tool release",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runRef, args = splitRunArgs(args)
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
				fmt.Fprintf(os.Stderr, "Error: %v\n", pullImageError(runRef, resp))
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
		imageArgs := args

		if len(imageArgs) > 0 && !strings.HasPrefix(imageArgs[0], "-") {
			cmdName := imageArgs[0]
			for index := range releaseSpec.Commands {
				c := &releaseSpec.Commands[index]
				if c.Name != nil && strings.EqualFold(*c.Name, cmdName) {
					selectedCommand = c
					imageArgs = imageArgs[1:]
					break
				}
			}
		}
		if selectedCommand == nil {
			for index := range releaseSpec.Commands {
				if releaseSpec.Commands[index].Name == nil {
					selectedCommand = &releaseSpec.Commands[index]
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

		imageArgs = trimSeparator(imageArgs)
		selectedVariant, err := inferVariant(selectedCommand, imageArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Tool argument error: %v\n", err)
			os.Exit(1)
		}

		entrypoint := []string{toolBinaryPath(runRef)}
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

		variantStr := "@" + selectedVariant.Name

		fmt.Fprintf(os.Stderr, "Executing: %s:%s %s%s\n", runRef, versionStr, commandDisplayName(selectedCommand.Name), variantStr)
		containerArgs := append(append([]string{}, selectedCommand.InvokeArgs...), validatedArgs...)
		err = containerEngine.RunContainer(cmd.Context(), imageRef, entrypoint, containerArgs, binds)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running container: %v\n", err)
			os.Exit(1)
		}
	},
}

func splitRunArgs(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}

	return args[0], trimSeparator(args[1:])
}

func trimSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

func getCommandNames(cmds []openapi.CommandSpecSchema) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		if c.Name == nil {
			names[i] = "<root>"
		} else {
			names[i] = *c.Name
		}
	}
	return names
}

func inferVariant(command *openapi.CommandSpecSchema, args []string) (*openapi.CommandVariantSpecSchema, error) {
	var matches []*openapi.CommandVariantSpecSchema
	for index := range command.Variants {
		variant := &command.Variants[index]
		if validateErr := validateToolArgs(variant, args); validateErr == nil {
			matches = append(matches, variant)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("arguments do not match any variant")
	default:
		return nil, fmt.Errorf("arguments match multiple variants; make the parameter schemas distinct")
	}
}

func toolBinaryPath(ref string) string {
	toolRef := ref
	if separator := strings.LastIndex(toolRef, "/"); separator >= 0 {
		toolRef = toolRef[separator+1:]
	}
	if tagSeparator := strings.LastIndex(toolRef, ":"); tagSeparator >= 0 {
		toolRef = toolRef[:tagSeparator]
	}
	toolRef = strings.ToLower(strings.ReplaceAll(toolRef, " ", "-"))
	return filepath.Join("/app", toolRef)
}

func validateToolArgs(variant *openapi.CommandVariantSpecSchema, args []string) error {
	toolFlags := pflag.NewFlagSet(variant.Name, pflag.ContinueOnError)
	toolFlags.SetInterspersed(true)

	var positionalParams []openapi.ParamSpecSchema
	for _, param := range variant.Parameters {
		if param.Prefix == nil {
			positionalParams = append(positionalParams, param)
			continue
		}

		name := strings.TrimLeft(*param.Prefix, "-")
		help := param.Name
		if param.Description != nil {
			help = *param.Description
		}

		switch param.Type {
		case openapi.ParameterTypeBoolean:
			toolFlags.Bool(name, false, help)
		case openapi.ParameterTypeNumeric:
			toolFlags.Float64(name, 0, help)
		case openapi.ParameterTypePath, openapi.ParameterTypeString:
			if param.IsArray {
				toolFlags.StringSlice(name, nil, help)
			} else {
				toolFlags.String(name, "", help)
			}
		default:
			return fmt.Errorf("unsupported parameter type %q for %s", param.Type, param.Name)
		}
	}

	if err := toolFlags.Parse(trimSeparator(args)); err != nil {
		return err
	}

	sort.SliceStable(positionalParams, func(left, right int) bool {
		leftPosition, rightPosition := 0, 0
		if positionalParams[left].Position != nil {
			leftPosition = *positionalParams[left].Position
		}
		if positionalParams[right].Position != nil {
			rightPosition = *positionalParams[right].Position
		}
		return leftPosition < rightPosition
	})
	positionalArgs := toolFlags.Args()
	if len(positionalArgs) > len(positionalParams) {
		return fmt.Errorf("unexpected positional arguments: %v", positionalArgs[len(positionalParams):])
	}
	for index, param := range positionalParams {
		if param.IsRequired && index >= len(positionalArgs) {
			return fmt.Errorf("missing required parameter: %s", param.Name)
		}
	}

	for _, param := range variant.Parameters {
		if param.IsRequired && param.Prefix != nil && !toolFlags.Changed(strings.TrimLeft(*param.Prefix, "-")) {
			return fmt.Errorf("missing required parameter: %s", param.Name)
		}
	}

	return nil
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
				if param.Type == openapi.ParameterTypePath {
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
