package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/queue"
)

var (
	filterTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(1)

	filterItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	filterSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	filterCheckStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	filterUncheckStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))
)

// allStatuses returns all possible queue statuses.
func allStatuses() []queue.Status {
	return []queue.Status{
		queue.StatusRunning,
		queue.StatusReady,
		queue.StatusWaiting,
		queue.StatusNeedsPrompt,
		queue.StatusDone,
		queue.StatusPaused,
	}
}

// FilterModel is a modal for selecting status filters.
type FilterModel struct {
	statuses []queue.Status      // all available statuses
	selected map[queue.Status]bool // which statuses are selected
	cursor   int                 // current cursor position
	parent   GridModel           // parent grid to return to
}

// NewFilterModel creates a new filter modal.
func NewFilterModel(parent GridModel) FilterModel {
	statuses := allStatuses()
	selected := make(map[queue.Status]bool)

	// Initialize with parent's current filter
	for _, s := range parent.statusFilter {
		selected[s] = true
	}

	// If no filter, default to all selected
	if len(parent.statusFilter) == 0 {
		for _, s := range statuses {
			selected[s] = true
		}
	}

	return FilterModel{
		statuses: statuses,
		selected: selected,
		cursor:   0,
		parent:   parent,
	}
}

// Init initializes the filter model.
func (m FilterModel) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m FilterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			// Cancel - return to parent without changes
			return m.parent, m.parent.Init()

		case "enter":
			// Apply filter and return to parent
			var filter []queue.Status
			for _, s := range m.statuses {
				if m.selected[s] {
					filter = append(filter, s)
				}
			}
			// If all selected, use empty filter (show all)
			if len(filter) == len(m.statuses) {
				filter = nil
			}
			m.parent.statusFilter = filter
			m.parent.cursor = 0 // Reset cursor since items may change
			return m.parent, m.parent.Init()

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.statuses)-1 {
				m.cursor++
			}

		case " ", "x":
			// Toggle current selection
			s := m.statuses[m.cursor]
			m.selected[s] = !m.selected[s]

		case "a":
			// Select all
			for _, s := range m.statuses {
				m.selected[s] = true
			}

		case "n":
			// Select none
			for _, s := range m.statuses {
				m.selected[s] = false
			}

		case "r":
			// Quick preset: running only
			for _, s := range m.statuses {
				m.selected[s] = (s == queue.StatusRunning)
			}

		case "w":
			// Quick preset: waiting (needs attention)
			for _, s := range m.statuses {
				m.selected[s] = (s == queue.StatusWaiting || s == queue.StatusNeedsPrompt)
			}
		}

	case tea.WindowSizeMsg:
		m.parent.width = msg.Width
		m.parent.height = msg.Height
	}

	return m, nil
}

// View renders the filter modal.
func (m FilterModel) View() string {
	var b strings.Builder

	b.WriteString(filterTitleStyle.Render("STATUS FILTER"))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("Select which statuses to show:"))
	b.WriteString("\n\n")

	for i, s := range m.statuses {
		// Checkbox
		check := filterUncheckStyle.Render("[ ]")
		if m.selected[s] {
			check = filterCheckStyle.Render("[✓]")
		}

		// Status icon and name
		label := fmt.Sprintf("%s %s", s.Icon(), s.Display())

		// Count tasks with this status
		count := 0
		for _, t := range m.parent.queueTasks {
			if t.Status == s {
				count++
			}
		}
		countStr := helpStyle.Render(fmt.Sprintf("(%d)", count))

		// Highlight if cursor is here
		style := filterItemStyle
		if i == m.cursor {
			style = filterSelectedStyle
			label = "▸ " + label
		} else {
			label = "  " + label
		}

		b.WriteString(fmt.Sprintf("%s %s %s\n", check, style.Render(label), countStr))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[space] toggle  [a]ll  [n]one  [r]unning  [w]aiting"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[enter] apply  [esc] cancel"))

	return b.String()
}
