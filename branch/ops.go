package branch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/container"
	"github.com/darklang/dark-multi/tmux"
)

// Start starts a branch container and sets up tmux.
func Start(b *Branch) error {
	if !b.Exists() {
		return fmt.Errorf("branch %s does not exist", b.Name)
	}

	if b.IsRunning() {
		return nil // Already running
	}

	// Generate override config
	overrideConfig, err := container.GenerateOverrideConfig(b)
	if err != nil {
		return fmt.Errorf("failed to generate override config: %w", err)
	}

	// Start the devcontainer
	cmd := exec.Command("devcontainer", "up",
		"--workspace-folder", b.Path,
		"--override-config", overrideConfig,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Get container ID and create tmux session
	containerID, err := b.ContainerID()
	if err != nil || containerID == "" {
		return fmt.Errorf("container started but couldn't get ID")
	}

	if err := tmux.CreateBranchSession(b.Name, containerID, b.Path); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	return nil
}

// Stop stops a branch container and cleans up tmux.
func Stop(b *Branch) error {
	tmux.KillBranchSession(b.Name)

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

// Create creates a new branch, cloning if needed.
func Create(name string) (*Branch, error) {
	b := New(name)

	if b.Exists() {
		if !b.IsManaged() {
			instanceID := FindNextInstanceID()
			b.WriteMetadata(instanceID)
		}
		return b, nil
	}

	source := FindSourceRepo()
	if source == "" {
		return nil, fmt.Errorf("no source repo found")
	}

	instanceID := FindNextInstanceID()
	os.MkdirAll(config.DarkRoot, 0755)

	cloneCmd := exec.Command("git", "clone", source, b.Path)
	if err := cloneCmd.Run(); err != nil {
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	exec.Command("git", "-C", b.Path, "fetch", "origin").Run()
	checkoutCmd := exec.Command("git", "-C", b.Path, "checkout", "-b", name, "origin/main")
	if err := checkoutCmd.Run(); err != nil {
		exec.Command("git", "-C", b.Path, "checkout", "-b", name, "main").Run()
	}

	b.WriteMetadata(instanceID)
	return b, nil
}

// Remove removes a branch entirely.
func Remove(b *Branch) error {
	Stop(b)
	tmux.KillBranchSession(b.Name)
	container.RemoveContainersByLabel(fmt.Sprintf("dark-dev-container=%s", b.Name))

	overrideDir := filepath.Join(config.ConfigDir, "overrides", b.Name)
	os.RemoveAll(overrideDir)

	if err := os.RemoveAll(b.Path); err != nil {
		return fmt.Errorf("failed to remove files: %w", err)
	}

	return nil
}
