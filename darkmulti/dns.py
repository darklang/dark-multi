"""DNS setup for dark-multi."""

import platform
import shutil
import socket
import time
from pathlib import Path

from .config import run, log, error, success, warn


def test_dns() -> bool:
    """Test if wildcard DNS is working."""
    try:
        result = socket.gethostbyname("test-wildcard.dlio.localhost")
        return result == "127.0.0.1"
    except socket.gaierror:
        return False


def setup_dns_darwin() -> int:
    """Set up wildcard DNS for macOS."""
    print("Setting up wildcard DNS for macOS...")
    print()

    # Check for Homebrew
    if not shutil.which("brew"):
        error("Homebrew not found. Install from https://brew.sh")
        return 1

    # Check/install dnsmasq
    dnsmasq_path = run(["brew", "--prefix", "dnsmasq"]).stdout.strip()
    if not Path(dnsmasq_path).exists():
        log("Installing dnsmasq via Homebrew...")
        result = run(["brew", "install", "dnsmasq"], capture_output=False)
        if result.returncode != 0:
            error("Failed to install dnsmasq")
            return 1
        dnsmasq_path = run(["brew", "--prefix", "dnsmasq"]).stdout.strip()

    # Configure dnsmasq
    dnsmasq_conf = Path(run(["brew", "--prefix"]).stdout.strip()) / "etc" / "dnsmasq.conf"
    conf_line = "address=/dlio.localhost/127.0.0.1"

    if dnsmasq_conf.exists() and conf_line in dnsmasq_conf.read_text():
        log("dnsmasq already configured")
    else:
        log("Configuring dnsmasq...")
        print(f"  Adding to {dnsmasq_conf}")
        result = run(["sudo", "sh", "-c", f"echo '{conf_line}' >> {dnsmasq_conf}"], capture_output=False)
        if result.returncode != 0:
            error("Failed to configure dnsmasq")
            return 1

    # Start dnsmasq
    log("Starting dnsmasq service...")
    run(["sudo", "brew", "services", "restart", "dnsmasq"], capture_output=False)

    # Configure resolver
    log("Configuring macOS resolver...")
    resolver_dir = Path("/etc/resolver")
    resolver_file = resolver_dir / "dlio.localhost"

    run(["sudo", "mkdir", "-p", str(resolver_dir)], capture_output=False)
    result = run(["sudo", "sh", "-c", f"echo 'nameserver 127.0.0.1' > {resolver_file}"], capture_output=False)
    if result.returncode != 0:
        error("Failed to configure resolver")
        return 1

    return 0


def setup_dns_linux() -> int:
    """Set up wildcard DNS for Linux."""
    print("Setting up wildcard DNS for Linux...")
    print()

    # Check/install dnsmasq
    if not shutil.which("dnsmasq"):
        log("Installing dnsmasq...")
        result = run(["sudo", "apt", "install", "-y", "dnsmasq"], capture_output=False)
        if result.returncode != 0:
            error("Failed to install dnsmasq. Try: sudo apt install dnsmasq")
            return 1

    # Configure dnsmasq
    dnsmasq_conf = Path("/etc/dnsmasq.d/dark-multi.conf")
    conf_content = "address=/dlio.localhost/127.0.0.1"

    if dnsmasq_conf.exists() and conf_content in dnsmasq_conf.read_text():
        log("dnsmasq already configured")
    else:
        log("Configuring dnsmasq...")
        result = run(["sudo", "sh", "-c", f"echo '{conf_content}' > {dnsmasq_conf}"], capture_output=False)
        if result.returncode != 0:
            error("Failed to configure dnsmasq")
            return 1

    # Configure systemd-resolved to use dnsmasq for .dlio.localhost
    resolved_conf_dir = Path("/etc/systemd/resolved.conf.d")
    resolved_conf = resolved_conf_dir / "dark-multi.conf"
    resolved_content = "[Resolve]\\nDNS=127.0.0.1\\nDomains=~dlio.localhost"

    log("Configuring systemd-resolved...")
    run(["sudo", "mkdir", "-p", str(resolved_conf_dir)], capture_output=False)
    run(["sudo", "sh", "-c", f"echo -e '{resolved_content}' > {resolved_conf}"], capture_output=False)

    # Restart services
    log("Restarting services...")
    run(["sudo", "systemctl", "restart", "dnsmasq"], capture_output=False)
    run(["sudo", "systemctl", "restart", "systemd-resolved"], capture_output=False)

    return 0


def setup_dns() -> int:
    """Set up wildcard DNS for *.dlio.localhost -> 127.0.0.1"""
    system = platform.system()
    print(f"Detected platform: {system}")
    print()

    # Check if already working
    if test_dns():
        success("Wildcard DNS already configured!")
        print("  test-wildcard.dlio.localhost -> 127.0.0.1")
        return 0

    if system == "Darwin":
        result = setup_dns_darwin()
    elif system == "Linux":
        result = setup_dns_linux()
    else:
        error(f"Unsupported platform: {system}")
        print("Supported: macOS (Darwin), Linux")
        return 1

    if result != 0:
        return result

    # Wait a moment for DNS to propagate
    print()
    log("Waiting for DNS to propagate...")
    time.sleep(2)

    # Test
    if test_dns():
        success("Wildcard DNS configured successfully!")
        print()
        print("Any *.dlio.localhost now resolves to 127.0.0.1")
        print("Example: http://dark-packages.main.dlio.localhost:9000/ping")
    else:
        warn("DNS test failed - may need a moment to propagate")
        print("Try: ping test.dlio.localhost")
        print("If it doesn't resolve, you may need to restart your browser/terminal")

    return 0
