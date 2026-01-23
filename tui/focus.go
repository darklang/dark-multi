package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/queue"
	"github.com/darklang/dark-multi/tmux"
)

var (
	focusTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	focusContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	focusStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)
)

// FocusModel shows a single container's output in full screen.
type FocusModel struct {
	task       *queue.Task
	branch     *branch.Branch
	content    string
	scrollPos  int
	width      int
	height     int
	parent     GridModel
	inputMode  bool
	inputText  string
}

type focusTickMsg time.Time
type focusContentMsg string

// NewFocusModel creates a new focus view for a task.
func NewFocusModel(task *queue.Task, parent GridModel) FocusModel {
	b := branch.New(task.ID)
	return FocusModel{
		task:   task,
		branch: b,
		parent: parent,
		width:  parent.width,
		height: parent.height,
	}
}

// Init initializes the focus model.
func (m FocusModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadContent,
		focusTickCmd(),
	)
}

func focusTickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return focusTickMsg(t)
	})
}

func (m FocusModel) loadContent() tea.Msg {
	if m.branch == nil || !tmux.BranchSessionExists(m.task.ID) {
		return focusContentMsg("")
	}
	// Capture more lines for full-screen view
	lines := m.height - 4 // Leave room for header and footer
	if lines < 20 {
		lines = 50
	}
	content := tmux.CapturePaneContent(m.task.ID, lines*2) // Get extra for scrolling
	return focusContentMsg(content)
}

// Update handles messages.
func (m FocusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inputMode {
			return m.handleInput(msg)
		}

		switch msg.String() {
		case "q", "esc":
			// Return to grid
			return m.parent, m.parent.Init()

		case "up", "k":
			if m.scrollPos > 0 {
				m.scrollPos--
			}

		case "down", "j":
			lines := strings.Split(m.content, "\n")
			maxScroll := len(lines) - (m.height - 4)
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollPos < maxScroll {
				m.scrollPos++
			}

		case "g":
			// Go to top
			m.scrollPos = 0

		case "G":
			// Go to bottom
			lines := strings.Split(m.content, "\n")
			maxScroll := len(lines) - (m.height - 4)
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollPos = maxScroll

		case "pgup":
			m.scrollPos -= 10
			if m.scrollPos < 0 {
				m.scrollPos = 0
			}

		case "pgdown":
			lines := strings.Split(m.content, "\n")
			maxScroll := len(lines) - (m.height - 4)
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollPos += 10
			if m.scrollPos > maxScroll {
				m.scrollPos = maxScroll
			}

		case "i":
			// Enter input mode to send text to Claude
			m.inputMode = true
			m.inputText = ""

		case "o":
			// Open in external terminal
			if m.branch != nil && m.branch.IsRunning() {
				containerID, err := m.branch.ContainerID()
				if err == nil {
					tmux.OpenClaude(m.task.ID, containerID)
				}
			}

		case "r":
			// Refresh content
			return m, m.loadContent
		}

	case focusContentMsg:
		m.content = string(msg)
		return m, nil

	case focusTickMsg:
		return m, tea.Batch(m.loadContent, focusTickCmd())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.parent.width = msg.Width
		m.parent.height = msg.Height
	}

	return m, nil
}

func (m FocusModel) handleInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.inputText != "" {
			// Send text to Claude session
			tmux.SendToClaude(m.task.ID, m.inputText)
		}
		m.inputMode = false
		m.inputText = ""
		return m, m.loadContent

	case "esc":
		m.inputMode = false
		m.inputText = ""
		return m, nil

	case "backspace":
		if len(m.inputText) > 0 {
			m.inputText = m.inputText[:len(m.inputText)-1]
		}
		return m, nil

	default:
		key := msg.String()
		if len(key) == 1 || key == "space" {
			if key == "space" {
				key = " "
			}
			m.inputText += key
		}
		return m, nil
	}
}

// View renders the focus view.
func (m FocusModel) View() string {
	var b strings.Builder

	// Header
	statusIcon := m.task.Status.Icon()
	title := fmt.Sprintf("%s %s", statusIcon, m.task.ID)

	branchRunning := m.branch != nil && m.branch.Exists() && m.branch.IsRunning()
	runStatus := "stopped"
	if branchRunning {
		runStatus = "running"
	}

	headerLeft := focusTitleStyle.Render(title)
	headerRight := focusStatusStyle.Render(runStatus)
	headerPadding := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if headerPadding < 0 {
		headerPadding = 0
	}
	b.WriteString(headerLeft + strings.Repeat(" ", headerPadding) + headerRight)
	b.WriteString("\n")

	// Content area
	contentHeight := m.height - 4 // header + footer + padding
	if contentHeight < 5 {
		contentHeight = 20
	}

	if m.content == "" {
		if !branchRunning {
			b.WriteString(stoppedStyle.Render("\n[Container not running - press 's' in grid to start]\n"))
		} else if !tmux.BranchSessionExists(m.task.ID) {
			b.WriteString(stoppedStyle.Render("\n[No Claude session - press 'o' to open one]\n"))
		} else {
			b.WriteString(stoppedStyle.Render("\n[Loading...]\n"))
		}
	} else {
		lines := strings.Split(m.content, "\n")

		// Apply scroll
		start := m.scrollPos
		if start >= len(lines) {
			start = 0
		}
		end := start + contentHeight
		if end > len(lines) {
			end = len(lines)
		}

		visibleLines := lines[start:end]

		// Truncate long lines
		for i, line := range visibleLines {
			if len(line) > m.width {
				visibleLines[i] = line[:m.width-1] + "…"
			}
		}

		b.WriteString(focusContentStyle.Render(strings.Join(visibleLines, "\n")))
		b.WriteString("\n")
	}

	// Pad to bottom
	currentLines := strings.Count(b.String(), "\n")
	for i := currentLines; i < m.height-2; i++ {
		b.WriteString("\n")
	}

	// Footer / Input
	if m.inputMode {
		b.WriteString(focusStatusStyle.Render("Send to Claude: "))
		b.WriteString(m.inputText)
		b.WriteString("█")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] send  [esc] cancel"))
	} else {
		scrollInfo := ""
		lines := strings.Split(m.content, "\n")
		if len(lines) > contentHeight {
			scrollInfo = fmt.Sprintf(" [line %d/%d]", m.scrollPos+1, len(lines))
		}
		b.WriteString(helpStyle.Render(fmt.Sprintf("[i]nput  [o]pen terminal  [↑↓] scroll  [g/G] top/bottom  [r]efresh  [esc] back%s", scrollInfo)))
	}

	return b.String()
}
