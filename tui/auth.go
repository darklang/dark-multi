package tui

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/darklang/dark-multi/branch"
)

// AuthModel handles Claude authentication for a branch.
type AuthModel struct {
	branch     *branch.Branch
	status     string
	authURL    string
	done       bool
	err        error
	cmd        *exec.Cmd
	width      int
	height     int
}

// Auth messages
type authURLMsg string
type authDoneMsg struct{}
type authErrMsg struct{ err error }
type authNeededMsg struct {
	branch *branch.Branch
	needed bool
}

// CheckAuthNeeded checks if a branch needs Claude authentication.
func CheckAuthNeeded(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		containerID, err := b.ContainerID()
		if err != nil {
			return authNeededMsg{b, false} // Can't check, assume not needed
		}

		// Check if credentials file exists
		cmd := exec.Command("docker", "exec", containerID, "test", "-f", "/home/dark/.claude/.credentials.json")
		err = cmd.Run()
		return authNeededMsg{b, err != nil} // needed if file doesn't exist
	}
}

// NewAuthModel creates an auth view for a branch.
func NewAuthModel(b *branch.Branch) AuthModel {
	return AuthModel{
		branch: b,
		status: "Starting Claude auth...",
	}
}

// Init starts the auth process.
func (m AuthModel) Init() tea.Cmd {
	return m.startAuth()
}

func (m AuthModel) startAuth() tea.Cmd {
	return func() tea.Msg {
		containerID, err := m.branch.ContainerID()
		if err != nil {
			return authErrMsg{fmt.Errorf("container not running: %w", err)}
		}

		// Run claude and capture output to find auth URL
		cmd := exec.Command("docker", "exec", "-i", containerID, "claude")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return authErrMsg{err}
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			return authErrMsg{err}
		}

		// Look for OAuth URL in output
		urlRegex := regexp.MustCompile(`https://claude\.ai/oauth/authorize\S+`)
		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			line := scanner.Text()
			if match := urlRegex.FindString(line); match != "" {
				// Found the URL - return it
				// Keep cmd running so it can receive the callback
				return authURLMsg(match)
			}
			// Check for success indicators
			if strings.Contains(line, "Successfully") || strings.Contains(line, "authenticated") {
				cmd.Process.Kill()
				return authDoneMsg{}
			}
		}

		cmd.Wait()
		return authDoneMsg{}
	}
}

// Update handles messages.
func (m AuthModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			if m.cmd != nil && m.cmd.Process != nil {
				m.cmd.Process.Kill()
			}
			grid := NewGridModel()
			return grid, grid.Init()
		}

	case authURLMsg:
		m.authURL = string(msg)
		m.status = "Open this URL to authenticate:"
		return m, nil

	case authDoneMsg:
		m.done = true
		m.status = "Authentication complete!"
		return m, nil

	case authErrMsg:
		m.err = msg.err
		m.status = fmt.Sprintf("Error: %v", msg.err)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View renders the auth screen.
func (m AuthModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("── Authenticate %s ──", m.branch.Name)))
	b.WriteString("\n\n")

	b.WriteString("  " + m.status)
	b.WriteString("\n\n")

	if m.authURL != "" {
		// Display URL - hopefully clickable in terminal
		b.WriteString("  ")
		b.WriteString(selectedStyle.Render(m.authURL))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  Click the URL or copy it to your browser"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  After authorizing, press ESC to return"))
		b.WriteString("\n")
	}

	if m.done {
		b.WriteString("\n")
		b.WriteString(runningStyle.Render("  ✓ Ready to use Claude"))
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  [esc] back"))
	b.WriteString("\n")

	return b.String()
}
