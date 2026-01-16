package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI application.
func Run() error {
	p := tea.NewProgram(
		NewGridModel(),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}
