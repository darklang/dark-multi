# Dark Multi

Manage multiple Dark devcontainer instances with tmux integration.

## Setup

```bash
# Make multi available globally
ln -sf ~/code/dark-multi/multi.py ~/.local/bin/multi
chmod +x ~/code/dark-multi/multi.py

# Ensure ~/.local/bin is in PATH
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc

# Set source repo (if not ~/code/dark)
export DARK_SOURCE=~/code/dark-source
```

## Usage

```bash
multi                    # Attach to tmux session
multi ls                 # List branches + system resources
multi new <branch>       # Create branch clone + start + tmux
multi start <branch>     # Start stopped branch
multi stop <branch>      # Stop branch (keeps files)
multi rm <branch>        # Remove branch entirely (full cleanup)
multi code <branch>      # Open VS Code for branch
```

## Resource Detection

`multi ls` shows system resources and suggests max concurrent instances:
```
Branches in /home/user/code/dark:
  System: 8 cores, 32GB RAM → suggested max: 4 concurrent
```

## How It Works

**Branches** live at `~/code/dark/<branch>/`:
```
~/code/dark/
├── main/           # Clone on main branch
├── fix-parser/     # Clone on fix-parser branch
└── feature-auth/   # Clone on feature-auth branch
```

Each branch:
- Is a full git clone (instant from local source)
- Has its own devcontainer with unique ports
- Gets a tmux window when started

**tmux session** called `dark`:
```
Session: dark
├── Window: dark-meta   [claude (70%) | quick ref (30%)]  ← control plane
├── Window: main        [CLI | claude]
├── Window: fix-parser  [CLI | claude]
└── Window: feature-x   [CLI | claude]
```

The **dark-meta** window is your control plane - a claude instance that knows about `multi` commands. Ask it to create/remove branches, start tasks, etc.

**Port allocation** (to avoid conflicts):
| Branch | Test Ports | BWD Ports |
|--------|------------|-----------|
| ID 1   | 10111-10130 | 11101-11102 |
| ID 2   | 10211-10230 | 11201-11202 |
| ID 3   | 10311-10330 | 11301-11302 |

## Cleanup

`multi rm <branch>` does full cleanup:
1. Kills tmux window
2. Stops and removes container
3. Removes any dangling containers with that label
4. Deletes the directory

## tmux Controls

Layout: One window per branch, each with side-by-side panes.

```
┌─────────────────────────────────────────────┐
│ Window: main                                │
│ ┌───────────────────┬─────────────────────┐ │
│ │ CLI (container)   │ claude (host)       │ │
│ │ build/test here   │ edits files here    │ │
│ └───────────────────┴─────────────────────┘ │
├─────────────────────────────────────────────┤
│ Window: fix-parser                          │
│ Window: feature-x                           │
│ ...                                         │
└─────────────────────────────────────────────┘
```

- **Left pane**: Shell inside the container (for building, testing, running)
- **Right pane**: Claude on host, working in `~/code/dark/<branch>` (uses your credentials)

| Keys | Action |
|------|--------|
| `Ctrl-b n` | Next window (next branch) |
| `Ctrl-b p` | Previous window |
| `Ctrl-b o` | Switch panes (CLI ↔ claude) |
| `Ctrl-b z` | Zoom current pane (fullscreen toggle) |
| `Ctrl-b d` | Detach from session |
| `Ctrl-b w` | List all windows |

## Typical Workflow

```bash
# Morning: see what's there
multi ls

# Start work on a new feature (opens tmux + VS Code automatically)
multi new fix-parser

# Attach to tmux to see all branches
multi

# Inside tmux: Ctrl-b n/p to switch branches, Ctrl-b o for CLI/claude

# End of day: stop but keep files
multi stop fix-parser

# Next day: resume (opens tmux + VS Code)
multi start fix-parser

# Done with branch: full cleanup
multi rm fix-parser
```

## Browser Access (BwdServer)

Each branch's BwdServer is accessible via the URL proxy:

```bash
# One-time DNS setup (configures dnsmasq for wildcard *.dlio.localhost)
multi setup-dns

# Proxy starts automatically with containers, or manually:
multi proxy start

# List URLs for running branches
multi urls
```

Then access canvases at: `http://dark-packages.<branch>.dlio.localhost:9000/ping`

**TODO**: Currently requires `:9000` in URL. Future: add port 80 redirect via iptables/pf.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DARK_ROOT` | `~/code/dark` | Where branches live |
| `DARK_SOURCE` | `~/code/dark` | Repo to clone from |
| `DARK_MULTI_PROXY_PORT` | `9000` | Proxy server port |
