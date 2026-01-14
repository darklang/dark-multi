# Dark Multi - Go Rewrite Plan

## Goal

Rewrite dark-multi in Go using Charm libraries for a modern TUI experience. Keep existing CLI commands working, add interactive mode when no args supplied.

## Behavior

```bash
multi                    # Interactive TUI mode
multi ls                 # CLI: list branches
multi new <branch>       # CLI: create branch
multi start <branch>     # CLI: start branch
multi stop <branch>      # CLI: stop branch
multi rm <branch>        # CLI: remove branch
multi code <branch>      # CLI: open VS Code
multi urls               # CLI: list URLs
multi proxy start|stop   # CLI: manage proxy
multi setup-dns          # CLI: configure DNS
```

## Tech Stack

- **Go 1.21+**
- **Charm libraries:**
  - `bubbletea` - TUI framework (Elm architecture)
  - `lipgloss` - Styling
  - `bubbles` - Reusable components (list, spinner, etc.)
  - `log` - Styled logging
- **cobra** - CLI framework (works well with bubbletea)

## Architecture

```
cmd/
  multi/
    main.go              # Entry point
internal/
  config/
    config.go            # Paths, ports, env vars
  branch/
    branch.go            # Branch struct + operations
    discovery.go         # Find branches, source repo
  container/
    devcontainer.go      # Override config generation
    docker.go            # Docker operations
  tmux/
    tmux.go              # Tmux session management
  proxy/
    proxy.go             # HTTP proxy server
    handler.go           # Request routing
  dns/
    dns.go               # DNS setup (Linux/macOS)
  claude/
    status.go            # Claude status detection
  tui/
    app.go               # Main bubbletea app
    home.go              # Home screen model
    branch.go            # Branch detail model
    styles.go            # Lipgloss styles
    keys.go              # Key bindings
  cli/
    commands.go          # Cobra commands (ls, new, etc.)
go.mod
go.sum
```

## Phases

1. **Setup + CLI port** - Basic Go project, port all CLI commands
2. **TUI home screen** - Branch list, status indicators, root actions
3. **TUI branch detail** - URLs, git status, quick actions
4. **Claude integration** - Status detection, conversation info
5. **Polish** - Error handling, help, edge cases

## File Map (Python → Go)

| Python | Go |
|--------|-----|
| `dark_multi/config.py` | `internal/config/config.go` |
| `dark_multi/branch.py` | `internal/branch/branch.go` |
| `dark_multi/tmux.py` | `internal/tmux/tmux.go` |
| `dark_multi/proxy.py` | `internal/proxy/proxy.go` |
| `dark_multi/dns.py` | `internal/dns/dns.go` |
| `dark_multi/devcontainer.py` | `internal/container/devcontainer.go` |
| `dark_multi/commands.py` | `internal/cli/commands.go` |
| `dark_multi/cli.py` | `cmd/multi/main.go` |
| (new) | `internal/tui/*.go` |
| (new) | `internal/claude/status.go` |
