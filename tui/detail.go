package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/queue"
	"github.com/darklang/dark-multi/tmux"
)

var (
	detailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(1)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("99")).
				Width(15)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	detailSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("214")).
				MarginTop(1).
				MarginBottom(1)

	detailURLStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117")).
			Underline(true)
)

// DetailModel shows detailed information about a task.
type DetailModel struct {
	task      *queue.Task
	branch    *branch.Branch
	parent    GridModel
	width     int
	height    int
	urlCursor int
	urls      []string
}

// NewDetailModel creates a new detail view for a task.
func NewDetailModel(task *queue.Task, parent GridModel) DetailModel {
	b := branch.New(task.ID)

	// Build list of URLs
	var urls []string
	if b.Exists() && b.IsRunning() {
		// Add canvas URLs
		urls = append(urls,
			fmt.Sprintf("http://dark-packages.%s.dlio.localhost:%d/ping", task.ID, config.ProxyPort),
			fmt.Sprintf("http://builtwithdark.%s.dlio.localhost:%d", task.ID, config.ProxyPort),
		)
	}

	return DetailModel{
		task:   task,
		branch: b,
		parent: parent,
		width:  parent.width,
		height: parent.height,
		urls:   urls,
	}
}

// Init initializes the detail model.
func (m DetailModel) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m DetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			// Return to grid
			return m.parent, m.parent.Init()

		case "up", "k":
			if m.urlCursor > 0 {
				m.urlCursor--
			}

		case "down", "j":
			if m.urlCursor < len(m.urls)-1 {
				m.urlCursor++
			}

		case "enter", "o":
			// Open selected URL
			if len(m.urls) > 0 && m.urlCursor < len(m.urls) {
				openInBrowser(m.urls[m.urlCursor])
			}

		case "s":
			// Start task
			if m.branch != nil && !m.branch.IsRunning() {
				globalPendingBranches[m.task.ID] = &PendingBranch{Name: m.task.ID, Status: "starting"}
				return m.parent, m.parent.startBranch(m.branch)
			} else if m.branch == nil || !m.branch.Exists() {
				globalPendingBranches[m.task.ID] = &PendingBranch{Name: m.task.ID, Status: "creating"}
				return m.parent, m.parent.createAndStartBranch(m.task.ID)
			}

		case "K":
			// Kill task
			if m.branch != nil && m.branch.IsRunning() {
				return m.parent, m.parent.stopBranch(m.branch)
			}

		case "c":
			// Open Claude
			if m.branch != nil && m.branch.IsRunning() {
				containerID, err := m.branch.ContainerID()
				if err == nil {
					tmux.OpenClaude(m.task.ID, containerID)
				}
			}

		case "t":
			// Open terminal
			if m.branch != nil && m.branch.IsRunning() {
				containerID, err := m.branch.ContainerID()
				if err == nil {
					tmux.OpenTerminal(m.task.ID, containerID)
				}
			}

		case "v":
			// Focus view
			focus := NewFocusModel(m.task, m.parent)
			return focus, focus.Init()

		case "e":
			// Open VS Code
			if m.branch != nil && m.branch.IsRunning() {
				return m.parent, m.parent.openCode(m.branch)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.parent.width = msg.Width
		m.parent.height = msg.Height
	}

	return m, nil
}

// View renders the detail view.
func (m DetailModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(detailTitleStyle.Render(fmt.Sprintf("%s %s", m.task.Status.Icon(), m.task.ID)))
	b.WriteString("\n\n")

	// Task info section
	b.WriteString(detailSectionStyle.Render("Task Info"))
	b.WriteString("\n")
	b.WriteString(m.renderRow("Name", m.task.Name))
	b.WriteString(m.renderRow("Status", m.task.Status.Display()))
	b.WriteString(m.renderRow("Priority", fmt.Sprintf("%d", m.task.Priority)))
	if !m.task.CreatedAt.IsZero() {
		b.WriteString(m.renderRow("Created", m.task.CreatedAt.Format(time.RFC822)))
	}
	if !m.task.StartedAt.IsZero() {
		b.WriteString(m.renderRow("Started", m.task.StartedAt.Format(time.RFC822)))
	}
	if !m.task.CompletedAt.IsZero() {
		b.WriteString(m.renderRow("Completed", m.task.CompletedAt.Format(time.RFC822)))
	}
	if m.task.Error != "" {
		b.WriteString(m.renderRow("Error", errorStyle.Render(m.task.Error)))
	}

	// Prompt section
	b.WriteString("\n")
	b.WriteString(detailSectionStyle.Render("Prompt"))
	b.WriteString("\n")
	if m.task.Prompt != "" {
		// Wrap and truncate prompt
		prompt := m.task.Prompt
		maxLen := m.width * 5 // Allow 5 lines
		if len(prompt) > maxLen {
			prompt = prompt[:maxLen] + "..."
		}
		// Simple word wrapping
		lines := wrapTextWords(prompt, m.width-5)
		for _, line := range lines {
			b.WriteString("  " + detailValueStyle.Render(line) + "\n")
		}
	} else {
		b.WriteString("  " + stoppedStyle.Render("[No prompt - task needs configuration]") + "\n")
	}

	// Branch/Container info
	if m.branch != nil && m.branch.Exists() {
		b.WriteString("\n")
		b.WriteString(detailSectionStyle.Render("Container"))
		b.WriteString("\n")
		if m.branch.IsRunning() {
			b.WriteString(m.renderRow("Status", runningStyle.Render("● Running")))
			if containerID, err := m.branch.ContainerID(); err == nil {
				b.WriteString(m.renderRow("Container", containerID[:12]))
			}
		} else {
			b.WriteString(m.renderRow("Status", stoppedStyle.Render("○ Stopped")))
		}

		// Git stats
		commits, added, removed := m.branch.GitStats()
		if commits > 0 || added > 0 || removed > 0 {
			b.WriteString(m.renderRow("Git", fmt.Sprintf("%d commits, +%d/-%d lines", commits, added, removed)))
		}
	}

	// URLs section
	if len(m.urls) > 0 {
		b.WriteString("\n")
		b.WriteString(detailSectionStyle.Render("URLs"))
		b.WriteString("\n")
		for i, url := range m.urls {
			prefix := "  "
			style := detailURLStyle
			if i == m.urlCursor {
				prefix = "▸ "
				style = style.Bold(true)
			}
			b.WriteString(prefix + style.Render(url) + "\n")
		}
	}

	// Footer
	b.WriteString("\n")
	var actions []string
	if m.branch != nil && m.branch.IsRunning() {
		actions = append(actions, "[c]laude", "[t]erm", "[v]iew", "[e]dit", "[K]ill")
	} else {
		actions = append(actions, "[s]tart")
	}
	if len(m.urls) > 0 {
		actions = append(actions, "[o]pen URL")
	}
	actions = append(actions, "[esc] back")
	b.WriteString(helpStyle.Render(strings.Join(actions, "  ")))

	return b.String()
}

func (m DetailModel) renderRow(label, value string) string {
	return detailLabelStyle.Render(label+":") + " " + detailValueStyle.Render(value) + "\n"
}

// wrapTextWords wraps text at word boundaries.
func wrapTextWords(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	var currentLine string

	for _, word := range words {
		if currentLine == "" {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
