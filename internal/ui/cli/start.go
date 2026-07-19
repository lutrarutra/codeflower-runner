package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"codeflower-runner/internal/core/client"
	openapi "codeflower-runner/internal/core/client"
	"codeflower-runner/internal/core/engine"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"
)

// --- Local response types for run API endpoints not in the generated client ---

type workflowRunResponse struct {
	Run         workflowRunSchema `json:"run"`
	AccessLevel int               `json:"access_level"`
}

type workflowRunSchema struct {
	ID              openapi_types.UUID    `json:"id"`
	Name            string                `json:"name"`
	Status          string                `json:"status"`
	WorkflowRelease workflowReleaseSchema `json:"workflow_release"`
}

type workflowReleaseSchema struct {
	ID         openapi_types.UUID `json:"id"`
	VersionTag string             `json:"version_tag"`
	Workflow   workflowSchema     `json:"workflow"`
}

type workflowSchema struct {
	ID   openapi_types.UUID `json:"id"`
	Name string             `json:"name"`
}

type runTasksResponse struct {
	Items []runTaskSummary `json:"items"`
}

type runTaskSummary struct {
	ID             openapi_types.UUID  `json:"id"`
	Name           *string             `json:"name"`
	Status         string              `json:"status"`
	WorkflowNodeID *openapi_types.UUID `json:"workflow_node_id"`
	Parameters     map[string]any      `json:"parameters"`
}

