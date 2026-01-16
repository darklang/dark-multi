package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
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
)

// GridModel displays all Claude sessions in a grid layout.
type GridModel struct {
	branches    []*branch.Branch
	paneContent map[string]string // branch name -> captured content
	cursor      int               // selected cell index
	width       int
	height      int
	message     string
}

// Grid layout messages
type paneContentMsg map[string]string
type gridTickMsg time.Time

// NewGridModel creates a new grid view.
func NewGridModel(branches []*branch.Branch) GridModel {
	return GridModel{
		branches:    branches,
		paneContent: make(map[string]string),
	}
}

// Init initializes the grid model.
func (m GridModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadPaneContent,
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
		switch msg.String() {
		case "esc", "q":
			// Return to home
			home := NewHomeModel()
			return home, home.Init()

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
			// Focus on selected branch - open in separate terminal window
			if len(m.branches) > 0 && m.cursor < len(m.branches) {
				b := m.branches[m.cursor]
				if b.IsRunning() {
					// Ensure session exists
					if !tmux.BranchSessionExists(b.Name) {
						containerID, _ := b.ContainerID()
						tmux.CreateBranchSession(b.Name, containerID, b.Path)
					}
					// Open in separate terminal window
					if err := tmux.OpenBranchInTerminal(b.Name); err != nil {
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
					return m, m.startBranch(b)
				}
			}
		}

	case paneContentMsg:
		if msg != nil {
			m.paneContent = msg
		}
		// Content loaded, no immediate reload (tick handles periodic refresh)
		return m, nil

	case gridTickMsg:
		// Refresh branches and content periodically
		m.branches = branch.GetManagedBranches()
		if m.cursor >= len(m.branches) {
			m.cursor = max(0, len(m.branches)-1)
		}
		return m, tea.Batch(m.loadPaneContent, gridTickCmd())

	case operationDoneMsg:
		m.message = msg.message
		m.branches = branch.GetManagedBranches()
		return m, m.loadPaneContent

	case operationErrMsg:
		m.message = fmt.Sprintf("Error: %v", msg.err)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m GridModel) numCols() int {
	n := len(m.branches)
	if n == 0 {
		return 1
	}
	// 2 rows, ceil(n/2) columns
	return (n + 1) / 2
}

// View renders the grid.
func (m GridModel) View() string {
	var b strings.Builder

	if len(m.branches) == 0 {
		b.WriteString(stoppedStyle.Render("No branches. Press 'esc' to go back and create one."))
		b.WriteString("\n")
		return b.String()
	}

	// Calculate cell dimensions - maximize screen usage
	cols := m.numCols()

	// Use sensible defaults if window size not yet known
	width := m.width
	if width < 40 {
		width = 120 // reasonable default
	}
	height := m.height
	if height < 10 {
		height = 40 // reasonable default
	}

	// 2 rows, 1 line for help
	cellHeight := (height - 1) / 2

	// Build rows (2 rows)
	var rows []string
	for row := 0; row < 2; row++ {
		var cells []string
		remainingWidth := width
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			// Last column gets remaining width to avoid gaps
			cellWidth := remainingWidth / (cols - col)
			remainingWidth -= cellWidth
			cell := m.renderCell(idx, cellWidth, cellHeight)
			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))

	// Message or help on last line
	if m.message != "" {
		b.WriteString(m.message)
	} else {
		b.WriteString(helpStyle.Render("←↑↓→:nav  enter:focus  s:start  esc:back"))
	}

	return b.String()
}

func (m GridModel) renderCell(idx int, width, height int) string {
	// Account for border (1 char each side)
	innerWidth := width - 2
	innerHeight := height - 2

	if idx >= len(m.branches) {
		// Empty cell
		return cellBorderStyle.
			Width(innerWidth).
			Height(innerHeight).
			Render("")
	}

	br := m.branches[idx]
	selected := idx == m.cursor

	// Header: branch name + status
	var header string
	if br.IsRunning() {
		header = runningStyle.Render("●") + " " + cellHeaderStyle.Render(br.Name)
	} else {
		header = stoppedStyle.Render("○") + " " + cellHeaderStyle.Render(br.Name)
	}

	// Content
	var content string
	if br.IsRunning() {
		if !tmux.BranchSessionExists(br.Name) {
			content = stoppedStyle.Render("[no session - press enter to create]")
		} else if pane, ok := m.paneContent[br.Name]; ok && pane != "" {
			// Show captured pane content, truncate lines to fit width
			lines := strings.Split(pane, "\n")
			maxLines := innerHeight - 1 // -1 for header
			if len(lines) > maxLines {
				lines = lines[len(lines)-maxLines:]
			}
			// Truncate each line to fit cell width
			for i, line := range lines {
				if len(line) > innerWidth {
					lines[i] = line[:innerWidth-1] + "…"
				}
			}
			content = strings.Join(lines, "\n")
		} else {
			content = stoppedStyle.Render("[capturing...]")
		}
	} else {
		content = cellStoppedStyle.Render("[stopped] 's' to start")
	}

	// Combine header and content
	cellContent := header + "\n" + content

	// Apply style
	style := cellBorderStyle
	if selected {
		style = cellSelectedStyle
	}

	return style.
		Width(innerWidth).
		Height(innerHeight).
		Render(cellContent)
}

func (m GridModel) startBranch(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		if err := startBranchFull(b); err != nil {
			return operationErrMsg{err}
		}
		return operationDoneMsg{fmt.Sprintf("Started %s", b.Name)}
	}
}
