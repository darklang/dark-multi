package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/tmux"
)

var (
	cellBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("241"))

	cellSelectedStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("212"))

	cellHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	cellStoppedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Italic(true)

	// Package-level pending branches - survives model recreation during navigation
	globalPendingBranches = make(map[string]*PendingBranch)
)

// GridInputMode represents input modes.
type GridInputMode int

const (
	GridInputNone GridInputMode = iota
	GridInputNewBranch
	GridInputClaudePrompt
	GridInputConfirmDelete
)

// GridModel displays all Claude sessions in a grid layout.
type GridModel struct {
	branches       []*branch.Branch
	paneContent    map[string]string // branch name -> captured content
	cursor         int
	width          int
	height         int
	message        string
	err            error
	inputMode      GridInputMode
	inputText      string
	pendingName    string            // branch name being created (for prompt input)
	claudePrompts  map[string]string // branch name -> initial prompt for Claude
	proxyRunning   bool
	loading        bool
}

// Grid layout messages
type paneContentMsg map[string]string
type gridTickMsg time.Time
type sendClaudePromptMsg struct {
	branchName string
	prompt     string
}

// NewGridModel creates a new grid view.
func NewGridModel() GridModel {
	return GridModel{
		branches:      branch.GetManagedBranches(),
		paneContent:   make(map[string]string),
		claudePrompts: make(map[string]string),
	}
}

// Init initializes the grid model.
func (m GridModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadPaneContent,
		checkProxyStatus,
		gridTickCmd(),
	)
}

func gridTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return gridTickMsg(t)
	})
}

func (m GridModel) loadPaneContent() tea.Msg {
	content := make(map[string]string)
	for _, b := range m.branches {
		if b.IsRunning() && tmux.BranchSessionExists(b.Name) {
			content[b.Name] = tmux.CapturePaneContent(b.Name, 8)
		}
	}
	return paneContentMsg(content)
}

