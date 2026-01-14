package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stachu/dark-multi/internal/branch"
	"github.com/stachu/dark-multi/internal/config"
	"github.com/stachu/dark-multi/internal/proxy"
	"github.com/stachu/dark-multi/internal/tmux"
)

// HomeModel is the main TUI model.
type HomeModel struct {
	branches     []*branch.Branch
	cursor       int
	proxyRunning bool
	width        int
	height       int
	message      string
	err          error
	quitting     bool
	loading      bool
}

// Messages
type branchesLoadedMsg []*branch.Branch
type proxyStatusMsg bool
type operationDoneMsg struct{ message string }
type operationErrMsg struct{ err error }
type attachTmuxMsg struct{}

// NewHomeModel creates a new home model.
func NewHomeModel() HomeModel {
	return HomeModel{
		loading: true,
	}
}

// Init initializes the model.
func (m HomeModel) Init() tea.Cmd {
	return tea.Batch(
		loadBranches,
		checkProxyStatus,
	)
}

func loadBranches() tea.Msg {
	return branchesLoadedMsg(branch.GetManagedBranches())
}

func checkProxyStatus() tea.Msg {
	_, running := proxy.IsRunning()
	return proxyStatusMsg(running)
}

// Update handles messages.
func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Clear any previous message/error on keypress
		m.message = ""
		m.err = nil

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}

		case "enter":
			// Attach to tmux session
			if tmux.SessionExists() {
				m.quitting = true
				return m, func() tea.Msg { return attachTmuxMsg{} }
			}
			m.message = "No tmux session. Start a branch first."

		case "s":
			// Start selected branch
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				if b.IsRunning() {
					m.message = fmt.Sprintf("%s is already running", b.Name)
				} else {
					m.loading = true
					m.message = fmt.Sprintf("Starting %s...", b.Name)
					return m, m.startBranch(b)
				}
			}

		case "S":
			// Stop selected branch
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = fmt.Sprintf("%s is already stopped", b.Name)
				} else {
					m.loading = true
					m.message = fmt.Sprintf("Stopping %s...", b.Name)
					return m, m.stopBranch(b)
				}
			}

		case "c":
			// Open VS Code for selected branch
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				return m, m.openCode(b)
			}

		case "p":
			// Toggle proxy
			if m.proxyRunning {
				m.message = "Stopping proxy..."
				return m, m.stopProxy()
			} else {
				m.message = "Starting proxy..."
				return m, m.startProxy()
			}

		case "r":
			// Refresh
			m.loading = true
			return m, loadBranches
		}

	case branchesLoadedMsg:
		m.branches = msg
		m.loading = false
		if m.cursor >= len(m.branches) {
			m.cursor = max(0, len(m.branches)-1)
		}
		return m, nil

	case proxyStatusMsg:
		m.proxyRunning = bool(msg)
		return m, nil

	case operationDoneMsg:
		m.message = msg.message
		m.loading = false
		return m, tea.Batch(loadBranches, checkProxyStatus)

	case operationErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case attachTmuxMsg:
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View renders the UI.
func (m HomeModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("DARK MULTI"))
	b.WriteString("\n\n")

	// Branches
	if len(m.branches) == 0 {
		b.WriteString(stoppedStyle.Render("  No branches yet. Create one with 'multi new <name>'"))
		b.WriteString("\n")
	} else {
		for i, br := range m.branches {
			cursor := "  "
			style := lipgloss.NewStyle()
			if i == m.cursor {
				cursor = "> "
				style = selectedStyle
			}

			// Running indicator
			indicator := stoppedStyle.Render("○")
			status := stoppedStyle.Render("stopped")
			if br.IsRunning() {
				indicator = runningStyle.Render("●")
				status = runningStyle.Render("running")
			}

			// Git status
			gitStatus := ""
			if br.HasChanges() {
				gitStatus = modifiedStyle.Render(" [modified]")
			}

			// Port info
			ports := fmt.Sprintf("ports %d+/%d+", br.PortBase(), br.BwdPortBase())

			line := fmt.Sprintf("%s%s %-12s  %-8s  %s%s",
				cursor, indicator, style.Render(br.Name), status, ports, gitStatus)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// System status
	cpuCores, ramGB := config.GetSystemResources()
	running := 0
	for _, br := range m.branches {
		if br.IsRunning() {
			running++
		}
	}
	maxSuggested := config.SuggestMaxInstances()
	proxyIndicator := stoppedStyle.Render("○ stopped")
	if m.proxyRunning {
		proxyIndicator = runningStyle.Render("● running")
	}
	statusLine := fmt.Sprintf("System: %d cores, %dGB RAM  •  %d/%d running  •  Proxy: %s",
		cpuCores, ramGB, running, maxSuggested, proxyIndicator)
	b.WriteString(statusBarStyle.Render(statusLine))
	b.WriteString("\n\n")

	// Message or error
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	} else if m.message != "" {
		b.WriteString(m.message)
		b.WriteString("\n")
	}

	// Help
	b.WriteString(helpStyle.Render("[s]tart  [S]top  [c]ode  [p]roxy  [r]efresh  [enter] tmux  [q]uit"))
	b.WriteString("\n")

	return b.String()
}

// Commands

func (m HomeModel) startBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		// This would call the start logic
		// For now, simplified version
		if err := startBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Started %s", b.Name)}
	}
}

func (m HomeModel) stopBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := stopBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Stopped %s", b.Name)}
	}
}

func (m HomeModel) openCode(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := openVSCode(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Opened VS Code for %s", b.Name)}
	}
}

func (m HomeModel) startProxy() tea.Cmd {
	return func() tea.Msg {
		_, err := proxy.Start(config.ProxyPort, true)
		if err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Proxy started on :%d", config.ProxyPort)}
	}
}

func (m HomeModel) stopProxy() tea.Cmd {
	return func() tea.Msg {
		if !proxy.Stop() {
			return operationErrMsg{fmt.Errorf("failed to stop proxy")}
		}
		return operationDoneMsg{"Proxy stopped"}
	}
}
