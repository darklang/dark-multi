# Dark Multi

Manage multiple Dark devcontainer instances.

## Install

```bash
# Build
cd ~/code/dark-multi
go build -o multi ./cmd/multi
cp multi ~/.local/bin/

# Ensure in PATH
export PATH="$HOME/.local/bin:$PATH"
```

## Usage

```bash
multi                    # TUI
multi ls                 # List branches
multi new <branch>       # Create + start branch
multi start <branch>     # Start stopped branch
multi stop <branch>      # Stop branch (keeps files)
multi rm <branch>        # Remove branch
multi code <branch>      # Open VS Code
multi urls               # List URLs
multi proxy start|stop   # Manage proxy
multi setup-dns          # One-time DNS setup
```

## TUI Keys

| Key | Action |
|-----|--------|
| `s` | Start branch |
| `k` | Kill (stop) branch |
| `t` | Open terminal (CLI + claude panes) |
| `c` | Open VS Code |
| `m` | Open Matter canvas |
| `l` | View logs (detail view) |
| `p` | Toggle proxy |
| `?` | Help |

## Config

| Variable | Default | Description |
|----------|---------|-------------|
| `DARK_ROOT` | `~/code/dark` | Where branches live |
| `DARK_SOURCE` | `~/code/dark` | Repo to clone from |
| `DARK_MULTI_TERMINAL` | `auto` | Terminal: gnome-terminal, kitty, alacritty, iterm2, etc |
| `DARK_MULTI_PROXY_PORT` | `9000` | Proxy port |

## Browser Access

```bash
multi setup-dns          # One-time
multi proxy start        # Auto-starts with branches
# Visit: http://dark-packages.<branch>.dlio.localhost:9000/ping
```
