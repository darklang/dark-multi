"""Devcontainer configuration management for dark-multi."""

import json
from pathlib import Path

from .config import CONFIG_DIR, error
from .branch import Branch


def get_override_config_path(branch: Branch) -> Path:
    """Get path to override config for a branch."""
    return CONFIG_DIR / "overrides" / branch.name / "devcontainer.json"


def generate_override_config(branch: Branch) -> Path:
    """Generate a devcontainer override config for this branch.

    This reads the original devcontainer.json and merges in branch-specific
    overrides for ports, container name, etc. The original file is untouched.
    """
    override_dir = CONFIG_DIR / "overrides" / branch.name
    override_dir.mkdir(parents=True, exist_ok=True)
    override_path = override_dir / "devcontainer.json"

    # Read original devcontainer.json
    original_path = branch.path / ".devcontainer" / "devcontainer.json"
    if not original_path.exists():
        error(f"No devcontainer.json found at {original_path}")
        return None

    # Parse JSON (strip comments first - devcontainer.json allows them)
    content = original_path.read_text()
    # Remove // comments
    lines = []
    for line in content.split("\n"):
        stripped = line.lstrip()
        if not stripped.startswith("//"):
            # Also remove inline comments (crude but works for this format)
            if "//" in line and '"' not in line.split("//")[1]:
                line = line.split("//")[0].rstrip()
            lines.append(line)
    content = "\n".join(lines)

    try:
        config = json.loads(content)
    except json.JSONDecodeError as e:
        error(f"Failed to parse devcontainer.json: {e}")
        return None

    # Build port mappings: map container's standard ports to branch-specific host ports
    port_args = []
    # BwdServer: container 11001,11002 -> host bwd_port_base, bwd_port_base+1
    port_args.extend(["-p", f"{branch.bwd_port_base}:11001"])
    port_args.extend(["-p", f"{branch.bwd_port_base + 1}:11002"])
    # Test server ports: container 10011-10030 -> host port_base+0 to port_base+19
    for i in range(20):
        port_args.extend(["-p", f"{branch.port_base + i}:{10011 + i}"])

    # Host ports for forwardPorts (VS Code)
    host_ports = [branch.port_base + i for i in range(20)] + [branch.bwd_port_base, branch.bwd_port_base + 1]

    # Apply overrides
    config["name"] = f"dark-{branch.name}"
    config["forwardPorts"] = host_ports

    # Merge runArgs - keep original args, add our overrides
    original_run_args = config.get("runArgs", [])
    # Filter out any existing hostname/label/name args
    filtered_args = []
    skip_next = False
    for i, arg in enumerate(original_run_args):
        if skip_next:
            skip_next = False
            continue
        if arg in ["--hostname", "--label", "--name", "-p"]:
            skip_next = True  # Skip this and next arg
            continue
        if arg.startswith("--hostname=") or arg.startswith("--label=") or arg.startswith("--name=") or arg.startswith("-p="):
            continue
        filtered_args.append(arg)

    # Add our args
    config["runArgs"] = [
        *filtered_args,
        "--hostname", f"dark-{branch.name}",
        "--label", f"dark-dev-container={branch.name}",
        "--name", f"dark-{branch.name}",
        *port_args
    ]

    # Override mounts with branch-specific volumes
    config["mounts"] = [
        f"type=volume,src=dark_nuget_{branch.name},dst=/home/dark/.nuget",
        f"type=volume,src=dark-vscode-ext-{branch.name},dst=/home/dark/.vscode-server/extensions",
        f"type=volume,src=dark-vscode-ext-insiders-{branch.name},dst=/home/dark/.vscode-server-insiders/extensions"
    ]

    # Write merged config
    override_path.write_text(json.dumps(config, indent=2))
    return override_path
