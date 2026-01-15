// Package inotify provides inotify limit setup for dark-multi.
package inotify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const (
	// Recommended values for multiple containers with file watchers
	RecommendedWatches   = 524288
	RecommendedInstances = 512
)

// CurrentLimits returns current inotify limits.
func CurrentLimits() (watches, instances int, err error) {
	if runtime.GOOS != "linux" {
		return 0, 0, fmt.Errorf("inotify limits only apply to Linux")
	}

	watchesOut, err := exec.Command("sysctl", "-n", "fs.inotify.max_user_watches").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get max_user_watches: %w", err)
	}
	watches, _ = strconv.Atoi(strings.TrimSpace(string(watchesOut)))

	instancesOut, err := exec.Command("sysctl", "-n", "fs.inotify.max_user_instances").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get max_user_instances: %w", err)
	}
	instances, _ = strconv.Atoi(strings.TrimSpace(string(instancesOut)))

	return watches, instances, nil
}

// NeedsIncrease returns true if current limits are below recommended.
func NeedsIncrease() bool {
	watches, instances, err := CurrentLimits()
	if err != nil {
		return false
	}
	return watches < RecommendedWatches || instances < RecommendedInstances
}

// Setup increases inotify limits for running multiple containers with file watchers.
func Setup() error {
	if runtime.GOOS != "linux" {
		fmt.Println("\033[1;33m!\033[0m inotify limits only apply to Linux")
		fmt.Println("  macOS uses FSEvents which doesn't have these limits")
		return nil
	}

	watches, instances, err := CurrentLimits()
	if err != nil {
		return err
	}

	fmt.Printf("Current inotify limits:\n")
	fmt.Printf("  max_user_watches:   %d\n", watches)
	fmt.Printf("  max_user_instances: %d\n", instances)
	fmt.Println()

	// Check if already sufficient
	if watches >= RecommendedWatches && instances >= RecommendedInstances {
		fmt.Println("\033[0;32m✓\033[0m inotify limits already sufficient!")
		return nil
	}

	fmt.Printf("Recommended limits for multiple containers:\n")
	fmt.Printf("  max_user_watches:   %d\n", RecommendedWatches)
	fmt.Printf("  max_user_instances: %d\n", RecommendedInstances)
	fmt.Println()

	// Apply temporary changes
	fmt.Println("\033[0;34m>\033[0m Applying changes (requires sudo)...")

	if watches < RecommendedWatches {
		cmd := exec.Command("sudo", "sysctl", "-w", fmt.Sprintf("fs.inotify.max_user_watches=%d", RecommendedWatches))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set max_user_watches: %w", err)
		}
	}

	if instances < RecommendedInstances {
		cmd := exec.Command("sudo", "sysctl", "-w", fmt.Sprintf("fs.inotify.max_user_instances=%d", RecommendedInstances))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set max_user_instances: %w", err)
		}
	}

	// Make persistent
	fmt.Println()
	fmt.Println("\033[0;34m>\033[0m Making changes persistent...")

	confContent := fmt.Sprintf(`# dark-multi: Increased inotify limits for multiple dev containers
fs.inotify.max_user_watches=%d
fs.inotify.max_user_instances=%d
`, RecommendedWatches, RecommendedInstances)

	confPath := "/etc/sysctl.d/99-dark-multi-inotify.conf"
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s << 'EOF'\n%sEOF", confPath, confContent))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("\033[1;33m!\033[0m Could not persist changes - they will reset on reboot")
		fmt.Printf("  To make permanent, add to /etc/sysctl.conf:\n")
		fmt.Printf("    fs.inotify.max_user_watches=%d\n", RecommendedWatches)
		fmt.Printf("    fs.inotify.max_user_instances=%d\n", RecommendedInstances)
	} else {
		fmt.Printf("  Created %s\n", confPath)
	}

	fmt.Println()
	fmt.Println("\033[0;32m✓\033[0m inotify limits increased!")
	fmt.Println("  Each container's file watcher can now handle more files")

	return nil
}
