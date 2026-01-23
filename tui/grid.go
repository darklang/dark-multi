package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/queue"
	"github.com/darklang/dark-multi/summary"
	"github.com/darklang/dark-multi/task"
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

// cellStyleForStatus returns a subtle border color based on queue status.
func cellStyleForStatus(status queue.Status, selected bool) lipgloss.Style {
	// Subtle border colors based on status
	var borderColor string
	switch status {
	case queue.StatusDone:
		borderColor = "34" // subtle green
	case queue.StatusRunning:
		borderColor = "33" // subtle blue
	case queue.StatusWaiting:
		borderColor = "130" // subtle orange/yellow
	case queue.StatusReady:
		borderColor = "241" // default gray
	case queue.StatusNeedsPrompt:
		borderColor = "241" // default gray
	case queue.StatusPaused:
		borderColor = "241" // default gray
	default:
		borderColor = "241"
	}

	if selected {
		// Brighter version when selected
		switch status {
		case queue.StatusDone:
			borderColor = "42" // brighter green
		case queue.StatusRunning:
			borderColor = "39" // brighter blue
		case queue.StatusWaiting:
			borderColor = "214" // brighter orange
		default:
			borderColor = "212" // pink (default selected)
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor))
}

// GridInputMode represents input modes.
type GridInputMode int

const (
	GridInputNone GridInputMode = iota
	GridInputNewBranch
	GridInputConfirmDelete
)

// ContainerStats holds CPU/memory usage for a container.
type ContainerStats struct {
	CPU    string // e.g., "12.5%"
	Memory string // e.g., "1.2GB"
}

// TaskInfo holds cached task information for display.
type TaskInfo struct {
	Phase      task.Phase
	StatusLine string // e.g., "3/7 todos"
	Summary    string // AI-generated summary of current activity
}

// GridModel displays all Claude sessions in a grid layout.
type GridModel struct {
	branches        []*branch.Branch
	queueTasks      []*queue.Task             // all tasks from queue
	paneContent     map[string]string         // branch name -> captured content
	containerStats  map[string]ContainerStats // branch name -> stats
	gitStats        map[string]*GitStatsInfo  // cached git stats
	runningState    map[string]bool           // cached IsRunning state
	taskInfo        map[string]*TaskInfo      // cached task info
	cursor          int
	width           int
	height          int
	message         string
	err             error
	inputMode       GridInputMode
	inputText       string
	proxyRunning    bool
	loading         bool
	statusFilter    []queue.Status            // filter by these statuses (empty = show all)
	processorOn     bool                      // queue processor running
}

// Grid layout messages
type paneContentMsg map[string]string
type containerStatsMsg map[string]ContainerStats
type runningStateMsg map[string]bool
type taskInfoMsg map[string]*TaskInfo
type queueTasksMsg []*queue.Task
type gridTickMsg time.Time

// NewGridModel creates a new grid view.
func NewGridModel() GridModel {
	// Start the queue processor
	queue.StartProcessor()

	// Run health check on startup
	issues := queue.RunHealthCheck()
	var startupMessage string
	if len(issues) > 0 {
		// Summarize issues
		var warnings, errors int
		for _, issue := range issues {
			if issue.Severity == "error" {
				errors++
			} else if issue.Severity == "warning" {
				warnings++
			}
		}
		if errors > 0 || warnings > 0 {
			startupMessage = fmt.Sprintf("Health check: %d errors, %d warnings", errors, warnings)
		}
		// Auto-fix what we can
		queue.AutoFix(issues)
	}

	// Default filter: show running + ready (most actionable)
	defaultFilter := []queue.Status{queue.StatusRunning, queue.StatusReady}

	return GridModel{
		branches:       branch.GetManagedBranches(),
		queueTasks:     queue.Get().GetAll(),
		paneContent:    make(map[string]string),
		containerStats: make(map[string]ContainerStats),
		gitStats:       make(map[string]*GitStatsInfo),
		runningState:   make(map[string]bool),
		taskInfo:       make(map[string]*TaskInfo),
		statusFilter:   defaultFilter,
		processorOn:    true,
		message:        startupMessage,
	}
}

// Init initializes the grid model.
func (m GridModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadPaneContent,
		loadContainerStats,
		m.loadGridGitStats,
		m.loadRunningState,
		m.loadTaskInfo,
		loadQueueTasks,
		checkProxyStatus,
		gridTickCmd(),
	)
}

func loadQueueTasks() tea.Msg {
	return queueTasksMsg(queue.Get().GetAll())
}

func (m GridModel) loadTaskInfo() tea.Msg {
	info := make(map[string]*TaskInfo)
	for _, b := range m.branches {
		t := task.New(b.Name, b.Path)
		phase := t.Phase()
		ti := &TaskInfo{
			Phase:      phase,
			StatusLine: t.StatusLine(),
		}
		// Get AI summary for executing branches
		if phase == task.PhaseExecuting {
			ti.Summary = summary.GetSummary(b.Name)
		}
		info[b.Name] = ti
	}
	return taskInfoMsg(info)
}

