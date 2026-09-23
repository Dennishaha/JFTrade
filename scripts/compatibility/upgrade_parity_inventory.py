#!/usr/bin/env python3
"""Upgrade the historical parity mapping into the versioned evidence envelope."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import shlex
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))

import audit_test_parity as audit
from parity_anchor_reconcile import collect_anchors
from parity_inventory import SCHEMA_VERSION


def git(*args: str) -> str:
    result = subprocess.run(["git", *args], check=True, capture_output=True, text=True)
    return result.stdout.strip()


def source_blob(path: str) -> str:
    return git("rev-parse", f"go:{path}")


def rust_crate(path: str) -> str:
    if path.startswith("crates/"):
        return path.split("/", 2)[1]
    if path.startswith("apps/desktop/src-tauri/"):
        return "jftrade-desktop"
    return path.split("/", 1)[0]


def referenced_tests(entry: str, tests_by_file: dict, redirects: dict) -> list[dict]:
    evidence: list[dict] = []
    seen: set[tuple[str, str]] = set()
    for path, module in audit._RUST_REF.findall(entry or ""):
        function = module.split("::")[-1]
        candidates = audit._rust_reference_candidates(tests_by_file, redirects, path, module)
        selected = next((candidate for candidate in candidates if function in tests_by_file.get(candidate, set())), path)
        key = (selected, function)
        if key in seen:
            continue
        seen.add(key)
        evidence.append({"file": selected, "test": function, "crate": rust_crate(selected)})
    known = {name: path for path, names in tests_by_file.items() for name in names}
    for _crate, module in audit._CRATE_QUALIFIED_REF.findall(entry or ""):
        function = module.split("::")[-1]
        selected = known.get(function)
        if selected is None:
            continue
        key = (selected, function)
        if key in seen:
            continue
        seen.add(key)
        evidence.append({"file": selected, "test": function, "crate": rust_crate(selected)})
    return evidence


def command_argv(command: str) -> list[str]:
    value = (command or "").strip().strip("`")
    if not value:
        return []
    try:
        return shlex.split(value)
    except ValueError:
        return [value]


def upgrade(input_path: Path, output_path: Path) -> None:
    old = json.loads(input_path.read_text(encoding="utf-8"))
    mappings = old["mappings"] if isinstance(old, dict) and "mappings" in old else old
    if not isinstance(mappings, dict):
        raise ValueError("legacy parity mappings must be an object")

    tests_by_file, redirects = audit._rust_test_index()
    anchors = collect_anchors()
    go_commit = git("rev-parse", "go")
    go_tree = git("rev-parse", "go^{tree}")
    upgraded: dict[str, dict] = {}
    reuse: defaultdict[str, list[str]] = defaultdict(list)

    go_tests = {(test["file"], test["line"], test["name"]): test for test in audit.extract_go_tests()}
    for key, old_item in mappings.items():
        parts = key.rsplit(":", 2)
        if len(parts) != 3:
            raise ValueError(f"invalid mapping key: {key}")
        go_file, line_text, go_test = parts
        line = int(line_text)
        source_info = go_tests.get((go_file, line, go_test), {})
        evidence = referenced_tests(old_item.get("rust_entry", ""), tests_by_file, redirects)
        for target in evidence:
            target["anchor"] = None
            for anchor in anchors.get((go_file, line), []):
                if anchor["rust_file"] == target["file"] and (
                    not anchor.get("go_test") or anchor["go_test"] == go_test
                ):
                    target["anchor"] = {
                        "goFile": go_file,
                        "goLine": line,
                        "rustFile": anchor["rust_file"],
                        "rustLine": anchor["rust_line"],
                    }
                    break
            reuse_key = f"{target['file']}::{target['test']}"
            reuse[reuse_key].append(key)

        exact = old_item.get("evidence_type") == "function_exact"
        conclusion = old_item.get("conclusion", "")
        covered_assertions = [conclusion] if exact and conclusion else []
        uncovered_assertions = [conclusion] if not exact and conclusion else []
        upgraded[key] = {
            **old_item,
            "source": {
                "file": go_file,
                "line": line,
                "test": go_test,
                "blobSha": source_blob(go_file),
            },
            "rustEvidence": evidence,
            "assertionCoverage": {
                "covered": covered_assertions,
                "uncovered": uncovered_assertions,
                "source": "legacy-conclusion",
            },
            "coveredAssertions": covered_assertions,
            "uncoveredAssertions": uncovered_assertions,
            "boundaryReason": conclusion if old_item.get("evidence_type") == "boundary" and conclusion else None,
            "verification": {
                "argv": command_argv(old_item.get("command", "")),
                "testFilter": [target["test"] for target in evidence],
                "runnerVersion": None,
                "toolchain": None,
                "status": "unverified",
                "exitCode": None,
                "verifiedCommit": None,
                "verifiedAt": None,
                "receiptDigest": None,
            },
            "risk": "high" if source_info.get("high_risk") else "normal",
            "owner": source_info.get("domain", "other"),
            "priority": "P1" if source_info.get("high_risk") else "P2",
            "nextAction": (
                "record_receipt_and_review_assertions"
                if exact
                else "review_uncovered_assertions_or_confirm_boundary"
            ),
            "blockedBy": ["verification-receipt"] if exact else ["behavioral-parity-review"],
            "waiverExpiresAt": None,
        }

    document = {
        "$schema": "./parity-mappings.schema.json",
        "schemaVersion": SCHEMA_VERSION,
        "baseline": {
            "sourceRef": "go",
            "commit": go_commit,
            "treeSha": go_tree,
        },
        "generatorVersion": "upgrade_parity_inventory.py@1",
        "generatedAt": dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
        "mappings": upgraded,
        "reuse": {
            target: {
                "referenceCount": len(keys),
                "referenceKeys": keys,
                "allowed": len(keys) == 1,
                "reviewStatus": "single" if len(keys) == 1 else "unreviewed",
            }
            for target, keys in sorted(reuse.items())
        },
    }
    output_path.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", default="docs/history/go-to-rust/manual-test-mappings.json")
    parser.add_argument("--output", default="docs/history/go-to-rust/manual-test-mappings.json")
    args = parser.parse_args(argv)
    upgrade(Path(args.input), Path(args.output))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
