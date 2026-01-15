# Dark Multi - Claude Context

## go

When user says "go", ask: **"Are you Stachu or Ocean?"**

### If Stachu
Continue developing. Ask what to work on next.

### If Ocean
Guide through macOS setup - see **macOS Setup** below.

---

## Linux Setup (Ubuntu/Pop!_OS)

**Prerequisites**
- [ ] Docker: running (`docker ps` works)
- [ ] Node.js/npm: for devcontainer CLI
- [ ] tmux: `sudo apt install tmux`

**Install Go 1.21+**
`apt install golang-go` gives old versions (1.18). Install manually:
```bash
wget -qO- https://go.dev/dl/go1.23.5.linux-amd64.tar.gz | tar -C ~/.local -xzf -
echo 'export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
go version  # should be 1.21+
```

**Install devcontainer CLI**
```bash
npm install -g @devcontainers/cli
```

**Build & Install**
```bash
cd ~/code/dark-multi
go build -o multi .
mkdir -p ~/.local/bin
cp multi ~/.local/bin/
```

**Test**
```bash
multi new test-branch   # Clones from GitHub, starts container
multi                   # TUI
```

---

## macOS Setup (Ocean)

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
- [ ] Build: `cd ~/code/dark-multi && go build -o multi .`
- [ ] Install: `mkdir -p ~/.local/bin && cp multi ~/.local/bin/`
- [ ] Add to PATH in ~/.zshrc: `export PATH="$HOME/.local/bin:$PATH"`
- [ ] Reload: `source ~/.zshrc`
- [ ] Verify: `multi --help`

**Phase 3: Install devcontainer CLI**
- [ ] `npm install -g @devcontainers/cli`
- [ ] Verify: `devcontainer --version`

**Phase 4: Test**
- [ ] `multi new branch1` - clones from GitHub, creates branch, starts container
- [ ] `multi` - launches TUI
- [ ] `t` - opens terminal
- [ ] `c` - opens VS Code
- [ ] `?` - shows help
- [ ] `q` - quit

**Phase 5: Cleanup (optional)**
- [ ] `multi stop branch1`
- [ ] `multi rm branch1`

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
- Claude status detection (waiting, working)
- Clones from GitHub automatically if no local source
- `multi new` idempotent - just starts if branch exists
- Branch metadata stored in `~/.config/dark-multi/overrides/<branch>/metadata`

## TUI Shortcuts

```
Home screen:
  up/down     Navigate branches
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
  up/down     Navigate URLs
  o/enter     Open URL in browser
  s/k         Start/Kill branch
  c           VS Code
  t           Terminal
  l           View logs
  esc         Back

Display:
  o / .       Running / stopped
  3c +50 -10  Commits, lines added/removed vs main
```

## Architecture

```
main.go           # Entry point
branch/           # Branch struct, discovery
cli/              # Cobra commands
claude/           # Claude status detection
config/           # Paths, ports, env vars
container/        # Devcontainer + Docker ops
dns/              # DNS setup (Linux/macOS)
proxy/            # HTTP proxy server
tmux/             # Tmux session management
tui/              # Bubbletea TUI (home, detail, logs, help)
```

## Key Concepts

### Port Mapping
Container uses standard ports internally. Host ports by instance ID:
- `bwd_port = 11001 + (instance_id * 100)` -> 11101, 11201, ...
- `test_port = 10011 + (instance_id * 100)` -> 10111, 10211, ...

### Override Configs
Generated at `~/.config/dark-multi/overrides/<branch>/devcontainer.json`
- Unique container names, ports, volumes per branch
- Branch metadata in `metadata` file (ID, name, created)

### URL Proxy
Routes `<canvas>.<branch>.dlio.localhost:9000` -> container's BwdServer port
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
| `DARK_SOURCE` | GitHub | Repo to clone from (falls back to git@github.com:darklang/dark.git) |
| `DARK_MULTI_TERMINAL` | `auto` | Terminal: gnome-terminal, kitty, alacritty, iterm2, etc |
| `DARK_MULTI_PROXY_PORT` | `9000` | Proxy port |

## Building

```bash
# Requires Go 1.21+
go build -o multi .
cp multi ~/.local/bin/
```

## Known Issues

- Proxy can crash silently when backgrounded; use `multi proxy fg` to debug
- "canvas not found" means Dark canvases aren't loaded in container (not a multi issue)
