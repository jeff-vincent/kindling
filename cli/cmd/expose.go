package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var exposeCmd = &cobra.Command{
	Use:   "expose",
	Short: "Expose the local cluster via a public HTTPS tunnel",
	Long: `Creates a secure tunnel from a public HTTPS URL to the Kind cluster's
ingress controller, enabling external OAuth/OIDC providers (Auth0, Okta,
Firebase Auth, etc.) to call back into local services.

The tunnel provider handles TLS termination automatically.

Supported providers:
  cloudflared  — Cloudflare Tunnel (free, no account required for quick tunnels)
  ngrok        — ngrok tunnel (requires free account + auth token)

Examples:
  kindling expose                          # auto-detect provider, expose port 80
  kindling expose --provider cloudflared   # use cloudflared explicitly
  kindling expose --provider ngrok         # use ngrok explicitly
  kindling expose --port 443               # expose a different port

The public URL is printed to stdout and saved to .kindling/tunnel.yaml so that
other commands (kindling generate) can reference it.

Press Ctrl+C to stop the tunnel.`,
	RunE: runExpose,
}

var (
	exposeProvider string
	exposePort     int
)

func init() {
	exposeCmd.Flags().StringVar(&exposeProvider, "provider", "", "Tunnel provider: cloudflared or ngrok (auto-detected if omitted)")
	exposeCmd.Flags().IntVar(&exposePort, "port", 80, "Local port to expose (default: 80, the ingress controller)")
	rootCmd.AddCommand(exposeCmd)
}

func runExpose(cmd *cobra.Command, args []string) error {
	header("Public HTTPS tunnel")

	// ── Resolve provider ────────────────────────────────────────
	provider := exposeProvider
	if provider == "" {
		provider = detectTunnelProvider()
	}
	if provider == "" {
		fail("No tunnel provider found")
		fmt.Println()
		fmt.Println("  Install one of:")
		fmt.Printf("    cloudflared  → %shttps://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/%s\n", colorCyan, colorReset)
		fmt.Printf("    ngrok        → %shttps://ngrok.com/download%s\n", colorCyan, colorReset)
		fmt.Println()
		return fmt.Errorf("install cloudflared or ngrok and try again")
	}
	step("🔍", fmt.Sprintf("Using provider: %s%s%s", colorBold, provider, colorReset))

	// ── Verify cluster is running ───────────────────────────────
	if !clusterExists(clusterName) {
		return fmt.Errorf("Kind cluster %q not found — run 'kindling init' first", clusterName)
	}
	step("✅", fmt.Sprintf("Cluster %q is running", clusterName))

	// ── Start tunnel ────────────────────────────────────────────
	switch provider {
	case "cloudflared":
		return runCloudflaredTunnel()
	case "ngrok":
		return runNgrokTunnel()
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}
}

// detectTunnelProvider checks for available tunnel binaries.
func detectTunnelProvider() string {
	if commandExists("cloudflared") {
		return "cloudflared"
	}
	if commandExists("ngrok") {
		return "ngrok"
	}
	return ""
}

// ── Cloudflared ─────────────────────────────────────────────────

func runCloudflaredTunnel() error {
	step("🚇", fmt.Sprintf("Starting cloudflared tunnel → localhost:%d", exposePort))
	fmt.Println()

	// cloudflared quick tunnels print the URL to stderr in a line like:
	//   | https://some-random-name.trycloudflare.com |
	// We capture it from a log file.

	logFile, err := os.CreateTemp("", "kindling-cloudflared-*.log")
	if err != nil {
		return fmt.Errorf("cannot create temp log file: %w", err)
	}
	defer os.Remove(logFile.Name())

	tunnelCmd := exec.Command("cloudflared", "tunnel",
		"--url", fmt.Sprintf("http://localhost:%d", exposePort),
		"--logfile", logFile.Name(),
	)
	tunnelCmd.Stdout = os.Stdout
	tunnelCmd.Stderr = os.Stderr

	if err := tunnelCmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloudflared: %w", err)
	}

	// Wait for the URL to appear in the log
	var publicURL string
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		data, _ := os.ReadFile(logFile.Name())
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.Contains(line, ".trycloudflare.com") {
				// Extract URL
				for _, word := range strings.Fields(line) {
					if strings.HasPrefix(word, "https://") && strings.Contains(word, ".trycloudflare.com") {
						publicURL = strings.TrimRight(word, "|, ")
						break
					}
				}
			}
		}
		if publicURL != "" {
			break
		}
	}

	if publicURL == "" {
		// Fallback: tunnel is running but we couldn't parse the URL
		warn("Tunnel is running but could not detect public URL from logs")
		fmt.Println("  Check cloudflared output above for the public URL")
	} else {
		fmt.Println()
		success(fmt.Sprintf("Public URL: %s%s%s", colorBold, publicURL, colorReset))
		saveTunnelInfo(publicURL, "cloudflared")
		printTunnelUsage(publicURL)
	}

	// Wait for interrupt
	waitForInterrupt(tunnelCmd)
	return nil
}

