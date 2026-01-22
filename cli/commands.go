// Package cli provides CLI commands for dark-multi.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/dns"
	"github.com/darklang/dark-multi/inotify"
	"github.com/darklang/dark-multi/proxy"
	"github.com/darklang/dark-multi/queue"
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
	rootCmd.AddCommand(lsCmd())
	rootCmd.AddCommand(newCmd())
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(rmCmd())
	rootCmd.AddCommand(setForkCmd())
	rootCmd.AddCommand(queueCmd())

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

func lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all managed branches",
		Run: func(cmd *cobra.Command, args []string) {
			branches := branch.GetManagedBranches()
			if len(branches) == 0 {
				fmt.Println("No branches. Create one with: multi new <name>")
				return
			}
			for _, b := range branches {
				status := "\033[0;31m○\033[0m" // red stopped
				if b.IsRunning() {
					status = "\033[0;32m●\033[0m" // green running
				}
				fmt.Printf("%s %s\n", status, b.Name)
			}
		},
	}
}

func newCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new branch",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]

			fmt.Printf("Creating %s...\n", name)
			b, err := branch.Create(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\033[0;32m✓\033[0m Created %s (ID=%d)\n", name, b.InstanceID())
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a branch's container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m branch %s does not exist\n", name)
				os.Exit(1)
			}

			if b.IsRunning() {
				fmt.Printf("\033[1;33m!\033[0m %s is already running\n", name)
				return
			}

			fmt.Printf("Starting %s...\n", name)
			if err := branch.Start(b); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\033[0;32m✓\033[0m Started %s\n", name)
		},
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a branch's container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m branch %s does not exist\n", name)
				os.Exit(1)
			}

			if !b.IsRunning() {
				fmt.Printf("\033[1;33m!\033[0m %s is not running\n", name)
				return
			}

			fmt.Printf("Stopping %s...\n", name)
			if err := branch.Stop(b); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\033[0;32m✓\033[0m Stopped %s\n", name)
		},
	}
}

func rmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a branch entirely",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() && !b.IsManaged() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m branch %s does not exist\n", name)
				os.Exit(1)
			}

			fmt.Printf("Removing %s...\n", name)
			if err := branch.Remove(b); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\033[0;32m✓\033[0m Removed %s\n", name)
		},
	}
}

func setForkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-fork <url>",
		Short: "Set your GitHub fork URL",
		Long: `Set the GitHub fork URL for your Dark repository.

This is where branches will push to. Use your personal fork:
  multi set-fork git@github.com:USERNAME/dark.git

Current setting can be viewed with:
  multi set-fork`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				// Show current setting
				current := config.GetGitHubFork()
				if current == "" {
					fmt.Println("GitHub fork not configured")
					fmt.Println("Set with: multi set-fork git@github.com:USERNAME/dark.git")
				} else {
					fmt.Printf("GitHub fork: %s\n", current)
				}
				return
			}

			url := args[0]
			if err := config.SetGitHubFork(url); err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\033[0;32m✓\033[0m GitHub fork set to: %s\n", url)
		},
	}
}

func queueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue <action>",
		Short: "Manage the task queue",
		Long: `Manage the automated task queue.

Actions:
  init    Initialize queue with predefined tasks
  ls      List all tasks in queue
  add     Add a task (multi queue add <id> <prompt>)
  status  Show queue status summary`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			action := args[0]
			q := queue.Get()

			switch action {
			case "init":
				fmt.Println("Initializing task queue...")
				if err := queue.PopulateInitialQueue(); err != nil {
					fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
					os.Exit(1)
				}
				tasks := q.GetAll()
				fmt.Printf("\033[0;32m✓\033[0m Queue initialized with %d tasks\n", len(tasks))

				// Show summary by status
				ready := len(q.GetByStatus(queue.StatusReady))
				needsPrompt := len(q.GetByStatus(queue.StatusNeedsPrompt))
				fmt.Printf("  %d ready, %d need prompts\n", ready, needsPrompt)

			case "ls":
				tasks := q.GetAll()
				if len(tasks) == 0 {
					fmt.Println("Queue is empty. Run 'multi queue init' to populate.")
					return
				}

				fmt.Printf("%-25s %-15s %s\n", "ID", "STATUS", "NAME")
				fmt.Println("─────────────────────────────────────────────────────────────")
				for _, t := range tasks {
					fmt.Printf("%-25s %s %-12s %s\n", t.ID, t.Status.Icon(), t.Status.Display(), t.Name)
				}

			case "status":
				tasks := q.GetAll()
				running := q.CountRunning()
				ready := len(q.GetByStatus(queue.StatusReady))
				waiting := len(q.GetByStatus(queue.StatusWaiting))
				done := len(q.GetByStatus(queue.StatusDone))
				needsPrompt := len(q.GetByStatus(queue.StatusNeedsPrompt))

				fmt.Printf("Queue Status:\n")
				fmt.Printf("  🔄 Running:      %d / %d max\n", running, queue.MaxConcurrent)
				fmt.Printf("  ⏳ Ready:        %d\n", ready)
				fmt.Printf("  📝 Needs Prompt: %d\n", needsPrompt)
				fmt.Printf("  ⏸️  Waiting:      %d\n", waiting)
				fmt.Printf("  ✅ Done:         %d\n", done)
				fmt.Printf("  ─────────────────\n")
				fmt.Printf("  Total:          %d\n", len(tasks))

			case "add":
				if len(args) < 3 {
					fmt.Fprintln(os.Stderr, "Usage: multi queue add <id> <prompt>")
					os.Exit(1)
				}
				id := args[1]
				prompt := args[2]
				q.Add(id, id, prompt, 50)
				q.Save()
				fmt.Printf("\033[0;32m✓\033[0m Added task: %s\n", id)

			default:
				fmt.Fprintf(os.Stderr, "Unknown action: %s\nUse: init, ls, status, add\n", action)
				os.Exit(1)
			}
		},
	}

	return cmd
}
