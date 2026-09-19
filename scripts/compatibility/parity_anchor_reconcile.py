#!/usr/bin/env python3
"""Reconcile Rust ``// Parity:`` anchors against the Go→Rust parity inventory.

The inventory in ``manual-test-mappings.json`` records, per Go test, which Rust
test proves the same behaviour. Several Rust tests already carry an explicit
``// Parity: <go file>:<line>`` anchor, but the inventory can still list the Go
test as ``missing`` — the mapping exists in code yet was never recorded. This
tool finds those rows so they can be reviewed instead of re-discovered.

It is deliberately read-only: it never rewrites the inventory, because deciding
that an anchor proves behavioural equivalence requires a human to compare the
assertions. The output is a review list, not an approval.

Usage:
    python3 scripts/compatibility/parity_anchor_reconcile.py
    python3 scripts/compatibility/parity_anchor_reconcile.py --json report.json
"""

from __future__ import annotations

import argparse
import collections
import glob
import json
import os
import re
import sys


DEFAULT_INVENTORY = "docs/history/go-to-rust/manual-test-mappings.json"

# ``// Parity:`` anchors appear in three shapes in this workspace:
#   // Parity: go:452dea11:internal/api/routes_test.go:42 TestSomeBehaviour
#   // Parity: internal/api/routes_test.go:42 TestSomeBehaviour
#   // Parity: go:452dea11:internal/api/routes_test.go:42
# A bare revision prefix is optional, and the trailing test name is optional.
_PARITY_MARKER = re.compile(r'Parity:\s*')

# Everything after a ``Parity:`` marker is scanned for test references so a
# single marker listing more than one Go test cannot silently lose anchors.
_ANCHOR_REFERENCE = re.compile(
    r'((?:internal|pkg|cmd)/[^\s:]+_test\.go):(\d+)'
    r'(?:\s+(Test[A-Za-z0-9_]+))?'
)

_GO_REVISION_PREFIX = re.compile(r'go:[0-9a-f]{6,40}:')

# ``Parity:`` also introduces prose about production files, for example
# ``//! Parity: `internal/marketdata/service.go::GetCandles```. Those lines name
# implementation, not a Go test, and must not be treated as anchors.
_TEST_FILE_SUFFIX = "_test.go"


def rust_source_files() -> list:
    """Return every Rust file that may carry a parity anchor."""
    files = glob.glob("crates/**/*.rs", recursive=True)
    files += glob.glob("apps/desktop/src-tauri/**/*.rs", recursive=True)
    return sorted(files)


def collect_anchors(files: list = None) -> dict:
    """Map ``(go_file, go_line)`` to the anchors that reference it.

    Only anchors naming a ``_test.go`` file are kept, so prose references to
    production sources never enter the review list.
    """
    anchors = collections.defaultdict(list)
    for path in files if files is not None else rust_source_files():
        with open(path, "r", encoding="utf-8", errors="ignore") as handle:
            for number, line in enumerate(handle, 1):
                for marker in _PARITY_MARKER.finditer(line):
                    # The revision prefix (``go:452dea11:``) is provenance, not
                    # part of the reference, so drop it before scanning.
                    remainder = _GO_REVISION_PREFIX.sub("", line[marker.end():])
                    for match in _ANCHOR_REFERENCE.finditer(remainder):
                        go_file, go_line, go_test = match.groups()
                        if not go_file.endswith(_TEST_FILE_SUFFIX):
                            continue
                        anchors[(go_file, int(go_line))].append(
                            {"rust_file": path, "rust_line": number, "go_test": go_test}
                        )
                    # A marker describes one contiguous reference list; scanning
                    # the whole line once is enough.
                    break
    return anchors


def index_inventory(inventory: dict) -> dict:
    """Group inventory keys by their ``(go_file, go_line)`` portion."""
    index = collections.defaultdict(list)
    for key in inventory:
        parts = key.rsplit(":", 2)
        if len(parts) != 3:
            continue
        index[(parts[0], int(parts[1]))].append(key)
    return index