// Update handles messages.
func (m GridModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle input modes first
		if m.inputMode != GridInputNone {
			return m.handleInputMode(msg)
		}

		// Clear message on keypress
		m.message = ""
		m.err = nil

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "left", "h":
			if m.cursor > 0 {
				m.cursor--
			}

		case "right", "l":
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}

		case "up", "k":
			cols := m.numCols()
			if m.cursor >= cols {
				m.cursor -= cols
			}

		case "down", "j":
			cols := m.numCols()
			if m.cursor+cols < len(m.branches) {
				m.cursor += cols
			}

		case "enter":
			// Go to branch detail view
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				detail := NewBranchDetailModel(b)
				return detail, detail.Init()
			}

		case "t":
			// Open terminal for selected branch
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if b.IsRunning() {
					containerID, err := b.ContainerID()
					if err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
					if err := tmux.OpenTerminal(b.Name, containerID); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					}
				} else {
					m.message = fmt.Sprintf("%s is stopped - press 's' to start", b.Name)
				}
			}

		case "c":
			// Open Claude for selected branch
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if b.IsRunning() {
					containerID, err := b.ContainerID()
					if err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
					if err := tmux.OpenClaude(b.Name, containerID); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					}
				} else {
					m.message = fmt.Sprintf("%s is stopped - press 's' to start", b.Name)
				}
			}

		case "s":
			// Start selected branch
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = fmt.Sprintf("Starting %s...", b.Name)
					m.loading = true
					return m, m.startBranch(b)
				}
			}

		case "K":
			// Kill selected branch (capital K to avoid conflict with up)
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if b.IsRunning() {
					m.message = fmt.Sprintf("Killing %s...", b.Name)
					m.loading = true
					return m, m.stopBranch(b)
				}
			}

		case "n":
			// New branch
			m.inputMode = GridInputNewBranch
			m.inputText = ""
			return m, nil

		case "x":
			// Delete branch
			if len(m.branches) > 0 {
				m.inputMode = GridInputConfirmDelete
				return m, nil
			}

		case "e":
			// Open VS Code (editor)
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				return m, m.openCode(b)
			}

		case "m":
			// Open Matter
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				url := fmt.Sprintf("dark-packages.%s.dlio.localhost:%d/ping", b.Name, config.ProxyPort)
				openInBrowser(url)
				m.message = "Opened Matter"
			}

		case "d":
			// Open diff (gitk)
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				return m, m.openDiff(b)
			}

		case "a":
			// Auth Claude - kill any stuck tmux session first
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = "Start the branch first"
					return m, nil
				}
				// Kill existing tmux session if stuck on theme prompt or other bad state
				if tmux.BranchSessionExists(b.Name) {
					tmux.KillBranchSession(b.Name)
				}
				auth := NewAuthModel(b)
				return auth, auth.Init()
			}

		case "?":
			return NewHelpModel(), nil
		}

	case paneContentMsg:
		if msg != nil {
			m.paneContent = msg
		}
		return m, nil

	case proxyStatusMsg:
		m.proxyRunning = bool(msg)
		return m, nil

	case gridTickMsg:
		// Refresh branches and content periodically
		m.branches = branch.GetManagedBranches()
		if m.cursor >= len(m.branches) && len(m.branches) > 0 {
			m.cursor = len(m.branches) - 1
		}
		// Note: Don't clean up globalPendingBranches here - let authNeededMsg handle it
		// to avoid race conditions with the auth check flow
		return m, tea.Batch(m.loadPaneContent, gridTickCmd())

	case createStepMsg:
		if msg.step == 1 {
			if pending, ok := globalPendingBranches[msg.name]; ok {
				pending.Status = "starting container"
			}
			return m, startBranchStep(msg.branch, msg.name)
		}
		// Step 2: check auth
		if pending, ok := globalPendingBranches[msg.name]; ok {
			pending.Status = "checking auth"
		}
		return m, CheckAuthNeeded(msg.branch)

	case authNeededMsg:
		delete(globalPendingBranches, msg.branch.Name)
		m.loading = false
		m.branches = branch.GetManagedBranches()
		if msg.needed {
			auth := NewAuthModel(msg.branch)
			return auth, auth.Init()
		}
		// Auth OK - create tmux session if needed
		containerID, _ := msg.branch.ContainerID()
		if containerID != "" && !tmux.BranchSessionExists(msg.branch.Name) {
			tmux.CreateBranchSession(msg.branch.Name, containerID, msg.branch.Path)
		}
		// Check if we have a queued prompt for this branch
		if prompt, ok := m.claudePrompts[msg.branch.Name]; ok && prompt != "" {
			delete(m.claudePrompts, msg.branch.Name)
			// Wait for container to be ready then send prompt
			return m, tea.Batch(m.loadPaneContent, waitAndSendPrompt(msg.branch.Name, prompt))
		}
		return m, m.loadPaneContent

	case sendClaudePromptMsg:
		// Send the prompt to Claude
		if err := tmux.SendToClaudePane(msg.branchName, msg.prompt); err != nil {
			m.message = fmt.Sprintf("Error sending prompt: %v", err)
		} else {
			m.message = fmt.Sprintf("Sent prompt to %s", msg.branchName)
		}
		return m, nil

	case operationDoneMsg:
		m.message = msg.message
		m.loading = false
		m.branches = branch.GetManagedBranches()
		return m, m.loadPaneContent

	case operationErrMsg:
		for name := range globalPendingBranches {
			delete(globalPendingBranches, name)
		}
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m GridModel) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.inputMode {
	case GridInputNewBranch:
		switch msg.String() {
		case "enter":
			if m.inputText == "" {
				m.inputMode = GridInputNone
				return m, nil
			}
			name := m.inputText
			m.pendingName = name
			m.inputText = ""
			m.inputMode = GridInputClaudePrompt // Switch to prompt input
			return m, nil

		case "esc":
			m.inputMode = GridInputNone
			m.inputText = ""
			return m, nil

		case "backspace":
			if len(m.inputText) > 0 {
				m.inputText = m.inputText[:len(m.inputText)-1]
			}
			return m, nil

		default:
			key := msg.String()
			if len(key) == 1 && isValidBranchChar(key[0]) {
				m.inputText += key
			}
			return m, nil
		}

	case GridInputClaudePrompt:
		switch msg.String() {
		case "enter":
			name := m.pendingName
			prompt := m.inputText
			if prompt != "" {
				m.claudePrompts[name] = prompt
			}
			m.inputMode = GridInputNone
			m.inputText = ""
			m.pendingName = ""
			m.loading = true
			b := branch.New(name)
			if b.Exists() {
				globalPendingBranches[name] = &PendingBranch{Name: name, Status: "starting container"}
			} else {
				globalPendingBranches[name] = &PendingBranch{Name: name, Status: "cloning from GitHub"}
			}
			return m, m.createAndStartBranch(name)

		case "esc":
			// Cancel the whole operation
			m.inputMode = GridInputNone
			m.inputText = ""
			m.pendingName = ""
			return m, nil

		case "backspace":
			if len(m.inputText) > 0 {
				m.inputText = m.inputText[:len(m.inputText)-1]
			}
			return m, nil

		default:
			// Allow any printable character for the prompt
			key := msg.String()
			if len(key) == 1 {
				m.inputText += key
			} else if key == "space" {
				m.inputText += " "
			}
			return m, nil
		}

	case GridInputConfirmDelete:
		switch msg.String() {
		case "y", "Y":
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				m.inputMode = GridInputNone
				m.loading = true
				m.message = fmt.Sprintf("Removing %s...", b.Name)
				return m, m.removeBranch(b)
			}
			m.inputMode = GridInputNone
			return m, nil

		case "n", "N", "esc":
			m.inputMode = GridInputNone
			m.message = "Cancelled"
			return m, nil
		}
	}

	return m, nil
}

