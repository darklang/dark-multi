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

**Increase inotify limits (multiple containers need more file watchers)**
```bash
multi setup-inotify
```

**Test**
```bash
multi         # Launch TUI
# Press 'n', type branch name, press enter
# First clone is from GitHub (takes a while)
```

---

## macOS Setup (Ocean)

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

**Phase 3: Install devcontainer CLI**
- [ ] `npm install -g @devcontainers/cli`
- [ ] Verify: `devcontainer --version`

**Phase 4: Test**
- [ ] `multi` - launches TUI
- [ ] `n` - type branch name, enter to create (clones from GitHub first time)
- [ ] `t` - opens terminal
- [ ] `c` - opens VS Code
- [ ] `?` - shows help
- [ ] `q` - quit

**Phase 5: Cleanup (optional)**
- [ ] `d` on a branch, then `y` to confirm deletion

**Report Issues**
- Terminal spawning (iTerm2 vs Terminal.app detection)
- DNS resolution for .localhost URLs
- Error messages
- Anything confusing

---

## What This Is

A TUI tool for managing multiple parallel Dark devcontainer instances with tmux integration.

## Current State

Everything happens in the TUI. Just run `multi`.

**TUI shortcuts:**
```
n           New branch (type name, enter)
d           Delete branch (y/n confirm)
s           Start branch
k           Kill (stop) branch
t           Open terminal (tmux)
c           Open VS Code
m           Open Matter (dark-packages canvas)
p           Toggle proxy
enter       View branch details & URLs
?           Help
q           Quit
```

**CLI commands:**
- `multi ls` - list branches
- `multi new <name>` - create a new branch
- `multi start <name>` - start a branch
- `multi stop <name>` - stop a branch
- `multi rm <name>` - remove a branch
- `multi proxy start|stop|status|fg` - manage proxy
- `multi setup-dns` - one-time DNS setup
- `multi setup-inotify` - increase file watcher limits (Linux only)

**Features:**
- Clones from GitHub automatically
- Claude status detection (waiting/working indicators)
- Container startup progress (tree-sitter → F# → BwdServer → packages → ready)
- Branch metadata in `~/.config/dark-multi/overrides/<branch>/`

## Architecture

```
main.go           # Entry point
branch/           # Branch struct, discovery
cli/              # Cobra commands (proxy, setup-dns, setup-inotify)
claude/           # Claude status detection
config/           # Paths, ports, env vars
container/        # Devcontainer + Docker ops
dns/              # DNS setup (Linux/macOS)
inotify/          # inotify limit setup (Linux)
proxy/            # HTTP proxy server
tmux/             # Tmux session management
tui/              # Bubbletea TUI (home, detail, logs, help)
```

## Key Concepts

### Port Mapping
Host ports by instance ID:
- `bwd_port = 11001 + (instance_id * 100)` -> 11101, 11201, ...
- `test_port = 10011 + (instance_id * 100)` -> 10111, 10211, ...

### URL Proxy
Routes `<canvas>.<branch>.dlio.localhost:9000` -> container's BwdServer port

### DNS
`.localhost` TLD handled by systemd-resolved (RFC 6761) - no setup needed on modern Linux.

## Config

| Variable | Default |
|----------|---------|
| `DARK_ROOT` | `~/code/dark` |
| `DARK_SOURCE` | GitHub |
| `DARK_MULTI_TERMINAL` | `auto` |
| `DARK_MULTI_PROXY_PORT` | `9000` |

## Building

**Stachu's machine:** Use the build script (handles Go path, kills running processes):
```bash
./build.sh
```

**Other machines (Go 1.21+ in PATH):**
```bash
go build -o multi . && cp multi ~/.local/bin/
```

**Note:** System Go on Stachu's machine is 1.18 (too old). Go 1.25 is at `/home/stachu/go-sdk/go/bin/go`.

## Development Guidelines

**Always use `multi` for branch operations** - don't use raw docker/rm commands:
```bash
# Good
multi stop <branch>
multi rm <branch>

# Bad - don't do this
docker stop dark-<branch>
rm -rf ~/code/dark/<branch>
```

This ensures proper cleanup of containers, tmux sessions, override configs, and metadata.

**No attribution in commits** - don't add Co-Authored-By or similar lines.
