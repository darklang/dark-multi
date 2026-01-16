package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/claude"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/proxy"
	"github.com/darklang/dark-multi/tmux"
)

// InputMode represents the current input mode.
type InputMode int

const (
	InputNone InputMode = iota
	InputNewBranch
	InputConfirmDelete
)

// GitStatsInfo holds cached git stats for a branch.
type GitStatsInfo struct {
	Commits int
	Added   int
	Removed int
}

// PendingBranch tracks a branch being created.
type PendingBranch struct {
	Name   string
	Status string // "cloning", "starting", etc.
}

// HomeModel is the main TUI model.
type HomeModel struct {
	branches        []*branch.Branch
	pendingBranches map[string]*PendingBranch // branches being created
	claudeStatus    map[string]*claude.Status
	gitStats        map[string]*GitStatsInfo
	startupStatus   map[string]*branch.StartupStatus
	cursor          int
	proxyRunning    bool
	width           int
	height          int
	message         string
	err             error
	quitting        bool
	loading         bool
	inputMode       InputMode
	inputText       string
	spinner         spinner.Model
}

// Messages
type branchesLoadedMsg []*branch.Branch
type proxyStatusMsg bool
type claudeStatusMsg map[string]*claude.Status
type gitStatsMsg map[string]*GitStatsInfo
type startupStatusMsg map[string]*branch.StartupStatus
type tickMsg time.Time
type operationDoneMsg struct{ message string }
type operationErrMsg struct{ err error }
type attachTmuxMsg struct{}
type progressMsg struct{ message string }

// Pending operation for multi-step creates
type createStepMsg struct {
	name   string
	branch *branch.Branch
	step   int // 1=clone done, 2=start done
}

// NewHomeModel creates a new home model.
func NewHomeModel() HomeModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return HomeModel{
		loading:         true,
		spinner:         s,
		pendingBranches: make(map[string]*PendingBranch),
	}
}

