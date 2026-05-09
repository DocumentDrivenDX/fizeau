#!/usr/bin/env python3
"""inject-shared-network: write a per-task docker-compose.yaml under each
preflight task's environment/ directory that declares an external 'harbor-tasks'
network, so all Harbor trials reuse one network instead of allocating a fresh
/24 subnet per trial.

Why: Harbor brings up each trial as its own docker compose project. By default
compose creates a network per project (`<project>_default`), which under our
sweep workload (89 tasks × 5 reps × 3 lanes = ~1300 trials) churns through
subnets faster than docker's IPAM can reclaim them, causing intermittent
"all predefined address pools have been fully subnetted" errors.

Pre-creating one external network and pointing every trial at it eliminates
the churn entirely. Acceptable cost: trials that run concurrently share a
network namespace — they can't accidentally collide on ports because each
runs in its own container, but they ARE on the same bridge. For benchmark
cells (independent tasks, not adversarial) this is fine.

Usage:
  scripts/benchmark/inject-shared-network.py
  scripts/benchmark/inject-shared-network.py --network harbor-tasks
  scripts/benchmark/inject-shared-network.py --remove   # tear down injection

Idempotent. Safe to re-run.
"""
from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

DEFAULT_NETWORK = "harbor-tasks"
DEFAULT_PREFLIGHT = "benchmark-results/external/terminal-bench-2-1-arm64-preflight"
COMPOSE_FILENAME = "docker-compose.yaml"

COMPOSE_TEMPLATE = """\
# Auto-injected by scripts/benchmark/inject-shared-network.py.
# Routes Harbor trials onto a shared external `{network}` network so we don't
# allocate a fresh subnet per trial. Trials are independent benchmark cells,
# so cross-trial network isolation is not required and the subnet churn that
# the per-project default would cause is the bigger correctness/operability
# concern. Remove this file (or run with --remove) to revert.
services:
  main:
    networks:
      - default
networks:
  default:
    external: true
    name: {network}
"""


def repo_root() -> Path:
    here = Path(__file__).resolve()
    for parent in [here.parent] + list(here.parents):
        if (parent / "go.mod").exists() or (parent / ".git").exists():
            return parent
    return Path.cwd()


def ensure_network(name: str) -> None:
    """Create the external network if missing."""
    cp = subprocess.run(
        ["docker", "network", "inspect", name],
        capture_output=True, text=True, check=False,
    )
    if cp.returncode == 0:
        return
    print(f"creating shared network: {name}", file=sys.stderr)
    subprocess.run(["docker", "network", "create", name], check=True)


def remove_network_if_orphaned(name: str) -> None:
    """Best-effort: remove the network if no containers are attached."""
    cp = subprocess.run(
        ["docker", "network", "rm", name],
        capture_output=True, text=True, check=False,
    )
    if cp.returncode == 0:
        print(f"removed shared network: {name}", file=sys.stderr)
    else:
        print(f"left network {name} in place: {cp.stderr.strip()}", file=sys.stderr)


def discover_environment_dirs(preflight: Path) -> list[Path]:
    """Find every <task>/<digest>/environment/ dir."""
    out: list[Path] = []
    for env_dir in preflight.glob("terminal-bench/*/*/environment"):
        if env_dir.is_dir():
            out.append(env_dir)
    return sorted(out)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--network", default=DEFAULT_NETWORK,
                        help=f"External network name (default: {DEFAULT_NETWORK})")
    parser.add_argument("--preflight", default=None,
                        help=f"Path to preflight overlay (default: <repo>/{DEFAULT_PREFLIGHT})")
    parser.add_argument("--remove", action="store_true",
                        help="Remove the injection (delete docker-compose.yaml from each env dir)")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print actions without making changes")
    args = parser.parse_args()

    repo = repo_root()
    preflight = Path(args.preflight) if args.preflight else (repo / DEFAULT_PREFLIGHT)
    if not preflight.exists():
        print(f"preflight overlay not found: {preflight}", file=sys.stderr)
        return 1

    env_dirs = discover_environment_dirs(preflight)
    if not env_dirs:
        print(f"no task environments found under {preflight}", file=sys.stderr)
        return 1

    if args.remove:
        removed = 0
        for env_dir in env_dirs:
            target = env_dir / COMPOSE_FILENAME
            if target.exists():
                if args.dry_run:
                    print(f"would remove: {target}")
                else:
                    target.unlink()
                removed += 1
        print(f"removed {removed} injected compose files from {len(env_dirs)} env dirs",
              file=sys.stderr)
        if not args.dry_run:
            remove_network_if_orphaned(args.network)
        return 0

    if not args.dry_run:
        ensure_network(args.network)

    body = COMPOSE_TEMPLATE.format(network=args.network)
    written = skipped = 0
    for env_dir in env_dirs:
        target = env_dir / COMPOSE_FILENAME
        if target.exists() and target.read_text() == body:
            skipped += 1
            continue
        if args.dry_run:
            print(f"would write: {target}")
        else:
            target.write_text(body)
        written += 1
    print(f"injected {written} compose files into {len(env_dirs)} env dirs ({skipped} already up to date)",
          file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
