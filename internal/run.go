package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	openapi "codeflower-runner/internal/backend"
	"codeflower-runner/internal/engine"

	"github.com/spf13/cobra"
)

var (
    runRef string
    force  bool
)

func init() {
    runCmd.Flags().BoolVarP(&force, "force", "f", false, "Ignore validation errors and force execution")
    rootCmd.AddCommand(runCmd)
}

func getCachePath(ref string) string {
    cacheDir, err := os.UserCacheDir()
    if err != nil {
        cacheDir = os.TempDir()
    }
    safeName := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "*", "_").Replace(ref) + ".json"
    return filepath.Join(cacheDir, "codeflower", "specs", safeName)
}

func loadFromCache(ref string) (*openapi.RegistryImagePullResponse, error) {
    path := getCachePath(ref)
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    info, _ := os.Stat(path)
    if time.Since(info.ModTime()) > 24 * time.Second {
        return nil, fmt.Errorf("cache expired")
    }

    var resp openapi.RegistryImagePullResponse
    if err := json.Unmarshal(data, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}

func saveToCache(ref string, resp *openapi.RegistryImagePullResponse) {
    path := getCachePath(ref)
    _ = os.MkdirAll(filepath.Dir(path), 0755)
    data, _ := json.Marshal(resp)
    _ = os.WriteFile(path, data, 0644)
}

var runCmd = &cobra.Command{
    Use:   "run [ref] [command] [variant] -- [args...]",
    Short: "Pull (if needed) and run a tool release",
    Args:  cobra.MinimumNArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        runRef = args[0]
		var pullResp *openapi.RegistryImagePullResponse

		if cached, err := loadFromCache(runRef); err == nil && !force {
            pullResp = cached
        } else {
            clientResp, err := GetAPIClient()
            if err != nil {
                fmt.Fprintf(os.Stderr, "Error: %v\n", err)
                os.Exit(1)
            }

            params := &openapi.PullImageApiRegistryPullGetParams{Ref: &runRef}
            resp, err := clientResp.PullImageApiRegistryPullGetWithResponse(cmd.Context(), params)
            if err != nil || resp.JSON200 == nil {
                fmt.Fprintf(os.Stderr, "Error getting image ref from server: %v\n", err)
                os.Exit(1)
            }
            pullResp = resp.JSON200
            saveToCache(runRef, pullResp)
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
            fmt.Fprintf(os.Stderr, "Image not found locally, pulling %s...\n", imageRef)
            containerEngine.PullImage(cmd.Context(), imageRef)
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