# Phase 1: Setup + CLI Port

## 1.1 Project Setup

```bash
# Initialize Go module
go mod init github.com/stachu/dark-multi

# Install dependencies
go get github.com/spf13/cobra@latest
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/log@latest
```

Create directory structure:
```
cmd/multi/main.go
internal/config/config.go
internal/branch/branch.go
internal/branch/discovery.go
internal/container/devcontainer.go
internal/container/docker.go
internal/tmux/tmux.go
internal/proxy/proxy.go
internal/proxy/handler.go
internal/dns/dns.go
internal/cli/commands.go
```

## 1.2 Config Package

Port from `dark_multi/config.py`:

```go
// internal/config/config.go
package config

import (
    "os"
    "path/filepath"
)

var (
    DarkRoot    = getEnvOrDefault("DARK_ROOT", filepath.Join(os.Getenv("HOME"), "code", "dark"))
    DarkSource  = getEnvOrDefault("DARK_SOURCE", DarkRoot)
    ConfigDir   = getEnvOrDefault("DARK_MULTI_CONFIG", filepath.Join(os.Getenv("HOME"), ".config", "dark-multi"))
    TmuxSession = "dark"
    ProxyPort   = getEnvOrDefaultInt("DARK_MULTI_PROXY_PORT", 9000)
)

const (
    RAMPerInstanceGB = 6
    CPUPerInstance   = 2
)

func GetSystemResources() (cpuCores int, ramGB int) { ... }
func SuggestMaxInstances() int { ... }
```

## 1.3 Branch Package

Port from `dark_multi/branch.py`:

```go
// internal/branch/branch.go
package branch

type Branch struct {
    Name         string
    Path         string
    MetadataFile string
}

func New(name string) *Branch { ... }
func (b *Branch) Exists() bool { ... }
func (b *Branch) IsManaged() bool { ... }
func (b *Branch) Metadata() map[string]string { ... }
func (b *Branch) InstanceID() int { ... }
func (b *Branch) ContainerName() string { ... }
func (b *Branch) ContainerID() (string, error) { ... }
func (b *Branch) IsRunning() bool { ... }
func (b *Branch) HasChanges() bool { ... }
func (b *Branch) PortBase() int { ... }
func (b *Branch) BwdPortBase() int { ... }
func (b *Branch) WriteMetadata(instanceID int) error { ... }
func (b *Branch) StatusLine() string { ... }

// internal/branch/discovery.go
func FindNextInstanceID() int { ... }
func FindSourceRepo() string { ... }
func GetManagedBranches() []*Branch { ... }
```

## 1.4 Container Package

Port from `dark_multi/devcontainer.py`:

```go
// internal/container/devcontainer.go
package container

func GetOverrideConfigPath(b *branch.Branch) string { ... }
func GenerateOverrideConfig(b *branch.Branch) (string, error) { ... }

// internal/container/docker.go
func StopContainer(containerID string) error { ... }
func RemoveContainer(containerID string) error { ... }
func ExecInContainer(containerID string, cmd []string) error { ... }
```

## 1.5 Tmux Package

Port from `dark_multi/tmux.py`:

```go
// internal/tmux/tmux.go
package tmux

func IsAvailable() bool { ... }
func SessionExists() bool { ... }
func WindowExists(name string) bool { ... }
func CreateWindow(name string, containerID string, branchPath string) error { ... }
func KillWindow(name string) error { ... }
func EnsureMetaWindow() error { ... }
func Attach() error { ... }
```

## 1.6 Proxy Package

Port from `dark_multi/proxy.py`:

```go
// internal/proxy/proxy.go
package proxy

func Start(port int, background bool) (int, error) { ... }
func Stop() error { ... }
func IsRunning() (int, bool) { ... }
func EnsureRunning() error { ... }

// internal/proxy/handler.go
type ProxyHandler struct { ... }
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { ... }
```

## 1.7 DNS Package

Port from `dark_multi/dns.py`:

```go
// internal/dns/dns.go
package dns

func Setup() error { ... }
func TestDNS() bool { ... }
func setupDarwin() error { ... }
func setupLinux() error { ... }
```

## 1.8 CLI Commands

Port from `dark_multi/commands.py` + `dark_multi/cli.py`:

```go
// internal/cli/commands.go
package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command { ... }
func lsCmd() *cobra.Command { ... }
func newCmd() *cobra.Command { ... }
func startCmd() *cobra.Command { ... }
func stopCmd() *cobra.Command { ... }
func rmCmd() *cobra.Command { ... }
func codeCmd() *cobra.Command { ... }
func urlsCmd() *cobra.Command { ... }
func proxyCmd() *cobra.Command { ... }
func setupDNSCmd() *cobra.Command { ... }
```

## 1.9 Main Entry Point

```go
// cmd/multi/main.go
package main

import (
    "os"
    "github.com/stachu/dark-multi/internal/cli"
    "github.com/stachu/dark-multi/internal/tui"
)

func main() {
    // No args → interactive mode
    if len(os.Args) == 1 {
        tui.Run()
        return
    }

    // Otherwise → CLI mode
    cmd := cli.NewRootCmd()
    cmd.Execute()
}
```

## 1.10 Build + Test

```bash
# Build
go build -o multi ./cmd/multi

# Test each command
./multi ls
./multi urls
./multi proxy status

# Install
ln -sf $(pwd)/multi ~/.local/bin/multi
```

## Checklist

- [ ] Go module initialized
- [ ] All dependencies installed
- [ ] config package ported
- [ ] branch package ported
- [ ] container package ported
- [ ] tmux package ported
- [ ] proxy package ported
- [ ] dns package ported
- [ ] CLI commands ported
- [ ] All CLI commands tested
- [ ] Build working
