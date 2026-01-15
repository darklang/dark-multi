package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
)

// BranchDetailModel shows details for a single branch.
type BranchDetailModel struct {
	branch        *branch.Branch
	containerInfo string
	gitStatus     string
	urls          []string
	urlCursor     int
	width         int
	height        int
	message       string
}

// Messages for async loading
type containerInfoMsg string
type gitStatusMsg string

// NewBranchDetailModel creates a branch detail view.
func NewBranchDetailModel(b *branch.Branch) BranchDetailModel {
	urls := []string{
		fmt.Sprintf("dark-packages.%s.dlio.localhost:%d/ping", b.Name, config.ProxyPort),
		fmt.Sprintf("dark-editor.%s.dlio.localhost:%d/a/dark-editor", b.Name, config.ProxyPort),
	}

	return BranchDetailModel{
		branch:        b,
		urls:          urls,
		containerInfo: "loading...",
		gitStatus:     "loading...",
	}
}

// Init starts async loading.
func (m BranchDetailModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadContainerInfo(),
		m.loadGitStatus(),
	)
}

func (m BranchDetailModel) loadContainerInfo() tea.Cmd {
	return func() tea.Msg {
		id, err := m.branch.ContainerID()
		if err != nil || id == "" {
			return containerInfoMsg("not running")
		}

		// Get container start time
		out, err := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", id).Output()
		if err != nil {
			return containerInfoMsg(fmt.Sprintf("dark-%s (running)", m.branch.Name))
		}

		// Parse time and calculate uptime
		startStr := strings.TrimSpace(string(out))
		startTime, err := time.Parse(time.RFC3339Nano, startStr)
		if err != nil {
			return containerInfoMsg(fmt.Sprintf("dark-%s (running)", m.branch.Name))
		}

		uptime := time.Since(startTime)
		uptimeStr := formatDuration(uptime)
		return containerInfoMsg(fmt.Sprintf("dark-%s (up %s)", m.branch.Name, uptimeStr))
	}
}

func (m BranchDetailModel) loadGitStatus() tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", "-C", m.branch.Path, "status", "--porcelain").Output()
		if err != nil {
			return gitStatusMsg("unknown")
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
			return gitStatusMsg("clean")
		}

		modified := 0
		untracked := 0
		for _, line := range lines {
			if strings.HasPrefix(line, "??") {
				untracked++
			} else if line != "" {
				modified++
			}
		}

		parts := []string{}
		if modified > 0 {
			parts = append(parts, fmt.Sprintf("%d modified", modified))
		}
		if untracked > 0 {
			parts = append(parts, fmt.Sprintf("%d untracked", untracked))
		}
		return gitStatusMsg(strings.Join(parts, ", "))
	}
}

// Update handles input.
func (m BranchDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.message = ""

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc", "backspace", "left", "h":
			// Back to home
			home := NewHomeModel()
			return home, home.Init()

		case "up":
			if m.urlCursor > 0 {
				m.urlCursor--
			}

		case "down":
			if m.urlCursor < len(m.urls)-1 {
				m.urlCursor++
			}

		case "enter", "o":
			// Open selected URL in browser
			if len(m.urls) > 0 {
				url := m.urls[m.urlCursor]
				openInBrowser(url)
				m.message = fmt.Sprintf("Opened %s", url)
			}

		case "c":
			// Open VS Code
			go openVSCode(m.branch)
			m.message = "Opening VS Code..."

		case "t":
			// Attach to tmux
			return m, tea.Sequence(
				tea.ExitAltScreen,
				func() tea.Msg { return attachTmuxMsg{} },
			)

		case "s":
			// Start branch
			if !m.branch.IsRunning() {
				m.message = "Starting..."
				return m, func() tea.Msg {
					if err := startBranchFull(m.branch); err != nil {
						return operationErrMsg{err}
					}
					return operationDoneMsg{"Started"}
				}
			}

		case "k":
			// Kill branch
			if m.branch.IsRunning() {
				m.message = "Killing..."
				return m, func() tea.Msg {
					if err := stopBranchFull(m.branch); err != nil {
						return operationErrMsg{err}
					}
					return operationDoneMsg{"Killed"}
				}
			}

		case "l":
			// View logs
			logs := NewLogViewerModel(m.branch)
			return logs, logs.Init()
		}

	case containerInfoMsg:
		m.containerInfo = string(msg)
		return m, nil

	case gitStatusMsg:
		m.gitStatus = string(msg)
		return m, nil

	case operationDoneMsg:
		m.message = msg.message
		return m, tea.Batch(m.loadContainerInfo(), m.loadGitStatus())

	case operationErrMsg:
		m.message = fmt.Sprintf("Error: %v", msg.err)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View renders the detail screen.
func (m BranchDetailModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render(fmt.Sprintf("── %s ──", m.branch.Name))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Status info
	statusStyle := stoppedStyle
	statusText := "stopped"
	if m.branch.IsRunning() {
		statusStyle = runningStyle
		statusText = "running"
	}

	b.WriteString(fmt.Sprintf("  Container: %s\n", m.containerInfo))
	b.WriteString(fmt.Sprintf("  Status:    %s\n", statusStyle.Render(statusText)))
	b.WriteString(fmt.Sprintf("  Git:       %s\n", m.gitStatus))
	b.WriteString("\n")

	// URLs section
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  URLS"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 50) + "\n")

	for i, url := range m.urls {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.urlCursor {
			cursor = "> "
			style = selectedStyle
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, style.Render(url)))
	}

	b.WriteString("\n")

	// Quick actions
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("  ACTIONS"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 50) + "\n")

	actions := "  [s]tart  [k]ill  [c]ode  [l]ogs  [t]mux  [o]pen url"
	b.WriteString(helpStyle.Render(actions))
	b.WriteString("\n\n")

	// Message
	if m.message != "" {
		b.WriteString("  " + m.message + "\n\n")
	}

	// Footer
	b.WriteString(helpStyle.Render("  ← back (esc)  q quit"))
	b.WriteString("\n")

	return b.String()
}

// Helper to open URL in browser
func openInBrowser(url string) {
	fullURL := "http://" + url
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", fullURL)
	case "linux":
		cmd = exec.Command("xdg-open", fullURL)
	default:
		return
	}
	cmd.Start()
}

// Format duration as "Xh Ym" or "Xm Ys"
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
