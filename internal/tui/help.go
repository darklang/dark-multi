package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
)

// HelpModel displays help information.
type HelpModel struct {
	width  int
	height int
}

// NewHelpModel creates a help screen.
func NewHelpModel() HelpModel {
	return HelpModel{}
}

// Init initializes help model.
func (m HelpModel) Init() tea.Cmd {
	return nil
}

// Update handles input.
func (m HelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Any key returns to home
		home := NewHomeModel()
		return home, home.Init()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View renders help.
func (m HelpModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("DARK MULTI - Help"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("Navigation"))
	b.WriteString("\n")
	b.WriteString("  ↑/k         Move up\n")
	b.WriteString("  ↓/j         Move down\n")
	b.WriteString("  enter/l     Select / View details\n")
	b.WriteString("  esc/h       Back\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Branch Actions"))
	b.WriteString("\n")
	b.WriteString("  s           Start selected branch\n")
	b.WriteString("  S           Stop selected branch\n")
	b.WriteString("  c           Open VS Code for branch\n")
	b.WriteString("  t           Attach to tmux session\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("System"))
	b.WriteString("\n")
	b.WriteString("  p           Toggle proxy server\n")
	b.WriteString("  r           Refresh branch list\n")
	b.WriteString("  ?           Show this help\n")
	b.WriteString("  q           Quit\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Branch Detail View"))
	b.WriteString("\n")
	b.WriteString("  ↑/↓         Navigate URLs\n")
	b.WriteString("  enter/o     Open URL in browser\n")
	b.WriteString("  s/S         Start/Stop branch\n")
	b.WriteString("  c           Open VS Code\n")
	b.WriteString("  t           Attach to tmux\n")
	b.WriteString("  esc         Back to home\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Claude Status Icons"))
	b.WriteString("\n")
	b.WriteString("  ⏳          Claude waiting for your input\n")
	b.WriteString("  🔄          Claude is working\n")
	b.WriteString("  (none)      Claude idle or no recent activity\n")
	b.WriteString("\n")

	b.WriteString(helpStyle.Render("Press any key to close"))
	b.WriteString("\n")

	return b.String()
}
