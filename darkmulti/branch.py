"""Branch management for dark-multi."""

from datetime import datetime
from pathlib import Path

from .config import DARK_ROOT, Colors, run


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
    from .config import DARK_SOURCE

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
