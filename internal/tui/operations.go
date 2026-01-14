package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/stachu/dark-multi/internal/branch"
	"github.com/stachu/dark-multi/internal/container"
	"github.com/stachu/dark-multi/internal/tmux"
)

// startBranchFull starts a branch container and sets up tmux.
func startBranchFull(b *branch.Branch) error {
	if !b.Exists() {
		return fmt.Errorf("branch %s does not exist", b.Name)
	}

	if b.IsRunning() {
		return nil // Already running
	}

	// Generate override config (always regenerate to pick up any changes)
	overrideConfig, err := container.GenerateOverrideConfig(b)
	if err != nil {
		return fmt.Errorf("failed to generate override config: %w", err)
	}

	// Start the devcontainer using the override
	cmd := exec.Command("devcontainer", "up",
		"--workspace-folder", b.Path,
		"--override-config", overrideConfig,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Get container ID
	containerID, err := b.ContainerID()
	if err != nil || containerID == "" {
		return fmt.Errorf("container started but couldn't get ID")
	}

	// Create tmux window
	if err := tmux.CreateWindow(b.Name, containerID, b.Path); err != nil {
		return fmt.Errorf("failed to create tmux window: %w", err)
	}

	return nil
}

// stopBranchFull stops a branch container and cleans up tmux.
func stopBranchFull(b *branch.Branch) error {
	// Kill tmux window
	tmux.KillWindow(b.Name)

	// Stop the container
	containerID, err := b.ContainerID()
	if err != nil {
		return nil // No container
	}
	if containerID != "" {
		if err := container.StopContainer(containerID); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}

	return nil
}

// openVSCode opens VS Code for a branch.
func openVSCode(b *branch.Branch) error {
	if !b.Exists() {
		return fmt.Errorf("branch %s does not exist", b.Name)
	}

	overrideConfig := filepath.Join(b.OverrideDir, "devcontainer.json")

	// Build the devcontainer URI
	// Format: vscode-remote://dev-container+<hex-encoded-config-path>/home/dark/app
	hexPath := fmt.Sprintf("%x", overrideConfig)
	uri := fmt.Sprintf("vscode-remote://dev-container+%s/home/dark/app", hexPath)

	cmd := exec.Command("code", "--folder-uri", uri)
	return cmd.Start()
}
