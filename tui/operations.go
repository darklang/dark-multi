package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/container"
	"github.com/darklang/dark-multi/tmux"
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

// createBranchFull creates a new branch, cloning from GitHub if needed.
func createBranchFull(name string) (*branch.Branch, error) {
	b := branch.New(name)

	// If branch already exists, just return it (will be started separately)
	if b.Exists() {
		if !b.IsManaged() {
			instanceID := branch.FindNextInstanceID()
			b.WriteMetadata(instanceID)
		}
		return b, nil
	}

	// Find source to clone from
	source := branch.FindSourceRepo()
	if source == "" {
		return nil, fmt.Errorf("no source repo found")
	}

	instanceID := branch.FindNextInstanceID()

	// Ensure parent dir exists
	os.MkdirAll(config.DarkRoot, 0755)

	// Clone
	cloneCmd := exec.Command("git", "clone", source, b.Path)
	if err := cloneCmd.Run(); err != nil {
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	// Checkout branch
	exec.Command("git", "-C", b.Path, "fetch", "origin").Run()
	checkoutCmd := exec.Command("git", "-C", b.Path, "checkout", "-b", name, "origin/main")
	if err := checkoutCmd.Run(); err != nil {
		exec.Command("git", "-C", b.Path, "checkout", "-b", name, "main").Run()
	}

	// Write metadata
	b.WriteMetadata(instanceID)

	return b, nil
}

// removeBranchFull removes a branch entirely.
func removeBranchFull(b *branch.Branch) error {
	// Stop container first
	stopBranchFull(b)

	// Kill tmux session
	tmux.KillBranchSession(b.Name)

	// Remove any lingering containers
	container.RemoveContainersByLabel(fmt.Sprintf("dark-dev-container=%s", b.Name))

	// Remove override config
	overrideDir := filepath.Join(config.ConfigDir, "overrides", b.Name)
	os.RemoveAll(overrideDir)

	// Remove directory
	if err := os.RemoveAll(b.Path); err != nil {
		return fmt.Errorf("failed to remove files: %w", err)
	}

	return nil
}
