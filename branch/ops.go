package branch

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/container"
	"github.com/darklang/dark-multi/tmux"
)

// logToFile writes debug output to /tmp/dark-multi.log
func logToFile(format string, args ...interface{}) {
	f, err := os.OpenFile("/tmp/dark-multi.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	msg := fmt.Sprintf(format, args...)
	f.WriteString(fmt.Sprintf("[branch] %s\n", msg))
}

// Start starts a branch container and sets up tmux.
func Start(b *Branch) error {
	return StartWithProgress(b, nil)
}

// StartWithProgress starts a branch container with progress callback.
// The callback receives short status messages suitable for display.
func StartWithProgress(b *Branch, onProgress func(status string)) error {
	logToFile("StartWithProgress called for %s", b.Name)

	if !b.Exists() {
		return fmt.Errorf("branch %s does not exist", b.Name)
	}

	if b.IsRunning() {
		logToFile("Branch %s already running, skipping", b.Name)
		return nil // Already running
	}

	progress := func(s string) {
		logToFile("Progress: %s", s)
		if onProgress != nil {
			onProgress(s)
		}
	}

	// Reset progress tracking for fresh start
	ResetProgressLevel(b.Name)

	progress("preparing container")

	// Generate override config
	overrideConfig, err := container.GenerateOverrideConfig(b)
	if err != nil {
		return fmt.Errorf("failed to generate override config: %w", err)
	}

	progress("starting container")

	// Start the devcontainer with output capture
	cmd := exec.Command("devcontainer", "up",
		"--workspace-folder", b.Path,
		"--override-config", overrideConfig,
	)

	// Capture combined output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start devcontainer: %w", err)
	}

	// Parse output for progress
	scanner := bufio.NewScanner(stdout)
	stepRegex := regexp.MustCompile(`\[(\d+)/(\d+)\]`)
	for scanner.Scan() {
		line := scanner.Text()
		logToFile("devcontainer: %s", line)
		// Extract meaningful status from devcontainer output
		status := parseDevcontainerLine(line, stepRegex, b.Name)
		if status != "" {
			progress(status)
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	progress("container ready")

	// Note: Don't create tmux session here - wait for auth to complete
	return nil
}

// Progress levels in order - higher number = further along
var progressLevels = map[string]int{
	"pulling image":        1,
	"building image":       2,
	"creating container":   3,
	"container started":    4,
	"post-create setup":    5,
	"post-start setup":     6,
	"building tree-sitter": 7,
	"restoring packages":   8,
	"building F#":          9,
	"starting build server": 10,
	"ready":                11,
}

// currentProgressLevel tracks the highest progress seen per branch
var currentProgressLevel = make(map[string]int)

// parseDevcontainerLine extracts a short status from devcontainer output.
// Returns empty string if this status is lower than what we've already seen.
func parseDevcontainerLine(line string, stepRegex *regexp.Regexp, branchName string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Docker build steps: "[5/17] RUN apt-get..."
	if matches := stepRegex.FindStringSubmatch(line); matches != nil {
		return fmt.Sprintf("build [%s/%s]", matches[1], matches[2])
	}

	lower := strings.ToLower(line)
	var status string

	// Devcontainer lifecycle phases FIRST (these lines may contain metadata with misleading strings)
	switch {
	case strings.Contains(lower, "pulling from") || strings.Contains(line, "Pull complete"):
		status = "pulling image"
	case strings.Contains(lower, "step ") && strings.Contains(lower, "run ") && !strings.Contains(lower, "docker"):
		status = "building image"
	case strings.Contains(lower, "start: run: docker run"):
		status = "creating container"
	case strings.Contains(lower, "start: run: docker start"):
		status = "container started"
	case strings.Contains(lower, "running the postcreatecommand"):
		status = "post-create setup"
	case strings.Contains(lower, "running the poststartcommand"):
		status = "post-start setup"
	}

	// Dark-specific build phases (from postStartCommand output) - only if not a lifecycle phase
	if status == "" {
		switch {
		case strings.Contains(lower, "tree-sitter") || strings.Contains(lower, "tree_sitter"):
			status = "building tree-sitter"
		case strings.Contains(lower, "dotnet build") || (strings.Contains(lower, "fsdark.sln") && !strings.Contains(lower, "docker")):
			status = "building F#"
		case strings.Contains(lower, "dotnet restore"):
			status = "restoring packages"
		case strings.Contains(lower, "build-server"):
			status = "starting build server"
		case strings.Contains(lower, "shipit ready") || strings.Contains(lower, "ready to ship"):
			status = "ready"
		}
	}

	if status == "" {
		return ""
	}

	// Only return if this is higher progress than we've seen
	level := progressLevels[status]
	if level > currentProgressLevel[branchName] {
		currentProgressLevel[branchName] = level
		return status
	}
	return ""
}

// ResetProgressLevel resets progress tracking for a branch (call when starting fresh)
func ResetProgressLevel(branchName string) {
	delete(currentProgressLevel, branchName)
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
	return CreateWithProgress(name, nil)
}

// CreateWithProgress creates a new branch with progress callback.
func CreateWithProgress(name string, onProgress func(status string)) (*Branch, error) {
	b := New(name)

	progress := func(s string) {
		if onProgress != nil {
			onProgress(s)
		}
	}

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

	progress("cloning repo")

	instanceID := FindNextInstanceID()
	os.MkdirAll(config.DarkRoot, 0755)

	// Check GitHub fork is configured
	githubFork := config.GetGitHubFork()
	if githubFork == "" {
		return nil, fmt.Errorf("GitHub fork not configured. Run: multi set-fork git@github.com:USERNAME/dark.git")
	}

	// Clone from GitHub fork directly (faster if no local source, and ensures correct remote)
	var cloneCmd *exec.Cmd
	if source != "" {
		// Clone from local source for speed, then fix remote
		cloneCmd = exec.Command("git", "clone", "--progress", source, b.Path)
	} else {
		// Clone directly from GitHub
		cloneCmd = exec.Command("git", "clone", "--progress", githubFork, b.Path)
	}
	if err := cloneCmd.Run(); err != nil {
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	progress("setting up branch")

	// Ensure remote points to GitHub fork
	exec.Command("git", "-C", b.Path, "remote", "set-url", "origin", githubFork).Run()

	// Also add upstream remote pointing to darklang/dark
	exec.Command("git", "-C", b.Path, "remote", "add", "upstream", "git@github.com:darklang/dark.git").Run()

	// Fetch from both remotes
	exec.Command("git", "-C", b.Path, "fetch", "origin").Run()
	exec.Command("git", "-C", b.Path, "fetch", "upstream").Run()

	// Hard reset to upstream/main to ensure clean state (ignore any changes from source repo)
	exec.Command("git", "-C", b.Path, "checkout", "main").Run()
	exec.Command("git", "-C", b.Path, "reset", "--hard", "upstream/main").Run()

	// Create new branch from clean main
	checkoutCmd := exec.Command("git", "-C", b.Path, "checkout", "-b", name)
	if err := checkoutCmd.Run(); err != nil {
		// Branch might already exist, just check it out
		exec.Command("git", "-C", b.Path, "checkout", name).Run()
	}

	// Clean any untracked files
	exec.Command("git", "-C", b.Path, "clean", "-fd").Run()

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
