"""Command implementations for dark-multi."""

import shutil
import time

from .config import (
    DARK_ROOT, PROXY_PORT,
    get_system_resources, suggest_max_instances,
    Colors, log, error, success, warn, run
)
from .branch import Branch, find_next_instance_id, find_source_repo, get_managed_branches
from .tmux import Tmux
from .proxy import is_proxy_running, start_proxy_server, stop_proxy_server, ensure_proxy_running
from .dns import setup_dns
from .devcontainer import get_override_config_path, generate_override_config


def cmd_ls(args) -> int:
    """List all branches."""
    cpu_cores, ram_gb = get_system_resources()
    suggested = suggest_max_instances()

    print(f"Branches in {DARK_ROOT}:")
    print(f"  System: {cpu_cores} cores, {ram_gb}GB RAM -> suggested max: {suggested} concurrent\n")

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
        print(f"tmux session 'dark' exists. Attach: multi")
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

    # Generate override config (keeps repo's devcontainer.json untouched)
    override_path = generate_override_config(branch)
    log(f"Generated override config at {override_path}")

    # Start container
    log("Starting devcontainer...")
    if not shutil.which("devcontainer"):
        error("devcontainer CLI not found. Install: npm install -g @devcontainers/cli")
        return 1

    result = run([
        "devcontainer", "up",
        "--workspace-folder", str(branch.path),
        "--override-config", str(override_path)
    ], capture_output=False)
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

    # Ensure proxy is running for URL access
    ensure_proxy_running()

    success(f"Branch '{name}' ready!")
    print(f"\nAttach tmux: multi")
    print(f"URLs: multi urls")
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

    # Ensure override config exists
    override_path = get_override_config_path(branch)
    if not override_path.exists():
        override_path = generate_override_config(branch)
        log(f"Generated override config at {override_path}")

    result = run([
        "devcontainer", "up",
        "--workspace-folder", str(branch.path),
        "--override-config", str(override_path)
    ], capture_output=False)
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

    # Ensure proxy is running for URL access
    ensure_proxy_running()

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
    from .config import CONFIG_DIR

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

    # 4. Remove override config
    override_dir = CONFIG_DIR / "overrides" / name
    if override_dir.exists():
        shutil.rmtree(override_dir)

    # 5. Remove directory
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
        import binascii
        container_id = branch.container_id
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


def cmd_proxy(args) -> int:
    """Start/stop/status the URL proxy server."""
    action = args.action

    if action == "start":
        pid = is_proxy_running()
        if pid:
            warn(f"Proxy already running (PID {pid})")
            return 0

        log(f"Starting proxy on port {PROXY_PORT}...")
        pid = start_proxy_server(PROXY_PORT, background=True)
        success(f"Proxy started (PID {pid})")
        print()
        print(f"For browser access, run: multi setup-dns (one-time)")
        print(f"Then visit: http://dark-packages.<branch>.dlio.localhost:{PROXY_PORT}/ping")
        return 0

    elif action == "stop":
        if stop_proxy_server():
            success("Proxy stopped")
        else:
            warn("Proxy not running")
        return 0

    elif action == "status":
        pid = is_proxy_running()
        if pid:
            print(f"Proxy running (PID {pid}) on port {PROXY_PORT}")
        else:
            print("Proxy not running")
        return 0

    elif action == "fg":
        # Run in foreground (for debugging)
        pid = is_proxy_running()
        if pid:
            warn(f"Proxy already running in background (PID {pid})")
            return 1
        log(f"Starting proxy on port {PROXY_PORT} (foreground)...")
        start_proxy_server(PROXY_PORT, background=False)
        return 0

    return 1


def cmd_urls(args) -> int:
    """List available URLs for running branches."""
    branches = get_managed_branches()
    running = [b for b in branches if b.is_running]

    if not running:
        print("No running branches.")
        return 0

    proxy_pid = is_proxy_running()
    proxy_status = f"running (PID {proxy_pid})" if proxy_pid else "not running"

    print(f"Proxy: {proxy_status}")
    print(f"Port: {PROXY_PORT}")
    print()

    print("Running branches:")
    for b in running:
        print()
        print(f"  {Colors.BOLD}{b.name}{Colors.NC} (ID={b.instance_id})")
        print(f"    Direct:  curl -H 'Host: dark-packages.dlio.localhost' http://localhost:{b.bwd_port_base}/ping")
        if proxy_pid:
            print(f"    Proxy:   http://dark-packages.{b.name}.dlio.localhost:{PROXY_PORT}/ping")
        print(f"    BwdServer ports: {b.bwd_port_base}-{b.bwd_port_base + 1}")
        print(f"    Test ports: {b.port_base}-{b.port_base + 19}")

    if not proxy_pid:
        print()
        print("Start proxy for nice URLs: multi proxy start")

    print()
    print("For browser access, run: multi setup-dns (one-time)")

    return 0


def cmd_setup_dns(args) -> int:
    """Set up wildcard DNS for *.dlio.localhost -> 127.0.0.1"""
    return setup_dns()