// filteredPendingBranches returns pending branches that don't overlap with existing branches
func (m GridModel) filteredPendingBranches() []*PendingBranch {
	var result []*PendingBranch
	for _, pb := range globalPendingBranches {
		found := false
		for _, br := range m.branches {
			if br.Name == pb.Name {
				found = true
				break
			}
		}
		if !found {
			result = append(result, pb)
		}
	}
	return result
}

func (m GridModel) numCols() int {
	pending := m.filteredPendingBranches()
	n := len(m.branches) + len(pending)
	if n == 0 {
		return 1
	}
	// 2 rows, ceil(n/2) columns
	return (n + 1) / 2
}

// View renders the grid.
func (m GridModel) View() string {
	var b strings.Builder

	pendingBranches := m.filteredPendingBranches()
	totalBranches := len(m.branches) + len(pendingBranches)

	// Handle input modes
	if m.inputMode == GridInputNewBranch {
		b.WriteString(titleStyle.Render("NEW BRANCH"))
		b.WriteString("\n\n")
		b.WriteString(selectedStyle.Render("Name: "))
		b.WriteString(m.inputText)
		b.WriteString("█\n\n")
		b.WriteString(helpStyle.Render("[enter] continue  [esc] cancel"))
		return b.String()
	}

	if m.inputMode == GridInputClaudePrompt {
		b.WriteString(titleStyle.Render(fmt.Sprintf("NEW BRANCH: %s", m.pendingName)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Container will start building. Enter a prompt for Claude:\n"))
		b.WriteString(helpStyle.Render("(leave empty to skip, or type your task)\n\n"))
		b.WriteString(selectedStyle.Render("Prompt: "))
		b.WriteString(m.inputText)
		b.WriteString("█\n\n")
		b.WriteString(helpStyle.Render("[enter] start  [esc] cancel"))
		return b.String()
	}

	if m.inputMode == GridInputConfirmDelete {
		b.WriteString(titleStyle.Render("DELETE BRANCH"))
		b.WriteString("\n\n")
		if len(m.branches) > 0 && m.cursor < len(m.branches) {
			br := m.branches[m.cursor]
			if br.HasChanges() {
				b.WriteString(errorStyle.Render(fmt.Sprintf("⚠ '%s' has uncommitted changes!\n", br.Name)))
			}
			b.WriteString(fmt.Sprintf("Delete '%s'? [y/n]", br.Name))
		}
		return b.String()
	}

	if totalBranches == 0 {
		b.WriteString(titleStyle.Render("DARK MULTI"))
		b.WriteString("\n\n")
		b.WriteString(stoppedStyle.Render("No branches. Press 'n' to create one."))
		b.WriteString("\n\n")
		b.WriteString(m.renderStatusBar())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[n]ew  [?]help  [q]uit"))
		return b.String()
	}

	// Calculate cell dimensions
	cols := m.numCols()
	width := m.width
	if width < 40 {
		width = 120
	}
	height := m.height
	if height < 10 {
		height = 40
	}

	// Reserve 2 lines for status bar and help
	cellHeight := (height - 2) / 2

	// Build rows (2 rows)
	var rows []string
	branchIdx := 0
	for row := 0; row < 2; row++ {
		var cells []string
		remainingWidth := width
		for col := 0; col < cols; col++ {
			cellWidth := remainingWidth / (cols - col)
			remainingWidth -= cellWidth

			if branchIdx < len(m.branches) {
				cells = append(cells, m.renderCell(branchIdx, cellWidth, cellHeight))
				branchIdx++
			} else {
				// Render pending branches (filtered to avoid duplicates)
				pendingIdx := branchIdx - len(m.branches)
				if pendingIdx < len(pendingBranches) {
					cells = append(cells, m.renderPendingCell(pendingBranches[pendingIdx], cellWidth, cellHeight))
				} else {
					// Empty cell
					cells = append(cells, cellBorderStyle.Width(cellWidth-2).Height(cellHeight-2).Render(""))
				}
				branchIdx++
			}
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))

	// Status bar
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Message or help
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	} else if m.message != "" {
		b.WriteString(m.message)
	} else {
		b.WriteString(helpStyle.Render("[n]ew [x]del [s]tart [K]ill [a]uth [d]iff [t]mux [c]ode [m]atter [?]help [q]uit"))
	}

	return b.String()
}

func (m GridModel) renderStatusBar() string {
	cpuCores, ramGB := config.GetSystemResources()
	running := 0
	for _, br := range m.branches {
		if br.IsRunning() {
			running++
		}
	}
	maxSuggested := config.SuggestMaxInstances()
	proxyStatus := stoppedStyle.Render("○")
	if m.proxyRunning {
		proxyStatus = runningStyle.Render("●")
	}
	return statusBarStyle.Render(fmt.Sprintf("%d cores, %dGB  •  %d/%d running  •  proxy %s",
		cpuCores, ramGB, running, maxSuggested, proxyStatus))
}

func (m GridModel) renderCell(idx int, width, height int) string {
	innerWidth := width - 2
	innerHeight := height - 2

	if idx >= len(m.branches) {
		return cellBorderStyle.Width(innerWidth).Height(innerHeight).Render("")
	}

	br := m.branches[idx]
	selected := idx == m.cursor

	// Check if this branch has a pending operation
	if pending, ok := globalPendingBranches[br.Name]; ok {
		// Show pending status instead of normal content
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐") + " " + cellHeaderStyle.Render(br.Name)
		content := helpStyle.Render(pending.Status)
		style := cellBorderStyle
		if selected {
			style = cellSelectedStyle
		}
		return style.Width(innerWidth).Height(innerHeight).Render(header + "\n" + content)
	}

	// Header
	var header string
	if br.IsRunning() {
		header = runningStyle.Render("●") + " " + cellHeaderStyle.Render(br.Name)
	} else {
		header = stoppedStyle.Render("○") + " " + cellHeaderStyle.Render(br.Name)
	}

	// Content
	var content string
	if br.IsRunning() {
		// Check if Claude is authenticated (look for oauthAccount in .claude.json)
		needsAuth := false
		if containerID, err := br.ContainerID(); err == nil {
			authCmd := exec.Command("docker", "exec", containerID, "grep", "-q", "oauthAccount", "/home/dark/.claude.json")
			needsAuth = authCmd.Run() != nil
		}

		if needsAuth {
			content = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[needs auth - press 'a']")
		} else if !tmux.BranchSessionExists(br.Name) {
			content = stoppedStyle.Render("[ready - press 't' for terminal]")
		} else if pane, ok := m.paneContent[br.Name]; ok && pane != "" {
			// Check if pane is stuck on theme prompt (bad state)
			if strings.Contains(pane, "Dark mode") && strings.Contains(pane, "Light mode") {
				content = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("[auth stuck - press 'a' to fix]")
			} else {
				lines := strings.Split(pane, "\n")
				maxLines := innerHeight - 1
				if len(lines) > maxLines {
					lines = lines[len(lines)-maxLines:]
				}
				for i, line := range lines {
					if len(line) > innerWidth {
						lines[i] = line[:innerWidth-1] + "…"
					}
				}
				content = strings.Join(lines, "\n")
			}
		} else {
			content = stoppedStyle.Render("[capturing...]")
		}
	} else {
		content = cellStoppedStyle.Render("[stopped]")
	}

	cellContent := header + "\n" + content

	style := cellBorderStyle
	if selected {
		style = cellSelectedStyle
	}

	return style.Width(innerWidth).Height(innerHeight).Render(cellContent)
}

func (m GridModel) renderPendingCell(pb *PendingBranch, width, height int) string {
	innerWidth := width - 2
	innerHeight := height - 2

	header := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐") + " " + cellHeaderStyle.Render(pb.Name)
	content := helpStyle.Render(pb.Status)

	return cellBorderStyle.Width(innerWidth).Height(innerHeight).Render(header + "\n" + content)
}

// Commands

func (m GridModel) startBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := startBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Started %s", b.Name)}
	}
}

