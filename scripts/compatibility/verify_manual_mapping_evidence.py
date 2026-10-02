#!/usr/bin/env python3
"""Verify the non-exact rows have explicit, reviewable parity evidence.

The strict parity audit intentionally gates only ``function_exact`` rows. This
companion check records the evidence contract for partial and boundary rows so
their passed verification receipts cannot be mistaken for exact behavior.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def git_revision() -> str:
    return subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=ROOT, check=True, capture_output=True, text=True
    ).stdout.strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mapping", default="docs/history/go-to-rust/manual-test-mappings.json")
    parser.add_argument("--workspace-receipt", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    mapping_path = ROOT / args.mapping
    workspace_receipt_path = ROOT / args.workspace_receipt
    document = json.loads(mapping_path.read_text(encoding="utf-8"))
    mappings = document["mappings"]
    workspace_receipt = json.loads(workspace_receipt_path.read_text(encoding="utf-8"))
    passed = {
        name.rsplit("::", 1)[-1].rsplit("$", 1)[-1]
        for name in workspace_receipt.get("passedTests", [])
    }

    failures: list[str] = []
    counts: dict[str, int] = {}
    for key, item in mappings.items():
        evidence = item.get("evidence_type")
        counts[evidence] = counts.get(evidence, 0) + 1
        coverage = item.get("assertionCoverage", {})
        if coverage.get("source") != "reviewed":
            failures.append(f"{key}: assertion coverage is not reviewed")
        if item.get("coveredAssertions") != coverage.get("covered"):
            failures.append(f"{key}: coveredAssertions is out of sync")
        if item.get("uncoveredAssertions") != coverage.get("uncovered"):
            failures.append(f"{key}: uncoveredAssertions is out of sync")
        if not coverage.get("covered") and not coverage.get("uncovered"):
            failures.append(f"{key}: no assertion evidence or residual")
        if evidence in {"partial", "boundary"} and not coverage.get("uncovered"):
            failures.append(f"{key}: {evidence} row has no explicit residual")
        for target in item.get("rustEvidence", []):
            test = target.get("test")
            if test and test in passed:
                continue
            # Production symbols and explicit boundary context are allowed for
            # non-exact rows, but exact rows must be proved by a runnable test.
            if evidence == "function_exact":
                failures.append(f"{key}: exact owner test is absent from workspace receipt: {test}")

    report = {
        "schemaVersion": "jftrade.go-rust-manual-evidence-review.v1",
        "command": [
            "python3",
            "scripts/compatibility/verify_manual_mapping_evidence.py",
            "--mapping",
            args.mapping,
            "--workspace-receipt",
            args.workspace_receipt,
            "--output",
            args.output,
        ],
        "status": "passed" if not failures else "failed",
        "exitCode": 0 if not failures else 1,
        "verifiedCommit": git_revision(),
        "verifiedAt": dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
        "mappingCount": len(mappings),
        "evidenceCounts": counts,
        "workspaceReceipt": args.workspace_receipt,
        "workspacePassedTestCount": len(passed),
        "failures": failures,
    }
    output = ROOT / args.output
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": report["status"], "mappingCount": len(mappings), "failures": len(failures)}))
    return report["exitCode"]


if __name__ == "__main__":
    raise SystemExit(main())