// toolExec describes a single tool to execute.
type toolExec struct {
	ImageRef   string
	Entrypoint []string
	Mounts     []string // docker volume binds
	CLIArgs    []string // resolved CLI arguments
	ToolName   string
}

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start <run_id>",
	Short: "Start a workflow run locally",
	Long: `Fetch a workflow run from the backend, interactively ask for
required file inputs, then execute every tool container locally.

Example:
  codeflower start 3b3c3b3c-3b3c-3b3c-3b3c-3b3c3b3c3b3c`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runID := args[0]

		if apiToken == "" {
			fmt.Println("Error: not authenticated. Run 'codeflower init' first.")
			os.Exit(1)
		}

		baseURL := strings.TrimRight(client.PUBLIC_BACKEND_URL, "/")
		doGET := func(path string, dest any) {
			url := baseURL + path
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, url, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
				os.Exit(1)
			}
			req.Header.Set("X-API-Token", apiToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error contacting backend: %v\n", err)
				os.Exit(1)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
				os.Exit(1)
			}
			if resp.StatusCode != 200 {
				fmt.Fprintf(os.Stderr, "Backend returned %s: %s\n", resp.Status, string(body))
				os.Exit(1)
			}
			if err := json.Unmarshal(body, dest); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
				os.Exit(1)
			}
		}

		// --- 1. Fetch run details ---
		fmt.Fprintf(os.Stderr, "Fetching run %s …\n", runID)
		var runResp workflowRunResponse
		doGET("/api/runs/"+runID, &runResp)
		run := runResp.Run
		fmt.Fprintf(os.Stderr, "Run: %s (status: %s)\n", run.Name, run.Status)

		// --- 2. Fetch workflow graph ---
		fmt.Fprintf(os.Stderr, "Fetching workflow graph …\n")
		apiClient, err := client.GetAPIClient(apiToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating API client: %v\n", err)
			os.Exit(1)
		}

		wfID := run.WorkflowRelease.Workflow.ID
		relID := run.WorkflowRelease.ID

		graphResp, err := apiClient.GetGraphApiWorkflowsWorkflowIdReleasesReleaseIdGraphGetWithResponse(
			cmd.Context(), wfID, relID, &openapi.GetGraphApiWorkflowsWorkflowIdReleasesReleaseIdGraphGetParams{},
		)
		if err != nil || graphResp.JSON200 == nil {
			fmt.Fprintf(os.Stderr, "Error fetching graph: %v\n", err)
			os.Exit(1)
		}
		graph := graphResp.JSON200

		// --- 3. Fetch run tasks (parameter overrides per node) ---
		fmt.Fprintf(os.Stderr, "Fetching run tasks …\n")
		var tasksResp runTasksResponse
		doGET("/api/runs/"+runID+"/tasks", &tasksResp)

		runParamsByNode := make(map[string]map[string]any)
		for _, t := range tasksResp.Items {
			if t.WorkflowNodeID != nil {
				nid := t.WorkflowNodeID.String()
				if t.Parameters != nil {
					runParamsByNode[nid] = t.Parameters
				} else {
					runParamsByNode[nid] = make(map[string]any)
				}
			}
		}

		// --- 4. Build tool execution list ---
		scanner := bufio.NewScanner(os.Stdin)
		var tools []toolExec
		var toolNodeIDs []string // node ID → index in tools
		var toolNameByID []string

		for _, rawItem := range graph.Nodes {
			node, err := rawItem.AsToolNodeSchema()
			if err != nil {
				continue // skip data nodes
			}

			variant := node.CommandVariant
			toolName := variant.Command.ToolRelease.Tool.Name

			fmt.Fprintf(os.Stderr, "\n=== Tool: %s (variant: %s) ===\n", toolName, variant.Name)

			// --- 4a. Ask for unconnected input file paths ---
			var mounts []string
			for _, input := range variant.InputSpecs {
				connected := false
				for _, c := range graph.Connections {
					if c.TargetParamSpec.Id == input.Id {
						connected = true
						break
					}
				}
				if connected {
					continue
				}

				extensions := input.IoSpec.Extensions
				extHint := ""
				if len(extensions) > 0 {
					extHint = " (" + strings.Join(extensions, ", ") + ")"
				}
				required := ""
				if input.IsRequired {
					required = " [required]"
				}

				fmt.Fprintf(os.Stderr, "  Input %q%s: ", input.Name, extHint+required)
				scanner.Scan()
				userPath := strings.TrimSpace(scanner.Text())
				if userPath == "" && input.IsRequired {
					fmt.Fprintf(os.Stderr, "  Error: %s is required.\n", input.Name)
					os.Exit(1)
				}
				if userPath == "" {
					continue
				}

				absPath, err := filepath.Abs(userPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error resolving path: %v\n", err)
					os.Exit(1)
				}
				if _, err := os.Stat(absPath); os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "  Error: file does not exist: %s\n", absPath)
					os.Exit(1)
				}

				containerPath := filepath.Join("/cf", "inputs", input.Name+filepath.Ext(absPath))
				mounts = append(mounts, fmt.Sprintf("%s:%s", absPath, containerPath))
			}

			// --- 4b. Pull image to get image ref and release spec ---
			relUUID := variant.Command.ToolRelease.Id
			pullResp, err := apiClient.PullImageApiRegistryPullGetWithResponse(
				cmd.Context(),
				&openapi.PullImageApiRegistryPullGetParams{
					ReleaseId: &relUUID,
				},
			)
			if err != nil || pullResp.JSON200 == nil {
				fmt.Fprintf(os.Stderr, "  Error pulling image info for %s: %v\n", toolName, err)
				os.Exit(1)
			}
			imageRef := pullResp.JSON200.ImageRef
			releaseSpec := pullResp.JSON200.ReleaseSpec
			fmt.Fprintf(os.Stderr, "  Image: %s\n", imageRef)

			// --- 4c. Find the matching command/variant in the release spec ---
			var invokeArgs []string
			var detailedParams []openapi.ParamSpecSchema

			for _, c := range releaseSpec.Commands {
				if c.Name == nil || *c.Name != variant.Command.Name {
					continue
				}
				for _, v := range c.Variants {
					if v.Name == variant.Name {
						invokeArgs = c.InvokeArgs
						detailedParams = v.Parameters
						break
					}
				}
				if invokeArgs == nil && len(c.Variants) == 1 {
					invokeArgs = c.InvokeArgs
					detailedParams = c.Variants[0].Parameters
				}
				break
			}

			if invokeArgs == nil {
				fmt.Fprintf(os.Stderr, "  Error: could not find command invocation for %s/%s\n", toolName, variant.Name)
				os.Exit(1)
			}
			entrypoint := []string{filepath.Join("/app", strings.ToLower(strings.ReplaceAll(toolName, " ", "-")))}

			// --- 4d. Resolve CLI arguments from parameters ---
			runOverrides := runParamsByNode[node.Id.String()]

			var cliArgs []string
			for _, param := range detailedParams {
				var value any
				var found bool

				if v, ok := runOverrides[param.Name]; ok {
					value = v
					found = true
				} else if param.DefaultValue != nil {
					if dv, ok := (*param.DefaultValue)["value"]; ok {
						value = dv
						found = true
					}
				}

				if !found && param.IsRequired {
					fmt.Fprintf(os.Stderr, "  Error: required parameter %q has no value.\n", param.Name)
					os.Exit(1)
				}
				if !found {
					continue
				}

				prefix := ""
				if param.Prefix != nil {
					prefix = *param.Prefix
				}
				valStr := fmt.Sprintf("%v", value)

				if param.Type == openapi.ParameterTypePath {
					isOutput := prefix != "" && len(prefix) >= 3 && prefix[:3] == "--o"
					subDir := "inputs"
					if isOutput {
						subDir = "outputs"
					}
					ext := filepath.Ext(valStr)
					containerPath := filepath.Join("/cf", subDir, param.Name+ext)
					mounts = append(mounts, fmt.Sprintf("%s:%s", valStr, containerPath))
					if prefix != "" {
						cliArgs = append(cliArgs, prefix, containerPath)
					} else {
						cliArgs = append(cliArgs, containerPath)
					}
				} else {
					if prefix != "" {
						cliArgs = append(cliArgs, prefix, valStr)
					} else {
						cliArgs = append(cliArgs, valStr)
					}
				}
			}

			toolNodeIDs = append(toolNodeIDs, node.Id.String())
			toolNameByID = append(toolNameByID, toolName)
			tools = append(tools, toolExec{
				ImageRef:   imageRef,
				Entrypoint: entrypoint,
				Mounts:     mounts,
				CLIArgs:    append(append([]string{}, invokeArgs...), cliArgs...),
				ToolName:   toolName,
			})
		}

		if len(tools) == 0 {
			fmt.Fprintf(os.Stderr, "No tools found in this workflow.\n")
			os.Exit(0)
		}

		// --- 5. Topological sort ---
		nodeIDSet := make(map[string]int)
		for i, nid := range toolNodeIDs {
			nodeIDSet[nid] = i
		}
		depCount := make([]int, len(tools))
		dependents := make([][]int, len(tools))
		for _, conn := range graph.Connections {
			srcIdx, srcOK := nodeIDSet[conn.SourceNodeId.String()]
			tgtIdx, tgtOK := nodeIDSet[conn.TargetNodeId.String()]
			if srcOK && tgtOK {
				depCount[tgtIdx]++
				dependents[srcIdx] = append(dependents[srcIdx], tgtIdx)
			}
		}

		type orderItem struct {
			Idx      int
			ToolName string
		}
		var order []orderItem
		var queue []int
		for i, c := range depCount {
			if c == 0 {
				queue = append(queue, i)
			}
		}
		for len(queue) > 0 {
			idx := queue[0]
			queue = queue[1:]
			order = append(order, orderItem{Idx: idx, ToolName: toolNameByID[idx]})
			for _, dep := range dependents[idx] {
				depCount[dep]--
				if depCount[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}
		if len(order) != len(tools) {
			fmt.Fprintf(os.Stderr, "Warning: cycle detected in workflow. Running in arbitrary order.\n")
			for i := range tools {
				order = append(order, orderItem{Idx: i, ToolName: toolNameByID[i]})
			}
		}

		// --- 6. Execute each tool container ---
		containerEngine, err := engine.NewDockerEngine()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing Docker engine: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "\n=== Executing %d tools ===\n\n", len(order))
		for _, item := range order {
			t := tools[item.Idx]

			fmt.Fprintf(os.Stderr, "[%s] Starting …\n", item.ToolName)

			exists, _ := containerEngine.ImageExists(cmd.Context(), t.ImageRef)
			if !exists {
				fmt.Fprintf(os.Stderr, "[%s] Pulling image …\n", item.ToolName)
				if err := containerEngine.PullImage(cmd.Context(), t.ImageRef); err != nil {
					fmt.Fprintf(os.Stderr, "[%s] Error pulling image: %v\n", item.ToolName, err)
					os.Exit(1)
				}
			}

			fmt.Fprintf(os.Stderr, "[%s] Running container …\n", item.ToolName)
			if err := containerEngine.RunContainer(cmd.Context(), t.ImageRef, t.Entrypoint, t.CLIArgs, t.Mounts, false); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] Error: %v\n", item.ToolName, err)
				os.Exit(1)
			}

			fmt.Fprintf(os.Stderr, "[%s] Completed.\n\n", item.ToolName)
		}

		fmt.Fprintf(os.Stderr, "Workflow completed successfully!\n")
	},
}
