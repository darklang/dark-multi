// Package cli provides CLI commands for dark-multi.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/dns"
	"github.com/darklang/dark-multi/inotify"
	"github.com/darklang/dark-multi/proxy"
	"github.com/darklang/dark-multi/tui"
)

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "multi",
		Short: "Manage multiple Dark devcontainer instances",
		Long: `Dark Multi - Manage multiple Dark devcontainer instances with tmux integration.

Run 'multi' with no arguments to launch the interactive TUI.

TUI shortcuts:
  n           New branch (prompts for name)
  d           Delete branch
  s/k         Start/Kill branch
  t           Terminal (tmux)
  c           VS Code
  p           Toggle proxy
  enter       Branch details & URLs
  ?           Help`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := tui.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.AddCommand(proxyCmd())
	rootCmd.AddCommand(setupDNSCmd())
	rootCmd.AddCommand(setupInotifyCmd())

	return rootCmd
}

func proxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy <action>",
		Short: "Manage URL proxy server",
		Long: `Manage the URL proxy server.

Actions:
  start   Start proxy in background
  stop    Stop proxy
  status  Check if proxy is running
  fg      Run proxy in foreground (for debugging)`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			action := args[0]

			switch action {
			case "start":
				if pid, running := proxy.IsRunning(); running {
					fmt.Printf("\033[1;33m!\033[0m Proxy already running (PID %d)\n", pid)
					return
				}

				fmt.Printf("\033[0;34m>\033[0m Starting proxy on port %d...\n", config.ProxyPort)
				pid, err := proxy.Start(config.ProxyPort, true)
				if err != nil {
					fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("\033[0;32m✓\033[0m Proxy started (PID %d)\n", pid)

			case "stop":
				if proxy.Stop() {
					fmt.Println("\033[0;32m✓\033[0m Proxy stopped")
				} else {
					fmt.Println("\033[1;33m!\033[0m Proxy not running")
				}

			case "status":
				if pid, running := proxy.IsRunning(); running {
					fmt.Printf("Proxy running (PID %d) on port %d\n", pid, config.ProxyPort)
				} else {
					fmt.Println("Proxy not running")
				}

			case "fg":
				fmt.Printf("\033[0;34m>\033[0m Starting proxy on port %d (foreground)...\n", config.ProxyPort)
				proxy.Start(config.ProxyPort, false)

			default:
				fmt.Fprintf(os.Stderr, "Unknown action: %s\nUse: start, stop, status, fg\n", action)
				os.Exit(1)
			}
		},
	}

	return cmd
}

func setupDNSCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup-dns",
		Short: "Set up wildcard DNS for *.dlio.localhost",
		Run: func(cmd *cobra.Command, args []string) {
			if err := dns.Setup(); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func setupInotifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup-inotify",
		Short: "Increase inotify limits for multiple containers (Linux only)",
		Long: `Increase inotify file watcher limits for running multiple dev containers.

Each container's file watcher (for hot reload, etc.) consumes inotify watches.
The default Linux limits are too low for multiple containers.

This command:
  1. Increases fs.inotify.max_user_watches to 524288
  2. Increases fs.inotify.max_user_instances to 512
  3. Makes changes persistent via /etc/sysctl.d/

Requires sudo. Only needed on Linux (macOS uses FSEvents).`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := inotify.Setup(); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
		},
	}
}
