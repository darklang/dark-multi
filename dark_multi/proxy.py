"""URL proxy server for dark-multi."""

import http.server
import os
import signal
import socketserver
import sys
import urllib.request
import urllib.error

from .config import CONFIG_DIR, PROXY_PORT, PROXY_PID_FILE, log
from .branch import get_managed_branches


class DarkProxyHandler(http.server.BaseHTTPRequestHandler):
    """Proxy that routes requests to branch-specific ports based on hostname.

    URL scheme: <canvas>.<branch>.dlio.localhost:<proxy_port>
    Example: dark-packages.main.dlio.localhost:9000 -> localhost:11101

    The proxy:
    1. Extracts branch name from hostname (e.g., 'main' from 'dark-packages.main.dlio.localhost')
    2. Looks up the branch's BwdServer port
    3. Forwards the request with proper Host header (e.g., 'dark-packages.dlio.localhost')
    """

    # Cache of branch name -> port mappings
    branch_ports = {}

    @classmethod
    def refresh_branch_ports(cls):
        """Refresh the branch -> port cache."""
        cls.branch_ports = {}
        for branch in get_managed_branches():
            if branch.is_running:
                cls.branch_ports[branch.name] = branch.bwd_port_base

    def log_message(self, format, *args):
        """Suppress default logging."""
        pass

    def do_GET(self):
        self._proxy_request()

    def do_POST(self):
        self._proxy_request()

    def do_PUT(self):
        self._proxy_request()

    def do_DELETE(self):
        self._proxy_request()

    def do_HEAD(self):
        self._proxy_request()

    def _proxy_request(self):
        host = self.headers.get("Host", "")

        # Strip port if present (e.g., "host:9000" -> "host")
        if ":" in host:
            host = host.split(":")[0]

        # Parse hostname: <canvas>.<branch>.dlio.localhost
        # e.g., dark-packages.main.dlio.localhost -> branch=main, canvas_host=dark-packages.dlio.localhost
        parts = host.split(".")
        # Must have at least 4 parts and end with dlio.localhost
        if len(parts) >= 4 and parts[-2:] == ["dlio", "localhost"]:
            # branch is second-to-last before dlio.localhost
            # ['dark-packages', 'main', 'dlio', 'localhost'] -> branch='main'
            dlio_idx = parts.index("dlio")
            branch_name = parts[dlio_idx - 1]
            canvas_parts = parts[:dlio_idx - 1] + parts[dlio_idx:]
            canvas_host = ".".join(canvas_parts)
        else:
            self._send_error(400, f"Invalid hostname format: {host}\nExpected: <canvas>.<branch>.dlio.localhost")
            return

        # Look up port for branch
        if branch_name not in self.branch_ports:
            # Refresh cache and try again
            self.refresh_branch_ports()

        if branch_name not in self.branch_ports:
            self._send_error(404, f"Branch '{branch_name}' not running.\nRunning branches: {list(self.branch_ports.keys())}")
            return

        port = self.branch_ports[branch_name]

        # Forward request
        try:
            url = f"http://localhost:{port}{self.path}"

            # Read request body if present
            content_length = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(content_length) if content_length > 0 else None

            # Build request with modified Host header
            req = urllib.request.Request(url, data=body, method=self.command)
            for key, value in self.headers.items():
                if key.lower() not in ("host", "content-length"):
                    req.add_header(key, value)
            req.add_header("Host", canvas_host)

            # Make request
            with urllib.request.urlopen(req, timeout=30) as resp:
                self.send_response(resp.status)
                for key, value in resp.getheaders():
                    if key.lower() not in ("transfer-encoding",):
                        self.send_header(key, value)
                self.end_headers()
                self.wfile.write(resp.read())

        except urllib.error.HTTPError as e:
            self.send_response(e.code)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(e.read())
        except urllib.error.URLError as e:
            self._send_error(502, f"Backend error: {e.reason}")
        except Exception as e:
            self._send_error(500, f"Proxy error: {e}")

    def _send_error(self, code: int, message: str):
        self.send_response(code)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        self.wfile.write(message.encode())


def start_proxy_server(port: int = PROXY_PORT, background: bool = True) -> int | None:
    """Start the proxy server. Returns PID if backgrounded."""
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)

    if background:
        # Fork to background
        pid = os.fork()
        if pid > 0:
            # Parent - save PID and return
            PROXY_PID_FILE.write_text(str(pid))
            return pid
        else:
            # Child - become daemon
            os.setsid()
            # Redirect stdio to /dev/null
            devnull = os.open(os.devnull, os.O_RDWR)
            os.dup2(devnull, 0)  # stdin
            os.dup2(devnull, 1)  # stdout
            os.dup2(devnull, 2)  # stderr
            os.close(devnull)

    # Refresh branch ports
    DarkProxyHandler.refresh_branch_ports()

    # Start server
    with socketserver.TCPServer(("", port), DarkProxyHandler) as httpd:
        httpd.serve_forever()

    return None


def stop_proxy_server() -> bool:
    """Stop the proxy server if running."""
    if not PROXY_PID_FILE.exists():
        return False

    try:
        pid = int(PROXY_PID_FILE.read_text().strip())
        os.kill(pid, signal.SIGTERM)
        PROXY_PID_FILE.unlink()
        return True
    except (ProcessLookupError, ValueError):
        PROXY_PID_FILE.unlink(missing_ok=True)
        return False


def is_proxy_running() -> int | None:
    """Check if proxy is running. Returns PID if running."""
    if not PROXY_PID_FILE.exists():
        return None

    try:
        pid = int(PROXY_PID_FILE.read_text().strip())
        os.kill(pid, 0)  # Check if process exists
        return pid
    except (ProcessLookupError, ValueError):
        PROXY_PID_FILE.unlink(missing_ok=True)
        return None


def ensure_proxy_running() -> None:
    """Start proxy if not already running."""
    if is_proxy_running():
        return
    pid = start_proxy_server(PROXY_PORT, background=True)
    if pid:
        log(f"Started proxy on port {PROXY_PORT} (PID {pid})")
