package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	execCmd.SetHelpFunc(execHelpFunc)
	rootCmd.AddCommand(execCmd)
}

var execCmd = &cobra.Command{
	Use:   "exec <ref> [docker-flags...] -- [args...]",
	Short: "Run a tool container via docker run",
	Long: `Resolve a tool ref to its full image digest, then run
'docker run <image> [args...]'. All flags and arguments after the
ref are passed straight through to docker.

Useful for running tools with custom Docker flags like -v, -e, etc.

Examples:
  codeflower exec fastq2usam -- --r1 /path/to/R1.fastq.gz
  codeflower exec fastq2usam -v /data:/data -- --r1 /data/R1.fastq.gz`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Check for --help before the ref
		for _, a := range args {
			if a == "--help" || a == "-h" {
				execHelpFunc(cmd, args)
				return
			}
		}

		execRef := args[0]
		rest := args[1:]

		// Split at '--': docker flags go before the image, container args after
		dockerFlags := rest
		containerArgs := []string{}
		for i, a := range rest {
			if a == "--" {
				dockerFlags = rest[:i]
				containerArgs = rest[i+1:]
				break
			}
		}

		imageRef, err := resolveImageRef(cmd, execRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// docker run [dockerFlags...] <image> [containerArgs...]
		cmdArgs := append([]string{"run"}, dockerFlags...)
		cmdArgs = append(cmdArgs, imageRef)
		cmdArgs = append(cmdArgs, containerArgs...)
		dockerCmd := exec.Command("docker", cmdArgs...)
		dockerCmd.Stdin = os.Stdin
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr

		if err := dockerCmd.Run(); err != nil {
			os.Exit(dockerCmd.ProcessState.ExitCode())
		}
	},
}

func execHelpFunc(cmd *cobra.Command, args []string) {
	ref := extractRefFromArgs("exec")
	if ref == "" {
		cmd.Root().HelpFunc()(cmd, args)
		return
	}
	printToolHelp(cmd, ref)
}