package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stachu/dark-multi/internal/tmux"
)

// Run starts the TUI application.
func Run() error {
	p := tea.NewProgram(
		NewHomeModel(),
		tea.WithAltScreen(),
	)

	model, err := p.Run()
	if err != nil {
		return err
	}

	// If we quit to attach to tmux, do it after the TUI exits
	if m, ok := model.(HomeModel); ok && m.quitting {
		if tmux.SessionExists() {
			// Attach to tmux (this replaces the process)
			tmux.AttachExec()
		}
	}

	return nil
}