// Init initializes the model.
func (m HomeModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
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
	if !running {
		// Auto-start proxy
		proxy.Start(config.ProxyPort, true)
		_, running = proxy.IsRunning()
	}
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

func loadGitStats(branches []*branch.Branch) tea.Cmd {
	return func() tea.Msg {
		stats := make(map[string]*GitStatsInfo)
		for _, b := range branches {
			commits, added, removed := b.GitStats()
			stats[b.Name] = &GitStatsInfo{
				Commits: commits,
				Added:   added,
				Removed: removed,
			}
		}
		return gitStatsMsg(stats)
	}
}

func loadStartupStatus(branches []*branch.Branch) tea.Cmd {
	return func() tea.Msg {
		statuses := make(map[string]*branch.StartupStatus)
		for _, b := range branches {
			if b.IsRunning() {
				status := b.GetStartupStatus()
				statuses[b.Name] = &status
			}
		}
		return startupStatusMsg(statuses)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages.
func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle input modes first
		if m.inputMode != InputNone {
			return m.handleInputMode(msg)
		}

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

		case "g":
			// Grid view - show all Claude sessions
			grid := NewGridModel()
			return grid, grid.Init()

		case "n":
			// New branch - enter input mode
			m.inputMode = InputNewBranch
			m.inputText = ""
			m.message = ""
			return m, nil

		case "d":
			// Open diff view (gitk)
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				return m, m.openDiff(b)
			}

		case "x":
			// Delete branch - enter confirmation mode
			if len(m.branches) > 0 {
				m.inputMode = InputConfirmDelete
				m.message = ""
				return m, nil
			}

		case "a":
			// Auth Claude for selected branch
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = "Start the branch first"
					return m, nil
				}
				auth := NewAuthModel(b)
				return auth, auth.Init()
			}

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
		// Load Claude status, git stats, and startup status after branches load
		return m, tea.Batch(loadClaudeStatus(m.branches), loadGitStats(m.branches), loadStartupStatus(m.branches))

	case proxyStatusMsg:
		m.proxyRunning = bool(msg)
		return m, nil

	case claudeStatusMsg:
		m.claudeStatus = msg
		return m, nil

	case gitStatsMsg:
		m.gitStats = msg
		return m, nil

	case startupStatusMsg:
		m.startupStatus = msg
		return m, nil

	case tickMsg:
		// Periodic refresh of Claude status, git stats, and startup status
		return m, tea.Batch(loadClaudeStatus(m.branches), loadGitStats(m.branches), loadStartupStatus(m.branches), tickCmd())

	case progressMsg:
		m.message = msg.message
		return m, nil

	case createStepMsg:
		if msg.step == 1 {
			// Clone done, now start
			if pending, ok := m.pendingBranches[msg.name]; ok {
				pending.Status = "starting container"
			}
			return m, startBranchStep(msg.branch, msg.name)
		}
		// Step 2: container started, check if auth needed
		if pending, ok := m.pendingBranches[msg.name]; ok {
			pending.Status = "checking auth"
		}
		return m, CheckAuthNeeded(msg.branch)

	case authNeededMsg:
		// Remove from pending
		delete(m.pendingBranches, msg.branch.Name)
		m.loading = false
		if msg.needed {
			// Auth needed - transition to auth view
			auth := NewAuthModel(msg.branch)
			return auth, auth.Init()
		}
		// No auth needed - done
		return m, tea.Batch(loadBranches, checkProxyStatus)

	case operationDoneMsg:
		m.message = msg.message
		m.loading = false
		return m, tea.Batch(loadBranches, checkProxyStatus)

	case operationErrMsg:
		// Remove from pending on error
		for name := range m.pendingBranches {
			delete(m.pendingBranches, name)
		}
		m.err = msg.err
		m.loading = false
		return m, nil

	case attachTmuxMsg:
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	default:
		// Handle spinner updates when loading or have pending branches
		if m.loading || len(m.pendingBranches) > 0 {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
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

	// Branches (including pending ones)
	if len(m.branches) == 0 && len(m.pendingBranches) == 0 {
		b.WriteString(stoppedStyle.Render("  No branches yet. Press 'n' to create one."))
		b.WriteString("\n")
	} else {
		// Find max branch name length for alignment (including pending)
		maxLen := 0
		for _, br := range m.branches {
			if len(br.Name) > maxLen {
				maxLen = len(br.Name)
			}
		}
		for _, pb := range m.pendingBranches {
			if len(pb.Name) > maxLen {
				maxLen = len(pb.Name)
			}
		}

		for i, br := range m.branches {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			// Running indicator with startup status
			indicator := stoppedStyle.Render("○")
			startupInfo := ""
			if br.IsRunning() {
				if ss, ok := m.startupStatus[br.Name]; ok && ss != nil && ss.Phase != branch.PhaseReady {
					// Show startup progress
					indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐") // orange half
					startupInfo = " " + helpStyle.Render(ss.Progress()+" "+ss.Description)
				} else {
					indicator = runningStyle.Render("●")
				}
			}

			// Branch name (padded, then styled if selected)
			name := fmt.Sprintf("%-*s", maxLen, br.Name)
			if i == m.cursor {
				name = selectedStyle.Render(name)
			}

			// Git stats (commits ahead, total +/- vs origin/main including uncommitted)
			var stats string
			if gs, ok := m.gitStats[br.Name]; ok && gs != nil {
				if gs.Commits > 0 || gs.Added > 0 || gs.Removed > 0 {
					parts := []string{}
					if gs.Commits > 0 {
						parts = append(parts, fmt.Sprintf("%dc", gs.Commits))
					}
					if gs.Added > 0 || gs.Removed > 0 {
						parts = append(parts, fmt.Sprintf("+%d -%d", gs.Added, gs.Removed))
					}
					stats = " " + strings.Join(parts, " ")
					stats = modifiedStyle.Render(stats)
				}
			}

			// Claude status with activity snippet
			claudeIndicator := ""
			if cs, ok := m.claudeStatus[br.Name]; ok && cs != nil {
				switch cs.State {
				case "waiting":
					claudeIndicator = " 💬" // Waiting for user input
				case "working":
					claudeIndicator = runningStyle.Render(" ⚡")
					// Show what Claude is doing
					if cs.LastTool != "" {
						claudeIndicator += " " + helpStyle.Render(cs.LastTool)
						if cs.LastMsg != "" {
							claudeIndicator += helpStyle.Render(": "+cs.LastMsg)
						}
					} else if cs.LastMsg != "" {
						claudeIndicator += " " + helpStyle.Render(cs.LastMsg)
					}
				}
			}

			suffix := startupInfo + stats + claudeIndicator

			b.WriteString(fmt.Sprintf("%s%s %s%s\n", cursor, indicator, name, suffix))
		}

		// Show pending branches (being created)
		for _, pb := range m.pendingBranches {
			// Check if already in branches list (avoid duplicates)
			found := false
			for _, br := range m.branches {
				if br.Name == pb.Name {
					found = true
					break
				}
			}
			if found {
				continue
			}
			indicator := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐")
			name := fmt.Sprintf("%-*s", maxLen, pb.Name)
			status := " " + helpStyle.Render(pb.Status)
			b.WriteString(fmt.Sprintf("  %s %s%s\n", indicator, name, status))
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

	// Input mode prompts
	switch m.inputMode {
	case InputNewBranch:
		b.WriteString(selectedStyle.Render("New branch name: "))
		b.WriteString(m.inputText)
		b.WriteString("█")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] create  [esc] cancel"))
		b.WriteString("\n")
		return b.String()

	case InputConfirmDelete:
		if len(m.branches) > 0 {
			br := m.branches[m.cursor]
			if br.HasChanges() {
				b.WriteString(errorStyle.Render(fmt.Sprintf("⚠ '%s' has uncommitted changes! ", br.Name)))
				b.WriteString("Delete anyway? [y/n]")
			} else {
				b.WriteString(fmt.Sprintf("Delete '%s'? [y/n]", br.Name))
			}
			b.WriteString("\n")
		}
		return b.String()
	}

	// Message or error (with spinner when loading)
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	} else if m.message != "" {
		if m.loading {
			b.WriteString(m.spinner.View())
			b.WriteString(" ")
		}
		b.WriteString(m.message)
		b.WriteString("\n")
	}

	// Help
	b.WriteString(helpStyle.Render("[n]ew  [x]del  [s]tart  [k]ill  [a]uth  [d]iff  [g]rid  [t]mux  [c]ode  [?]  [q]uit"))
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

func (m HomeModel) openDiff(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := openGitDiff(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{""}
	}
}

// handleInputMode handles keypresses during input modes.
func (m HomeModel) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.inputMode {
	case InputNewBranch:
		switch msg.String() {
		case "enter":
			if m.inputText == "" {
				m.inputMode = InputNone
				return m, nil
			}
			name := m.inputText
			m.inputMode = InputNone
			m.inputText = ""
			m.loading = true
			// Check if branch already exists and set initial status
			b := branch.New(name)
			if b.Exists() {
				m.pendingBranches[name] = &PendingBranch{Name: name, Status: "starting container"}
			} else {
				m.pendingBranches[name] = &PendingBranch{Name: name, Status: "cloning from GitHub"}
			}
			return m, m.createAndStartBranch(name)

		case "esc":
			m.inputMode = InputNone
			m.inputText = ""
			return m, nil

		case "backspace":
			if len(m.inputText) > 0 {
				m.inputText = m.inputText[:len(m.inputText)-1]
			}
			return m, nil

		default:
			// Only accept valid branch name characters
			key := msg.String()
			if len(key) == 1 && isValidBranchChar(key[0]) {
				m.inputText += key
			}
			return m, nil
		}

	case InputConfirmDelete:
		switch msg.String() {
		case "y", "Y":
			if len(m.branches) > 0 {
				b := m.branches[m.cursor]
				m.inputMode = InputNone
				m.loading = true
				m.message = fmt.Sprintf("Removing %s...", b.Name)
				return m, m.removeBranch(b)
			}
			m.inputMode = InputNone
			return m, nil

		case "n", "N", "esc":
			m.inputMode = InputNone
			m.message = "Cancelled"
			return m, nil

		default:
			return m, nil
		}
	}

	return m, nil
}

func isValidBranchChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

func (m HomeModel) createAndStartBranch(name string) tea.Cmd {
	return func() tea.Msg {
		b, err := createBranchFull(name)
		if err != nil {
			return operationErrMsg{err}
		}
		// Return step 1 done - UI will show progress and trigger step 2
		return createStepMsg{name: name, branch: b, step: 1}
	}
}

func startBranchStep(b *branch.Branch, name string) tea.Cmd {
	return func() tea.Msg {
		if err := startBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return createStepMsg{name: name, branch: b, step: 2}
	}
}

func (m HomeModel) removeBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := removeBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Removed %s", b.Name)}
	}
}
