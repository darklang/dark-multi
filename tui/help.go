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
	b.WriteString("  ↑/↓         Move up/down\n")
	b.WriteString("  enter       Select / View details\n")
	b.WriteString("  esc         Back\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Branch Actions"))
	b.WriteString("\n")
	b.WriteString("  n           New branch (prompts for name)\n")
	b.WriteString("  d           Delete branch (with confirmation)\n")
	b.WriteString("  s           Start branch\n")
	b.WriteString("  k           Kill (stop) branch\n")
	b.WriteString("  t           Open terminal (CLI + claude panes)\n")
	b.WriteString("  c           Open VS Code\n")
	b.WriteString("  m           Open Matter (dark-packages canvas)\n")
	b.WriteString("  l           View logs (from detail view)\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Views"))
	b.WriteString("\n")
	b.WriteString("  g           Grid view (all Claude sessions tiled)\n")
	b.WriteString("  enter       Detail view (branch info + URLs)\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Grid View"))
	b.WriteString("\n")
	b.WriteString("  arrows      Navigate cells\n")
	b.WriteString("  enter       Focus on session (attach tmux)\n")
	b.WriteString("  s           Start stopped branch\n")
	b.WriteString("  esc         Back to dashboard\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Focused View (tmux)"))
	b.WriteString("\n")
	b.WriteString("  ctrl-b d    Detach (back to grid)\n")
	b.WriteString("  ctrl-b [    Scroll mode\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("System"))
	b.WriteString("\n")
	b.WriteString("  p           Toggle proxy server\n")
	b.WriteString("  r           Refresh\n")
	b.WriteString("  ?           Help\n")
	b.WriteString("  q           Quit\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Display"))
	b.WriteString("\n")
	b.WriteString("  ● / ○       Running / stopped\n")
	b.WriteString("  3c +50 -10  Commits, lines added/removed vs main\n")
	b.WriteString("  ⏳ / ⚡      Claude waiting / working\n")
	b.WriteString("\n")

	b.WriteString(helpStyle.Render("Press any key to close"))
	b.WriteString("\n")

	return b.String()
}
