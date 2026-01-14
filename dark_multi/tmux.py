"""tmux session management for dark-multi."""

import os
import shutil
import sys
from pathlib import Path

from .config import DARK_ROOT, TMUX_SESSION, run, warn, error


class Tmux:
    """tmux session management.

    Layout: One window per branch, each with 2 panes (CLI left, claude right)

    Keys:
        Ctrl-b n/p: switch between branch windows
        Ctrl-b o: switch between CLI and claude panes
        Ctrl-b z: zoom current pane (fullscreen toggle)
    """

    @staticmethod
    def is_available() -> bool:
        return shutil.which("tmux") is not None

    @staticmethod
    def session_exists() -> bool:
        if not Tmux.is_available():
            return False
        result = run(["tmux", "has-session", "-t", TMUX_SESSION])
        return result.returncode == 0

    @staticmethod
    def window_exists(name: str) -> bool:
        if not Tmux.session_exists():
            return False
        result = run(["tmux", "list-windows", "-t", TMUX_SESSION, "-F", "#{window_name}"])
        return result.returncode == 0 and name in result.stdout.split("\n")

    @staticmethod
    def create_window(name: str, container_id: str, branch_path: Path = None) -> None:
        """Create a tmux window with CLI + claude panes."""
        if not Tmux.is_available():
            warn("tmux not available, skipping window creation")
            return

        if not Tmux.session_exists():
            # Create session with first window
            run(["tmux", "new-session", "-d", "-s", TMUX_SESSION, "-n", name])
        else:
            # Kill existing window if present, then create new
            if Tmux.window_exists(name):
                run(["tmux", "kill-window", "-t", f"{TMUX_SESSION}:{name}"])
            run(["tmux", "new-window", "-a", "-t", TMUX_SESSION, "-n", name])

        # Left pane: CLI inside container
        run(["tmux", "send-keys", "-t", f"{TMUX_SESSION}:{name}",
             f"docker exec -it -w /home/dark/app {container_id} bash", "Enter"])

        # Split and create right pane: claude on host
        run(["tmux", "split-window", "-h", "-t", f"{TMUX_SESSION}:{name}"])
        workspace = branch_path or DARK_ROOT / name
        run(["tmux", "send-keys", "-t", f"{TMUX_SESSION}:{name}.1",
             f"cd {workspace} && claude", "Enter"])

        # Select left pane (CLI)
        run(["tmux", "select-pane", "-t", f"{TMUX_SESSION}:{name}.0"])

    @staticmethod
    def kill_window(name: str) -> None:
        if Tmux.window_exists(name):
            run(["tmux", "kill-window", "-t", f"{TMUX_SESSION}:{name}"])

    @staticmethod
    def ensure_meta_window() -> None:
        """Create the dark-meta control plane window if it doesn't exist."""
        if Tmux.window_exists("dark-meta"):
            return

        if not Tmux.session_exists():
            # Create session with meta window
            run(["tmux", "new-session", "-d", "-s", TMUX_SESSION, "-n", "dark-meta"])
            # Enable mouse support for scrolling
            run(["tmux", "set-option", "-t", TMUX_SESSION, "-g", "mouse", "on"])
        else:
            run(["tmux", "new-window", "-t", TMUX_SESSION, "-n", "dark-meta"])

        # Move meta window to be first (index 0)
        run(["tmux", "move-window", "-t", f"{TMUX_SESSION}:dark-meta", "-t", f"{TMUX_SESSION}:0"])

        # Left pane (70%): claude in dark-multi directory
        run(["tmux", "send-keys", "-t", f"{TMUX_SESSION}:dark-meta",
             f"cd {DARK_ROOT.parent / 'dark-multi'} && claude", "Enter"])

        # Right pane (30%): quick reference
        run(["tmux", "split-window", "-h", "-p", "30", "-t", f"{TMUX_SESSION}:dark-meta"])

        # Create quick reference content
        ref_text = r'''echo -e "
\033[1m=== DARK MULTI ===\033[0m

\033[1mBranch commands:\033[0m
  multi ls          - list branches
  multi new <name>  - create branch
  multi stop <name> - stop branch
  multi rm <name>   - remove branch
  multi code <name> - open VS Code

\033[1mtmux:\033[0m
  Ctrl-b n/p  - next/prev window
  Ctrl-b w    - list windows
  Ctrl-b o    - switch pane
  Ctrl-b z    - zoom pane
  Ctrl-b d    - detach

\033[1mWindows:\033[0m
  dark-meta   - this control plane
  <branch>    - CLI | claude
"
'''
        run(["tmux", "send-keys", "-t", f"{TMUX_SESSION}:dark-meta.1", ref_text, "Enter"])

        # Select the claude pane
        run(["tmux", "select-pane", "-t", f"{TMUX_SESSION}:dark-meta.0"])

    @staticmethod
    def attach() -> None:
        if not Tmux.is_available():
            error("tmux not installed")
            sys.exit(1)
        if not Tmux.session_exists():
            error("No tmux session. Start a branch first: multi start <branch>")
            sys.exit(1)
        os.execvp("tmux", ["tmux", "attach", "-t", TMUX_SESSION])