def reconcile(inventory: dict, anchors: dict) -> dict:
    """Classify anchors against inventory rows.

    Returns a dict with four buckets:

    ``already_recorded``
        Anchor lands on a row that is already exact/partial/boundary. Nothing
        to do; useful only as a sanity total.
    ``unrecorded``
        Anchor lands on a row still marked ``missing``. These are the review
        candidates — the code claims a mapping the inventory has not captured.
    ``unknown_go_test``
        Anchor names a ``(file, line)`` that the inventory does not contain at
        all, which usually means the Go line drifted after an edit.
    ``stale_anchor``
        Anchor resolves to an inventory row whose recorded Rust entry is not
        the anchoring file, so either the anchor or the entry has moved on.
    """
    index = index_inventory(inventory)
    report = {
        "already_recorded": [],
        "unrecorded": [],
        "unknown_go_test": [],
        "stale_anchor": [],
    }
    for (go_file, go_line), refs in sorted(anchors.items()):
        keys = index.get((go_file, go_line))
        if not keys:
            report["unknown_go_test"].append(
                {"go_file": go_file, "go_line": go_line, "anchors": refs}
            )
            continue
        for key in keys:
            item = inventory[key]
            entry = {
                "inventory_key": key,
                "evidence_type": item.get("evidence_type"),
                "go_test": key.rsplit(":", 1)[-1],
                "anchors": refs,
            }
            if item.get("evidence_type") == "missing":
                report["unrecorded"].append(entry)
            elif item.get("evidence_type") in {"function_exact", "partial", "boundary"}:
                anchored_files = {ref["rust_file"] for ref in refs}
                recorded = item.get("rust_entry", "") or ""
                if recorded and not any(f in recorded for f in anchored_files):
                    report["stale_anchor"].append(entry)
                else:
                    report["already_recorded"].append(entry)
    return report


def anchor_test_matches(entry: dict) -> bool:
    """Return True when every anchor names the inventory's Go test function.

    An anchor without an explicit test name cannot be checked, so it counts as
    matching; an anchor naming a different function does not.
    """
    inventory_test = entry["go_test"]
    named = [ref["go_test"] for ref in entry["anchors"] if ref["go_test"]]
    return all(name == inventory_test for name in named)


def count_gradable(report: dict) -> tuple:
    """Split unrecorded candidates into name-consistent and name-mismatched."""
    consistent, mismatched = [], []
    for entry in report["unrecorded"]:
        (consistent if anchor_test_matches(entry) else mismatched).append(entry)
    return consistent, mismatched


