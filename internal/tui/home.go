package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stachu/dark-multi/internal/branch"
	"github.com/stachu/dark-multi/internal/claude"
	"github.com/stachu/dark-multi/internal/config"
	"github.com/stachu/dark-multi/internal/proxy"
	"github.com/stachu/dark-multi/internal/tmux"
)

// HomeModel is the main TUI model.
type HomeModel struct {
	branches      []*branch.Branch
	claudeStatus  map[string]*claude.Status
	cursor        int
	proxyRunning  bool
	width         int
	height        int
	message       string
	err           error
	quitting      bool
	loading       bool
}

// Messages
type branchesLoadedMsg []*branch.Branch
type proxyStatusMsg bool
type claudeStatusMsg map[string]*claude.Status
type tickMsg time.Time
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
		tickCmd(),
	)
}

func loadBranches() tea.Msg {
	return branchesLoadedMsg(branch.GetManagedBranches())
}

func checkProxyStatus() tea.Msg {
	_, running := proxy.IsRunning()
	return proxyStatusMsg(running)
}

func loadClaudeStatus(branches []*branch.Branch) tea.Cmd {
	return func() tea.Msg {
		statuses := make(map[string]*claude.Status)
		for _, b := range branches {
			statuses[b.Name] = claude.GetStatus(b.Path)
		}
		return claudeStatusMsg(statuses)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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

		case "up":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down":
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}

		case "enter":
			// Go to branch detail view
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				detail := NewBranchDetailModel(b)
				return detail, detail.Init()
			}

		case "t":
			// Open selected branch in terminal
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = fmt.Sprintf("%s is not running", b.Name)
					return m, nil
				}
				// Create session if needed
				if !tmux.BranchSessionExists(b.Name) {
					containerID, _ := b.ContainerID()
					if err := tmux.CreateBranchSession(b.Name, containerID, b.Path); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
				}
				// Open in terminal
				if err := tmux.OpenBranchInTerminal(b.Name); err != nil {
					m.message = fmt.Sprintf("Error: %v", err)
				} else {
					m.message = fmt.Sprintf("Opened %s in terminal", b.Name)
				}
				return m, nil
			}

		case "s":
			// Start selected branch
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				if b.IsRunning() {
					m.message = fmt.Sprintf("%s is already running. Press 't' to open terminal.", b.Name)
				} else {
					m.loading = true
					m.message = fmt.Sprintf("Starting %s...", b.Name)
					return m, m.startBranch(b)
				}
			}

		case "k":
			// Kill selected branch
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = fmt.Sprintf("%s is already stopped", b.Name)
				} else {
					m.loading = true
					m.message = fmt.Sprintf("Killing %s...", b.Name)
					return m, m.stopBranch(b)
				}
			}

		case "m":
			// Open Matter (dark-packages canvas)
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				url := fmt.Sprintf("dark-packages.%s.dlio.localhost:%d/ping", b.Name, config.ProxyPort)
				openInBrowser(url)
				m.message = "Opened Matter"
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

		case "?":
			// Show help
			return NewHelpModel(), nil
		}

	case branchesLoadedMsg:
		m.branches = msg
		m.loading = false
		if m.cursor >= len(m.branches) {
			m.cursor = max(0, len(m.branches)-1)
		}
		// Also load Claude status after branches load
		return m, loadClaudeStatus(m.branches)

	case proxyStatusMsg:
		m.proxyRunning = bool(msg)
		return m, nil

	case claudeStatusMsg:
		m.claudeStatus = msg
		return m, nil

	case tickMsg:
		// Periodic refresh of Claude status
		return m, tea.Batch(loadClaudeStatus(m.branches), tickCmd())

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
		// Find max branch name length for alignment
		maxLen := 0
		for _, br := range m.branches {
			if len(br.Name) > maxLen {
				maxLen = len(br.Name)
			}
		}

		for i, br := range m.branches {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			// Running indicator
			indicator := stoppedStyle.Render("○")
			if br.IsRunning() {
				indicator = runningStyle.Render("●")
			}

			// Branch name (padded, then styled if selected)
			name := fmt.Sprintf("%-*s", maxLen, br.Name)
			if i == m.cursor {
				name = selectedStyle.Render(name)
			}

			// Git stats (commits, +/- vs main)
			var stats string
			commits, added, removed := br.GitStats()
			if commits > 0 || added > 0 || removed > 0 {
				stats = fmt.Sprintf(" %dc +%d -%d", commits, added, removed)
				stats = modifiedStyle.Render(stats)
			}

			// Claude status
			claudeIndicator := ""
			if cs, ok := m.claudeStatus[br.Name]; ok && cs != nil {
				switch cs.State {
				case "waiting":
					claudeIndicator = " ⏳"
				case "working":
					claudeIndicator = runningStyle.Render(" ⚡")
				}
			}

			suffix := stats + claudeIndicator

			b.WriteString(fmt.Sprintf("%s%s %s%s\n", cursor, indicator, name, suffix))
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
	b.WriteString(helpStyle.Render("[s]tart  [k]ill  [m]atter  [c]ode  [p]roxy  [t]mux  [enter] details  [?] help  [q]uit"))
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
