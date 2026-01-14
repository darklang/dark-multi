// Package proxy provides the URL proxy server for dark-multi.
package proxy

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/stachu/dark-multi/internal/branch"
	"github.com/stachu/dark-multi/internal/config"
)

// BranchPorts caches branch name -> port mappings.
var BranchPorts = make(map[string]int)

// RefreshBranchPorts updates the branch port cache.
func RefreshBranchPorts() {
	BranchPorts = make(map[string]int)
	for _, b := range branch.GetManagedBranches() {
		if b.IsRunning() {
			BranchPorts[b.Name] = b.BwdPortBase()
		}
	}
}

// Start starts the proxy server. Returns PID if backgrounded.
func Start(port int, background bool) (int, error) {
	if err := os.MkdirAll(config.ConfigDir, 0755); err != nil {
		return 0, err
	}

	if background {
		// Fork to background using a new process
		execPath, err := os.Executable()
		if err != nil {
			return 0, err
		}

		cmd := exec.Command(execPath, "proxy", "fg")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}

		// Redirect to /dev/null
		devnull, _ := os.Open(os.DevNull)
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull

		if err := cmd.Start(); err != nil {
			return 0, err
		}

		pid := cmd.Process.Pid
		os.WriteFile(config.ProxyPIDFile, []byte(strconv.Itoa(pid)), 0644)
		return pid, nil
	}

	// Foreground mode - run the server
	RefreshBranchPorts()

	// Listen on both IPv4 and IPv6
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: &ProxyHandler{},
	}

	// Use a listener that supports dual-stack
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return 0, err
	}

	return 0, server.Serve(ln)
}

// Stop stops the proxy server.
func Stop() bool {
	data, err := os.ReadFile(config.ProxyPIDFile)
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(config.ProxyPIDFile)
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(config.ProxyPIDFile)
		return false
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		os.Remove(config.ProxyPIDFile)
		return false
	}

	os.Remove(config.ProxyPIDFile)
	return true
}

// IsRunning checks if the proxy is running. Returns PID if running.
func IsRunning() (int, bool) {
	data, err := os.ReadFile(config.ProxyPIDFile)
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(config.ProxyPIDFile)
		return 0, false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(config.ProxyPIDFile)
		return 0, false
	}

	// Signal 0 checks if process exists
	if err := process.Signal(syscall.Signal(0)); err != nil {
		os.Remove(config.ProxyPIDFile)
		return 0, false
	}

	return pid, true
}

// EnsureRunning starts the proxy if not already running.
func EnsureRunning() error {
	if _, running := IsRunning(); running {
		return nil
	}
	pid, err := Start(config.ProxyPort, true)
	if err != nil {
		return err
	}
	if pid > 0 {
		fmt.Printf("> Started proxy on port %d (PID %d)\n", config.ProxyPort, pid)
	}
	return nil
}
