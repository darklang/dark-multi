// Package tmux provides tmux session management for dark-multi.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/darklang/dark-multi/config"
)

// Session types
const (
	SessionClaude   = "claude"
	SessionTerminal = "term"
)

// IsAvailable returns true if tmux is installed.
func IsAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// sessionName returns the tmux session name for a branch and type.
func sessionName(branchName, sessionType string) string {
	return fmt.Sprintf("dark-%s-%s", branchName, sessionType)
}

// sessionExists returns true if a session exists.
func sessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// OpenClaude opens or attaches to the Claude session for a branch.
func OpenClaude(branchName, containerID string) error {
	if !IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	session := sessionName(branchName, SessionClaude)

	// Create session if it doesn't exist
	if !sessionExists(session) {
		if err := exec.Command("tmux", "new-session", "-d", "-s", session).Run(); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		exec.Command("tmux", "set-option", "-t", session, "-g", "mouse", "on").Run()

		// Start bash in container with API key, then run claude
		dockerBash := dockerExecWithEnv(containerID)
		exec.Command("tmux", "send-keys", "-t", session, dockerBash, "Enter").Run()
		exec.Command("tmux", "send-keys", "-t", session, "sleep 1 && claude --dangerously-skip-permissions", "Enter").Run()
	}

	return openInTerminal(session)
}

// OpenTerminal opens or attaches to the terminal session for a branch.
func OpenTerminal(branchName, containerID string) error {
	if !IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	session := sessionName(branchName, SessionTerminal)

	// Create session if it doesn't exist
	if !sessionExists(session) {
		if err := exec.Command("tmux", "new-session", "-d", "-s", session).Run(); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		exec.Command("tmux", "set-option", "-t", session, "-g", "mouse", "on").Run()

		// Start bash in container with API key
		dockerBash := dockerExecWithEnv(containerID)
		exec.Command("tmux", "send-keys", "-t", session, dockerBash, "Enter").Run()
	}

	return openInTerminal(session)
}

// dockerExecWithEnv returns the docker exec command with ANTHROPIC_API_KEY passed through.
func dockerExecWithEnv(containerID string) string {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		return fmt.Sprintf("docker exec -it -e ANTHROPIC_API_KEY=%s -w /home/dark/app %s bash", apiKey, containerID)
	}
	return fmt.Sprintf("docker exec -it -w /home/dark/app %s bash", containerID)
}

// openInTerminal opens a tmux session in a terminal window.
// If already attached, focuses the existing window.
func openInTerminal(session string) error {
	// Check if already attached
	out, _ := exec.Command("tmux", "list-clients", "-t", session).Output()
	if len(strings.TrimSpace(string(out))) > 0 {
		// Try to focus existing window
		if focusTerminalByTitle(session) {
			return nil
		}
	}

	return spawnTerminalForSession(session)
}

// CapturePaneContent captures content from the Claude session for a branch.
func CapturePaneContent(branchName string, lines int) string {
	session := sessionName(branchName, SessionClaude)
	if !sessionExists(session) {
		return ""
	}

	// Capture last N lines from scrollback
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", fmt.Sprintf("-%d", lines))
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SendToClaude sends text to the Claude session for a branch.
func SendToClaude(branchName string, text string) error {
	session := sessionName(branchName, SessionClaude)
	if !sessionExists(session) {
		return fmt.Errorf("no Claude session for %s", branchName)
	}
	return exec.Command("tmux", "send-keys", "-t", session, text, "Enter").Run()
}

// KillBranchSessions kills all tmux sessions for a branch.
func KillBranchSessions(branchName string) error {
	for _, typ := range []string{SessionClaude, SessionTerminal} {
		session := sessionName(branchName, typ)
		if sessionExists(session) {
			exec.Command("tmux", "kill-session", "-t", session).Run()
		}
	}
	return nil
}

// ClaudeSessionExists returns true if the Claude session exists for a branch.
func ClaudeSessionExists(branchName string) bool {
	return sessionExists(sessionName(branchName, SessionClaude))
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

// StartRalphLoop starts the Ralph loop in the Claude session.
// Kills any existing session and starts fresh.
func StartRalphLoop(branchName, containerID string) error {
	if !IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	session := sessionName(branchName, SessionClaude)

	// Kill existing session - cleaner than trying to interrupt
	if sessionExists(session) {
		exec.Command("tmux", "kill-session", "-t", session).Run()
	}

	// Create fresh session
	if err := exec.Command("tmux", "new-session", "-d", "-s", session).Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	exec.Command("tmux", "set-option", "-t", session, "-g", "mouse", "on").Run()

	// Set up pipe-pane to log all output for summarization
	// The log file will be inside the container at /home/dark/app/.claude-task/output.log
	exec.Command("tmux", "pipe-pane", "-t", session, "-o", "cat >> /tmp/claude-output-"+branchName+".log").Run()

	// Start bash in container with API key, then run ralph
	dockerBash := dockerExecWithEnv(containerID)
	exec.Command("tmux", "send-keys", "-t", session, dockerBash, "Enter").Run()
	exec.Command("tmux", "send-keys", "-t", session, "sleep 1 && .claude-task/ralph.sh", "Enter").Run()

	return openInTerminal(session)
}

// GetOutputLogPath returns the path to the Claude output log for a branch.
func GetOutputLogPath(branchName string) string {
	return "/tmp/claude-output-" + branchName + ".log"
}

// Legacy compatibility

// BranchSessionExists returns true if the Claude session exists (for grid status).
func BranchSessionExists(branchName string) bool {
	return ClaudeSessionExists(branchName)
}

// KillBranchSession kills all sessions for a branch.
func KillBranchSession(branchName string) error {
	return KillBranchSessions(branchName)
}

// CreateBranchSession creates a Claude session (legacy - use OpenClaude instead).
func CreateBranchSession(branchName string, containerID string, branchPath string) error {
	session := sessionName(branchName, SessionClaude)
	if sessionExists(session) {
		return nil
	}
	if err := exec.Command("tmux", "new-session", "-d", "-s", session).Run(); err != nil {
		return err
	}
	exec.Command("tmux", "set-option", "-t", session, "-g", "mouse", "on").Run()
	dockerBash := dockerExecWithEnv(containerID)
	exec.Command("tmux", "send-keys", "-t", session, dockerBash, "Enter").Run()
	exec.Command("tmux", "send-keys", "-t", session, "sleep 1 && claude --dangerously-skip-permissions", "Enter").Run()
	return nil
}

// OpenBranchInTerminal opens the Claude session in a terminal (legacy).
func OpenBranchInTerminal(branchName string) error {
	session := sessionName(branchName, SessionClaude)
	if !sessionExists(session) {
		return fmt.Errorf("no session for %s", branchName)
	}
	return openInTerminal(session)
}

// SendToClaudePane sends text to the Claude session (legacy name).
func SendToClaudePane(branchName string, text string) error {
	return SendToClaude(branchName, text)
}
