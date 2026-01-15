// Package dns provides DNS setup for dark-multi.
package dns

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// TestDNS checks if wildcard DNS is working.
func TestDNS() bool {
	addrs, err := net.LookupHost("test-wildcard.dlio.localhost")
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if addr == "127.0.0.1" {
			return true
		}
	}
	return false
}

// Setup configures wildcard DNS for *.dlio.localhost -> 127.0.0.1
func Setup() error {
	fmt.Printf("Detected platform: %s\n\n", runtime.GOOS)

	// Check if already working
	if TestDNS() {
		fmt.Println("\033[0;32m✓\033[0m Wildcard DNS already configured!")
		fmt.Println("  test-wildcard.dlio.localhost -> 127.0.0.1")
		return nil
	}

	var err error
	switch runtime.GOOS {
	case "darwin":
		err = setupDarwin()
	case "linux":
		err = setupLinux()
	default:
		return fmt.Errorf("unsupported platform: %s (supported: darwin, linux)", runtime.GOOS)
	}

	if err != nil {
		return err
	}

	// Wait for DNS to propagate
	fmt.Println()
	fmt.Println("\033[0;34m>\033[0m Waiting for DNS to propagate...")
	time.Sleep(2 * time.Second)

	// Test
	if TestDNS() {
		fmt.Println("\033[0;32m✓\033[0m Wildcard DNS configured successfully!")
		fmt.Println()
		fmt.Println("Any *.dlio.localhost now resolves to 127.0.0.1")
		fmt.Println("Example: http://dark-packages.main.dlio.localhost:9000/ping")
	} else {
		fmt.Println("\033[1;33m!\033[0m DNS test failed - may need a moment to propagate")
		fmt.Println("Try: ping test.dlio.localhost")
		fmt.Println("If it doesn't resolve, you may need to restart your browser/terminal")
	}

	return nil
}

func setupDarwin() error {
	fmt.Println("Setting up wildcard DNS for macOS...")
	fmt.Println()

	// Check for Homebrew
	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("homebrew not found. Install from https://brew.sh")
	}

	// Check/install dnsmasq
	cmd := exec.Command("brew", "--prefix", "dnsmasq")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		fmt.Println("\033[0;34m>\033[0m Installing dnsmasq via Homebrew...")
		if err := exec.Command("brew", "install", "dnsmasq").Run(); err != nil {
			return fmt.Errorf("failed to install dnsmasq: %w", err)
		}
	}

	// Get brew prefix
	cmd = exec.Command("brew", "--prefix")
	prefixOut, _ := cmd.Output()
	prefix := string(prefixOut)
	if len(prefix) > 0 && prefix[len(prefix)-1] == '\n' {
		prefix = prefix[:len(prefix)-1]
	}

	dnsmasqConf := prefix + "/etc/dnsmasq.conf"
	confLine := "address=/dlio.localhost/127.0.0.1"

	// Check if already configured
	content, _ := os.ReadFile(dnsmasqConf)
	if !containsLine(string(content), confLine) {
		fmt.Println("\033[0;34m>\033[0m Configuring dnsmasq...")
		fmt.Printf("  Adding to %s\n", dnsmasqConf)
		cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo '%s' >> %s", confLine, dnsmasqConf))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to configure dnsmasq: %w", err)
		}
	} else {
		fmt.Println("\033[0;34m>\033[0m dnsmasq already configured")
	}

	// Start dnsmasq
	fmt.Println("\033[0;34m>\033[0m Starting dnsmasq service...")
	cmd = exec.Command("sudo", "brew", "services", "restart", "dnsmasq")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	// Configure resolver
	fmt.Println("\033[0;34m>\033[0m Configuring macOS resolver...")
	exec.Command("sudo", "mkdir", "-p", "/etc/resolver").Run()
	cmd = exec.Command("sudo", "sh", "-c", "echo 'nameserver 127.0.0.1' > /etc/resolver/dlio.localhost")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to configure resolver: %w", err)
	}

	return nil
}

func setupLinux() error {
	fmt.Println("Setting up wildcard DNS for Linux...")
	fmt.Println()

	// Check/install dnsmasq
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		fmt.Println("\033[0;34m>\033[0m Installing dnsmasq...")
		cmd := exec.Command("sudo", "apt", "install", "-y", "dnsmasq")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install dnsmasq: %w", err)
		}
	}

	// Configure dnsmasq
	dnsmasqConf := "/etc/dnsmasq.d/dark-multi.conf"
	confContent := "address=/dlio.localhost/127.0.0.1"

	content, _ := os.ReadFile(dnsmasqConf)
	if !containsLine(string(content), confContent) {
		fmt.Println("\033[0;34m>\033[0m Configuring dnsmasq...")
		cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo '%s' > %s", confContent, dnsmasqConf))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to configure dnsmasq: %w", err)
		}
	} else {
		fmt.Println("\033[0;34m>\033[0m dnsmasq already configured")
	}

	// Configure systemd-resolved
	fmt.Println("\033[0;34m>\033[0m Configuring systemd-resolved...")
	exec.Command("sudo", "mkdir", "-p", "/etc/systemd/resolved.conf.d").Run()
	resolvedContent := "[Resolve]\\nDNS=127.0.0.1\\nDomains=~dlio.localhost"
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' > /etc/systemd/resolved.conf.d/dark-multi.conf", resolvedContent))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	// Restart services
	fmt.Println("\033[0;34m>\033[0m Restarting services...")
	exec.Command("sudo", "systemctl", "restart", "dnsmasq").Run()
	exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run()

	return nil
}

func containsLine(content, line string) bool {
	for _, l := range splitLines(content) {
		if l == line {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