func (m GridModel) loadRunningState() tea.Msg {
	state := make(map[string]bool)
	for _, b := range m.branches {
		state[b.Name] = b.IsRunning()
	}
	return runningStateMsg(state)
}

func (m GridModel) loadGridGitStats() tea.Msg {
	stats := make(map[string]*GitStatsInfo)
	for _, b := range m.branches {
		commits, added, removed := b.GitStats()
		stats[b.Name] = &GitStatsInfo{
			Commits: commits,
			Added:   added,
			Removed: removed,
		}
	}
	return gitStatsMsg(stats)
}

func gridTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
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

func loadContainerStats() tea.Msg {
	stats := make(map[string]ContainerStats)
	// Get stats for all dark- containers in one call
	out, err := exec.Command("docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}").Output()
	if err != nil {
		return containerStatsMsg(stats)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 && strings.HasPrefix(fields[0], "dark-") {
			name := strings.TrimPrefix(fields[0], "dark-")
			// Parse memory - just take the used part (before " / ")
			mem := fields[2]
			if idx := strings.Index(mem, " / "); idx > 0 {
				mem = mem[:idx]
			}
			stats[name] = ContainerStats{
				CPU:    fields[1],
				Memory: mem,
			}
		}
	}
	return containerStatsMsg(stats)
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

		case "left":
			if m.cursor > 0 {
				m.cursor--
			}

		case "right":
			tasks := m.filteredTasks()
			if m.cursor < len(tasks)-1 {
				m.cursor++
			}

		case "up":
			_, cols := m.gridDimensions()
			if m.cursor >= cols {
				m.cursor -= cols
			}

		case "down":
			tasks := m.filteredTasks()
			_, cols := m.gridDimensions()
			if m.cursor+cols < len(tasks) {
				m.cursor += cols
			}

		case "enter":
			// Open Claude for selected task (same as 'c')
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if b.Exists() && b.IsRunning() {
					containerID, err := b.ContainerID()
					if err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
					if err := tmux.OpenClaude(b.Name, containerID); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					}
				} else {
					m.message = fmt.Sprintf("%s is not running - press 's' to start", t.ID)
				}
			}

		case "t":
			// Open terminal for selected task
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if b.Exists() && b.IsRunning() {
					containerID, err := b.ContainerID()
					if err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
					if err := tmux.OpenTerminal(b.Name, containerID); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					}
				} else {
					m.message = fmt.Sprintf("%s is not running - press 's' to start", t.ID)
				}
			}

		case "c":
			// Open Claude for selected task
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				qt := tasks[m.cursor]
				b := branch.New(qt.ID)
				if b.Exists() && b.IsRunning() {
					// Inject task context if there's an active task
					t := task.New(b.Name, b.Path)
					if t.Exists() {
						phase := t.Phase()
						if phase == task.PhasePlanning || phase == task.PhaseReady || phase == task.PhaseExecuting {
							t.InjectTaskContext()
							// Also ensure .claude-task dir exists
							t.EnsureClaudeTaskDir()
						}
					}

					containerID, err := b.ContainerID()
					if err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
					if err := tmux.OpenClaude(b.Name, containerID); err != nil {
						m.message = fmt.Sprintf("Error opening Claude: %v", err)
					} else {
						m.message = fmt.Sprintf("Opened Claude terminal for %s", b.Name)
					}
				} else {
					m.message = fmt.Sprintf("%s is not running - press 's' to start", qt.ID)
				}
			}

		case "s":
			// Start selected task
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if b.Exists() && b.IsRunning() {
					m.message = fmt.Sprintf("%s is already running", t.ID)
				} else if b.Exists() {
					globalPendingBranches[b.Name] = &PendingBranch{Name: b.Name, Status: "starting container"}
					m.loading = true
					return m, m.startBranch(b)
				} else {
					// Branch doesn't exist yet - create and start
					globalPendingBranches[t.ID] = &PendingBranch{Name: t.ID, Status: "creating branch"}
					m.loading = true
					return m, m.createAndStartBranch(t.ID)
				}
			}

		case "k":
			// Kill (stop) selected task
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if !b.Exists() || !b.IsRunning() {
					m.message = fmt.Sprintf("%s is not running", t.ID)
				} else {
					m.message = fmt.Sprintf("Killing %s...", t.ID)
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
			// Delete task/branch
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				m.inputMode = GridInputConfirmDelete
				return m, nil
			}

		case "e":
			// Open VS Code (editor)
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if !b.Exists() || !b.IsRunning() {
					m.message = fmt.Sprintf("%s is not running - press 's' to start", t.ID)
					return m, nil
				}
				return m, m.openCode(b)
			}

		case "m":
			// Open Matter
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				url := fmt.Sprintf("dark-packages.%s.dlio.localhost:%d/ping", t.ID, config.ProxyPort)
				openInBrowser(url)
				m.message = "Opened Matter"
			}

		case "d":
			// Open diff (gitk)
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if b.Exists() {
					return m, m.openDiff(b)
				}
				m.message = fmt.Sprintf("%s branch not created yet", t.ID)
			}

		case "l":
			// View logs
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				b := branch.New(t.ID)
				if b.Exists() {
					logs := NewLogViewerModel(b)
					return logs, logs.Init()
				}
				m.message = fmt.Sprintf("%s branch not created yet", t.ID)
			}

		case "p":
			// Edit pre-prompt (task definition)
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				qt := tasks[m.cursor]
				q := queue.Get()

				// Edit the queue task's prompt
				editor := findEditor()
				// Write prompt to temp file for editing
				tmpFile := fmt.Sprintf("/tmp/dark-multi-prompt-%s.md", qt.ID)
				content := qt.Prompt
				if content == "" {
					content = fmt.Sprintf("# Task: %s\n\n[Write your prompt here]\n", qt.Name)
				}
				if err := writeFile(tmpFile, content); err != nil {
					m.message = fmt.Sprintf("Error: %v", err)
					return m, nil
				}

				c := exec.Command(editor, tmpFile)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					if err != nil {
						return operationErrMsg{err}
					}
					// Read back the edited prompt
					newContent, err := readFile(tmpFile)
					if err != nil {
						return operationErrMsg{err}
					}
					if newContent != "" && newContent != content {
						q.SetPrompt(qt.ID, newContent)
						q.Save()
						return operationDoneMsg{fmt.Sprintf("Prompt updated for %s", qt.ID)}
					}
					return operationDoneMsg{"Prompt unchanged"}
				})
			}

		case "f":
			// Open filter modal
			filter := NewFilterModel(m)
			return filter, filter.Init()

		case "Q":
			// Toggle queue processor (Shift+Q)
			if queue.IsProcessorRunning() {
				queue.StopProcessor()
				m.processorOn = false
				m.message = "Queue processor stopped (manual mode)"
			} else {
				queue.StartProcessor()
				m.processorOn = true
				m.message = "Queue processor started (auto mode)"
			}

		case "v":
			// Open focus view for selected task
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				focus := NewFocusModel(t, m)
				return focus, focus.Init()
			}

		case "i":
			// Open detail/info view for selected task
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				detail := NewDetailModel(t, m)
				return detail, detail.Init()
			}

		case "?":
			return NewHelpModel(), nil
		}

	case paneContentMsg:
		if msg != nil {
			m.paneContent = msg
		}
		return m, nil

	case containerStatsMsg:
		if msg != nil {
			m.containerStats = msg
		}
		return m, nil

	case gitStatsMsg:
		m.gitStats = msg
		return m, nil

	case runningStateMsg:
		m.runningState = msg
		return m, nil

	case taskInfoMsg:
		m.taskInfo = msg
		return m, nil

	case proxyStatusMsg:
		m.proxyRunning = bool(msg)
		return m, nil

	case queueTasksMsg:
		m.queueTasks = msg
		return m, nil

	case gridTickMsg:
		// Refresh branches and content periodically
		m.branches = branch.GetManagedBranches()
		m.queueTasks = queue.Get().GetAll()
		m.processorOn = queue.IsProcessorRunning()
		// Keep cursor in bounds (grid shows filtered tasks)
		tasks := m.filteredTasks()
		if m.cursor >= len(tasks) && len(tasks) > 0 {
			m.cursor = len(tasks) - 1
		}
		// Note: Don't clean up globalPendingBranches here - let branchStartedMsg handle it
		return m, tea.Batch(m.loadPaneContent, loadContainerStats, m.loadGridGitStats, m.loadRunningState, m.loadTaskInfo, loadQueueTasks, gridTickCmd())

	case createStepMsg:
		if pending, ok := globalPendingBranches[msg.name]; ok {
			pending.Status = "starting container"
		}
		return m, startBranchStep(msg.branch, msg.name)

	case branchStartedMsg:
		delete(globalPendingBranches, msg.name)
		m.loading = false
		m.branches = branch.GetManagedBranches()
		return m, m.loadPaneContent

	case operationDoneMsg:
		m.message = msg.message
		m.loading = false
		m.branches = branch.GetManagedBranches()
		// Clean up any pending branches that are now running
		for _, b := range m.branches {
			if b.IsRunning() {
				delete(globalPendingBranches, b.Name)
			}
		}
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
			m.inputMode = GridInputNone
			m.inputText = ""

			// Add to queue (doesn't start it yet)
			q := queue.Get()
			if existing := q.Get(name); existing != nil {
				m.message = fmt.Sprintf("Task '%s' already exists", name)
				return m, nil
			}
			q.Add(name, name, "", 50) // Empty prompt, needs-prompt status
			q.Save()
			m.queueTasks = q.GetAll()
			m.message = fmt.Sprintf("Added task '%s' - press 'p' to set prompt, 's' to start", name)
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

	case GridInputConfirmDelete:
		switch msg.String() {
		case "y", "Y":
			tasks := m.filteredTasks()
			if len(tasks) > 0 && m.cursor < len(tasks) {
				t := tasks[m.cursor]
				m.inputMode = GridInputNone
				m.loading = true
				m.message = fmt.Sprintf("Removing %s...", t.ID)
				return m, m.removeTask(t)
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

// filteredTasks returns queue tasks filtered by status.
func (m GridModel) filteredTasks() []*queue.Task {
	if len(m.statusFilter) == 0 {
		return m.queueTasks
	}

	filterSet := make(map[queue.Status]bool)
	for _, s := range m.statusFilter {
		filterSet[s] = true
	}

	var result []*queue.Task
	for _, t := range m.queueTasks {
		if filterSet[t.Status] {
			result = append(result, t)
		}
	}
	return result
}

// nextFilter cycles through filter presets.
func (m GridModel) nextFilter() []queue.Status {
	// Filter presets to cycle through
	presets := [][]queue.Status{
		{queue.StatusRunning},                                        // Running only
		{queue.StatusRunning, queue.StatusReady},                     // Running + Ready
		{queue.StatusRunning, queue.StatusReady, queue.StatusWaiting}, // Active
		{},                                                           // All
	}

	// Find current preset
	currentKey := filterKey(m.statusFilter)
	for i, preset := range presets {
		if filterKey(preset) == currentKey {
			return presets[(i+1)%len(presets)]
		}
	}
	return presets[0]
}

func filterKey(statuses []queue.Status) string {
	var parts []string
	for _, s := range statuses {
		parts = append(parts, string(s))
	}
	return strings.Join(parts, ",")
}

// filterDescription returns a human-readable description of current filter.
func (m GridModel) filterDescription() string {
	if len(m.statusFilter) == 0 {
		return "all"
	}
	if len(m.statusFilter) == 1 {
		return string(m.statusFilter[0])
	}
	return fmt.Sprintf("%d statuses", len(m.statusFilter))
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

// isRunning returns cached running state for a branch
func (m GridModel) isRunning(name string) bool {
	if running, ok := m.runningState[name]; ok {
		return running
	}
	return false
}

// gridDimensions calculates optimal rows and cols for the grid.
func (m GridModel) gridDimensions() (rows, cols int) {
	pending := m.filteredPendingBranches()
	tasks := m.filteredTasks()
	n := len(tasks) + len(pending)
	if n == 0 {
		return 1, 1
	}

	// Get available space (reserve 5 lines for status/help)
	availHeight := m.height - 5
	if availHeight < 10 {
		availHeight = 35
	}
	availWidth := m.width
	if availWidth < 40 {
		availWidth = 120
	}

	// Minimum cell dimensions for readability
	minCellWidth := 40
	minCellHeight := 8

	// Calculate max possible rows and cols
	maxRows := availHeight / minCellHeight
	maxCols := availWidth / minCellWidth

	if maxRows < 1 {
		maxRows = 1
	}
	if maxCols < 1 {
		maxCols = 1
	}

	// Find optimal grid that fits all items with balanced aspect ratio
	// Try to fill screen while keeping cells readable
	for rows = 1; rows <= maxRows; rows++ {
		cols = (n + rows - 1) / rows // ceiling division
		if cols <= maxCols {
			// Check if cells would be too wide (prefer more rows for balance)
			cellWidth := availWidth / cols
			if cellWidth > 80 && rows < maxRows && rows*2 >= n {
				continue // Try more rows for better balance
			}
			return rows, cols
		}
	}

	// Fallback: use max rows
	return maxRows, (n + maxRows - 1) / maxRows
}

// View renders the grid.
func (m GridModel) View() string {
	var b strings.Builder

	pendingBranches := m.filteredPendingBranches()
	tasks := m.filteredTasks()
	totalItems := len(tasks) + len(pendingBranches)

	// Handle input modes
	if m.inputMode == GridInputNewBranch {
		b.WriteString(titleStyle.Render("NEW TASK"))
		b.WriteString("\n\n")
		b.WriteString(selectedStyle.Render("Name: "))
		b.WriteString(m.inputText)
		b.WriteString("█\n\n")
		b.WriteString(helpStyle.Render("[enter] create  [esc] cancel"))
		return b.String()
	}

	if m.inputMode == GridInputConfirmDelete {
		b.WriteString(titleStyle.Render("DELETE TASK"))
		b.WriteString("\n\n")
		if len(tasks) > 0 && m.cursor < len(tasks) {
			t := tasks[m.cursor]
			br := branch.New(t.ID)
			if br.Exists() && br.HasChanges() {
				b.WriteString(errorStyle.Render(fmt.Sprintf("⚠ '%s' has uncommitted changes!\n", t.ID)))
			}
			b.WriteString(fmt.Sprintf("Delete '%s'? [y/n]", t.ID))
		}
		return b.String()
	}

	if totalItems == 0 {
		b.WriteString(titleStyle.Render("DARK MULTI"))
		b.WriteString("\n\n")
		b.WriteString(stoppedStyle.Render("No tasks. Press 'n' to create one or run 'multi queue init'."))
		b.WriteString("\n\n")
		b.WriteString(m.renderStatusBar())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[n]ew  [f]ilter  [?]help  [q]uit"))
		return b.String()
	}

	// Calculate grid dimensions
	numRows, numCols := m.gridDimensions()
	width := m.width
	if width < 40 {
		width = 120
	}
	height := m.height
	if height < 10 {
		height = 40
	}

	// Reserve 5 lines for newline, status bar, newline, and help/message
	cellHeight := (height - 5) / numRows
	if cellHeight < 6 {
		cellHeight = 6
	}

	// Build rows dynamically
	var rows []string
	itemIdx := 0
	for row := 0; row < numRows; row++ {
		var cells []string
		remainingWidth := width
		for col := 0; col < numCols; col++ {
			cellWidth := remainingWidth / (numCols - col)
			remainingWidth -= cellWidth

			if itemIdx < len(tasks) {
				cells = append(cells, m.renderTaskCell(tasks[itemIdx], itemIdx, cellWidth, cellHeight))
				itemIdx++
			} else {
				// Render pending branches (filtered to avoid duplicates)
				pendingIdx := itemIdx - len(tasks)
				if pendingIdx < len(pendingBranches) {
					cells = append(cells, m.renderPendingCell(pendingBranches[pendingIdx], cellWidth, cellHeight))
				} else {
					// Empty cell
					cells = append(cells, cellBorderStyle.Width(cellWidth-2).Height(cellHeight-2).Render(""))
				}
				itemIdx++
			}
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))

	// Status bar
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Message or help
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	} else if m.message != "" {
		b.WriteString(m.message)
	} else {
		b.WriteString(helpStyle.Render("[n]ew [x]del [s]tart [k]ill [c]laude [t]erm [v]iew [i]nfo [p]rompt [f]ilter [?] [q]"))
	}

	return b.String()
}

func (m GridModel) renderStatusBar() string {
	cpuCores, ramGB := config.GetSystemResources()

	// Count running from queue tasks
	q := queue.Get()
	running := q.CountRunning()

	// Calculate total CPU and RAM usage
	var totalCPU float64
	var totalMemMB float64
	for _, stats := range m.containerStats {
		// Parse CPU like "12.5%"
		var cpu float64
		fmt.Sscanf(strings.TrimSuffix(stats.CPU, "%"), "%f", &cpu)
		totalCPU += cpu
		// Parse memory like "1.2GiB" or "500MiB"
		mem := stats.Memory
		var memVal float64
		if strings.HasSuffix(mem, "GiB") {
			fmt.Sscanf(strings.TrimSuffix(mem, "GiB"), "%f", &memVal)
			totalMemMB += memVal * 1024
		} else if strings.HasSuffix(mem, "MiB") {
			fmt.Sscanf(strings.TrimSuffix(mem, "MiB"), "%f", &memVal)
			totalMemMB += memVal
		}
	}

	maxSuggested := config.SuggestMaxInstances()
	proxyStatus := stoppedStyle.Render("○")
	if m.proxyRunning {
		proxyStatus = runningStyle.Render("●")
	}

	// Calculate percentages of host resources
	hostCpuPct := totalCPU / float64(cpuCores)
	hostMemPct := totalMemMB / (float64(ramGB) * 1024) * 100

	// Format total memory
	memStr := fmt.Sprintf("%.0fMB", totalMemMB)
	if totalMemMB >= 1024 {
		memStr = fmt.Sprintf("%.1fGB", totalMemMB/1024)
	}

	// Queue stats (q already declared above)
	qReady := len(q.GetByStatus(queue.StatusReady))
	qTotal := len(m.queueTasks)
	queueInfo := fmt.Sprintf("queue: %d run, %d ready, %d total", running, qReady, qTotal)

	// Filter info
	filterInfo := fmt.Sprintf("filter: %s", m.filterDescription())

	// Processor status
	procStatus := "auto"
	if !m.processorOn {
		procStatus = "manual"
	}

	return statusBarStyle.Render(fmt.Sprintf("%d cores, %dGB  •  %d/%d running (%.0f%% CPU, %s/%.0f%% RAM)  •  %s  •  %s  •  mode: %s  •  proxy %s",
		cpuCores, ramGB, running, maxSuggested, hostCpuPct, memStr, hostMemPct, queueInfo, filterInfo, procStatus, proxyStatus))
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

		// Show CPU/RAM stats if container is already running (even during setup)
		if stats, ok := m.containerStats[br.Name]; ok {
			cpuCores, ramGB := config.GetSystemResources()
			var cpuPct float64
			fmt.Sscanf(strings.TrimSuffix(stats.CPU, "%"), "%f", &cpuPct)
			hostCpuPct := cpuPct / float64(cpuCores)
			mem := stats.Memory
			var memMB float64
			if strings.HasSuffix(mem, "GiB") {
				var v float64
				fmt.Sscanf(strings.TrimSuffix(mem, "GiB"), "%f", &v)
				memMB = v * 1024
			} else if strings.HasSuffix(mem, "MiB") {
				fmt.Sscanf(strings.TrimSuffix(mem, "MiB"), "%f", &memMB)
			}
			memPct := memMB / (float64(ramGB) * 1024) * 100
			memStr := fmt.Sprintf("%.0fMB", memMB)
			if memMB >= 1024 {
				memStr = fmt.Sprintf("%.1fGB", memMB/1024)
			}
			header += helpStyle.Render(fmt.Sprintf(", CPU: %.0f%%, RAM: %s/%.0f%%", hostCpuPct, memStr, memPct))
		}

		content := helpStyle.Render(pending.Status)
		cellContent := header + "\n" + content
		// Enforce strict height limit
		cellLines := strings.Split(cellContent, "\n")
		if len(cellLines) > innerHeight {
			cellLines = cellLines[:innerHeight]
			cellContent = strings.Join(cellLines, "\n")
		}
		style := cellBorderStyle
		if selected {
			style = cellSelectedStyle
		}
		return style.Width(innerWidth).Height(innerHeight).Render(cellContent)
	}

	// Header with status icon and branch name
	var header string
	statusIcon := stoppedStyle.Render("○")
	if m.isRunning(br.Name) {
		statusIcon = runningStyle.Render("●")
	}
	header = statusIcon + " " + cellHeaderStyle.Render(br.Name)

	// Add git stats (commits ahead, lines changed) - use cached values
	if gs, ok := m.gitStats[br.Name]; ok && gs != nil {
		if gs.Commits > 0 || gs.Added > 0 || gs.Removed > 0 {
			header += helpStyle.Render(fmt.Sprintf(", git: %dc +%d/-%d", gs.Commits, gs.Added, gs.Removed))
		}
	}

	// Add task status if task exists
	if ti, ok := m.taskInfo[br.Name]; ok && ti != nil && ti.Phase != task.PhaseNone {
		taskStatus := ti.Phase.Icon() + " " + ti.Phase.Display()
		if ti.StatusLine != "" {
			taskStatus += " " + ti.StatusLine
		}
		if ti.Summary != "" {
			taskStatus += ": " + ti.Summary
		}
		header += "\n" + helpStyle.Render(taskStatus)
	}

	// Add CPU/RAM stats if running
	if stats, ok := m.containerStats[br.Name]; ok && m.isRunning(br.Name) {
		cpuCores, ramGB := config.GetSystemResources()
		// Convert CPU percentage to % of total host CPU
		var cpuPct float64
		fmt.Sscanf(strings.TrimSuffix(stats.CPU, "%"), "%f", &cpuPct)
		hostCpuPct := cpuPct / float64(cpuCores)
		// Parse memory and calculate % of host RAM
		mem := stats.Memory
		var memMB float64
		if strings.HasSuffix(mem, "GiB") {
			var v float64
			fmt.Sscanf(strings.TrimSuffix(mem, "GiB"), "%f", &v)
			memMB = v * 1024
		} else if strings.HasSuffix(mem, "MiB") {
			fmt.Sscanf(strings.TrimSuffix(mem, "MiB"), "%f", &memMB)
		}
		memPct := memMB / (float64(ramGB) * 1024) * 100
		memStr := fmt.Sprintf("%.0fMB", memMB)
		if memMB >= 1024 {
			memStr = fmt.Sprintf("%.1fGB", memMB/1024)
		}
		header += helpStyle.Render(fmt.Sprintf(", CPU: %.0f%%, RAM: %s/%.0f%%", hostCpuPct, memStr, memPct))
	}

	// Content
	var content string
	if m.isRunning(br.Name) {
		if !tmux.BranchSessionExists(br.Name) {
			content = stoppedStyle.Render("[ready - press 'c' for Claude]")
		} else if pane, ok := m.paneContent[br.Name]; ok && pane != "" {
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
		} else {
			content = stoppedStyle.Render("[Claude session active]")
		}
	} else {
		content = cellStoppedStyle.Render("[stopped]")
	}

	cellContent := header + "\n" + content

	// Enforce strict height limit - truncate to innerHeight lines
	cellLines := strings.Split(cellContent, "\n")
	if len(cellLines) > innerHeight {
		cellLines = cellLines[:innerHeight]
		cellContent = strings.Join(cellLines, "\n")
	}

	style := cellBorderStyle
	if selected {
		style = cellSelectedStyle
	}

	return style.Width(innerWidth).Height(innerHeight).Render(cellContent)
}

// renderTaskCell renders a queue task cell.
func (m GridModel) renderTaskCell(t *queue.Task, idx int, width, height int) string {
	innerWidth := width - 2
	innerHeight := height - 2

	selected := idx == m.cursor

	// Check if this task has a pending operation
	if pending, ok := globalPendingBranches[t.ID]; ok {
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐") + " " + cellHeaderStyle.Render(t.ID)

		// Show CPU/RAM stats if container is already running
		if stats, ok := m.containerStats[t.ID]; ok {
			cpuCores, ramGB := config.GetSystemResources()
			var cpuPct float64
			fmt.Sscanf(strings.TrimSuffix(stats.CPU, "%"), "%f", &cpuPct)
			hostCpuPct := cpuPct / float64(cpuCores)
			mem := stats.Memory
			var memMB float64
			if strings.HasSuffix(mem, "GiB") {
				var v float64
				fmt.Sscanf(strings.TrimSuffix(mem, "GiB"), "%f", &v)
				memMB = v * 1024
			} else if strings.HasSuffix(mem, "MiB") {
				fmt.Sscanf(strings.TrimSuffix(mem, "MiB"), "%f", &memMB)
			}
			memPct := memMB / (float64(ramGB) * 1024) * 100
			memStr := fmt.Sprintf("%.0fMB", memMB)
			if memMB >= 1024 {
				memStr = fmt.Sprintf("%.1fGB", memMB/1024)
			}
			header += helpStyle.Render(fmt.Sprintf(", CPU: %.0f%%, RAM: %s/%.0f%%", hostCpuPct, memStr, memPct))
		}

		content := helpStyle.Render(pending.Status)
		cellContent := header + "\n" + content
		// Enforce strict height limit
		cellLines := strings.Split(cellContent, "\n")
		if len(cellLines) > innerHeight {
			cellLines = cellLines[:innerHeight]
			cellContent = strings.Join(cellLines, "\n")
		}
		style := cellStyleForStatus(t.Status, selected)
		return style.Width(innerWidth).Height(innerHeight).Render(cellContent)
	}

	// Header with status icon and task name
	statusIcon := t.Status.Icon()
	header := statusIcon + " " + cellHeaderStyle.Render(t.ID)

	// Check if branch exists and is running
	b := branch.New(t.ID)
	branchRunning := b.Exists() && m.isRunning(t.ID)

	// Add git stats if branch exists
	if gs, ok := m.gitStats[t.ID]; ok && gs != nil {
		if gs.Commits > 0 || gs.Added > 0 || gs.Removed > 0 {
			header += helpStyle.Render(fmt.Sprintf(", git: %dc +%d/-%d", gs.Commits, gs.Added, gs.Removed))
		}
	}

	// Add task phase info if available
	if ti, ok := m.taskInfo[t.ID]; ok && ti != nil && ti.Phase != task.PhaseNone {
		taskStatus := ti.Phase.Icon() + " " + ti.Phase.Display()
		if ti.StatusLine != "" {
			taskStatus += " " + ti.StatusLine
		}
		if ti.Summary != "" {
			taskStatus += ": " + ti.Summary
		}
		header += "\n" + helpStyle.Render(taskStatus)
	} else {
		// Show queue status
		header += "\n" + helpStyle.Render(t.Status.Display())
	}

	// Add CPU/RAM stats if running
	if stats, ok := m.containerStats[t.ID]; ok && branchRunning {
		cpuCores, ramGB := config.GetSystemResources()
		var cpuPct float64
		fmt.Sscanf(strings.TrimSuffix(stats.CPU, "%"), "%f", &cpuPct)
		hostCpuPct := cpuPct / float64(cpuCores)
		mem := stats.Memory
		var memMB float64
		if strings.HasSuffix(mem, "GiB") {
			var v float64
			fmt.Sscanf(strings.TrimSuffix(mem, "GiB"), "%f", &v)
			memMB = v * 1024
		} else if strings.HasSuffix(mem, "MiB") {
			fmt.Sscanf(strings.TrimSuffix(mem, "MiB"), "%f", &memMB)
		}
		memPct := memMB / (float64(ramGB) * 1024) * 100
		memStr := fmt.Sprintf("%.0fMB", memMB)
		if memMB >= 1024 {
			memStr = fmt.Sprintf("%.1fGB", memMB/1024)
		}
		header += helpStyle.Render(fmt.Sprintf(", CPU: %.0f%%, RAM: %s/%.0f%%", hostCpuPct, memStr, memPct))
	}

	// Content
	var content string
	if branchRunning {
		if !tmux.BranchSessionExists(t.ID) {
			content = stoppedStyle.Render("[ready - press 'c' for Claude]")
		} else if pane, ok := m.paneContent[t.ID]; ok && pane != "" {
			// Clean up Claude branding and OAuth noise
			cleanedPane := cleanPaneContent(pane)
			if cleanedPane == "" {
				content = stoppedStyle.Render("[Claude starting...]")
			} else {
				lines := strings.Split(cleanedPane, "\n")
				maxLines := innerHeight - 2
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
			content = stoppedStyle.Render("[Claude session active]")
		}
	} else {
		// Show task prompt preview for non-running tasks
		if t.Prompt != "" {
			preview := t.Prompt
			if len(preview) > innerWidth*2 {
				preview = preview[:innerWidth*2] + "..."
			}
			// Wrap to multiple lines
			lines := wrapText(preview, innerWidth)
			maxLines := innerHeight - 2
			if len(lines) > maxLines {
				lines = lines[:maxLines]
			}
			content = cellStoppedStyle.Render(strings.Join(lines, "\n"))
		} else {
			content = cellStoppedStyle.Render("[no prompt - press 'p' to add]")
		}
	}

	cellContent := header + "\n" + content

	// Enforce strict height limit - truncate to innerHeight lines
	cellLines := strings.Split(cellContent, "\n")
	if len(cellLines) > innerHeight {
		cellLines = cellLines[:innerHeight]
		cellContent = strings.Join(cellLines, "\n")
	}

	style := cellStyleForStatus(t.Status, selected)

	return style.Width(innerWidth).Height(innerHeight).Render(cellContent)
}

// cleanPaneContent filters out Claude branding, OAuth URLs, and other noise from tmux output.
func cleanPaneContent(content string) string {
	lines := strings.Split(content, "\n")
	var cleaned []string

	for _, line := range lines {
		// Skip Claude ASCII art and branding
		if strings.Contains(line, "╭") || strings.Contains(line, "╰") ||
			strings.Contains(line, "│") && (strings.Contains(line, "░") || strings.Contains(line, "▓")) {
			continue
		}
		// Skip OAuth URLs
		if strings.Contains(line, "platform.claude.com/oauth") ||
			strings.Contains(line, "https://") && strings.Contains(line, "auth") {
			continue
		}
		// Skip login prompts
		if strings.Contains(line, "Browser didn't open") ||
			strings.Contains(line, "Paste code here") ||
			strings.Contains(line, "Please run /login") {
			continue
		}
		// Skip empty lines at start
		if len(cleaned) == 0 && strings.TrimSpace(line) == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}

	return strings.Join(cleaned, "\n")
}

// wrapText wraps text to fit within a given width.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for len(text) > width {
		lines = append(lines, text[:width])
		text = text[width:]
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	return lines
}

func (m GridModel) renderPendingCell(pb *PendingBranch, width, height int) string {
	innerWidth := width - 2
	innerHeight := height - 2

	header := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◐") + " " + cellHeaderStyle.Render(pb.Name)

	// Show CPU/RAM stats if container is running (during setup phases)
	if stats, ok := m.containerStats[pb.Name]; ok {
		cpuCores, ramGB := config.GetSystemResources()
		var cpuPct float64
		fmt.Sscanf(strings.TrimSuffix(stats.CPU, "%"), "%f", &cpuPct)
		hostCpuPct := cpuPct / float64(cpuCores)
		mem := stats.Memory
		var memMB float64
		if strings.HasSuffix(mem, "GiB") {
			var v float64
			fmt.Sscanf(strings.TrimSuffix(mem, "GiB"), "%f", &v)
			memMB = v * 1024
		} else if strings.HasSuffix(mem, "MiB") {
			fmt.Sscanf(strings.TrimSuffix(mem, "MiB"), "%f", &memMB)
		}
		memPct := memMB / (float64(ramGB) * 1024) * 100
		memStr := fmt.Sprintf("%.0fMB", memMB)
		if memMB >= 1024 {
			memStr = fmt.Sprintf("%.1fGB", memMB/1024)
		}
		header += helpStyle.Render(fmt.Sprintf(", CPU: %.0f%%, RAM: %s/%.0f%%", hostCpuPct, memStr, memPct))
	}

	content := helpStyle.Render(pb.Status)
	cellContent := header + "\n" + content

	// Enforce strict height limit
	cellLines := strings.Split(cellContent, "\n")
	if len(cellLines) > innerHeight {
		cellLines = cellLines[:innerHeight]
		cellContent = strings.Join(cellLines, "\n")
	}

	return cellBorderStyle.Width(innerWidth).Height(innerHeight).Render(cellContent)
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
		return createStepMsg{name: name, branch: b}
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

func (m GridModel) removeTask(t *queue.Task) tea.Cmd {
	return func() tea.Msg {
		// Remove branch if it exists
		b := branch.New(t.ID)
		if b.Exists() {
			if err := removeBranchFull(b); err != nil {
				return operationErrMsg{err}
			}
		}
		// Remove from queue
		q := queue.Get()
		q.Remove(t.ID)
		q.Save()
		return operationDoneMsg{fmt.Sprintf("Removed %s", t.ID)}
	}
}

// findEditor returns the user's preferred editor.
func findEditor() string {
	// Try micro first (simple, works well in terminals)
	if _, err := exec.LookPath("micro"); err == nil {
		return "micro"
	}
	// Try nano
	if _, err := exec.LookPath("nano"); err == nil {
		return "nano"
	}
	// Try vim
	if _, err := exec.LookPath("vim"); err == nil {
		return "vim"
	}
	// Fall back to vi
	return "vi"
}

// isTemplateOnly checks if content is still just the template.
func isTemplateOnly(content string) bool {
	return strings.Contains(content, "[What should this accomplish?]") &&
		strings.Contains(content, "[Relevant background")
}

// writeFile writes content to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// readFile reads content from a file.
func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
