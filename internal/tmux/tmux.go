// Package tmux provides tmux session management for dark-multi.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stachu/dark-multi/internal/config"
)

// IsAvailable returns true if tmux is installed.
func IsAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// SessionExists returns true if the dark tmux session exists.
func SessionExists() bool {
	if !IsAvailable() {
		return false
	}
	cmd := exec.Command("tmux", "has-session", "-t", config.TmuxSession)
	return cmd.Run() == nil
}

// WindowExists returns true if a window with the given name exists.
func WindowExists(name string) bool {
	if !SessionExists() {
		return false
	}
	cmd := exec.Command("tmux", "list-windows", "-t", config.TmuxSession, "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// CreateWindow creates a tmux window with CLI + claude panes.
func CreateWindow(name string, containerID string, branchPath string) error {
	if !IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	if !SessionExists() {
		// Create session with first window
		cmd := exec.Command("tmux", "new-session", "-d", "-s", config.TmuxSession, "-n", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	} else {
		// Kill existing window if present
		if WindowExists(name) {
			exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", config.TmuxSession, name)).Run()
		}
		// Create new window
		cmd := exec.Command("tmux", "new-window", "-a", "-t", config.TmuxSession, "-n", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create window: %w", err)
		}
	}

	// Left pane: CLI inside container
	exec.Command("tmux", "send-keys", "-t", fmt.Sprintf("%s:%s", config.TmuxSession, name),
		fmt.Sprintf("docker exec -it -w /home/dark/app %s bash", containerID), "Enter").Run()

	// Split and create right pane: claude on host
	exec.Command("tmux", "split-window", "-h", "-t", fmt.Sprintf("%s:%s", config.TmuxSession, name)).Run()

	workspace := branchPath
	if workspace == "" {
		workspace = filepath.Join(config.DarkRoot, name)
	}
	exec.Command("tmux", "send-keys", "-t", fmt.Sprintf("%s:%s.1", config.TmuxSession, name),
		fmt.Sprintf("cd %s && claude", workspace), "Enter").Run()

	// Select left pane (CLI)
	exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s:%s.0", config.TmuxSession, name)).Run()

	return nil
}

// KillWindow kills a tmux window.
func KillWindow(name string) error {
	if WindowExists(name) {
		return exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", config.TmuxSession, name)).Run()
	}
	return nil
}

// EnsureMetaWindow creates the dark-meta control plane window if it doesn't exist.
func EnsureMetaWindow() error {
	if WindowExists("dark-meta") {
		return nil
	}

	if !SessionExists() {
		// Create session with meta window
		cmd := exec.Command("tmux", "new-session", "-d", "-s", config.TmuxSession, "-n", "dark-meta")
		if err := cmd.Run(); err != nil {
			return err
		}
		// Enable mouse support
		exec.Command("tmux", "set-option", "-t", config.TmuxSession, "-g", "mouse", "on").Run()
	} else {
		exec.Command("tmux", "new-window", "-t", config.TmuxSession, "-n", "dark-meta").Run()
	}

	// Move meta window to be first (index 0)
	exec.Command("tmux", "move-window", "-t", fmt.Sprintf("%s:dark-meta", config.TmuxSession),
		"-t", fmt.Sprintf("%s:0", config.TmuxSession)).Run()

	// Left pane (70%): claude in dark-multi directory
	darkMultiDir := filepath.Join(config.DarkRoot, "..", "dark-multi")
	exec.Command("tmux", "send-keys", "-t", fmt.Sprintf("%s:dark-meta", config.TmuxSession),
		fmt.Sprintf("cd %s && claude", darkMultiDir), "Enter").Run()

	// Right pane (30%): quick reference
	exec.Command("tmux", "split-window", "-h", "-p", "30", "-t", fmt.Sprintf("%s:dark-meta", config.TmuxSession)).Run()

	// Quick reference content
	refText := `echo -e "
\033[1m=== DARK MULTI ===\033[0m

\033[1mBranch commands:\033[0m
  multi ls          - list branches
  multi new <name>  - create branch
  multi stop <name> - stop branch
  multi rm <name>   - remove branch
  multi code <name> - open VS Code

\033[1mtmux:\033[0m
  Ctrl-b n/p  - next/prev window
  Ctrl-b w    - list windows
  Ctrl-b o    - switch pane
  Ctrl-b z    - zoom pane
  Ctrl-b d    - detach

\033[1mWindows:\033[0m
  dark-meta   - this control plane
  <branch>    - CLI | claude
"`
	exec.Command("tmux", "send-keys", "-t", fmt.Sprintf("%s:dark-meta.1", config.TmuxSession), refText, "Enter").Run()

	// Select the claude pane
	exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s:dark-meta.0", config.TmuxSession)).Run()

	return nil
}

// Attach attaches to the tmux session, replacing the current process.
func Attach() error {
	if !IsAvailable() {
		return fmt.Errorf("tmux not installed")
	}
	if !SessionExists() {
		return fmt.Errorf("no tmux session")
	}
	return Exec("tmux", "attach", "-t", config.TmuxSession)
}

// Exec replaces the current process with the given command.
func Exec(name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	argv := append([]string{name}, args...)
	return exec.Command(path, argv[1:]...).Run()
}

// AttachExec attaches to tmux by replacing the current process.
func AttachExec() {
	if !IsAvailable() {
		fmt.Fprintln(os.Stderr, "error: tmux not installed")
		os.Exit(1)
	}
	if !SessionExists() {
		fmt.Fprintln(os.Stderr, "error: no tmux session. Start a branch first: multi start <branch>")
		os.Exit(1)
	}

	path, _ := exec.LookPath("tmux")
	// Use syscall.Exec to replace the process
	os.Exit(execCommand(path, "tmux", "attach", "-t", config.TmuxSession))
}

func execCommand(path string, args ...string) int {
	cmd := exec.Command(path, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 1
	}
	return 0
}