def render(report: dict, inventory_size: int) -> str:
    consistent, mismatched = count_gradable(report)
    lines = [
        "# Rust `// Parity:` 锚点对账",
        "",
        "该报告由 `scripts/compatibility/parity_anchor_reconcile.py` 生成，**只读**；",
        "它不修改 `manual-test-mappings.json`。锚点证明“代码里写了映射”，",
        "不等于断言等价——升级条目仍需人工比对断言。",
        "",
        "## 汇总",
        "",
        f"- 清单条目：{inventory_size}",
        f"- 锚点已记账：{len(report['already_recorded'])}",
        f"- **锚点未记账（仍标 missing）**：{len(report['unrecorded'])}"
        f"（函数名一致 {len(consistent)}，函数名不同 {len(mismatched)}）",
        f"- 锚点指向的 Go 行不在清单：{len(report['unknown_go_test'])}",
        f"- 锚点与已记账入口不一致：{len(report['stale_anchor'])}",
        "",
        "## 一、可复核候选（锚点已存在，清单仍标 missing）",
        "",
        "按“锚点里写的测试函数名 = 清单里的 Go 测试名”分组。",
        "**函数名一致**的条目只要人工确认断言，即可从 `missing` 升级；",
        "**函数名不同**的条目需要先确认锚点是否真的对应。",
        "",
    ]
    if consistent:
        lines += [
            f"### 1.1 函数名一致（{len(consistent)} 条，优先复核）",
            "",
            "| Go 测试 | 锚点所在 Rust 位置 |",
            "| --- | --- |",
        ]
        for entry in consistent:
            refs = "; ".join(
                f"`{ref['rust_file']}:{ref['rust_line']}`" for ref in entry["anchors"]
            )
            lines.append(f"| `{entry['inventory_key']}` | {refs} |")
        lines.append("")
    else:
        lines += ["### 1.1 函数名一致", "", "无。", ""]

    if mismatched:
        lines += [
            f"### 1.2 函数名不同（{len(mismatched)} 条，需先确认对应关系）",
            "",
            "| Go 测试 | 锚点中记录的测试名 | Rust 位置 |",
            "| --- | --- | --- |",
        ]
        for entry in mismatched:
            names = ", ".join(
                ref["go_test"] or "(未标注)" for ref in entry["anchors"]
            )
            refs = "; ".join(
                f"`{ref['rust_file']}:{ref['rust_line']}`" for ref in entry["anchors"]
            )
            lines.append(f"| `{entry['inventory_key']}` | {names} | {refs} |")
        lines.append("")

    lines += [
        "## 二、锚点指向的 Go 行不在清单",
        "",
        "通常是 Go 测试行号漂移或锚点写错。需要人工核对后再决定是修锚点还是补清单。",
        "",
        "| 锚点 | Rust 位置 |",
        "| --- | --- |",
    ]
    for entry in report["unknown_go_test"]:
        refs = "; ".join(
            f"`{ref['rust_file']}:{ref['rust_line']}`" for ref in entry["anchors"]
        )
        lines.append(f"| `{entry['go_file']}:{entry['go_line']}` | {refs} |")
    lines.append("")

    lines += [
        "## 三、锚点与已记账入口不一致",
        "",
        "清单已记账，但记录的 Rust 入口不是当前锚点所在文件，说明其中一方已移动。",
        "",
        "| Go 测试 | 清单证据 | 锚点位置 |",
        "| --- | --- | --- |",
    ]
    for entry in report["stale_anchor"]:
        refs = "; ".join(
            f"`{ref['rust_file']}:{ref['rust_line']}`" for ref in entry["anchors"]
        )
        lines.append(
            f"| `{entry['inventory_key']}` | {entry['evidence_type']} | {refs} |"
        )
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inventory", default=DEFAULT_INVENTORY)
    parser.add_argument("--json", help="also write the raw buckets as JSON")
    parser.add_argument("--markdown", help="write the review report to this path")
    args = parser.parse_args()

    if not os.path.exists(args.inventory):
        print(f"inventory not found: {args.inventory}", file=sys.stderr)
        return 2

    with open(args.inventory, "r", encoding="utf-8") as handle:
        inventory = json.load(handle)

    anchors = collect_anchors()
    report = reconcile(inventory, anchors)
    consistent, mismatched = count_gradable(report)

    print(f"anchors: {len(anchors)} unique Go (file,line) references")
    print(f"  already recorded : {len(report['already_recorded'])}")
    print(f"  unrecorded       : {len(report['unrecorded'])} "
          f"(name-consistent {len(consistent)}, name-mismatch {len(mismatched)})")
    print(f"  unknown go line  : {len(report['unknown_go_test'])}")
    print(f"  stale anchor     : {len(report['stale_anchor'])}")

    if args.json:
        with open(args.json, "w", encoding="utf-8") as handle:
            json.dump(report, handle, ensure_ascii=False, indent=2)
        print(f"json written to {args.json}")
    if args.markdown:
        with open(args.markdown, "w", encoding="utf-8") as handle:
            handle.write(render(report, len(inventory)))
        print(f"markdown written to {args.markdown}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
