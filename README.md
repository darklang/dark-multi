# Dark Multi

**WIP** - Tool for managing multiple Dark devcontainer instances in parallel.

```
┌─────────────────────────────────────────────────────────┐
│ DARK MULTI                                              │
│                                                         │
│ > ● main           3c +150 -42  ⚡                      │
│   ● feature-auth   1c +20 -5                            │
│   ○ bugfix-login                                        │
│                                                         │
│ System: 8 cores, 32GB RAM  •  2/4 running  •  Proxy: ●  │
│                                                         │
│ [s]tart [k]ill [t]mux [c]ode [m]atter [p]roxy [?] [q]   │
└─────────────────────────────────────────────────────────┘
```

## Install

```bash
go build -o multi ./cmd/multi
cp multi ~/.local/bin/
```

## Usage

```bash
multi                    # TUI (above)
multi ls                 # List branches
multi new <branch>       # Create branch
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
| `DARK_SOURCE` | `~/code/dark` |
| `DARK_MULTI_TERMINAL` | `auto` (gnome-terminal, kitty, iterm2, etc) |
| `DARK_MULTI_PROXY_PORT` | `9000` |
