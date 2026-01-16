package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/darklang/dark-multi/branch"
)

const (
	logTailLines   = 30
	logRefreshRate = 1 * time.Second
)

// LogViewerModel displays log files for a branch.
type LogViewerModel struct {
	branch     *branch.Branch
	logFiles   []string
	cursor     int
	content    string
	width      int
	height     int
	err        error
	autoScroll bool
}

// logRefreshMsg triggers a log content refresh.
type logRefreshMsg time.Time

// NewLogViewerModel creates a log viewer for a branch.
func NewLogViewerModel(b *branch.Branch) LogViewerModel {
	logsDir := filepath.Join(b.Path, "rundir", "logs")
	files := []string{}

	entries, err := os.ReadDir(logsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
				files = append(files, e.Name())
			}
		}
	}

	m := LogViewerModel{
		branch:     b,
		logFiles:   files,
		autoScroll: true,
	}

	// Load initial content
	if len(files) > 0 {
		m.content = m.loadLogContent(files[0])
	}

	return m
}

// Init starts the refresh ticker.
func (m LogViewerModel) Init() tea.Cmd {
	return tea.Tick(logRefreshRate, func(t time.Time) tea.Msg {
		return logRefreshMsg(t)
	})
}

// loadLogContent reads the tail of a log file.
func (m LogViewerModel) loadLogContent(filename string) string {
	path := filepath.Join(m.branch.Path, "rundir", "logs", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading %s: %v", filename, err)
	}

	lines := strings.Split(string(data), "\n")

	// Get last N lines
	start := 0
	if len(lines) > logTailLines {
		start = len(lines) - logTailLines
	}

	return strings.Join(lines[start:], "\n")
}

// Update handles input and messages.
func (m LogViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc", "backspace", "h", "left":
			// Back to branch detail
			detail := NewBranchDetailModel(m.branch)
			return detail, detail.Init()

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.content = m.loadLogContent(m.logFiles[m.cursor])
			}

		case "down", "j":
			if m.cursor < len(m.logFiles)-1 {
				m.cursor++
				m.content = m.loadLogContent(m.logFiles[m.cursor])
			}

		case "r":
			// Manual refresh
			if len(m.logFiles) > 0 {
				m.content = m.loadLogContent(m.logFiles[m.cursor])
			}

		case "a":
			// Toggle auto-scroll
			m.autoScroll = !m.autoScroll
		}

	case logRefreshMsg:
		// Auto-refresh: check for new log files and update content
		if m.autoScroll {
			logsDir := filepath.Join(m.branch.Path, "rundir", "logs")
			entries, err := os.ReadDir(logsDir)
			if err == nil {
				var newFiles []string
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
						newFiles = append(newFiles, e.Name())
					}
				}
				// Update file list if changed
				if len(newFiles) != len(m.logFiles) {
					m.logFiles = newFiles
					if len(newFiles) > 0 && m.cursor >= len(newFiles) {
						m.cursor = 0
					}
				}
			}
			if len(m.logFiles) > 0 {
				m.content = m.loadLogContent(m.logFiles[m.cursor])
			}
		}
		return m, tea.Tick(logRefreshRate, func(t time.Time) tea.Msg {
			return logRefreshMsg(t)
		})

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View renders the log viewer.
func (m LogViewerModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render(fmt.Sprintf("── %s logs ──", m.branch.Name))
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(m.logFiles) == 0 {
		// Check if container is running (likely still building)
		if m.branch.IsRunning() {
			b.WriteString(stoppedStyle.Render("  No log files yet - container is still building."))
			b.WriteString("\n")
			b.WriteString(stoppedStyle.Render("  Logs will appear once F# build completes."))
			b.WriteString("\n")
		} else {
			b.WriteString(stoppedStyle.Render("  No log files found - container is stopped."))
			b.WriteString("\n")
		}
	} else {
		// Two-column layout: file list | log content

		// Left column: file list
		var leftCol strings.Builder
		leftCol.WriteString(lipgloss.NewStyle().Bold(true).Render("  FILES"))
		leftCol.WriteString("\n")
		leftCol.WriteString("  " + strings.Repeat("─", 24) + "\n")

		for i, f := range m.logFiles {
			cursor := "  "
			style := lipgloss.NewStyle()
			if i == m.cursor {
				cursor = "> "
				style = selectedStyle
			}
			leftCol.WriteString(fmt.Sprintf("  %s%s\n", cursor, style.Render(f)))
		}

		// Right column: log content
		var rightCol strings.Builder
		rightCol.WriteString(lipgloss.NewStyle().Bold(true).Render("  CONTENT"))

		autoIndicator := ""
		if m.autoScroll {
			autoIndicator = runningStyle.Render(" [live]")
		}
		rightCol.WriteString(autoIndicator)
		rightCol.WriteString("\n")
		rightCol.WriteString("  " + strings.Repeat("─", 50) + "\n")

		// Wrap and indent content
		contentLines := strings.Split(m.content, "\n")
		maxLines := 20
		if len(contentLines) > maxLines {
			contentLines = contentLines[len(contentLines)-maxLines:]
		}

		contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		for _, line := range contentLines {
			// Truncate long lines
			if len(line) > 70 {
				line = line[:67] + "..."
			}
			rightCol.WriteString("  " + contentStyle.Render(line) + "\n")
		}

		// Combine columns
		b.WriteString(leftCol.String())
		b.WriteString("\n")
		b.WriteString(rightCol.String())
	}

	b.WriteString("\n")

	// Help
	b.WriteString(helpStyle.Render("  ↑/↓ select file  [r]efresh  [a]uto-scroll toggle  ← back  [q]uit"))
	b.WriteString("\n")

	return b.String()
}
