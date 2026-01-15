// Package cli provides CLI commands for dark-multi.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/darklang/dark-multi/internal/branch"
	"github.com/darklang/dark-multi/internal/config"
	"github.com/darklang/dark-multi/internal/container"
	"github.com/darklang/dark-multi/internal/dns"
	"github.com/darklang/dark-multi/internal/proxy"
	"github.com/darklang/dark-multi/internal/tmux"
	"github.com/darklang/dark-multi/internal/tui"
)

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "multi",
		Short: "Manage multiple Dark devcontainer instances",
		Long: `Dark Multi - Manage multiple Dark devcontainer instances with tmux integration.

Examples:
  multi                      Attach to tmux session (or interactive TUI)
  multi ls                   List branches and status
  multi new fix-parser       Create new branch from main
  multi start fix-parser     Start stopped branch
  multi stop fix-parser      Stop branch (keeps files)
  multi rm fix-parser        Remove branch entirely
  multi code fix-parser      Open VS Code for branch
  multi urls                 List available URLs
  multi proxy start          Start URL proxy server
  multi setup-dns            Set up wildcard DNS`,
		Run: func(cmd *cobra.Command, args []string) {
			// Default action: attach to tmux or show TUI
			cmdAttach(cmd, args)
		},
	}

	rootCmd.AddCommand(lsCmd())
	rootCmd.AddCommand(newCmd())
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(rmCmd())
	rootCmd.AddCommand(codeCmd())
	rootCmd.AddCommand(urlsCmd())
	rootCmd.AddCommand(proxyCmd())
	rootCmd.AddCommand(setupDNSCmd())

	return rootCmd
}

func lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List branches",
		Run: func(cmd *cobra.Command, args []string) {
			cpuCores, ramGB := config.GetSystemResources()
			suggested := config.SuggestMaxInstances()

			fmt.Printf("Branches in %s:\n", config.DarkRoot)
			fmt.Printf("  System: %d cores, %dGB RAM -> suggested max: %d concurrent\n\n", cpuCores, ramGB, suggested)

			branches := branch.GetManagedBranches()
			runningCount := 0
			for _, b := range branches {
				if b.IsRunning() {
					runningCount++
				}
			}

			if len(branches) > 0 {
				for _, b := range branches {
					status := "\033[0;31mstopped\033[0m"
					if b.IsRunning() {
						status = "\033[0;32mrunning\033[0m"
					}
					changes := ""
					if b.HasChanges() {
						changes = " \033[1;33m[modified]\033[0m"
					}
					fmt.Printf("  \033[1m%-20s\033[0m %s  ports %d+/%d+%s\n",
						b.Name, status, b.PortBase(), b.BwdPortBase(), changes)
				}
				fmt.Printf("\n  Running: %d/%d suggested max\n", runningCount, suggested)
			} else {
				fmt.Println("  (no branches)")
				fmt.Println("\n  Create one: multi new <branch>")
			}

			fmt.Println()
			if tmux.SessionExists() {
				fmt.Printf("tmux session '%s' exists. Attach: multi\n", config.TmuxSession)
			} else {
				fmt.Println("No tmux session yet.")
			}
		},
	}
}

func newCmd() *cobra.Command {
	var base string
	var noCode bool

	cmd := &cobra.Command{
		Use:   "new <branch>",
		Short: "Create new branch",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Branch '%s' already exists at %s\n", name, b.Path)
				os.Exit(1)
			}

			source := branch.FindSourceRepo()
			if source == "" {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m No source repo found. Set DARK_SOURCE or create 'main' first.")
				os.Exit(1)
			}

			instanceID := branch.FindNextInstanceID()

			fmt.Printf("\033[0;34m>\033[0m Creating branch '%s' from %s\n", name, source)
			fmt.Printf("\033[0;34m>\033[0m   Instance ID: %d, ports: %d+\n", instanceID, 10011+instanceID*100)

			os.MkdirAll(config.DarkRoot, 0755)

			// Clone
			cloneCmd := exec.Command("git", "clone", source, b.Path)
			cloneCmd.Stdout = os.Stdout
			cloneCmd.Stderr = os.Stderr
			if err := cloneCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m Clone failed")
				os.Exit(1)
			}

			// Setup branch
			fmt.Printf("\033[0;34m>\033[0m Checking out branch '%s' from '%s'...\n", name, base)
			exec.Command("git", "-C", b.Path, "fetch", "origin").Run()
			checkoutCmd := exec.Command("git", "-C", b.Path, "checkout", "-b", name, "origin/"+base)
			if err := checkoutCmd.Run(); err != nil {
				exec.Command("git", "-C", b.Path, "checkout", "-b", name, base).Run()
			}

			// Write metadata
			b.WriteMetadata(instanceID)

			// Generate override config
			overridePath, err := container.GenerateOverrideConfig(b)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Failed to generate config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\033[0;34m>\033[0m Generated override config at %s\n", overridePath)

			// Start container
			fmt.Println("\033[0;34m>\033[0m Starting devcontainer...")
			if _, err := exec.LookPath("devcontainer"); err != nil {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m devcontainer CLI not found. Install: npm install -g @devcontainers/cli")
				os.Exit(1)
			}

			devCmd := exec.Command("devcontainer", "up", "--workspace-folder", b.Path, "--override-config", overridePath)
			devCmd.Stdout = os.Stdout
			devCmd.Stderr = os.Stderr
			if err := devCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m Failed to start devcontainer")
				os.Exit(1)
			}

			time.Sleep(2 * time.Second)

			containerID, err := b.ContainerID()
			if err != nil || containerID == "" {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m Container started but couldn't find it")
				os.Exit(1)
			}

			// Create tmux window
			fmt.Println("\033[0;34m>\033[0m Setting up tmux window...")
			tmux.CreateWindow(name, containerID, b.Path)

			// Open VS Code
			if !noCode {
				fmt.Println("\033[0;34m>\033[0m Opening VS Code...")
				openVSCode(b)
			}

			// Ensure proxy
			proxy.EnsureRunning()

			fmt.Printf("\033[0;32m✓\033[0m Branch '%s' ready!\n", name)
			fmt.Println("\nAttach tmux: multi")
			fmt.Println("URLs: multi urls")
		},
	}

	cmd.Flags().StringVar(&base, "from", "main", "Base branch")
	cmd.Flags().BoolVar(&noCode, "no-code", false, "Don't open VS Code")

	return cmd
}

