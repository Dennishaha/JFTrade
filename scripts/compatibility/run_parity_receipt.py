#!/usr/bin/env python3
"""Run a focused cargo-nextest batch and record a parity receipt.

The runner intentionally requires both explicit packages and test names.  A
parity receipt is evidence for the named Rust owners, so silently falling back
to ``--workspace`` would make a slow, non-repeatable batch easy to trigger.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
NEXTEST_WRAPPER = REPOSITORY_ROOT / "scripts" / "quality" / "cargo-nextest.mjs"
RECEIPT_RECORDER = Path(__file__).with_name("record_parity_receipt.py")
TEST_NAME = re.compile(r"^[A-Za-z0-9_]+$")
PACKAGE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]*$")


def nextest_version() -> str:
    """Read the pinned nextest version from the repository wrapper."""

    source = NEXTEST_WRAPPER.read_text(encoding="utf-8")
    match = re.search(r'export const nextestVersion = "([^"]+)";', source)
    if match is None:
        raise RuntimeError("cargo-nextest wrapper has no pinned version")
    return f"cargo-nextest {match.group(1)}"


def validate_names(packages: list[str], tests: list[str]) -> None:
    if not packages:
        raise ValueError("at least one --package is required")
    if not tests:
        raise ValueError("at least one --test is required")
    invalid_packages = [value for value in packages if not PACKAGE_NAME.fullmatch(value)]
    if invalid_packages:
        raise ValueError(f"invalid package name: {', '.join(invalid_packages)}")
    invalid_tests = [value for value in tests if not TEST_NAME.fullmatch(value)]
    if invalid_tests:
        raise ValueError(f"invalid test name: {', '.join(invalid_tests)}")


def nextest_expression(tests: list[str]) -> str:
    if not tests:
        raise ValueError("at least one --test is required")
    invalid_tests = [value for value in tests if not TEST_NAME.fullmatch(value)]
    if invalid_tests:
        raise ValueError(f"invalid test name: {', '.join(invalid_tests)}")
    return " or ".join(f"test({name})" for name in dict.fromkeys(tests))


def build_command(packages: list[str], tests: list[str]) -> list[str]:
    validate_names(packages, tests)
    command = [
        "env",
        "NEXTEST_EXPERIMENTAL_LIBTEST_JSON=1",
        "node",
        "scripts/quality/cargo-nextest.mjs",
        "run",
    ]
    for package in dict.fromkeys(packages):
        command.extend(("-p", package))
    command.extend((
        "--all-targets",
        "--locked",
        "--message-format",
        "libtest-json-plus",
        "--no-fail-fast",
        "-E",
        nextest_expression(tests),
    ))
    return command


def shell_command(command: list[str]) -> str:
    return shlex.join(command)


def passed_test_names(path: Path) -> set[str]:
    passed: set[str] = set()
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "test" and event.get("event") == "ok":
            name = event.get("name")
            if isinstance(name, str):
                passed.add(name.rsplit("::", 1)[-1].rsplit("$", 1)[-1])
    return passed


def record_receipt(
    raw_output: Path,
    output: Path,
    command: list[str],
    status: int,
) -> int:
    toolchain = subprocess.run(
        ["rustc", "--version"],
        cwd=REPOSITORY_ROOT,
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    recorder = [
        sys.executable,
        str(RECEIPT_RECORDER),
        "--input",
        str(raw_output),
        "--output",
        str(output),
        "--command",
        *command,
        "--runner-version",
        nextest_version(),
        "--toolchain",
        toolchain,
        "--exit-code",
        str(status),
    ]
    result = subprocess.run(recorder, cwd=REPOSITORY_ROOT, check=False)
    return result.returncode


def run(args: argparse.Namespace) -> int:
    command = build_command(args.package, args.test)
    if args.dry_run:
        print(shell_command(command))
        return 0
    if args.output is None:
        raise ValueError("--output is required unless --dry-run is used")

    output = args.output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    environment = os.environ.copy()
    environment["NEXTEST_EXPERIMENTAL_LIBTEST_JSON"] = "1"
    with tempfile.NamedTemporaryFile(prefix="jftrade-parity-nextest-", suffix=".jsonl", delete=False) as handle:
        raw_output = Path(handle.name)

    try:
        print(f"> {shell_command(command)}")
        with raw_output.open("w", encoding="utf-8") as stream:
            result = subprocess.run(
                command[2:],
                cwd=REPOSITORY_ROOT,
                env=environment,
                stdout=stream,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
        if result.stderr:
            print(result.stderr, file=sys.stderr, end="")
        status = result.returncode
        passed = passed_test_names(raw_output)
        missing = [name for name in dict.fromkeys(args.test) if name not in passed]
        if status == 0 and missing:
            print(
                "nextest exited successfully but did not report requested tests: "
                + ", ".join(missing),
                file=sys.stderr,
            )
            status = 1
        return record_receipt(raw_output, output, command, status)
    finally:
        raw_output.unlink(missing_ok=True)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    # ``pnpm run <script> -- <args>`` leaves the separator in argv with the
    # package manager version used by this repository.  Accept that one
    # conventional separator while keeping all actual options explicit.
    arguments = sys.argv[1:] if argv is None else argv
    if arguments[:1] == ["--"]:
        arguments = arguments[1:]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", action="append", required=True, help="Rust package to test; repeatable")
    parser.add_argument("--test", action="append", required=True, help="Rust test function name; repeatable")
    parser.add_argument("--output", type=Path, help="receipt JSON output path")
    parser.add_argument("--dry-run", action="store_true", help="print the focused command without running it")
    args = parser.parse_args(arguments)
    validate_names(args.package, args.test)
    if args.dry_run and args.output is not None:
        parser.error("--output cannot be combined with --dry-run")
    return args


def main(argv: list[str] | None = None) -> int:
    try:
        return run(parse_args(argv))
    except (OSError, RuntimeError, ValueError, subprocess.CalledProcessError) as error:
        print(str(error), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
