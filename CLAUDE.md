# Dark Multi - Claude Context

## What This Is

A CLI tool (`multi.py`) for managing multiple parallel Dark devcontainer instances with tmux integration. Allows running multiple AI agents working on different branches simultaneously.

## Current State (Working)

- **Override config approach**: Instead of modifying the repo's devcontainer.json, we generate merged override configs at `~/.config/dark-multi/overrides/<branch>/devcontainer.json`
- **Port publishing works**: Docker `-p` flags map container ports to host ports
  - Branch 1 (ID=1): BwdServer at `localhost:11101`, test ports at `10111+`
  - Branch 2 (ID=2): BwdServer at `localhost:11201`, test ports at `10211+`
- **Container always uses standard ports internally** (11001, 11002, 10011-10030)
- **tmux integration**: dark-meta control plane + per-branch windows with CLI|claude panes

## Key Commands

```bash
multi                    # Attach to tmux
multi ls                 # List branches
multi new <branch>       # Create + start + tmux + vscode
multi start <branch>     # Start stopped branch
multi stop <branch>      # Stop (keeps files)
multi rm <branch>        # Full cleanup
multi code <branch>      # Open VS Code
multi urls               # List available URLs
multi proxy start        # Start URL proxy
multi setup-dns          # One-time wildcard DNS setup
```

## Architecture

```
~/code/dark/
├── main/           # Clone - devcontainer.json UNCHANGED
├── test/           # Clone - devcontainer.json UNCHANGED
└── feature-x/      # Clone - devcontainer.json UNCHANGED

~/.config/dark-multi/
└── overrides/
    ├── main/devcontainer.json      # Merged config with port mappings
    └── test/devcontainer.json      # Merged config with port mappings
```

## BwdServer Access

Container's BwdServer (port 11001) is mapped to host port based on branch ID:
- main (ID=1): `localhost:11101`
- test (ID=2): `localhost:11201`

To access canvases, need Host header:
```bash
curl -H "Host: dark-packages.dlio.localhost" http://localhost:11101/ping
# Returns: pong
```

For Chrome access, add to `/etc/hosts`:
```
127.0.0.1 dark-packages.dlio.localhost
```
Then: `http://dark-packages.dlio.localhost:11101/ping`

## TODO / Future Work

1. **Port 80 redirect**: Currently requires `:9000` in URL. Add iptables/pf redirect setup.
2. **More canvas types**: Currently assumes `dark-packages` canvas; support listing/selecting canvases per branch

## Code Structure

```
multi.py                    # Entry point (thin wrapper)
darkmulti/
├── __init__.py             # Package init
├── cli.py                  # Argparse + main()
├── config.py               # Constants, colors, logging
├── branch.py               # Branch class + discovery
├── tmux.py                 # Tmux session management
├── proxy.py                # URL proxy server
├── dns.py                  # Wildcard DNS setup
├── devcontainer.py         # Override config generation
└── commands.py             # All cmd_* implementations
```

**Runtime files:**
- `~/.config/dark-multi/overrides/` - Generated devcontainer configs
- `~/.config/dark-multi/proxy.pid` - Proxy process ID

## tmux Layout

```
Session: dark
├── dark-meta     [claude 70% | quick-ref 30%]  ← control plane
├── main          [CLI container | claude host]
└── test          [CLI container | claude host]

Keys: Ctrl-b n/p (windows), Ctrl-b o (panes), Ctrl-b z (zoom)
Mouse scroll enabled.
```

## How Port Mapping Works

### The Problem
- Dark's devcontainer always uses fixed ports internally (11001 for BwdServer, 10011-10030 for tests)
- Running multiple containers with same ports would conflict
- We want each branch accessible from the host without VS Code running

### The Solution: Docker Port Mapping
```
Container (always same)     Host (branch-specific)
─────────────────────────   ──────────────────────
11001 (BwdServer)      →    11101 (main), 11201 (test), 11301 (branch3)
11002 (K8s health)     →    11102, 11202, 11302
10011-10030 (tests)    →    10111-10130, 10211-10230, 10311-10330
```

### Implementation
1. **Override config generation** (`darkmulti/devcontainer.py`):
   - Reads original `devcontainer.json` from the repo
   - Merges in branch-specific `-p` port mappings in `runArgs`
   - Writes to `~/.config/dark-multi/overrides/<branch>/devcontainer.json`

2. **Container start** uses `--override-config`:
   ```bash
   devcontainer up --workspace-folder ~/code/dark/main \
                   --override-config ~/.config/dark-multi/overrides/main/devcontainer.json
   ```

3. **Port formula**:
   - `bwd_port_base = 11001 + (instance_id * 100)` → 11101, 11201, 11301...
   - `test_port_base = 10011 + (instance_id * 100)` → 10111, 10211, 10311...

### Accessing BwdServer
The BwdServer routes based on `Host` header, not port. So:

```bash
# Works - proper Host header
curl -H "Host: dark-packages.dlio.localhost" http://localhost:11101/ping
# Returns: pong (main) or pang (test, if modified)

# Doesn't work - no Host header
curl http://localhost:11101/ping
# Returns: canvas not found
```

For browser access, add to `/etc/hosts`:
```
127.0.0.1 dark-packages.dlio.localhost
```
Then visit: `http://dark-packages.dlio.localhost:11101/ping`

### Verified Working
```
main (ID=1): localhost:11101 → pong
test (ID=2): localhost:11201 → pang (after code change)
```

## URL Proxy

Built-in Python proxy for nice URLs without manual Host headers.

**Commands:**
```bash
multi proxy start    # Start proxy (auto-starts with new/start)
multi proxy stop     # Stop proxy
multi proxy status   # Check if running
multi urls           # List all available URLs
```

**URL scheme:** `<canvas>.<branch>.dlio.localhost:9000`
- `dark-packages.main.dlio.localhost:9000` → main's BwdServer
- `dark-packages.test.dlio.localhost:9000` → test's BwdServer

**Setup for browser:**
```bash
multi setup-dns    # One-time wildcard DNS setup
```

Then visit: `http://dark-packages.main.dlio.localhost:9000/ping`

Any new branch automatically works - no /etc/hosts editing needed.

## Recent Changes

1. Switched from modifying repo's devcontainer.json to override config approach
2. Added `-p` port publishing so ports accessible without VS Code
3. Merged original devcontainer.json with overrides (preserves build section, etc.)
4. Override configs stored in ~/.config/dark-multi/overrides/
5. Added Python proxy for nice URLs (auto-starts with containers)
6. Added `multi urls` to list available endpoints
7. Added `multi setup-dns` for wildcard DNS (dnsmasq) - works on Linux and macOS
8. Refactored into `darkmulti/` package with focused modules