func startCmd() *cobra.Command {
	var noCode bool

	cmd := &cobra.Command{
		Use:   "start <branch>",
		Short: "Start stopped branch",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Branch '%s' not found. Create it: multi new %s\n", name, name)
				os.Exit(1)
			}

			if !b.IsManaged() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m '%s' is not a managed branch\n", name)
				os.Exit(1)
			}

			if b.IsRunning() {
				fmt.Printf("\033[1;33m!\033[0m Branch '%s' already running\n", name)
				if !tmux.WindowExists(name) {
					fmt.Println("\033[0;34m>\033[0m Adding tmux window...")
					containerID, _ := b.ContainerID()
					tmux.CreateWindow(name, containerID, b.Path)
				}
				return
			}

			fmt.Printf("\033[0;34m>\033[0m Starting branch '%s'...\n", name)

			// Ensure override config exists
			overridePath := container.GetOverrideConfigPath(b)
			if _, err := os.Stat(overridePath); os.IsNotExist(err) {
				var err error
				overridePath, err = container.GenerateOverrideConfig(b)
				if err != nil {
					fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("\033[0;34m>\033[0m Generated override config at %s\n", overridePath)
			}

			devCmd := exec.Command("devcontainer", "up", "--workspace-folder", b.Path, "--override-config", overridePath)
			devCmd.Stdout = os.Stdout
			devCmd.Stderr = os.Stderr
			if err := devCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m Failed to start devcontainer")
				os.Exit(1)
			}

			time.Sleep(2 * time.Second)

			containerID, err := b.ContainerID()
			if err != nil || containerID == "" {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m Container started but couldn't find it")
				os.Exit(1)
			}

			fmt.Println("\033[0;34m>\033[0m Setting up tmux window...")
			tmux.CreateWindow(name, containerID, b.Path)

			if !noCode {
				fmt.Println("\033[0;34m>\033[0m Opening VS Code...")
				openVSCode(b)
			}

			proxy.EnsureRunning()

			fmt.Printf("\033[0;32m✓\033[0m Branch '%s' running\n", name)
		},
	}

	cmd.Flags().BoolVar(&noCode, "no-code", false, "Don't open VS Code")

	return cmd
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <branch>",
		Short: "Stop branch (keeps files)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Branch '%s' not found\n", name)
				os.Exit(1)
			}

			fmt.Printf("\033[0;34m>\033[0m Stopping branch '%s'...\n", name)

			tmux.KillWindow(name)

			if containerID, err := b.ContainerID(); err == nil && containerID != "" {
				fmt.Println("\033[0;34m>\033[0m Stopping container...")
				container.StopContainer(containerID)
				container.RemoveContainer(containerID)
			}

			fmt.Printf("\033[0;32m✓\033[0m Branch '%s' stopped. Files at %s\n", name, b.Path)
		},
	}
}

func rmCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "rm <branch>",
		Short: "Remove branch entirely",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Branch '%s' not found\n", name)
				os.Exit(1)
			}

			if !b.IsManaged() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m '%s' is not a managed branch\n", name)
				os.Exit(1)
			}

			if !force {
				if b.HasChanges() {
					fmt.Printf("\033[1;33m!\033[0m Branch '%s' has uncommitted changes!\n", name)
				}
				fmt.Printf("This will remove branch '%s':\n", name)
				fmt.Println("  - Stop and remove container")
				fmt.Println("  - Remove tmux window")
				fmt.Printf("  - Delete %s\n", b.Path)
				fmt.Print("Proceed? [y/N] ")

				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Aborted")
					return
				}
			}

			fmt.Printf("\033[0;34m>\033[0m Removing branch '%s'...\n", name)

			tmux.KillWindow(name)

			if containerID, err := b.ContainerID(); err == nil && containerID != "" {
				fmt.Println("\033[0;34m>\033[0m Stopping container...")
				container.StopContainer(containerID)
				container.RemoveContainer(containerID)
			}

			container.RemoveContainersByLabel(fmt.Sprintf("dark-dev-container=%s", name))

			// Remove override config
			overrideDir := filepath.Join(config.ConfigDir, "overrides", name)
			os.RemoveAll(overrideDir)

			// Remove directory
			fmt.Println("\033[0;34m>\033[0m Removing files...")
			os.RemoveAll(b.Path)

			fmt.Printf("\033[0;32m✓\033[0m Branch '%s' removed\n", name)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

func codeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "code <branch>",
		Short: "Open VS Code for branch",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			b := branch.New(name)

			if !b.Exists() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Branch '%s' not found\n", name)
				os.Exit(1)
			}

			if !b.IsRunning() {
				fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m Branch '%s' not running. Start it first: multi start %s\n", name, name)
				os.Exit(1)
			}

			fmt.Printf("\033[0;34m>\033[0m Opening VS Code for '%s'...\n", name)
			if !openVSCode(b) {
				fmt.Fprintln(os.Stderr, "\033[0;31merror:\033[0m Failed to open VS Code")
				os.Exit(1)
			}
		},
	}
}

func urlsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "urls",
		Short: "List available URLs for branches",
		Run: func(cmd *cobra.Command, args []string) {
			branches := branch.GetManagedBranches()
			var running []*branch.Branch
			for _, b := range branches {
				if b.IsRunning() {
					running = append(running, b)
				}
			}

			if len(running) == 0 {
				fmt.Println("No running branches.")
				return
			}

			pid, isRunning := proxy.IsRunning()
			status := "not running"
			if isRunning {
				status = fmt.Sprintf("running (PID %d)", pid)
			}

			fmt.Printf("Proxy: %s\n", status)
			fmt.Printf("Port: %d\n\n", config.ProxyPort)

			fmt.Println("Running branches:")
			for _, b := range running {
				fmt.Println()
				fmt.Printf("  \033[1m%s\033[0m (ID=%d)\n", b.Name, b.InstanceID())
				fmt.Printf("    Direct:  curl -H 'Host: dark-packages.dlio.localhost' http://localhost:%d/ping\n", b.BwdPortBase())
				if isRunning {
					fmt.Printf("    Proxy:   http://dark-packages.%s.dlio.localhost:%d/ping\n", b.Name, config.ProxyPort)
				}
				fmt.Printf("    BwdServer ports: %d-%d\n", b.BwdPortBase(), b.BwdPortBase()+1)
				fmt.Printf("    Test ports: %d-%d\n", b.PortBase(), b.PortBase()+19)
			}

			if !isRunning {
				fmt.Println()
				fmt.Println("Start proxy for nice URLs: multi proxy start")
			}

			fmt.Println()
			fmt.Println("For browser access, run: multi setup-dns (one-time)")
		},
	}
}

func proxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy <action>",
		Short: "Manage URL proxy server",
		Args:  cobra.ExactArgs(1),
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
				fmt.Println()
				fmt.Println("For browser access, run: multi setup-dns (one-time)")
				fmt.Printf("Then visit: http://dark-packages.<branch>.dlio.localhost:%d/ping\n", config.ProxyPort)

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
				// Foreground mode - used by backgrounded process
				// Don't check IsRunning() here because the parent already wrote our PID to the file
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

func cmdAttach(cmd *cobra.Command, args []string) {
	// Launch TUI
	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\033[0;31merror:\033[0m %v\n", err)
		os.Exit(1)
	}
}

func openVSCode(b *branch.Branch) bool {
	if !b.IsRunning() {
		return false
	}

	// Use devcontainer CLI
	if _, err := exec.LookPath("devcontainer"); err == nil {
		cmd := exec.Command("devcontainer", "open", b.Path)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return true
		}
	}

	// Fallback: use code --remote
	if _, err := exec.LookPath("code"); err == nil {
		containerID, _ := b.ContainerID()
		hexID := fmt.Sprintf("%x", containerID)
		cmd := exec.Command("code", "--remote", fmt.Sprintf("attached-container+%s", hexID), "/home/dark/app")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run() == nil
	}

	fmt.Println("\033[1;33m!\033[0m Neither devcontainer CLI nor VS Code found")
	return false
}
