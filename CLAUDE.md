# Dark Multi - Claude Context

## What This Is

A CLI/TUI tool for managing multiple parallel Dark devcontainer instances with tmux integration. Being rewritten from Python to Go.

## Current State

**Python version (working):** `multi.py` + `dark_multi/` package
- All CLI commands functional
- Proxy working
- DNS setup working

**Go rewrite (in progress):** See `todos/` for detailed plan

## Active Development Plan

```
todos/
├── 00-overview.md        # Architecture, tech stack, file map
├── 01-setup-cli.md       # Phase 1: Go setup + port CLI commands
├── 02-tui-home.md        # Phase 2: Interactive home screen
├── 03-tui-branch.md      # Phase 3: Branch detail view
├── 04-claude-integration.md  # Phase 4: Claude status detection
└── 05-polish.md          # Phase 5: Error handling, help, polish
```

**Start with:** `todos/01-setup-cli.md`

## Target Behavior

```bash
multi                    # Interactive TUI mode (NEW)
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

## Go Architecture (Target)

```
cmd/multi/main.go           # Entry point
internal/
├── config/config.go        # Paths, ports, env vars
├── branch/
│   ├── branch.go           # Branch struct + operations
│   └── discovery.go        # Find branches, source repo
├── container/
│   ├── devcontainer.go     # Override config generation
│   └── docker.go           # Docker operations
├── tmux/tmux.go            # Tmux session management
├── proxy/
│   ├── proxy.go            # HTTP proxy server
│   └── handler.go          # Request routing
├── dns/dns.go              # DNS setup (Linux/macOS)
├── claude/status.go        # Claude status detection
├── tui/
│   ├── app.go              # Main bubbletea app
│   ├── home.go             # Home screen model
│   ├── branch.go           # Branch detail model
│   ├── styles.go           # Lipgloss styles
│   └── keys.go             # Key bindings
└── cli/commands.go         # Cobra commands
```

## Tech Stack

- **Go 1.21+**
- **Charm libraries:** bubbletea, lipgloss, bubbles, log
- **cobra** for CLI

## Key Concepts (Preserved from Python)

### Port Mapping
Container always uses standard ports internally. Host ports calculated by branch ID:
- `bwd_port = 11001 + (instance_id * 100)` → 11101, 11201, ...
- `test_port = 10011 + (instance_id * 100)` → 10111, 10211, ...

### Override Configs
Don't modify repo's devcontainer.json. Generate merged configs at:
`~/.config/dark-multi/overrides/<branch>/devcontainer.json`

### URL Proxy
Routes `<canvas>.<branch>.dlio.localhost:9000` → appropriate port with correct Host header.

### DNS Setup
dnsmasq configured for wildcard `*.dlio.localhost → 127.0.0.1`

## Python Code Reference

When porting, reference these Python files:
- `dark_multi/config.py` - Constants, logging
- `dark_multi/branch.py` - Branch class
- `dark_multi/tmux.py` - Tmux operations
- `dark_multi/proxy.py` - Proxy server
- `dark_multi/dns.py` - DNS setup
- `dark_multi/devcontainer.py` - Override configs
- `dark_multi/commands.py` - Command implementations

## Testing Commands

```bash
# Python (current)
python3 multi.py ls
python3 multi.py urls
curl http://dark-packages.main.dlio.localhost:9000/ping

# Go (after port)
./multi ls
./multi urls
./multi  # should launch TUI
```
