// Package container provides devcontainer and Docker operations.
package container

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darklang/dark-multi/config"
)

// Pre-built image configuration.
// When the local Dockerfile matches this hash, we use the pre-built image
// instead of rebuilding, which saves significant startup time.
const (
	// SHA256 hash of the Dockerfile used to build the base image
	baseDockerfileHash = "83d9d227c58ffdcdb35cb1bfade4626d947007112cc1b4d59223f0031eca4fb2"
	// Pre-built image on Docker Hub
	baseImage = "darklang/dark-base:7dc786d"
)

// logToFile writes debug output to /tmp/dark-multi.log
func logToFile(format string, args ...interface{}) {
	f, err := os.OpenFile("/tmp/dark-multi.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	msg := fmt.Sprintf(format, args...)
	f.WriteString(fmt.Sprintf("[devcontainer] %s\n", msg))
}

// BranchInfo contains the branch information needed for container operations.
type BranchInfo interface {
	GetName() string
	GetPath() string
	PortBase() int
	BwdPortBase() int
}

// GetOverrideConfigPath returns the path to the override config for a branch.
func GetOverrideConfigPath(name string) string {
	return filepath.Join(config.ConfigDir, "overrides", name, "devcontainer.json")
}

// dockerfileMatchesBase checks if the Dockerfile in the branch matches
// the hash of the Dockerfile used to build the pre-built base image.
func dockerfileMatchesBase(branchPath string) bool {
	// Read the Dockerfile - it may be in root or .devcontainer
	dockerfilePath := filepath.Join(branchPath, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		dockerfilePath = filepath.Join(branchPath, ".devcontainer", "Dockerfile")
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return false // Can't read, fall back to build
	}

	hash := sha256.Sum256(content)
	hexHash := hex.EncodeToString(hash[:])
	return hexHash == baseDockerfileHash
}

// GenerateOverrideConfig generates a devcontainer override config for a branch.
// Returns the path to the generated config.
func GenerateOverrideConfig(b BranchInfo) (string, error) {
	name := b.GetName()
	branchPath := b.GetPath()

	overrideDir := filepath.Join(config.ConfigDir, "overrides", name)
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create override dir: %w", err)
	}
	overridePath := filepath.Join(overrideDir, "devcontainer.json")

	// Read original devcontainer.json
	originalPath := filepath.Join(branchPath, ".devcontainer", "devcontainer.json")
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
	cfg["name"] = fmt.Sprintf("dark-%s", name)
	cfg["forwardPorts"] = hostPorts

	// Use pre-built image if Dockerfile matches base, otherwise build locally
	if dockerfileMatchesBase(branchPath) {
		// Remove build section and use pre-built image
		delete(cfg, "build")
		cfg["image"] = baseImage
		logToFile("Using pre-built image: %s", baseImage)
	} else {
		logToFile("Dockerfile differs from base - will build locally")
	}

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
		"--hostname", fmt.Sprintf("dark-%s", name),
		"--label", fmt.Sprintf("dark-dev-container=%s", name),
		"--name", fmt.Sprintf("dark-%s", name),
	)
	for _, arg := range portArgs {
		newRunArgs = append(newRunArgs, arg)
	}
	cfg["runArgs"] = newRunArgs

	// Override mounts with branch-specific volumes
	homeDir, _ := os.UserHomeDir()
	claudeDir := filepath.Join(homeDir, ".claude")
	claudeJson := filepath.Join(homeDir, ".claude.json")
	cfg["mounts"] = []interface{}{
		fmt.Sprintf("type=volume,src=dark_nuget_%s,dst=/home/dark/.nuget", name),
		fmt.Sprintf("type=volume,src=dark-vscode-ext-%s,dst=/home/dark/.vscode-server/extensions", name),
		fmt.Sprintf("type=volume,src=dark-vscode-ext-insiders-%s,dst=/home/dark/.vscode-server-insiders/extensions", name),
		// Mount Claude credentials and config (shared across branches)
		fmt.Sprintf("type=bind,src=%s,dst=/home/dark/.claude,consistency=cached", claudeDir),
		// Mount .claude.json for auth/theme (writable - Claude needs to save settings)
		fmt.Sprintf("type=bind,src=%s,dst=/home/dark/.claude.json", claudeJson),
	}

	// Add Claude installation to postCreateCommand
	postCreate := ""
	if existing, ok := cfg["postCreateCommand"].(string); ok {
		postCreate = existing
	}

	// Ensure Claude is installed (auth comes from mounted .claude.json)
	claudeInstall := "sudo npm install -g @anthropic-ai/claude-code 2>/dev/null || true"

	if !strings.Contains(postCreate, "claude-code") {
		if postCreate != "" {
			postCreate = claudeInstall + " && " + postCreate
		} else {
			postCreate = claudeInstall
		}
		cfg["postCreateCommand"] = postCreate
	}

	// Inject OAuth token if available (from ~/.config/dark-multi/oauth_token)
	// Combined with mounted ~/.claude.json (which has hasCompletedOnboarding: true),
	// this enables auto-auth without /login
	oauthTokenPath := filepath.Join(config.ConfigDir, "oauth_token")
	if tokenBytes, err := os.ReadFile(oauthTokenPath); err == nil {
		token := strings.TrimSpace(string(tokenBytes))
		if token != "" {
			containerEnv, _ := cfg["containerEnv"].(map[string]interface{})
			if containerEnv == nil {
				containerEnv = make(map[string]interface{})
			}
			containerEnv["CLAUDE_CODE_OAUTH_TOKEN"] = token
			cfg["containerEnv"] = containerEnv
			logToFile("Injecting CLAUDE_CODE_OAUTH_TOKEN from %s", oauthTokenPath)
		}
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
