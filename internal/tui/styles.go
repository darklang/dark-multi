// Package tui provides the interactive terminal UI for dark-multi.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")) // green

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")) // gray

	modifiedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")) // yellow

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")) // red
)
