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
│ [n]ew [d]elete [s]tart [k]ill [t]mux [c]ode [p]roxy [?] [q] │
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

## Usage

Just run `multi` - everything happens in the interactive TUI.

```
n           New branch (type name, enter to create)
d           Delete branch (y/n confirmation)
s           Start branch
k           Kill (stop) branch
t           Open terminal (tmux session)
c           Open VS Code
p           Toggle proxy
enter       View branch details & URLs
?           Help
q           Quit
```

First branch clone is from GitHub. Subsequent clones use existing local repo.

## CLI Commands

Only two commands exist outside the TUI:

```bash
multi proxy start|stop|status|fg   # Manage proxy server
multi setup-dns                    # One-time DNS setup
```

## Config

| Variable | Default |
|----------|---------|
| `DARK_ROOT` | `~/code/dark` |
| `DARK_SOURCE` | GitHub (or local repo if exists) |
| `DARK_MULTI_TERMINAL` | `auto` (gnome-terminal, kitty, iterm2, etc) |
| `DARK_MULTI_PROXY_PORT` | `9000` |
