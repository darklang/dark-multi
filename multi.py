#!/usr/bin/env python3
"""
Dark Multi - Manage multiple Dark devcontainer instances.

Clones live at:
    ~/code/dark/main/
    ~/code/dark/fix-parser/
    ~/code/dark/feature-auth/
    ...

Each directory is named after the branch. One branch = one clone.

tmux structure:
    Session: dark
    ├── Window: main        [CLI | claude]
    ├── Window: fix-parser  [CLI | claude]
    └── ...

    Keys: Ctrl-b n/p (switch windows), Ctrl-b o (switch panes)

Usage:
    multi                       # Attach to tmux
    multi ls                    # List branches and status
    multi new <branch>          # Create branch clone + start container + tmux
    multi start <branch>        # Start stopped branch
    multi stop <branch>         # Stop branch (keeps files)
    multi rm <branch>           # Remove branch entirely (full cleanup)
    multi code <branch>         # Open VS Code for branch
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path

# Configuration
DARK_ROOT = Path(os.environ.get("DARK_ROOT", Path.home() / "code" / "dark"))
DARK_SOURCE = Path(os.environ.get("DARK_SOURCE", DARK_ROOT))
TMUX_SESSION = "dark"

# Resource estimation
RAM_PER_INSTANCE_GB = 6
CPU_PER_INSTANCE = 2


def get_system_resources() -> tuple[int, int]:
    """Get system CPU cores and RAM in GB."""
    try:
        cpu_cores = os.cpu_count() or 4
        with open("/proc/meminfo") as f:
            for line in f:
                if line.startswith("MemTotal:"):
                    kb = int(line.split()[1])
                    ram_gb = kb // (1024 * 1024)
                    return cpu_cores, ram_gb
    except:
        pass
    return 4, 16


def suggest_max_instances() -> int:
    """Suggest max concurrent instances based on system resources."""
    cpu_cores, ram_gb = get_system_resources()
    ram_limit = max(1, (ram_gb - 4) // RAM_PER_INSTANCE_GB)
    cpu_limit = max(1, cpu_cores // CPU_PER_INSTANCE)
    return min(ram_limit, cpu_limit, 10)


class Colors:
    RED = "\033[0;31m"
    GREEN = "\033[0;32m"
    YELLOW = "\033[1;33m"
    BLUE = "\033[0;34m"
    BOLD = "\033[1m"
    NC = "\033[0m"


def log(msg: str) -> None:
    print(f"{Colors.BLUE}>{Colors.NC} {msg}")


def error(msg: str) -> None:
    print(f"{Colors.RED}error:{Colors.NC} {msg}", file=sys.stderr)


def success(msg: str) -> None:
    print(f"{Colors.GREEN}✓{Colors.NC} {msg}")


def warn(msg: str) -> None:
    print(f"{Colors.YELLOW}!{Colors.NC} {msg}")


def run(cmd: list[str], **kwargs) -> subprocess.CompletedProcess:
    """Run a command."""
    kwargs.setdefault("capture_output", True)
    kwargs.setdefault("text", True)
    return subprocess.run(cmd, **kwargs)


class Branch:
    """Represents a branch clone."""

    def __init__(self, name: str):
        self.name = name
        self.path = DARK_ROOT / name
        self.metadata_file = self.path / ".multi-instance"

    @property
    def exists(self) -> bool:
        return self.path.is_dir() and (self.path / ".git").exists()

    @property
    def is_managed(self) -> bool:
        return self.metadata_file.is_file()

    @property
    def metadata(self) -> dict:
        data = {}
        if self.metadata_file.is_file():
            for line in self.metadata_file.read_text().strip().split("\n"):
                if "=" in line:
                    k, v = line.split("=", 1)
                    data[k] = v
        return data

    @property
    def instance_id(self) -> int:
        return int(self.metadata.get("ID", 0))

    @property
    def container_name(self) -> str:
        return f"dark-{self.name}"

    @property
    def container_id(self) -> str | None:
        # Try by name first (new containers)
        result = run(["docker", "ps", "-q", "--filter", f"name=^{self.container_name}$"])
        cid = result.stdout.strip()
        if cid:
            return cid
        # Fall back to label (old containers)
        result = run(["docker", "ps", "-q", "--filter", f"label=dark-dev-container={self.name}"])
        cid = result.stdout.strip()
        return cid if cid else None

    @property
    def is_running(self) -> bool:
        return self.container_id is not None

    @property
    def has_changes(self) -> bool:
        if not self.exists:
            return False
        result = run(["git", "status", "--porcelain"], cwd=self.path)
        return bool(result.stdout.strip())

    @property
    def port_base(self) -> int:
        return 10011 + self.instance_id * 100

    @property
    def bwd_port_base(self) -> int:
        return 11001 + self.instance_id * 100

    def write_metadata(self, instance_id: int) -> None:
        self.metadata_file.write_text(
            f"ID={instance_id}\n"
            f"NAME={self.name}\n"
            f"CREATED={datetime.now().isoformat()}\n"
        )

    def status_line(self) -> str:
        if self.is_running:
            status = f"{Colors.GREEN}running{Colors.NC}"
        else:
            status = f"{Colors.RED}stopped{Colors.NC}"
        changes = f" {Colors.YELLOW}[modified]{Colors.NC}" if self.has_changes else ""
        ports = f"ports {self.port_base}+/{self.bwd_port_base}+"
        return f"{Colors.BOLD}{self.name:20}{Colors.NC} {status:20} {ports}{changes}"


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


def find_next_instance_id() -> int:
    """Find the next available instance ID."""
    max_id = 0
    if DARK_ROOT.is_dir():
        for path in DARK_ROOT.iterdir():
            if path.is_dir():
                meta = path / ".multi-instance"
                if meta.is_file():
                    for line in meta.read_text().split("\n"):
                        if line.startswith("ID="):
                            try:
                                max_id = max(max_id, int(line.split("=")[1]))
                            except:
                                pass
    return max_id + 1


def find_source_repo() -> Path | None:
    """Find a repo to clone from."""
    # Check DARK_SOURCE
    if DARK_SOURCE != DARK_ROOT and DARK_SOURCE.is_dir() and (DARK_SOURCE / ".git").exists():
        return DARK_SOURCE

    # Check for 'main' branch
    main = DARK_ROOT / "main"
    if main.is_dir() and (main / ".git").exists():
        return main

    # Check any existing managed branch
    if DARK_ROOT.is_dir():
        for path in DARK_ROOT.iterdir():
            if path.is_dir() and (path / ".git").exists() and (path / ".multi-instance").is_file():
                return path

    return None


def modify_devcontainer(path: Path, branch: Branch) -> None:
    """Modify devcontainer.json for this branch."""
    devcontainer = path / ".devcontainer" / "devcontainer.json"
    if not devcontainer.exists():
        return

    content = devcontainer.read_text()

    # Port calculation - these are the HOST ports we'll expose
    # Container always uses standard ports (11001, 11002, 10011-10030)
    # We map them to branch-specific host ports
    host_ports = [branch.port_base + i for i in range(20)] + [branch.bwd_port_base, branch.bwd_port_base + 1]
    ports_str = "[" + ", ".join(map(str, host_ports)) + "]"

    # Update forwardPorts (for VS Code)
    content = re.sub(r'"forwardPorts": \[.*?\]', f'"forwardPorts": {ports_str}', content)

    # Update hostname
    content = re.sub(r'"dark-dev"', f'"dark-{branch.name}"', content)

    # Update label for container discovery
    content = re.sub(r'"dark-dev-container"', f'"dark-dev-container={branch.name}"', content)

    # Update container name in devcontainer
    content = re.sub(r'"name": "dark-builder"', f'"name": "dark-{branch.name}"', content)

    # Add --name for Docker container naming (insert after --workdir line)
    if f'"--name"' not in content:
        content = re.sub(
            r'("--workdir",\s*"/home/dark/app")',
            rf'\1,\n    "--name",\n    "dark-{branch.name}"',
            content
        )

    # Add -p port publishing for host access without VS Code
    # Map container's standard ports to branch-specific host ports
    # BwdServer: container 11001 → host bwd_port_base, container 11002 → host bwd_port_base+1
    # Test ports: container 10011-10030 → host port_base to port_base+19
    if '"-p"' not in content:
        port_mappings = []
        # BwdServer ports
        port_mappings.append(f'"-p", "{branch.bwd_port_base}:11001"')
        port_mappings.append(f'"-p", "{branch.bwd_port_base + 1}:11002"')
        # Test server ports (10011-10030 → port_base+0 to port_base+19)
        for i in range(20):
            port_mappings.append(f'"-p", "{branch.port_base + i}:{10011 + i}"')

        port_args = ",\n    ".join(port_mappings)
        content = re.sub(
            r'("--name",\s*"dark-' + branch.name + r'")',
            rf'\1,\n    {port_args}',
            content
        )

    # Make volumes per-instance to avoid conflicts between clones
    content = re.sub(r'"dark_nuget"', f'"dark_nuget_{branch.name}"', content)
    content = re.sub(r'"darklang-dark-extension-volume"', f'"dark-vscode-ext-{branch.name}"', content)
    content = re.sub(r'"darklang-dark-extension-volume-insiders"', f'"dark-vscode-ext-insiders-{branch.name}"', content)

    # Remove conflicting volumes
    lines = [l for l in content.split("\n")
             if "dark_build" not in l
             and "tree-sitter-build" not in l
             and "tree-sitter-node-modules" not in l]
    content = "\n".join(lines)

    devcontainer.write_text(content)


def get_managed_branches() -> list[Branch]:
    """Get all managed branches."""
    branches = []
    if DARK_ROOT.is_dir():
        for path in sorted(DARK_ROOT.iterdir()):
            if path.is_dir():
                b = Branch(path.name)
                if b.is_managed:
                    branches.append(b)
    return branches


# Commands

def cmd_ls(args) -> int:
    """List all branches."""
    cpu_cores, ram_gb = get_system_resources()
    suggested = suggest_max_instances()

    print(f"Branches in {DARK_ROOT}:")
    print(f"  System: {cpu_cores} cores, {ram_gb}GB RAM → suggested max: {suggested} concurrent\n")

    branches = get_managed_branches()
    running_count = sum(1 for b in branches if b.is_running)

    if branches:
        for b in branches:
            print(f"  {b.status_line()}")
        print(f"\n  Running: {running_count}/{suggested} suggested max")
    else:
        print("  (no branches)")
        print("\n  Create one: multi new <branch>")

    print()
    if Tmux.session_exists():
        print(f"tmux session '{TMUX_SESSION}' exists. Attach: multi")
    else:
        print("No tmux session yet.")

    return 0


def cmd_new(args) -> int:
    """Create a new branch clone."""
    name = args.branch
    base = args.base

    branch = Branch(name)

    if branch.exists:
        error(f"Branch '{name}' already exists at {branch.path}")
        return 1

    source = find_source_repo()
    if not source:
        error("No source repo found. Set DARK_SOURCE or create 'main' first.")
        return 1

    instance_id = find_next_instance_id()

    log(f"Creating branch '{name}' from {source}")
    log(f"  Instance ID: {instance_id}, ports: {10011 + instance_id * 100}+")

    DARK_ROOT.mkdir(parents=True, exist_ok=True)

    # Clone
    result = run(["git", "clone", str(source), str(branch.path)], capture_output=False)
    if result.returncode != 0:
        error("Clone failed")
        return 1

    # Setup branch
    log(f"Checking out branch '{name}' from '{base}'...")
    run(["git", "fetch", "origin"], cwd=branch.path)
    # Try tracking remote branch, or create new
    result = run(["git", "checkout", "-b", name, f"origin/{base}"], cwd=branch.path)
    if result.returncode != 0:
        run(["git", "checkout", "-b", name, base], cwd=branch.path)

    # Write metadata
    branch.write_metadata(instance_id)

    # Modify devcontainer
    modify_devcontainer(branch.path, branch)

    # Start container
    log("Starting devcontainer...")
    if not shutil.which("devcontainer"):
        error("devcontainer CLI not found. Install: npm install -g @devcontainers/cli")
        return 1

    result = run(["devcontainer", "up", "--workspace-folder", str(branch.path)], capture_output=False)
    if result.returncode != 0:
        error("Failed to start devcontainer")
        return 1

    time.sleep(2)

    container_id = branch.container_id
    if not container_id:
        error("Container started but couldn't find it")
        return 1

    # Create tmux window
    log("Setting up tmux window...")
    Tmux.create_window(name, container_id, branch.path)

    # Open VS Code unless --no-code
    if not args.no_code:
        log("Opening VS Code...")
        open_vscode(branch)

    success(f"Branch '{name}' ready!")
    print("\nAttach tmux: multi")
    return 0


def cmd_start(args) -> int:
    """Start a stopped branch."""
    name = args.branch
    branch = Branch(name)

    if not branch.exists:
        error(f"Branch '{name}' not found. Create it: multi new {name}")
        return 1

    if not branch.is_managed:
        error(f"'{name}' is not a managed branch")
        return 1

    if branch.is_running:
        warn(f"Branch '{name}' already running")
        if not Tmux.window_exists(name):
            log("Adding tmux window...")
            Tmux.create_window(name, branch.container_id, branch.path)
        return 0

    log(f"Starting branch '{name}'...")

    if not shutil.which("devcontainer"):
        error("devcontainer CLI not found")
        return 1

    result = run(["devcontainer", "up", "--workspace-folder", str(branch.path)], capture_output=False)
    if result.returncode != 0:
        error("Failed to start devcontainer")
        return 1

    time.sleep(2)

    container_id = branch.container_id
    if not container_id:
        error("Container started but couldn't find it")
        return 1

    log("Setting up tmux window...")
    Tmux.create_window(name, container_id, branch.path)

    # Open VS Code unless --no-code
    if not args.no_code:
        log("Opening VS Code...")
        open_vscode(branch)

    success(f"Branch '{name}' running")
    return 0


def cmd_stop(args) -> int:
    """Stop a branch (keeps files)."""
    name = args.branch
    branch = Branch(name)

    if not branch.exists:
        error(f"Branch '{name}' not found")
        return 1

    log(f"Stopping branch '{name}'...")

    # Kill tmux window
    Tmux.kill_window(name)

    # Stop and remove container
    container_id = branch.container_id
    if container_id:
        log("Stopping container...")
        run(["docker", "stop", container_id])
        run(["docker", "rm", container_id])

    success(f"Branch '{name}' stopped. Files at {branch.path}")
    return 0


def cmd_rm(args) -> int:
    """Remove a branch entirely with full cleanup."""
    name = args.branch
    branch = Branch(name)

    if not branch.exists:
        error(f"Branch '{name}' not found")
        return 1

    if not branch.is_managed:
        error(f"'{name}' is not a managed branch")
        return 1

    # Confirmation
    if not args.force:
        if branch.has_changes:
            warn(f"Branch '{name}' has uncommitted changes!")

        print(f"This will remove branch '{name}':")
        print(f"  - Stop and remove container")
        print(f"  - Remove tmux window")
        print(f"  - Delete {branch.path}")
        response = input("Proceed? [y/N] ").strip().lower()
        if response != "y":
            print("Aborted")
            return 1

    log(f"Removing branch '{name}'...")

    # 1. Kill tmux window
    Tmux.kill_window(name)

    # 2. Stop and remove container
    container_id = branch.container_id
    if container_id:
        log("Stopping container...")
        run(["docker", "stop", container_id])
        run(["docker", "rm", container_id])

    # 3. Clean up any dangling containers with this label (stopped ones)
    result = run(["docker", "ps", "-aq", "--filter", f"label=dark-dev-container={name}"])
    for cid in result.stdout.strip().split("\n"):
        if cid:
            run(["docker", "rm", "-f", cid])

    # 4. Remove directory
    log("Removing files...")
    shutil.rmtree(branch.path)

    success(f"Branch '{name}' removed")
    return 0


def open_vscode(branch: Branch) -> bool:
    """Open VS Code for a branch. Returns True on success."""
    if not branch.is_running:
        return False

    # Use devcontainer CLI to open VS Code
    if shutil.which("devcontainer"):
        result = run(["devcontainer", "open", str(branch.path)], capture_output=False)
        if result.returncode == 0:
            return True

    # Fallback: use code --remote
    if shutil.which("code"):
        container_id = branch.container_id
        import binascii
        hex_id = binascii.hexlify(container_id.encode()).decode()
        result = run(["code", "--remote", f"attached-container+{hex_id}", "/home/dark/app"], capture_output=False)
        return result.returncode == 0

    warn("Neither devcontainer CLI nor VS Code found")
    return False


def cmd_code(args) -> int:
    """Open VS Code attached to branch container."""
    name = args.branch
    branch = Branch(name)

    if not branch.exists:
        error(f"Branch '{name}' not found")
        return 1

    if not branch.is_running:
        error(f"Branch '{name}' not running. Start it first: multi start {name}")
        return 1

    log(f"Opening VS Code for '{name}'...")
    if open_vscode(branch):
        return 0
    else:
        error("Failed to open VS Code")
        return 1


def cmd_attach(args) -> int:
    """Attach to tmux session."""
    branches = get_managed_branches()

    if not branches:
        cpu_cores, ram_gb = get_system_resources()
        suggested = suggest_max_instances()

        print()
        print("No branches found. Let's set up!")
        print()
        print(f"System: {cpu_cores} cores, {ram_gb}GB RAM")
        print(f"Suggested max concurrent: {suggested}")
        print()
        print("To get started:")
        print("  1. Set DARK_SOURCE if your dark repo isn't at ~/code/dark:")
        print("     export DARK_SOURCE=~/code/dark-source")
        print()
        print("  2. Create your first branch:")
        print("     multi new main")
        print()
        return 1

    # Auto-create tmux windows for running branches
    running_branches = [b for b in branches if b.is_running]
    if running_branches:
        # Ensure meta window exists first
        Tmux.ensure_meta_window()
        for b in running_branches:
            if not Tmux.window_exists(b.name):
                log(f"Creating tmux window for {b.name}...")
                Tmux.create_window(b.name, b.container_id, b.path)
    else:
        print("No running branches. Start one first:")
        print("  multi start <branch>")
        print()
        cmd_ls(args)
        return 0

    Tmux.attach()
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Manage multiple Dark devcontainer instances",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  multi                      Attach to tmux session
  multi ls                   List branches and status
  multi new fix-parser       Create new branch from main
  multi new feat --from dev  Create from different base
  multi start fix-parser     Start stopped branch
  multi stop fix-parser      Stop branch (keeps files)
  multi rm fix-parser        Remove branch entirely
  multi code fix-parser      Open VS Code for branch
"""
    )

    sub = parser.add_subparsers(dest="cmd")

    sub.add_parser("ls", help="List branches")

    new_p = sub.add_parser("new", help="Create new branch")
    new_p.add_argument("branch", help="Branch name")
    new_p.add_argument("--from", dest="base", default="main", help="Base branch (default: main)")
    new_p.add_argument("--no-code", action="store_true", help="Don't open VS Code")

    start_p = sub.add_parser("start", help="Start stopped branch")
    start_p.add_argument("branch", help="Branch name")
    start_p.add_argument("--no-code", action="store_true", help="Don't open VS Code")

    stop_p = sub.add_parser("stop", help="Stop branch (keeps files)")
    stop_p.add_argument("branch", help="Branch name")

    rm_p = sub.add_parser("rm", help="Remove branch entirely")
    rm_p.add_argument("branch", help="Branch name")
    rm_p.add_argument("-f", "--force", action="store_true", help="Skip confirmation")

    code_p = sub.add_parser("code", help="Open VS Code for branch")
    code_p.add_argument("branch", help="Branch name")

    args = parser.parse_args()

    if args.cmd == "ls":
        return cmd_ls(args)
    elif args.cmd == "new":
        return cmd_new(args)
    elif args.cmd == "start":
        return cmd_start(args)
    elif args.cmd == "stop":
        return cmd_stop(args)
    elif args.cmd == "rm":
        return cmd_rm(args)
    elif args.cmd == "code":
        return cmd_code(args)
    else:
        return cmd_attach(args)


if __name__ == "__main__":
    sys.exit(main())
