package tui

import (
	"fmt"
	"os/exec"

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
	if !b.IsRunning() {
		return fmt.Errorf("branch %s is not running", b.Name)
	}

	// Use devcontainer CLI (preferred)
	if _, err := exec.LookPath("devcontainer"); err == nil {
		cmd := exec.Command("devcontainer", "open", b.Path)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback: use code --remote attached-container
	if _, err := exec.LookPath("code"); err == nil {
		containerID, _ := b.ContainerID()
		hexID := fmt.Sprintf("%x", containerID)
		cmd := exec.Command("code", "--remote", fmt.Sprintf("attached-container+%s", hexID), "/home/dark/app")
		return cmd.Start()
	}

	return fmt.Errorf("neither devcontainer CLI nor VS Code found")
}
