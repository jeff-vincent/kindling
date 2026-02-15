package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ── ANSI colours ────────────────────────────────────────────────
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// ── Pretty-print helpers ────────────────────────────────────────

func header(msg string) {
	fmt.Printf("\n%s%s▸ %s%s\n", colorBold, colorCyan, msg, colorReset)
}

func step(emoji, msg string) {
	fmt.Printf("  %s  %s\n", emoji, msg)
}

func success(msg string) {
	fmt.Printf("  %s✅ %s%s\n", colorGreen, msg, colorReset)
}

func warn(msg string) {
	fmt.Printf("  %s⚠️  %s%s\n", colorYellow, msg, colorReset)
}

func fail(msg string) {
	fmt.Printf("  %s❌ %s%s\n", colorRed, msg, colorReset)
}

func dimText(msg string) string {
	return fmt.Sprintf("%s%s%s", colorDim, msg, colorReset)
}

// ── Command execution helpers ───────────────────────────────────

// run executes a command, streaming stdout/stderr to the terminal.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runDir executes a command in a specific directory.
func runDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runSilent executes a command and returns combined output.
func runSilent(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// runCapture executes a command and returns stdout only.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), err
}

// commandExists checks if a binary is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// resolveProjectDir returns the project directory, defaulting to cwd.
func resolveProjectDir() (string, error) {
	if projectDir != "" {
		return projectDir, nil
	}
	return os.Getwd()
}

// clusterExists checks whether a Kind cluster with the given name exists.
func clusterExists(name string) bool {
	out, err := runCapture("kind", "get", "clusters")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}
