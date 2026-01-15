package main

import (
	"fmt"
	"os"

	"github.com/darklang/dark-multi/cli"
	"github.com/darklang/dark-multi/tui"
)

func main() {
	// If no args provided, launch interactive TUI
	if len(os.Args) == 1 {
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise, run CLI commands
	cmd := cli.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
