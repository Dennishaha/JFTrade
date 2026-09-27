#!/usr/bin/env python3
"""Create a canonical, hashable receipt from a structured nextest run.

The parity inventory stores the SHA-256 of the resulting receipt file.  The
receipt deliberately contains the complete passed/failed/ignored test-name
sets, the exact runner command, toolchain, commit and worktree provenance, so
the digest cannot be satisfied by hashing an unstructured terminal snippet.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import subprocess
from pathlib import Path


def run(*args: str) -> str:
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout.strip()


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def source_state() -> dict:
    tracked_diff = subprocess.run(
        ["git", "diff", "--binary", "HEAD", "--"],
        check=True,
        capture_output=True,
    ).stdout
    untracked = []
    status = run("git", "status", "--short").splitlines()
    for line in status:
        if not line.startswith("?? "):
            continue
        path = Path(line[3:])
        if path.is_file():
            untracked.append({"path": str(path), "sha256": sha256_bytes(path.read_bytes())})
    return {
        "verifiedCommit": run("git", "rev-parse", "HEAD"),
        "trackedDiffSha256": sha256_bytes(tracked_diff),
        "untrackedFiles": untracked,
    }


def parse_results(path: Path) -> tuple[list[str], list[str], list[str], list[dict]]:
    passed: list[str] = []
    failed: list[str] = []
    ignored: list[str] = []
    suites: list[dict] = []
    for raw in path.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            event = json.loads(raw)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "test":
            name = event.get("name")
            status = event.get("event")
            if not isinstance(name, str):
                continue
            if status == "ok":
                passed.append(name)
            elif status in {"failed", "failed-ignored"}:
                failed.append(name)
            elif status in {"ignored", "skipped"}:
                ignored.append(name)
        elif event.get("type") == "suite":
            suites.append(event)
    return sorted(set(passed)), sorted(set(failed)), sorted(set(ignored)), suites


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--command", required=True, nargs="+")
    parser.add_argument("--runner-version", required=True)
    parser.add_argument("--toolchain", required=True)
    parser.add_argument("--exit-code", required=True, type=int)
    parser.add_argument("--verified-at")
    args, command_tail = parser.parse_known_args()
    command = [*args.command, *command_tail]

    passed, failed, ignored, suites = parse_results(args.input)
    verified_at = args.verified_at or dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    receipt = {
        "schemaVersion": "jftrade.go-rust-parity-receipt.v1",
        "command": command,
        "runnerVersion": args.runner_version,
        "toolchain": args.toolchain,
        "exitCode": args.exit_code,
        "status": "passed" if args.exit_code == 0 and not failed else "failed",
        "verifiedAt": verified_at,
        "sourceState": source_state(),
        "rawOutputSha256": sha256_bytes(args.input.read_bytes()),
        "passedTests": passed,
        "failedTests": failed,
        "ignoredTests": ignored,
        "suites": suites,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"receipt={args.output}")
    print(f"receipt_sha256={sha256_bytes(args.output.read_bytes())}")
    print(f"passed={len(passed)} failed={len(failed)} ignored={len(ignored)}")
    return 0 if receipt["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
