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

	// Default filter: show running tasks
	defaultFilter := []queue.Status{queue.StatusRunning}

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
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}

		case "up":
			cols := m.numCols()
			if m.cursor >= cols {
				m.cursor -= cols
			}

		case "down":
			cols := m.numCols()
			if m.cursor+cols < len(m.branches) {
				m.cursor += cols
			}

		case "enter":
			// Open Claude for selected branch (same as 'c')
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
				if b.IsRunning() {
					m.message = fmt.Sprintf("%s is already running", b.Name)
				} else {
					globalPendingBranches[b.Name] = &PendingBranch{Name: b.Name, Status: "starting container"}
					m.loading = true
					return m, m.startBranch(b)
				}
			}

		case "k":
			// Kill (stop) selected branch
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if !b.IsRunning() {
					m.message = fmt.Sprintf("%s is already stopped", b.Name)
				} else {
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
				if !b.IsRunning() {
					m.message = fmt.Sprintf("%s is stopped - press 's' to start", b.Name)
					return m, nil
				}
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

		case "l":
			// View logs
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				logs := NewLogViewerModel(b)
				return logs, logs.Init()
			}

		case "p":
			// Edit pre-prompt (task definition)
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				t := task.New(b.Name, b.Path)

				// Create pre-prompt file with template if it doesn't exist
				if !t.Exists() {
					template := task.PrePromptTemplate(b.Name)
					if err := t.SetPrePrompt(template); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
						return m, nil
					}
				}

				// Use tea.ExecProcess to run editor (hands over terminal properly)
				editor := findEditor()
				c := exec.Command(editor, t.PrePromptPath())
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					if err != nil {
						return operationErrMsg{err}
					}
					// After editor closes, check content and set phase
					content := t.PrePrompt()
					if content != "" && !isTemplateOnly(content) {
						t.SetPhase(task.PhasePlanning)
						return operationDoneMsg{fmt.Sprintf("Task set for %s - press 'c' to start", b.Name)}
					}
					return operationDoneMsg{"Pre-prompt saved"}
				})
			}

		case "f":
			// Cycle through filter options
			m.statusFilter = m.nextFilter()
			m.cursor = 0
			return m, nil

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
		// Keep cursor in bounds (grid shows branches)
		if m.cursor >= len(m.branches) && len(m.branches) > 0 {
			m.cursor = len(m.branches) - 1
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
			m.loading = true
			b := branch.New(name)
			if b.Exists() {
				globalPendingBranches[name] = &PendingBranch{Name: name, Status: "starting container"}
			} else {
				globalPendingBranches[name] = &PendingBranch{Name: name, Status: "cloning from GitHub"}
			}
			return m, m.createAndStartBranch(name)

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
		b.WriteString(helpStyle.Render("[enter] create  [esc] cancel"))
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

	// Reserve 5 lines for newline, status bar, newline, and help/message
	cellHeight := (height - 5) / 2

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
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Message or help
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	} else if m.message != "" {
		b.WriteString(m.message)
	} else {
		b.WriteString(helpStyle.Render("[n]ew [x]del [s]tart [k]ill [c]laude [t]erm [p]rompt [f]ilter [?] [q]"))
	}

	return b.String()
}

func (m GridModel) renderStatusBar() string {
	cpuCores, ramGB := config.GetSystemResources()
	running := 0
	for _, br := range m.branches {
		if m.isRunning(br.Name) {
			running++
		}
	}

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

	// Queue stats
	q := queue.Get()
	qRunning := q.CountRunning()
	qReady := len(q.GetByStatus(queue.StatusReady))
	qTotal := len(m.queueTasks)
	queueInfo := fmt.Sprintf("queue: %d run, %d ready, %d total", qRunning, qReady, qTotal)

	// Filter info
	filterInfo := fmt.Sprintf("filter: %s", m.filterDescription())

	return statusBarStyle.Render(fmt.Sprintf("%d cores, %dGB  •  %d/%d running (%.0f%% CPU, %s/%.0f%% RAM)  •  %s  •  %s  •  proxy %s",
		cpuCores, ramGB, running, maxSuggested, hostCpuPct, memStr, hostMemPct, queueInfo, filterInfo, proxyStatus))
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
		style := cellBorderStyle
		if selected {
			style = cellSelectedStyle
		}
		return style.Width(innerWidth).Height(innerHeight).Render(header + "\n" + content)
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
