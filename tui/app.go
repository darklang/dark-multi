package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/darklang/dark-multi/config"
)

// Run starts the TUI application.
func Run() error {
	// First-run setup
	if config.IsFirstRun() {
		if err := firstRunSetup(); err != nil {
			return err
		}
	}

	p := tea.NewProgram(
		NewGridModel(),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}

// firstRunSetup prompts for initial configuration.
func firstRunSetup() error {
	cpuCores, ramGB := config.GetSystemResources()
	suggested := config.SuggestMaxInstances()

	fmt.Println("Welcome to Dark Multi!")
	fmt.Println()
	fmt.Printf("System: %d CPU cores, %dGB RAM\n", cpuCores, ramGB)
	fmt.Printf("Suggested max concurrent containers: %d\n", suggested)
	fmt.Println()
	fmt.Printf("How many containers to run at once? [%d]: ", suggested)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	input = strings.TrimSpace(input)
	maxConcurrent := suggested
	if input != "" {
		if n, err := strconv.Atoi(input); err == nil && n > 0 {
			maxConcurrent = n
		}
	}

	if err := config.SetMaxConcurrent(maxConcurrent); err != nil {
		return err
	}

	fmt.Printf("\nSet to %d concurrent containers. You can change this later with:\n", maxConcurrent)
	fmt.Println("  export DARK_MULTI_MAX_CONCURRENT=N")
	fmt.Println()

	return nil
}
