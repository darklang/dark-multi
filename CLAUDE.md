# Dark Multi - Claude Context

## go

When user says "go", ask: **"Are you Stachu or Ocean?"**

### If Stachu
Continue developing. Ask what to work on next.

### If Ocean
Guide through setup and testing. Work through this checklist together:

**Phase 1: Prerequisites**
- [ ] Xcode command line tools: `xcode-select --install`
- [ ] Homebrew: https://brew.sh
- [ ] Docker Desktop for Mac (running)
- [ ] VS Code with Dev Containers extension
- [ ] tmux: `brew install tmux`

**Phase 2: Install Go & Build**
- [ ] Install Go: `brew install go`
- [ ] Verify: `go version` (should be 1.21+)
- [ ] Clone: `git clone git@github.com:darklang/dark-multi.git ~/code/dark-multi`
- [ ] Build: `cd ~/code/dark-multi && go build -o multi ./cmd/multi`
- [ ] Install: `mkdir -p ~/.local/bin && cp multi ~/.local/bin/`
- [ ] Add to PATH in ~/.zshrc: `export PATH="$HOME/.local/bin:$PATH"`
- [ ] Reload: `source ~/.zshrc`
- [ ] Verify: `multi --help`

**Phase 3: Prepare Dark Directory**
- [ ] Backup existing: `mv ~/code/dark ~/code/dark-backup` (if exists)
- [ ] Clone dark repo: `git clone git@github.com:darklang/dark.git ~/code/dark`
- [ ] Clear old configs: `rm -rf ~/.config/dark-multi`

**Phase 4: Environment Variables (optional)**
Add to ~/.zshrc if defaults don't work:
```bash
export DARK_ROOT="$HOME/code/dark"           # where branches live
export DARK_SOURCE="$HOME/code/dark"         # repo to clone from
export DARK_MULTI_TERMINAL="iterm2"          # or "terminal" for Terminal.app
export DARK_MULTI_PROXY_PORT="9000"
```

**Phase 5: Test with TWO Branches**
Goal: Run 2 branches simultaneously to verify parallel instances work.

- [ ] `multi ls` - should show "main" (or empty)
- [ ] `multi new branch1` - creates first branch
- [ ] `multi new branch2` - creates second branch
- [ ] `multi start branch1` - start first (takes a while first time)
- [ ] `multi start branch2` - start second
- [ ] `multi ls` - both should show as running (●)

**Phase 6: Test TUI**
- [ ] `multi` - launches TUI
- [ ] Arrow keys to navigate between branches
- [ ] `t` on branch1 - opens terminal (iTerm2/Terminal.app with tmux)
- [ ] `t` on branch2 - opens SEPARATE terminal window
- [ ] `c` - opens VS Code for selected branch
- [ ] `?` - shows help
- [ ] `q` - quit

**Phase 7: Cleanup**
- [ ] `multi stop branch1`
- [ ] `multi stop branch2`
- [ ] `multi rm branch1` (optional - removes files)
- [ ] `multi rm branch2` (optional)

**Report Issues**
Note anything that doesn't work:
- Terminal spawning (iTerm2 vs Terminal.app detection)
- DNS resolution for .localhost URLs
- Error messages
- Anything confusing

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
