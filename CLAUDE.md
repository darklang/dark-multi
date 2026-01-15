# Dark Multi - Claude Context

## go

When user says "go", ask: **"Are you Stachu or Ocean?"**

### If Stachu
Continue developing. Check `todos/` for planned work, or ask what to work on next.

### If Ocean
Guide through setup and testing. Work through this checklist together:

**Phase 1: Setup**
- [ ] Install Go 1.21+ (`brew install go` or download from go.dev)
- [ ] Verify: `go version` shows 1.21+
- [ ] Clone this repo somewhere (e.g., `~/code/dark-multi`)
- [ ] Build: `cd ~/code/dark-multi && go build -o multi ./cmd/multi`
- [ ] Install: `cp multi ~/.local/bin/` (or add to PATH)
- [ ] Verify: `multi --help` works

**Phase 2: Prepare Dark Directory**
- [ ] Backup existing ~/code/dark if needed: `mv ~/code/dark ~/code/dark-backup`
- [ ] Clone fresh dark repo: `git clone <dark-repo-url> ~/code/dark`
- [ ] Clear any old override configs: `rm -rf ~/.config/dark-multi`

**Phase 3: Test CLI Commands**
- [ ] `multi ls` - should show empty or just "main"
- [ ] `multi new test1` - creates a new branch
- [ ] `multi ls` - should show test1 (stopped)
- [ ] `multi start test1` - starts the devcontainer (takes a while first time)
- [ ] `multi ls` - should show test1 as running
- [ ] `multi code test1` - opens VS Code attached to container
- [ ] `multi stop test1` - stops the container

**Phase 4: Test TUI**
- [ ] `multi` (no args) - launches interactive TUI
- [ ] Navigate with arrow keys
- [ ] Press `s` to start a branch
- [ ] Press `t` to open terminal (should open iTerm2 or Terminal.app)
- [ ] Press `c` to open VS Code
- [ ] Press `?` for help
- [ ] Press `q` to quit

**Phase 5: Test Proxy (if needed)**
- [ ] `multi proxy start` - starts the URL proxy on :9000
- [ ] `multi urls` - lists accessible URLs
- [ ] Try opening a URL in browser
- [ ] `multi proxy stop`

**Report Issues**
Note anything that doesn't work - especially:
- Terminal spawning (iTerm2 vs Terminal.app)
- DNS resolution for .localhost URLs
- Any error messages

---

## What This Is

A CLI/TUI tool for managing multiple parallel Dark devcontainer instances with tmux integration.

## Current State

**Go version (active):** Full rewrite complete, installed at `~/.local/bin/multi`
- Interactive TUI when run with no args
- All CLI commands: ls, new, start, stop, rm, code, urls, proxy, setup-dns
- Claude status detection (⏳ waiting, 🔄 working)
- Branch metadata stored in `~/.config/dark-multi/overrides/<branch>/metadata`

**Python version:** Still exists at `multi.py` + `dark_multi/` but not used

## TUI Shortcuts

```
Home screen:
  ↑/↓         Navigate branches
  s           Start selected branch
  k           Kill (stop) selected branch
  t           Open terminal (per-branch tmux session)
  c           Open VS Code
  m           Open Matter (dark-packages canvas in browser)
  p           Toggle proxy
  enter       View branch details
  ?           Help
  q           Quit

Branch detail:
  ↑/↓         Navigate URLs
  o/enter     Open URL in browser
  s/k         Start/Kill branch
  c           VS Code
  t           Terminal
  l           View logs
  esc         Back

Display:
  ● / ○       Running / stopped
  3c +50 -10  Commits, lines added/removed vs main
  ⏳ / ⚡      Claude waiting / working
```

## Architecture

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
│   ├── proxy.go            # HTTP proxy server (IPv4+IPv6)
│   └── handler.go          # Request routing
├── dns/dns.go              # DNS setup (Linux/macOS)
├── claude/status.go        # Claude status from conversation files
└── tui/
    ├── app.go              # Bubbletea app entry
    ├── home.go             # Home screen
    ├── branch_detail.go    # Branch detail view
    ├── logs.go             # Log viewer
    ├── help.go             # Help screen
    ├── operations.go       # Start/stop/code operations
    └── styles.go           # Lipgloss styles
```

## Key Concepts

### Port Mapping
Container uses standard ports internally. Host ports by instance ID:
- `bwd_port = 11001 + (instance_id * 100)` → 11101, 11201, ...
- `test_port = 10011 + (instance_id * 100)` → 10111, 10211, ...

### Override Configs
Generated at `~/.config/dark-multi/overrides/<branch>/devcontainer.json`
- Unique container names, ports, volumes per branch
- Branch metadata in `metadata` file (ID, name, created)

### URL Proxy
Routes `<canvas>.<branch>.dlio.localhost:9000` → container's BwdServer port
- Proxy listens on both IPv4 and IPv6
- Start with: `multi proxy start`

### DNS
`.localhost` TLD is handled by systemd-resolved (RFC 6761)
- Resolves to both 127.0.0.1 and ::1 automatically
- No dnsmasq needed on modern Linux

## Config

| Variable | Default | Description |
|----------|---------|-------------|
| `DARK_ROOT` | `~/code/dark` | Where branches live |
| `DARK_SOURCE` | `~/code/dark` | Repo to clone from |
| `DARK_MULTI_TERMINAL` | `auto` | Terminal: gnome-terminal, kitty, alacritty, iterm2, etc |
| `DARK_MULTI_PROXY_PORT` | `9000` | Proxy port |

## Building

```bash
# Requires Go 1.21+ (installed at ~/go-sdk/go)
~/go-sdk/go/bin/go build -o multi ./cmd/multi
cp multi ~/.local/bin/multi
```

## Known Issues

- Proxy can crash silently when backgrounded; use `multi proxy fg` to debug
- "canvas not found" means Dark canvases aren't loaded in container (not a multi issue)
