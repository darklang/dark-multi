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
		// Any key returns to grid
		grid := NewGridModel()
		return grid, grid.Init()

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
	b.WriteString("  x           Delete branch (with confirmation)\n")
	b.WriteString("  s           Start branch\n")
	b.WriteString("  k           Kill (stop) branch\n")
	b.WriteString("  c           Open Claude\n")
	b.WriteString("  t           Open terminal (bash)\n")
	b.WriteString("  e           Open VS Code (editor)\n")
	b.WriteString("  d           Diff (open gitk)\n")
	b.WriteString("  m           Open Matter (dark-packages canvas)\n")
	b.WriteString("  l           View logs\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Task Queue"))
	b.WriteString("\n")
	b.WriteString("  p           Edit pre-prompt (task definition)\n")
	b.WriteString("  f           Cycle filter (running/ready/all)\n")
	b.WriteString("\n")
	b.WriteString("  Tasks auto-start from queue when slots available.\n")
	b.WriteString("  Queue managed via: multi queue init/ls/status\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Queue Status"))
	b.WriteString("\n")
	b.WriteString("  📝 needs-prompt  Waiting for prompt to be written\n")
	b.WriteString("  ⏳ ready         Has prompt, waiting for slot\n")
	b.WriteString("  🔄 running       Container active, Claude working\n")
	b.WriteString("  ⏸️ waiting        Stuck or needs human input\n")
	b.WriteString("  ✅ done          Task complete\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Grid View"))
	b.WriteString("\n")
	b.WriteString("  arrows      Navigate branches\n")
	b.WriteString("  enter/c     Open Claude\n")
	b.WriteString("  g           Switch to grid view\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Focused View (tmux)"))
	b.WriteString("\n")
	b.WriteString("  ctrl-b d    Detach (back to grid)\n")
	b.WriteString("  ctrl-b [    Scroll mode\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("System"))
	b.WriteString("\n")
	b.WriteString("  ?           Help\n")
	b.WriteString("  q           Quit\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Display"))
	b.WriteString("\n")
	b.WriteString("  ● / ◐ / ○   Ready / starting / stopped\n")
	b.WriteString("  [3/5]       Container startup progress\n")
	b.WriteString("  3c +50 -10  Commits, lines added/removed vs main\n")
	b.WriteString("  💬 / ⚡      Claude waiting / working\n")
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("Startup Phases"))
	b.WriteString("\n")
	b.WriteString("  [1/6]       Starting container\n")
	b.WriteString("  [2/6]       Building tree-sitter\n")
	b.WriteString("  [3/6]       Building F#\n")
	b.WriteString("  [4/6]       Starting BwdServer\n")
	b.WriteString("  [5/6]       Loading packages\n")
	b.WriteString("  [6/6]       Ready\n")
	b.WriteString("\n")

	b.WriteString(helpStyle.Render("Press any key to close"))
	b.WriteString("\n")

	return b.String()
}
