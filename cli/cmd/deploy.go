package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Apply a DevStagingEnvironment from a YAML file",
	Long: `Applies one or more DevStagingEnvironment custom resources from a YAML
file into the current cluster.

Examples:
  kindling deploy -f examples/sample-app/dev-environment.yaml
  kindling deploy -f examples/platform-api/dev-environment.yaml`,
	RunE: runDeploy,
}

var deployFile string

func init() {
	deployCmd.Flags().StringVarP(&deployFile, "file", "f", "", "Path to DevStagingEnvironment YAML file (required)")
	_ = deployCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(deployFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", deployFile)
	}

	header("Deploying DevStagingEnvironment")

	step("📄", fmt.Sprintf("Applying %s", deployFile))
	if err := run("kubectl", "apply", "-f", deployFile); err != nil {
		return fmt.Errorf("kubectl apply failed: %w", err)
	}
	success("Resources applied")

	// ── Show what was created ───────────────────────────────────
	fmt.Println()
	step("📋", "Current DevStagingEnvironments:")
	fmt.Println()
	if err := run("kubectl", "get", "devstagingenvironments", "-o", "wide"); err != nil {
		warn("Could not list DevStagingEnvironments (CRD may not be installed)")
	}

	fmt.Println()
	fmt.Printf("  Track progress with: %skindling status%s\n", colorCyan, colorReset)
	fmt.Printf("  View controller logs: %skindling logs%s\n", colorCyan, colorReset)
	fmt.Println()

	return nil
}
