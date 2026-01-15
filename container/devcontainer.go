// Package container provides devcontainer and Docker operations.
package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
)

// GetOverrideConfigPath returns the path to the override config for a branch.
func GetOverrideConfigPath(b *branch.Branch) string {
	return filepath.Join(config.ConfigDir, "overrides", b.Name, "devcontainer.json")
}

// GenerateOverrideConfig generates a devcontainer override config for a branch.
// Returns the path to the generated config.
func GenerateOverrideConfig(b *branch.Branch) (string, error) {
	overrideDir := filepath.Join(config.ConfigDir, "overrides", b.Name)
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create override dir: %w", err)
	}
	overridePath := filepath.Join(overrideDir, "devcontainer.json")

	// Read original devcontainer.json
	originalPath := filepath.Join(b.Path, ".devcontainer", "devcontainer.json")
	content, err := os.ReadFile(originalPath)
	if err != nil {
		return "", fmt.Errorf("failed to read devcontainer.json: %w", err)
	}

	// Strip // comments (devcontainer.json allows them)
	var lines []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Remove inline comments (crude but works)
		if idx := strings.Index(line, "//"); idx > 0 {
			// Make sure it's not inside a string
			beforeComment := line[:idx]
			if strings.Count(beforeComment, "\"")%2 == 0 {
				line = strings.TrimRight(beforeComment, " \t")
			}
		}
		lines = append(lines, line)
	}
	content = []byte(strings.Join(lines, "\n"))

	// Parse JSON
	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}

	// Build port mappings
	var portArgs []string
	// BwdServer ports
	portArgs = append(portArgs, "-p", fmt.Sprintf("%d:11001", b.BwdPortBase()))
	portArgs = append(portArgs, "-p", fmt.Sprintf("%d:11002", b.BwdPortBase()+1))
	// Test server ports (10011-10030)
	for i := 0; i < 20; i++ {
		portArgs = append(portArgs, "-p", fmt.Sprintf("%d:%d", b.PortBase()+i, 10011+i))
	}

	// Host ports for forwardPorts
	var hostPorts []interface{}
	for i := 0; i < 20; i++ {
		hostPorts = append(hostPorts, b.PortBase()+i)
	}
	hostPorts = append(hostPorts, b.BwdPortBase(), b.BwdPortBase()+1)

	// Apply overrides
	cfg["name"] = fmt.Sprintf("dark-%s", b.Name)
	cfg["forwardPorts"] = hostPorts

	// Merge runArgs - filter out existing hostname/label/name/-p args
	var filteredArgs []string
	if originalArgs, ok := cfg["runArgs"].([]interface{}); ok {
		skipNext := false
		for _, arg := range originalArgs {
			argStr, ok := arg.(string)
			if !ok {
				continue
			}
			if skipNext {
				skipNext = false
				continue
			}
			if argStr == "--hostname" || argStr == "--label" || argStr == "--name" || argStr == "-p" {
				skipNext = true
				continue
			}
			if strings.HasPrefix(argStr, "--hostname=") || strings.HasPrefix(argStr, "--label=") ||
				strings.HasPrefix(argStr, "--name=") || strings.HasPrefix(argStr, "-p=") {
				continue
			}
			filteredArgs = append(filteredArgs, argStr)
		}
	}

	// Add our args
	var newRunArgs []interface{}
	for _, arg := range filteredArgs {
		newRunArgs = append(newRunArgs, arg)
	}
	newRunArgs = append(newRunArgs,
		"--hostname", fmt.Sprintf("dark-%s", b.Name),
		"--label", fmt.Sprintf("dark-dev-container=%s", b.Name),
		"--name", fmt.Sprintf("dark-%s", b.Name),
	)
	for _, arg := range portArgs {
		newRunArgs = append(newRunArgs, arg)
	}
	cfg["runArgs"] = newRunArgs

	// Override mounts with branch-specific volumes
	homeDir, _ := os.UserHomeDir()
	claudeDir := filepath.Join(homeDir, ".claude")
	cfg["mounts"] = []interface{}{
		fmt.Sprintf("type=volume,src=dark_nuget_%s,dst=/home/dark/.nuget", b.Name),
		fmt.Sprintf("type=volume,src=dark-vscode-ext-%s,dst=/home/dark/.vscode-server/extensions", b.Name),
		fmt.Sprintf("type=volume,src=dark-vscode-ext-insiders-%s,dst=/home/dark/.vscode-server-insiders/extensions", b.Name),
		// Mount Claude credentials and config (shared across branches)
		fmt.Sprintf("type=bind,src=%s,dst=/home/dark/.claude,consistency=cached", claudeDir),
	}

	// Add Claude installation to postCreateCommand if not already there
	postCreate := ""
	if existing, ok := cfg["postCreateCommand"].(string); ok {
		postCreate = existing
	}
	if !strings.Contains(postCreate, "claude-code") {
		// Use sudo since container user doesn't have root perms for global npm
		claudeInstall := "sudo npm install -g @anthropic-ai/claude-code 2>/dev/null || true"
		if postCreate != "" {
			postCreate = claudeInstall + " && " + postCreate
		} else {
			postCreate = claudeInstall
		}
		cfg["postCreateCommand"] = postCreate
	}

	// Write merged config
	output, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(overridePath, output, 0644); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return overridePath, nil
}
