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

// BranchSessionName returns the tmux session name for a branch.
func BranchSessionName(branchName string) string {
	return fmt.Sprintf("dark-%s", branchName)
}

// BranchSessionExists returns true if the branch's tmux session exists.
func BranchSessionExists(branchName string) bool {
	if !IsAvailable() {
		return false
	}
	session := BranchSessionName(branchName)
	cmd := exec.Command("tmux", "has-session", "-t", session)
	return cmd.Run() == nil
}

// CreateBranchSession creates a tmux session for a branch with CLI + claude panes.
func CreateBranchSession(branchName string, containerID string, branchPath string) error {
	if !IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	session := BranchSessionName(branchName)

	// Kill existing session if present
	if BranchSessionExists(branchName) {
		exec.Command("tmux", "kill-session", "-t", session).Run()
	}

	// Create new session
	cmd := exec.Command("tmux", "new-session", "-d", "-s", session)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Enable mouse support
	exec.Command("tmux", "set-option", "-t", session, "-g", "mouse", "on").Run()

	// Left pane: CLI inside container
	exec.Command("tmux", "send-keys", "-t", session,
		fmt.Sprintf("docker exec -it -w /home/dark/app %s bash", containerID), "Enter").Run()

	// Split and create right pane: claude on host
	exec.Command("tmux", "split-window", "-h", "-t", session).Run()

	workspace := branchPath
	if workspace == "" {
		workspace = filepath.Join(config.DarkRoot, branchName)
	}
	exec.Command("tmux", "send-keys", "-t", fmt.Sprintf("%s.1", session),
		fmt.Sprintf("cd %s && claude", workspace), "Enter").Run()

	// Select left pane (CLI)
	exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s.0", session)).Run()

	return nil
}

// KillBranchSession kills a branch's tmux session.
func KillBranchSession(branchName string) error {
	if BranchSessionExists(branchName) {
		session := BranchSessionName(branchName)
		return exec.Command("tmux", "kill-session", "-t", session).Run()
	}
	return nil
}

// BranchHasAttachedClients returns true if branch's session has clients attached.
func BranchHasAttachedClients(branchName string) bool {
	if !BranchSessionExists(branchName) {
		return false
	}
	session := BranchSessionName(branchName)
	out, err := exec.Command("tmux", "list-clients", "-t", session).Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// OpenBranchInTerminal opens a branch's tmux session in a terminal window.
// If already attached somewhere, tries to focus that window.
// Otherwise spawns a new terminal.
func OpenBranchInTerminal(branchName string) error {
	if !BranchSessionExists(branchName) {
		return fmt.Errorf("no tmux session for %s", branchName)
	}

	session := BranchSessionName(branchName)

	// If already attached, try to focus the existing window
	if BranchHasAttachedClients(branchName) {
		if focusTerminalByTitle(session) {
			return nil
		}
		// Couldn't focus, will spawn new terminal anyway
	}

	return spawnTerminalForSession(session)
}

// focusTerminalByTitle tries to focus a terminal window by title.
func focusTerminalByTitle(title string) bool {
	// Try xdotool (Linux)
	if _, err := exec.LookPath("xdotool"); err == nil {
		cmd := exec.Command("xdotool", "search", "--name", title)
		out, err := cmd.Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			windowID := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
			exec.Command("xdotool", "windowactivate", windowID).Run()
			return true
		}
	}

	// Try wmctrl (Linux)
	if _, err := exec.LookPath("wmctrl"); err == nil {
		if exec.Command("wmctrl", "-a", title).Run() == nil {
			return true
		}
	}

	// macOS: try osascript (less reliable for specific windows)
	if _, err := exec.LookPath("osascript"); err == nil {
		script := `tell application "System Events" to set frontmost of (first process whose name contains "Terminal" or name contains "iTerm") to true`
		exec.Command("osascript", "-e", script).Run()
	}

	return false
}

// spawnTerminalForSession spawns a new terminal window attached to a tmux session.
func spawnTerminalForSession(session string) error {
	terminal := detectTerminal()
	attachCmd := fmt.Sprintf("tmux attach -t %s", session)

	var cmd *exec.Cmd
	switch terminal {
	case "gnome-terminal":
		cmd = exec.Command("gnome-terminal", "--title", session, "--", "bash", "-c", attachCmd)
	case "kitty":
		cmd = exec.Command("kitty", "--title", session, "bash", "-c", attachCmd)
	case "alacritty":
		cmd = exec.Command("alacritty", "--title", session, "-e", "bash", "-c", attachCmd)
	case "hyper":
		cmd = exec.Command("hyper", attachCmd)
	case "iterm2":
		script := fmt.Sprintf(`tell application "iTerm2"
			create window with default profile command "%s"
		end tell`, attachCmd)
		cmd = exec.Command("osascript", "-e", script)
	case "terminal":
		script := fmt.Sprintf(`tell application "Terminal"
			do script "%s"
			activate
		end tell`, attachCmd)
		cmd = exec.Command("osascript", "-e", script)
	case "wezterm":
		cmd = exec.Command("wezterm", "start", "--", "bash", "-c", attachCmd)
	case "xterm":
		cmd = exec.Command("xterm", "-title", session, "-e", attachCmd)
	default:
		return fmt.Errorf("unknown terminal: %s. Set DARK_MULTI_TERMINAL", terminal)
	}

	return cmd.Start()
}

// detectTerminal returns the configured or auto-detected terminal.
func detectTerminal() string {
	if config.Terminal != "auto" {
		return config.Terminal
	}

	terminals := []string{
		"kitty", "alacritty", "gnome-terminal", "hyper", "wezterm", "xterm",
	}

	// macOS defaults
	if _, err := exec.LookPath("osascript"); err == nil {
		if _, err := os.Stat("/Applications/iTerm.app"); err == nil {
			return "iterm2"
		}
		return "terminal"
	}

	// Linux: check what's installed
	for _, t := range terminals {
		if _, err := exec.LookPath(t); err == nil {
			return t
		}
	}

	return "xterm"
}

// Legacy compatibility - these wrap the new per-branch functions

// SessionExists returns true if any dark session exists (legacy).
func SessionExists() bool {
	// Check for any dark-* session
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "dark-") {
			return true
		}
	}
	return false
}

// WindowExists is deprecated - use BranchSessionExists instead.
func WindowExists(name string) bool {
	return BranchSessionExists(name)
}

// CreateWindow is deprecated - use CreateBranchSession instead.
func CreateWindow(name string, containerID string, branchPath string) error {
	return CreateBranchSession(name, containerID, branchPath)
}

// KillWindow is deprecated - use KillBranchSession instead.
func KillWindow(name string) error {
	return KillBranchSession(name)
}
