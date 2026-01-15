# Dark Multi

**WIP** - Run multiple Dark devcontainer instances in parallel.

The intent: spin up several containers, unleash Claude agents in each, and jump between tasks ergonomically. Each branch gets its own isolated environment with dedicated terminal + VS Code.

```
┌─────────────────────────────────────────────────────────────┐
│ DARK MULTI                                                  │
│                                                             │
│ > ● main           3c +150 -42  ⚡                          │
│   ● feature-auth   1c +20 -5                                │
│   ○ bugfix-login                                            │
│                                                             │
│ System: 8 cores, 32GB RAM  •  2/4 running  •  Proxy: ●      │
│                                                             │
│ [s]tart [k]ill [t]mux [c]ode [m]atter [p]roxy [?] [q]       │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- **Go 1.21+** - `apt install golang-go` gives old versions; install from https://go.dev/dl/
- **Docker** - with daemon running
- **devcontainer CLI** - `npm install -g @devcontainers/cli`
- **tmux** - for terminal sessions

## Install

```bash
go build -o multi .
mkdir -p ~/.local/bin
cp multi ~/.local/bin/

# Add to PATH (in ~/.bashrc or ~/.zshrc):
export PATH="$HOME/.local/bin:$PATH"
```

## Quick Start

```bash
multi new my-feature    # Clone from GitHub, create branch, start container
multi                   # Launch TUI
```

If branch already exists, `multi new` just starts it.

## Usage

```bash
multi                    # TUI (above)
multi ls                 # List branches
multi new <branch>       # Create branch (clones from GitHub if needed)
multi start/stop <branch>
multi code <branch>      # Open VS Code
multi proxy start|stop
```

## Keys

`s` start | `k` kill | `t` terminal | `c` code | `m` matter | `p` proxy | `?` help | `q` quit

## Config

| Variable | Default |
|----------|---------|
| `DARK_ROOT` | `~/code/dark` |
| `DARK_SOURCE` | GitHub (or local repo if exists) |
| `DARK_MULTI_TERMINAL` | `auto` (gnome-terminal, kitty, iterm2, etc) |
| `DARK_MULTI_PROXY_PORT` | `9000` |
