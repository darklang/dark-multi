// Package branch provides branch management for dark-multi.
package branch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darklang/dark-multi/config"
)

// Branch represents a branch clone.
type Branch struct {
	Name         string
	Path         string
	OverrideDir  string
	MetadataFile string
}

// New creates a new Branch instance.
func New(name string) *Branch {
	path := filepath.Join(config.DarkRoot, name)
	overrideDir := filepath.Join(config.OverridesDir, name)
	return &Branch{
		Name:         name,
		Path:         path,
		OverrideDir:  overrideDir,
		MetadataFile: filepath.Join(overrideDir, "metadata"),
	}
}

// Exists returns true if the branch directory exists with a .git folder.
func (b *Branch) Exists() bool {
	gitPath := filepath.Join(b.Path, ".git")
	info, err := os.Stat(gitPath)
	return err == nil && info.IsDir()
}

// IsManaged returns true if this is a dark-multi managed branch.
func (b *Branch) IsManaged() bool {
	// Check override dir exists and has metadata file
	info, err := os.Stat(b.OverrideDir)
	if err != nil || !info.IsDir() {
		return false
	}
	info, err = os.Stat(b.MetadataFile)
	return err == nil && !info.IsDir()
}

// Metadata returns the branch metadata as a map.
func (b *Branch) Metadata() map[string]string {
	data := make(map[string]string)
	content, err := os.ReadFile(b.MetadataFile)
	if err != nil {
		return data
	}
	for _, line := range strings.Split(string(content), "\n") {
		if idx := strings.Index(line, "="); idx > 0 {
			data[line[:idx]] = line[idx+1:]
		}
	}
	return data
}

// GetName returns the branch name (implements container.BranchInfo).
func (b *Branch) GetName() string {
	return b.Name
}

// GetPath returns the branch path (implements container.BranchInfo).
func (b *Branch) GetPath() string {
	return b.Path
}

// InstanceID returns the branch instance ID.
func (b *Branch) InstanceID() int {
	if id, ok := b.Metadata()["ID"]; ok {
		if i, err := strconv.Atoi(id); err == nil {
			return i
		}
	}
	return 0
}

// ContainerName returns the Docker container name.
func (b *Branch) ContainerName() string {
	return fmt.Sprintf("dark-%s", b.Name)
}

// ContainerID returns the running container ID, if any.
func (b *Branch) ContainerID() (string, error) {
	// Try by name first (new containers)
	cmd := exec.Command("docker", "ps", "-q", "--filter", fmt.Sprintf("name=^%s$", b.ContainerName()))
	out, err := cmd.Output()
	if err == nil {
		if id := strings.TrimSpace(string(out)); id != "" {
			return id, nil
		}
	}

	// Fall back to label (old containers)
	cmd = exec.Command("docker", "ps", "-q", "--filter", fmt.Sprintf("label=dark-dev-container=%s", b.Name))
	out, err = cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRunning returns true if the container is running.
func (b *Branch) IsRunning() bool {
	id, err := b.ContainerID()
	return err == nil && id != ""
}

// HasChanges returns true if there are uncommitted changes.
func (b *Branch) HasChanges() bool {
	if !b.Exists() {
		return false
	}
	cmd := exec.Command("git", "-C", b.Path, "status", "--porcelain")
	out, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// GitStatus returns modified and untracked file counts.
func (b *Branch) GitStatus() (modified int, untracked int) {
	if !b.Exists() {
		return 0, 0
	}
	cmd := exec.Command("git", "-C", b.Path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "??") {
			untracked++
		} else {
			modified++
		}
	}
	return modified, untracked
}

// GitStats returns commits ahead of main and total lines added/removed (committed + uncommitted).
func (b *Branch) GitStats() (commits int, added int, removed int) {
	if !b.Exists() || b.Name == "main" {
		return 0, 0, 0
	}

	// Try different refs to compare against
	refs := []string{"origin/main", "main"}
	var baseRef string
	for _, ref := range refs {
		cmd := exec.Command("git", "-C", b.Path, "rev-parse", "--verify", ref)
		if cmd.Run() == nil {
			baseRef = ref
			break
		}
	}

	if baseRef != "" {
		// Count commits ahead of base
		cmd := exec.Command("git", "-C", b.Path, "rev-list", "--count", baseRef+"..HEAD")
		out, err := cmd.Output()
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &commits)
		}

		// Get total diff stats vs base (includes uncommitted)
		cmd = exec.Command("git", "-C", b.Path, "diff", "--numstat", baseRef)
		out, err = cmd.Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					var a, r int
					fmt.Sscanf(fields[0], "%d", &a)
					fmt.Sscanf(fields[1], "%d", &r)
					added += a
					removed += r
				}
			}
		}
	} else {
		// No base ref found - just show uncommitted changes
		cmd := exec.Command("git", "-C", b.Path, "diff", "--numstat", "HEAD")
		out, err := cmd.Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					var a, r int
					fmt.Sscanf(fields[0], "%d", &a)
					fmt.Sscanf(fields[1], "%d", &r)
					added += a
					removed += r
				}
			}
		}
	}

	return commits, added, removed
}

// PortBase returns the test port base for this branch.
func (b *Branch) PortBase() int {
	return 10011 + b.InstanceID()*100
}

// BwdPortBase returns the BwdServer port base for this branch.
func (b *Branch) BwdPortBase() int {
	return 11001 + b.InstanceID()*100
}

// WriteMetadata writes the branch metadata file.
func (b *Branch) WriteMetadata(instanceID int) error {
	if err := os.MkdirAll(b.OverrideDir, 0755); err != nil {
		return err
	}
	content := fmt.Sprintf("ID=%d\nNAME=%s\nCREATED=%s\n",
		instanceID, b.Name, time.Now().Format(time.RFC3339))
	return os.WriteFile(b.MetadataFile, []byte(content), 0644)
}

// StatusLine returns a formatted status line for display.
func (b *Branch) StatusLine() string {
	status := "stopped"
	if b.IsRunning() {
		status = "running"
	}
	changes := ""
	if b.HasChanges() {
		changes = " [modified]"
	}
	return fmt.Sprintf("%-20s %-10s ports %d+/%d+%s",
		b.Name, status, b.PortBase(), b.BwdPortBase(), changes)
}
