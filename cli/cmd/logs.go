package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail the kindling controller logs",
	Long: `Streams logs from the kindling controller-manager pod. Press Ctrl+C to stop.

Use --all to see logs from all containers in the pod (including kube-rbac-proxy).`,
	RunE: runLogs,
}

var (
	logsAll    bool
	logsSince  string
	logsFollow bool
)

func init() {
	logsCmd.Flags().BoolVar(&logsAll, "all", false, "Show logs from all containers")
	logsCmd.Flags().StringVar(&logsSince, "since", "5m", "Show logs since duration (e.g. 5m, 1h)")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", true, "Follow log output (stream)")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	header("Controller logs")

	kubectlArgs := []string{
		"logs",
		"-n", "kindling-system",
		"-l", "control-plane=controller-manager",
		"--since=" + logsSince,
	}

	if logsAll {
		kubectlArgs = append(kubectlArgs, "--all-containers=true")
	} else {
		kubectlArgs = append(kubectlArgs, "-c", "manager")
	}

	if logsFollow {
		kubectlArgs = append(kubectlArgs, "-f")
		fmt.Printf("  %sStreaming (Ctrl+C to stop)...%s\n\n", colorDim, colorReset)
	}

	return run("kubectl", kubectlArgs...)
}
