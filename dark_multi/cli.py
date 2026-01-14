"""CLI interface for dark-multi."""

import argparse
import sys

from .commands import (
    cmd_ls, cmd_new, cmd_start, cmd_stop, cmd_rm,
    cmd_code, cmd_attach, cmd_proxy, cmd_urls, cmd_setup_dns
)


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
  multi urls                 List available URLs for branches
  multi proxy start          Start the URL proxy server
  multi setup-dns            Set up wildcard DNS (one-time)
"""
    )

    sub = parser.add_subparsers(dest="cmd")

    sub.add_parser("ls", help="List branches")
    sub.add_parser("urls", help="List available URLs for branches")

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

    proxy_p = sub.add_parser("proxy", help="Manage URL proxy server")
    proxy_p.add_argument("action", choices=["start", "stop", "status", "fg"], help="Action")

    sub.add_parser("setup-dns", help="Set up wildcard DNS for *.dlio.localhost")

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
    elif args.cmd == "urls":
        return cmd_urls(args)
    elif args.cmd == "proxy":
        return cmd_proxy(args)
    elif args.cmd == "setup-dns":
        return cmd_setup_dns(args)
    else:
        return cmd_attach(args)


if __name__ == "__main__":
    sys.exit(main())
