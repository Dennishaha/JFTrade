#!/usr/bin/env python3
"""Triage parity rows still marked ``missing`` into reviewable evidence classes.

The inventory records the frozen Go baseline tests against Rust evidence.
Rows marked
``missing`` all look alike, but they are not: some describe behaviour Rust
already implements and tests (the mapping was simply never recorded), some
describe behaviour that exists but has no test, and some have no Rust
counterpart at all. Without separating them there is no way to schedule the
remaining work.

This tool adds that separation **conservatively**. It does not decide
equivalence and it never edits the inventory. For each ``missing`` row it
gathers signals and assigns a review class:

``anchor_present``
    A Rust test already carries ``// Parity:`` for this exact reference test. Strongest
    signal: the mapping is written in code and only needs review to be recorded.
``candidate_test_found``
    Rust tests exist whose names share enough vocabulary to be plausible
    matches. A human must confirm the assertions; the tool lists its
    suggestions and never copies them into the inventory.
``needs_test_or_boundary``
    No plausible Rust test name was found.

The candidate matcher is validated against the rows that already have recorded
evidence, so its precision is measurable rather than assumed. Run with
``--self-check`` to see that score.

Usage:
    python3 scripts/compatibility/parity_gap_triage.py
    python3 scripts/compatibility/parity_gap_triage.py --markdown docs/.../parity-gap-triage.md
    python3 scripts/compatibility/parity_gap_triage.py --self-check
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

# Keep in sync with ``audit_test_parity._RUST_TEST_FN``: both tools must agree
# on what counts as a Rust test, otherwise candidates and approvals diverge.
_RUST_TEST = re.compile(
    r'#\[(?:tokio::)?test(?:\([^\]]*\))?\]'
    r'(?:(?:\s*#\[[^\]]*\])|(?:\s*//[^\n]*)|(?:\s*\n\s*))*'
    r'\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z0-9_]+)'
)

# Words that carry no discriminating power between a reference test name and a Rust
# test name; leaving them in would inflate similarity.
_STOPWORDS = frozenset({
    "test", "tests", "behavior", "behaviour", "case", "cases", "works",
    "does", "and", "the", "with", "for", "from", "that", "when", "then",
    "returns", "return", "uses", "use", "via", "into", "after", "before",
})

# Minimum Jaccard similarity for a Rust test name to be offered as a
# suggestion. Calibrated against rows with recorded evidence: see --self-check.
_CANDIDATE_THRESHOLD = 0.5
_MAX_CANDIDATES = 5


def rust_test_index() -> dict:
    """Map every Rust test function name to the files defining it."""
    index = collections.defaultdict(list)
    files = glob.glob("crates/**/*.rs", recursive=True)
    files += glob.glob("apps/desktop/src-tauri/**/*.rs", recursive=True)
    for path in files:
        with open(path, "r", encoding="utf-8", errors="ignore") as handle:
            for name in _RUST_TEST.findall(handle.read()):
                index[name].append(path)
    return index


def go_name_tokens(go_test: str) -> set:
    """Split a reference test name into comparable lowercase words."""
    name = go_test[4:] if go_test.startswith("Test") else go_test
    # CamelCase boundaries, then acronym boundaries (e.g. KLine -> k_line).
    name = re.sub(r'(.)([A-Z][a-z]+)', r'\1_\2', name)
    name = re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name)
    return {
        token for token in name.lower().split("_")
        if token and token not in _STOPWORDS
    }


def suggest_candidates(go_test: str, index: dict, limit: int = _MAX_CANDIDATES) -> list:
    """Return plausible Rust test names ranked by token overlap."""
    wanted = go_name_tokens(go_test)
    if not wanted:
        return []
    scored = []
    for rust_name in index:
        have = set(rust_name.split("_")) - _STOPWORDS
        if not have:
            continue
        union = wanted | have
        if not union:
            continue
        similarity = len(wanted & have) / len(union)
        if similarity >= _CANDIDATE_THRESHOLD:
            scored.append((similarity, rust_name))
    scored.sort(key=lambda item: (-item[0], item[1]))
    return [
        {"rust_test": name, "similarity": round(score, 3), "files": index[name]}
        for score, name in scored[:limit]
    ]


def record_names(rust_entry: str) -> set:
    """Extract the Rust test names a recorded entry refers to."""
    return set(re.findall(r'::([a-z0-9_]+)', rust_entry or ""))


def self_check(inventory: dict, index: dict) -> dict:
    """Measure the matcher against rows that already have recorded evidence.

    Rows whose recorded entry names a Rust test act as a labelled sample: the
    matcher should surface that same test among its suggestions. This makes the
    suggestion quality observable instead of assumed.
    """
    recorded = {
        key: item for key, item in inventory.items()
        if item.get("evidence_type") in {"function_exact", "partial"}
    }
    recovered = 0
    unranked = 0
    no_suggestion = 0
    for key, item in recorded.items():
        go_test = key.rsplit(":", 1)[-1]
        expected = record_names(item.get("rust_entry", ""))
        if not expected:
            continue
        suggestions = {c["rust_test"] for c in suggest_candidates(go_test, index)}
        if not suggestions:
            no_suggestion += 1
        elif suggestions & expected:
            recovered += 1
        else:
            unranked += 1
    comparable = recovered + unranked
    return {
        "recorded_rows": len(recorded),
        "recovered": recovered,
        "not_recovered": unranked,
        "no_suggestion": no_suggestion,
        "recall": (recovered / comparable) if comparable else None,
    }


def triage(inventory: dict, index: dict, anchors: dict) -> dict:
    """Classify every ``missing`` row into a conservative review class."""
    classes = collections.defaultdict(list)
    for key, item in sorted(inventory.items()):
        if item.get("evidence_type") != "missing":
            continue
        parts = key.rsplit(":", 2)
        go_file, go_line, go_test = parts[0], int(parts[1]), parts[2]
        row = {
            "inventory_key": key,
            "go_file": go_file,
            "go_line": go_line,
            "go_test": go_test,
        }
        anchored = anchors.get((go_file, go_line))
        if anchored:
            row["anchors"] = anchored
            classes["anchor_present"].append(row)
            continue
        candidates = suggest_candidates(go_test, index)
        if candidates:
            row["candidates"] = candidates
            classes["candidate_test_found"].append(row)
        else:
            classes["needs_test_or_boundary"].append(row)
    return classes


def render(classes: dict, score: dict, inventory_size: int) -> str:
    def count(name: str) -> int:
        return len(classes.get(name, []))

    total_missing = count("anchor_present") + count("candidate_test_found") \
        + count("needs_test_or_boundary")
    lines = [
        "# `missing` 条目保守三分类",
        "",
        "由 `scripts/compatibility/parity_gap_triage.py` 生成，**只读**：",
        "不修改 `manual-test-mappings.json`。分类只表示“下一步该看哪里”，",
        "不表示行为等价；`candidate_test_found` 的候选项必须人工比对断言后才能升级。",
        "",
        "## 汇总",
        "",
        f"- 清单总条目：{inventory_size}",
        f"- 本报告覆盖的 `missing` 条目：{total_missing}",
        f"  - `anchor_present`（代码里已有 `// Parity:` 锚点）：{count('anchor_present')}",
        f"  - `candidate_test_found`（有名称相近的 Rust 测试，待确认）：{count('candidate_test_found')}",
        f"  - `needs_test_or_boundary`（未找到相近测试）：{count('needs_test_or_boundary')}",
        "",
        "## 匹配器自检",
        "",
        "用已记账条目（`function_exact` / `partial`）作为标注样本，检验建议里能否",
        "召回真实记录的 Rust 测试：",
        "",
        f"- 可比较样本：{score['recovered'] + score['not_recovered']}",
        f"- 召回：{score['recovered']}",
        f"- 未召回：{score['not_recovered']}",
        f"- 无建议：{score['no_suggestion']}",
    ]
    if score["recall"] is not None:
        lines.append(f"- 召回率：{score['recall'] * 100:.1f}%")
    lines += [
        "",
        "> 未召回不等于错误：抽查显示这些条目多数是 Rust 测试名采用了不同但合理的",
        "> 措辞（例如 Go `TestQueryBrokerMaxTradeQuantityReturnsSnapshot` 对应",
        "> `broker_read_projects_max_trade_quantity_snapshot`）。因此本工具只提供线索。",
        "",
        "## 一、`anchor_present`：可优先复核",
        "",
        "这些 Go 测试在 Rust 代码里已有显式锚点，只需人工确认断言后即可从 `missing` 升级。",
        "",
        "| Go 测试 | 锚点位置 |",
        "| --- | --- |",
    ]
    for row in classes.get("anchor_present", []):
        refs = "; ".join(
            f"`{a['rust_file']}:{a['rust_line']}`" for a in row["anchors"]
        )
        lines.append(f"| `{row['inventory_key']}` | {refs} |")
    lines.append("")

    lines += [
        "## 二、`candidate_test_found`：名称相近，需人工确认",
        "",
        "下表只是检索线索。**不得**据此直接标记 `function_exact`。",
        "",
        "| Go 测试 | 候选 Rust 测试（相似度） |",
        "| --- | --- |",
    ]
    for row in classes.get("candidate_test_found", []):
        shown = "；".join(
            f"`{c['rust_test']}` ({c['similarity']})" for c in row["candidates"][:3]
        )
        lines.append(f"| `{row['inventory_key']}` | {shown} |")
    lines.append("")

    lines += [
        "## 三、`needs_test_or_boundary`：需补测试或记录边界",
        "",
        "未找到名称相近的 Rust 测试。下一步是判断 Rust 是否实现了该行为：",
        "已实现→补测试；未实现或不适用→记录为边界结论。",
        "",
        "| Go 测试 |",
        "| --- |",
    ]
    for row in classes.get("needs_test_or_boundary", []):
        lines.append(f"| `{row['inventory_key']}` |")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inventory", default=DEFAULT_INVENTORY)
    parser.add_argument("--markdown", help="write the triage report to this path")
    parser.add_argument("--json", help="write the raw classes as JSON")
    parser.add_argument("--self-check", action="store_true",
                        help="only print matcher quality against recorded rows")
    parser.add_argument("--limit", type=int, default=0,
                        help="cap rows rendered per class in markdown (0 = all)")
    args = parser.parse_args()

    if not os.path.exists(args.inventory):
        print(f"inventory not found: {args.inventory}", file=sys.stderr)
        return 2
    with open(args.inventory, "r", encoding="utf-8") as handle:
        inventory = json.load(handle)

    index = rust_test_index()
    if args.self_check:
        score = self_check(inventory, index)
        print(f"recorded rows with a named Rust test: "
              f"{score['recovered'] + score['not_recovered']}")
        print(f"  recovered    : {score['recovered']}")
        print(f"  not recovered: {score['not_recovered']}")
        print(f"  no suggestion: {score['no_suggestion']}")
        if score["recall"] is not None:
            print(f"  recall       : {score['recall'] * 100:.1f}%")
        return 0

    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import parity_anchor_reconcile as anchors_module

    anchors = anchors_module.collect_anchors()
    classes = triage(inventory, index, anchors)
    score = self_check(inventory, index)

    print(f"missing rows triaged: "
          f"{sum(len(v) for v in classes.values())}")
    for name in ("anchor_present", "candidate_test_found", "needs_test_or_boundary"):
        print(f"  {name:24s}: {len(classes.get(name, []))}")
    if score["recall"] is not None:
        print(f"matcher recall vs recorded rows: {score['recall'] * 100:.1f}%")

    if args.limit:
        for name in classes:
            classes[name] = classes[name][:args.limit]

    if args.json:
        with open(args.json, "w", encoding="utf-8") as handle:
            json.dump(classes, handle, ensure_ascii=False, indent=2)
        print(f"json written to {args.json}")
    if args.markdown:
        with open(args.markdown, "w", encoding="utf-8") as handle:
            handle.write(render(classes, score, len(inventory)))
        print(f"markdown written to {args.markdown}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
