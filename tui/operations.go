package tui

import (
	"fmt"
	"os/exec"

	"github.com/darklang/dark-multi/branch"
)

// startBranchFull starts a branch container and sets up tmux.
func startBranchFull(b *branch.Branch) error {
	return startBranchWithProgress(b, "")
}

// startBranchWithProgress starts with progress updates to globalPendingBranches.
func startBranchWithProgress(b *branch.Branch, name string) error {
	if name == "" {
		name = b.Name
	}
	return branch.StartWithProgress(b, func(status string) {
		if pending, ok := globalPendingBranches[name]; ok {
			pending.Status = status
		}
	})
}

// stopBranchFull stops a branch container and cleans up tmux.
func stopBranchFull(b *branch.Branch) error {
	return branch.Stop(b)
}

// createBranchFull creates a new branch, cloning from GitHub if needed.
func createBranchFull(name string) (*branch.Branch, error) {
	return branch.CreateWithProgress(name, func(status string) {
		if pending, ok := globalPendingBranches[name]; ok {
			pending.Status = status
		}
	})
}

// removeBranchFull removes a branch entirely.
func removeBranchFull(b *branch.Branch) error {
	return branch.Remove(b)
}

// openGitDiff opens a git diff viewer for a branch.
func openGitDiff(b *branch.Branch) error {
	// Try gitk first
	if _, err := exec.LookPath("gitk"); err == nil {
		cmd := exec.Command("gitk", "--all")
		cmd.Dir = b.Path
		return cmd.Start()
	}

	// Try git gui
	if _, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command("git", "gui")
		cmd.Dir = b.Path
		return cmd.Start()
	}

	return fmt.Errorf("no git GUI found (tried gitk, git gui)")
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
