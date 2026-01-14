"""Configuration and utilities for dark-multi."""

import os
import subprocess
import sys
from pathlib import Path

# Paths
DARK_ROOT = Path(os.environ.get("DARK_ROOT", Path.home() / "code" / "dark"))
DARK_SOURCE = Path(os.environ.get("DARK_SOURCE", DARK_ROOT))
CONFIG_DIR = Path(os.environ.get("DARK_MULTI_CONFIG", Path.home() / ".config" / "dark-multi"))
OVERRIDES_DIR = CONFIG_DIR / "overrides"
TMUX_SESSION = "dark"
PROXY_PORT = int(os.environ.get("DARK_MULTI_PROXY_PORT", 9000))
PROXY_PID_FILE = CONFIG_DIR / "proxy.pid"

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