// ── Ngrok ───────────────────────────────────────────────────────

func runNgrokTunnel() error {
	step("🚇", fmt.Sprintf("Starting ngrok tunnel → localhost:%d", exposePort))
	fmt.Println()

	// Start ngrok in background with API
	tunnelCmd := exec.Command("ngrok", "http",
		fmt.Sprintf("%d", exposePort),
		"--log", "stdout",
		"--log-format", "json",
	)
	tunnelCmd.Stdout = os.Stdout
	tunnelCmd.Stderr = os.Stderr

	if err := tunnelCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ngrok: %w", err)
	}

	// Poll the ngrok local API for the public URL
	var publicURL string
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		url, err := getNgrokPublicURL()
		if err == nil && url != "" {
			publicURL = url
			break
		}
	}

	if publicURL == "" {
		warn("Tunnel is running but could not detect public URL from ngrok API")
		fmt.Println("  Check ngrok dashboard at http://localhost:4040")
	} else {
		fmt.Println()
		success(fmt.Sprintf("Public URL: %s%s%s", colorBold, publicURL, colorReset))
		saveTunnelInfo(publicURL, "ngrok")
		printTunnelUsage(publicURL)
	}

	// Wait for interrupt
	waitForInterrupt(tunnelCmd)
	return nil
}

// getNgrokPublicURL queries the ngrok local API for the tunnel URL.
func getNgrokPublicURL() (string, error) {
	out, err := runSilent("curl", "-s", "http://localhost:4040/api/tunnels")
	if err != nil {
		return "", err
	}
	// Parse the JSON response
	var resp struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
			Proto     string `json:"proto"`
		} `json:"tunnels"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return "", err
	}
	// Prefer HTTPS
	for _, t := range resp.Tunnels {
		if t.Proto == "https" {
			return t.PublicURL, nil
		}
	}
	if len(resp.Tunnels) > 0 {
		return resp.Tunnels[0].PublicURL, nil
	}
	return "", fmt.Errorf("no tunnels found")
}

// ── Shared helpers ──────────────────────────────────────────────

// saveTunnelInfo persists the tunnel URL to .kindling/tunnel.yaml.
func saveTunnelInfo(publicURL, provider string) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	kindlingDir := filepath.Join(cwd, ".kindling")
	_ = os.MkdirAll(kindlingDir, 0755)

	tunnelFile := filepath.Join(kindlingDir, "tunnel.yaml")
	content := fmt.Sprintf("# Generated by kindling expose — do not edit\nprovider: %s\nurl: %s\ncreated: %s\n",
		provider, publicURL, time.Now().Format(time.RFC3339))

	_ = os.WriteFile(tunnelFile, []byte(content), 0644)

	// Ensure .kindling/ is gitignored
	ensureTunnelGitignored(cwd)
}

// ensureTunnelGitignored makes sure .kindling/ is in .gitignore.
func ensureTunnelGitignored(repoRoot string) {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".kindling/" || trimmed == ".kindling" {
			return // already ignored
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		_, _ = f.WriteString("\n")
	}
	_, _ = f.WriteString(".kindling/\n")
}

// printTunnelUsage shows next steps after the tunnel is established.
func printTunnelUsage(publicURL string) {
	fmt.Println()
	step("📋", "Next steps:")
	fmt.Println()
	fmt.Printf("  1. Set the callback URL in your OAuth provider to:\n")
	fmt.Printf("     %s%s/callback%s  (or your app's callback path)\n", colorCyan, publicURL, colorReset)
	fmt.Println()
	fmt.Printf("  2. Set the public URL as a secret:\n")
	fmt.Printf("     %skindling secrets set PUBLIC_URL %s%s\n", colorCyan, publicURL, colorReset)
	fmt.Println()
	fmt.Printf("  3. Use the URL in your deploy env vars:\n")
	fmt.Printf("     The generated workflow can reference it via secretKeyRef\n")
	fmt.Println()
	fmt.Printf("  Tunnel dashboard: %shttp://localhost:4040%s (ngrok only)\n", colorDim, colorReset)
	fmt.Printf("  Stop tunnel:      %sCtrl+C%s\n", colorDim, colorReset)
	fmt.Println()
}

// waitForInterrupt blocks until SIGINT/SIGTERM, then kills the child process.
func waitForInterrupt(cmd *exec.Cmd) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	fmt.Println()
	step("🛑", "Stopping tunnel...")

	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		// Give it a moment to clean up
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}

	// Clean up tunnel file
	cwd, _ := os.Getwd()
	tunnelFile := filepath.Join(cwd, ".kindling", "tunnel.yaml")
	_ = os.Remove(tunnelFile)

	success("Tunnel stopped")
}
