// Package config provides configuration for dark-multi.
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var (
	// DarkRoot is where branch clones live
	DarkRoot = getEnvOrDefault("DARK_ROOT", filepath.Join(os.Getenv("HOME"), "code", "dark"))
	// DarkSource is the repo to clone from
	DarkSource = getEnvOrDefault("DARK_SOURCE", DarkRoot)
	// ConfigDir is where dark-multi stores its config
	ConfigDir = getEnvOrDefault("DARK_MULTI_CONFIG", filepath.Join(os.Getenv("HOME"), ".config", "dark-multi"))
	// OverridesDir is where branch override configs and metadata live
	OverridesDir = filepath.Join(ConfigDir, "overrides")
	// TmuxSession is the tmux session name
	TmuxSession = "dark"
	// ProxyPort is the port for the URL proxy
	ProxyPort = getEnvOrDefaultInt("DARK_MULTI_PROXY_PORT", 9000)
	// ProxyPIDFile stores the proxy process ID
	ProxyPIDFile = filepath.Join(ConfigDir, "proxy.pid")
)

const (
	// RAMPerInstanceGB is estimated RAM per devcontainer
	RAMPerInstanceGB = 6
	// CPUPerInstance is estimated CPU cores per devcontainer
	CPUPerInstance = 2
)

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvOrDefaultInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// GetSystemResources returns CPU cores and RAM in GB.
func GetSystemResources() (cpuCores int, ramGB int) {
	cpuCores = runtime.NumCPU()
	ramGB = 16 // default

	// Try to read from /proc/meminfo on Linux
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, err := strconv.Atoi(fields[1]); err == nil {
							ramGB = kb / (1024 * 1024)
						}
					}
					break
				}
			}
		}
	}

	return cpuCores, ramGB
}

// SuggestMaxInstances returns suggested max concurrent instances.
func SuggestMaxInstances() int {
	cpuCores, ramGB := GetSystemResources()
	ramLimit := max(1, (ramGB-4)/RAMPerInstanceGB)
	cpuLimit := max(1, cpuCores/CPUPerInstance)
	return min(ramLimit, cpuLimit, 10)
}