func (m GridModel) stopBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := stopBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Stopped %s", b.Name)}
	}
}

func (m GridModel) openCode(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := openVSCode(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{""}
	}
}

func (m GridModel) openDiff(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := openGitDiff(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{""}
	}
}

func (m GridModel) createAndStartBranch(name string) tea.Cmd {
	return func() tea.Msg {
		b, err := createBranchFull(name)
		if err != nil {
			return operationErrMsg{err}
		}
		return createStepMsg{name: name, branch: b, step: 1}
	}
}

func (m GridModel) removeBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := removeBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Removed %s", b.Name)}
	}
}

// waitAndSendPrompt waits for a branch to be ready, then sends a prompt to Claude.
func waitAndSendPrompt(branchName string, prompt string) tea.Cmd {
	return func() tea.Msg {
		b := branch.New(branchName)
		// Wait up to 10 minutes for the container to be ready
		maxWait := 600
		for i := 0; i < maxWait; i++ {
			status := b.GetStartupStatus()
			if status.Phase == branch.PhaseReady {
				// Give Claude a moment to fully initialize
				time.Sleep(3 * time.Second)
				return sendClaudePromptMsg{branchName: branchName, prompt: prompt}
			}
			time.Sleep(1 * time.Second)
		}
		// Timeout - try sending anyway
		return sendClaudePromptMsg{branchName: branchName, prompt: prompt}
	}
}
