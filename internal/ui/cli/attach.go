package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	attachCmd.SetHelpFunc(attachHelpFunc)
	rootCmd.AddCommand(attachCmd)
}

var attachCmd = &cobra.Command{
	Use:   "attach <ref> [docker-flags...] [-- command]",
	Short: "Attach to a tool container interactively",
	Long: `Resolve a tool ref to its full image digest, then run
'docker run -it <image> [args...]'. Without a command, runs the
tool's own entrypoint interactively. Pass a command after '--' to
override (e.g. '-- sh' if the image has a shell).

Useful for debugging or exploring a container interactively.

Examples:
  codeflower attach fastq2usam
  codeflower attach fastq2usam -v /data:/data -- sh`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Check for --help before the ref
		for _, a := range args {
			if a == "--help" || a == "-h" {
				attachHelpFunc(cmd, args)
				return
			}
		}

		attachRef := args[0]
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

		imageRef, err := resolveImageRef(cmd, attachRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// docker run -it [dockerFlags...] <image> [containerArgs...]
		cmdArgs := append([]string{"run", "-it"}, dockerFlags...)
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

func attachHelpFunc(cmd *cobra.Command, args []string) {
	ref := extractRefFromArgs("attach")
	if ref == "" {
		cmd.Root().HelpFunc()(cmd, args)
		return
	}
	printToolHelp(cmd, ref)
}