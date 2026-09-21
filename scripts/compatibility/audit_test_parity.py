#!/usr/bin/env python3
"""
Audit parity between Go tests in the `go` branch and Rust tests in the current workspace.
Categorizes tests by domain, calculates porting ratio, and highlights high-risk unported tests.
"""

import subprocess
import os
import re
import glob
import json
import sys


def branch_revision(branch: str) -> str:
    """Return a stable short revision for report provenance."""
    result = subprocess.run(
        ["git", "rev-parse", "--short", branch], capture_output=True, text=True
    )
    return result.stdout.strip() if result.returncode == 0 else "unknown"


_cargo_package_names_cache = None


def _cargo_package_names():
    """Return workspace package names, or None when cargo metadata is unusable.

    The audit checks every ``-p <crate>`` reference in the mappings; without
    caching, each check would re-run ``cargo metadata`` and the audit would
    spend most of its time forking subprocesses.
    """
    global _cargo_package_names_cache
    if _cargo_package_names_cache is None:
        result = subprocess.run(
            ["cargo", "metadata", "--no-deps", "--format-version", "1"],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            _cargo_package_names_cache = False
        else:
            try:
                packages = json.loads(result.stdout).get("packages", [])
                _cargo_package_names_cache = {
                    package.get("name") for package in packages
                }
            except json.JSONDecodeError:
                _cargo_package_names_cache = False
    if _cargo_package_names_cache is False:
        return None
    return _cargo_package_names_cache


def _is_known_cargo_package(name: str) -> bool:
    """Return True when ``name`` is a cargo package in this workspace.

    Workspace members live under ``crates/``, but the desktop shell is a
    non-member package at ``apps/desktop/src-tauri``. Both are valid targets
    for ``cargo nextest run -p``, so resolve through cargo metadata instead of
    assuming a ``crates/<name>`` directory exists.
    """
    names = _cargo_package_names()
    if names is None:
        # Fall back to the historical path probe when cargo is unavailable.
        return os.path.isdir(os.path.join("crates", name))
    return name in names


# Matches a ``#[test]`` / ``#[tokio::test]`` attribute followed by the test
# function name. Attributes, ``// Parity:`` comments, and blank lines may sit
# between the attribute and ``fn``; without allowing them the resolver misses
# the common ``#[test]`` + anchor-comment + ``fn`` layout used across this
# workspace. Keep in sync with ``parity_gap_triage._RUST_TEST``.
_RUST_TEST_FN = re.compile(
    r'#\[(?:tokio::)?test(?:\([^\]]*\))?\]'
    r'(?:(?:\s*#\[[^\]]*\])|(?:\s*//[^\n]*)|(?:\s*\n\s*))*'
    r'\s*(?:pub(?:\([^)]*\))?\s+)?'
    r'(?:async\s+)?fn\s+([A-Za-z0-9_]+)'
)

# ``#[path = "x_tests.rs"] mod tests;`` lives in the owner file, so a reference
# like ``owner.rs::tests::some_case`` only resolves after following the
# redirect into the real test file.
_RUST_PATH_REDIRECT = re.compile(
    r'#\[path\s*=\s*"([^"]+)"\]\s*(?:#\[[^\]]*\]\s*)*mod\s+([A-Za-z0-9_]+)\s*;'
)

_RUST_REF = re.compile(r'([A-Za-z0-9_\-/\.]+\.rs)::([A-Za-z0-9_:]+)')

# Some rows name the test by crate path instead of by source file, for example
# ``jftrade-desktop::desktop_contracts::some_case``. The trailing segment is
# still a test function name, so the same existence check applies.
_CRATE_QUALIFIED_REF = re.compile(r'\b(jftrade-[a-z0-9-]+)::([A-Za-z0-9_:]+)')


def _workspace_rust_files() -> list:
    """Return every Rust source file a parity reference may point at."""
    files = []
    for pattern in ("crates/**/*.rs", "apps/desktop/src-tauri/**/*.rs"):
        files.extend(glob.glob(pattern, recursive=True))
    return [os.path.normpath(path) for path in files]


def _rust_test_index() -> tuple:
    """Index test functions and ``#[path]`` redirects for the whole workspace.

    Returns ``(tests_by_file, redirects)`` where ``tests_by_file`` maps a
    normalized path to the set of ``#[test]`` function names defined in it, and
    ``redirects`` maps ``(owner_path, module_name)`` to the included file.
    """
    tests_by_file = {}
    for path in _workspace_rust_files():
        with open(path, "r", encoding="utf-8", errors="ignore") as handle:
            tests_by_file[path] = set(_RUST_TEST_FN.findall(handle.read()))

    redirects = {}
    for path in tests_by_file:
        with open(path, "r", encoding="utf-8", errors="ignore") as handle:
            content = handle.read()
        for target, module in _RUST_PATH_REDIRECT.findall(content):
            included = os.path.normpath(os.path.join(os.path.dirname(path), target))
            redirects[(path, module)] = included
    return tests_by_file, redirects


def _rust_reference_candidates(tests_by_file, redirects, path, module_path) -> list:
    """Return the files that may define the ``#[test]`` a reference names.

    ``path`` is the file named in the mapping and ``module_path`` is the
    ``mod::submod::case`` suffix. The module chain is followed through
    ``#[path]`` redirects, then the same-directory ``<stem>_tests.rs`` and
    ``<stem>/tests.rs`` conventions are tried, because the audit records the
    owner file while the test itself lives in the sibling module.
    """
    path = os.path.normpath(path)
    if path not in tests_by_file:
        return []

    parts = module_path.split("::")
    modules = parts[:-1]

    candidates = [path]
    current = path
    for module in modules:
        included = redirects.get((current, module))
        if included is None:
            break
        current = included
        candidates.append(current)

    stem = os.path.splitext(path)[0]
    for sibling in (stem + "_tests.rs", os.path.join(stem, "tests.rs")):
        candidate = os.path.normpath(sibling)
        if candidate in tests_by_file:
            candidates.append(candidate)
    return candidates


def _resolve_rust_test_reference(tests_by_file, redirects, path, module_path) -> bool:
    """Return True when a reference names a real ``#[test]`` function."""
    function = module_path.split("::")[-1]
    return any(
        function in tests_by_file.get(candidate, ())
        for candidate in _rust_reference_candidates(tests_by_file, redirects, path, module_path)
    )


def unresolved_parity_references(manual_details: dict) -> tuple:
    """Find mapping rows whose Rust entries name no real test function.

    Reference integrity is enforced where it carries a guarantee and reported
    where it does not:

    * ``function_exact`` is an approval. It must name at least one Rust test
      that exists in the workspace, otherwise the audit fails.
    * ``partial`` is an acknowledged gap. An unresolvable reference is a stale
      citation worth surfacing, but it never fails the audit.
    * ``boundary``/``missing`` rows describe the absence of a mapping and are
      not reference-checked.

    A row may cite production symbols as context next to the test that proves
    the behavior, so a row passes when *any* of its ``file::fn`` or
    ``crate::mod::fn`` references resolves to a real ``#[test]``.

    Returns ``(broken_approvals, stale_references)``, each a list of
    ``(mapping_key, reference_detail)`` tuples.
    """
    tests_by_file, redirects = _rust_test_index()
    known_test_names = set()
    for names in tests_by_file.values():
        known_test_names |= names

    broken_approvals = []
    stale_references = []
    for key, item in manual_details.items():
        evidence = item.get("evidence_type")
        if evidence not in {"function_exact", "partial"}:
            continue

        entry = item.get("rust_entry", "")
        file_references = _RUST_REF.findall(entry)
        crate_references = _CRATE_QUALIFIED_REF.findall(entry)

        if not file_references and not crate_references:
            # Only approvals must name a runnable test; a partial row may
            # describe an acknowledged gap in prose.
            if evidence == "function_exact":
                broken_approvals.append((key, "未引用任何 .rs::测试 或 crate::测试"))
            continue

        resolves = any(
            _resolve_rust_test_reference(tests_by_file, redirects, path, module)
            for path, module in file_references
        ) or any(
            module_path.split("::")[-1] in known_test_names
            for _, module_path in crate_references
        )
        if resolves:
            continue

        detail = "; ".join(
            [f"{path}::{module}" for path, module in file_references]
            + [f"{crate}::{module}" for crate, module in crate_references]
        )
        bucket = broken_approvals if evidence == "function_exact" else stale_references
        bucket.append((key, detail))
    return broken_approvals, stale_references


def _parity_anchor_index() -> dict:
    """Return ``(go_file, go_line) -> [anchor, ...]`` from the reconcile tool.

    A ``// Parity:`` anchor written next to a Rust test is the code-side half
    of a mapping claim, and the inventory is the other half. Reusing the
    reconcile implementation keeps a single definition of the anchor grammar
    instead of letting a second copy drift from it.
    """
    directory = os.path.dirname(os.path.abspath(__file__))
    added = directory not in sys.path
    if added:
        sys.path.insert(0, directory)
    try:
        import parity_anchor_reconcile

        return dict(parity_anchor_reconcile.collect_anchors())
    finally:
        if added:
            try:
                sys.path.remove(directory)
            except ValueError:
                pass


def unanchored_approvals(manual_details: dict, anchors: dict = None) -> list:
    """Find ``function_exact`` rows whose code anchor does not point back.

    A row claims one reference test is proved by a named Rust test. When that
    Rust test carries a ``// Parity:`` anchor for the same Go file and line, the
    claim is written in code as well as in the inventory, so a reviewer can
    find it next to the assertions. This reports the rows where only the
    inventory makes the claim.

    It is deliberately advisory: an unanchored row can still be correct, it
    just has no code-side evidence to trace. The check exists because
    reference *existence* alone cannot tell a genuine approval from an anchor
    that drifted onto the wrong test.

    Returns a list of ``(mapping_key, detail)`` tuples.
    """
    if anchors is None:
        anchors = _parity_anchor_index()
    tests_by_file, redirects = _rust_test_index()

    unanchored = []
    for key, item in manual_details.items():
        if item.get("evidence_type") != "function_exact":
            continue
        parts = key.rsplit(":", 2)
        if len(parts) != 3 or not parts[1].isdigit():
            continue
        go_file, go_line = parts[0], int(parts[1])

        references = _RUST_REF.findall(item.get("rust_entry", ""))
        if not references:
            # unresolvable_parity_references already fails an approval that
            # names no test at all; this check only judges anchored claims.
            continue

        anchored_files = {
            anchor["rust_file"] for anchor in anchors.get((go_file, go_line), ())
        }
        cited_files = set()
        for path, module in references:
            cited_files.update(
                _rust_reference_candidates(tests_by_file, redirects, path, module)
            )
        if cited_files & anchored_files:
            continue

        detail = "引用 " + "; ".join(
            f"{path}::{module}" for path, module in references
        )
        if anchored_files:
            detail += "；但该 Go 测试的锚点位于 " + "; ".join(sorted(anchored_files))
        else:
            detail += "；该 Go 测试没有任何 // Parity: 锚点"
        unanchored.append((key, detail))
    return unanchored


def _rust_test_bodies() -> dict:
    """Map ``(path, fn_name)`` to the source slice following the fn signature.

    The slice runs to the next ``#[test]`` in the same file (or EOF), which is
    enough for a heuristic content check without parsing Rust.
    """
    bodies = {}
    for path in _workspace_rust_files():
        with open(path, "r", encoding="utf-8", errors="ignore") as handle:
            content = handle.read()
        matches = list(_RUST_TEST_FN.finditer(content))
        for index, match in enumerate(matches):
            end = matches[index + 1].start() if index + 1 < len(matches) else len(content)
            bodies[(path, match.group(1))] = content[match.end():end]
    return bodies


def assertionless_approved_references(manual_details: dict) -> list:
    """List ``function_exact`` rows whose cited Rust tests contain no assertion.

    Existence alone is a weak approval: an empty ``#[test]`` resolves just as
    well as a real behavior test. Rows listed here are not audit failures —
    the assertion may live in a shared helper — but they are the cheapest
    place to inflate the ``function_exact`` count, so they are surfaced for
    manual review.
    """
    bodies = _rust_test_bodies()
    known_test_names = {name for (_path, name) in bodies}
    names_with_assert = {
        name for (_path, name), body in bodies.items() if "assert" in body
    }
    warnings = []
    for key, item in manual_details.items():
        if item.get("evidence_type") != "function_exact":
            continue
        entry = item.get("rust_entry", "")
        referenced = [
            module.split("::")[-1] for _path, module in _RUST_REF.findall(entry)
        ] + [
            module_path.split("::")[-1]
            for _crate, module_path in _CRATE_QUALIFIED_REF.findall(entry)
        ]
        resolvable = [name for name in referenced if name in known_test_names]
        if resolvable and not any(name in names_with_assert for name in resolvable):
            warnings.append((key, entry))
    return warnings


DOMAIN_MAPPING = [
    # (domain_key, domain_label, go_path_prefixes, rust_crates_or_paths)
    (
        "futu_opend",
        "Futu / OpenD Protocol & Integration",
        ["pkg/futu", "internal/integration/futu"],
        ["crates/jftrade-integration-futu"]
    ),
    (
        "marketdata_quotes",
        "MarketData / Quotes & Providers",
        # ``internal/productfeatures`` is the provider facade and typed-query
        # layer over market data; its recorded evidence lives in the engine and
        # marketdata crates. The earlier ``internal/productfeatures/marketdata``
        # entry named a path that never existed in the Go tree, so those rows
        # silently fell through to "other" and understated this domain.
        ["internal/marketdata", "pkg/market", "internal/productfeatures"],
        ["crates/jftrade-marketdata", "crates/jftrade-integration-marketdata-helper"]
    ),
    (
        "trading_broker",
        "Trading & Broker Execution",
        ["pkg/broker", "internal/trading"],
        ["crates/jftrade-trading", "crates/jftrade-broker"]
    ),
    (
        "strategy_pine",
        "Strategy & Pine Runtime",
        ["pkg/strategy", "internal/strategy"],
        ["crates/jftrade-strategy", "crates/jftrade-integration-pine"]
    ),
    (
        "backtest_calendar",
        "Backtest & Exchange Calendar",
        # ``internal/exchangecalendar`` carries the exchange trading-calendar
        # behaviour this domain is named for; it was previously unmapped and
        # its fully-migrated rows were counted under "other".
        ["pkg/backtest", "pkg/market/calendar", "internal/backtest", "internal/exchangecalendar"],
        ["crates/jftrade-backtest", "crates/jftrade-calendar", "crates/jftrade-integration-calendar"]
    ),
    (
        "assistant_workflow",
        "Assistant & Workflow ADK",
        ["internal/assistant"],
        ["crates/jftrade-assistant"]
    ),
    (
        "storage_sqlite",
        "Storage & SQLite Persistence",
        ["internal/store"],
        ["crates/jftrade-store-sqlite", "crates/jftrade-store-settings-file", "crates/jftrade-owner-lock"]
    ),
    (
        "settings_watchlist",
        "Settings & Watchlist",
        ["internal/settings", "internal/watchlist"],
        ["crates/jftrade-settings", "crates/jftrade-watchlist"]
    ),
    (
        "api_transport",
        "API Server & Transport Wire",
        ["internal/api", "internal/app/apiserver", "cmd/" + "jftrade-api"],
        ["crates/jftrade-api"]
    ),
]

OTHER_CANDIDATE_CRATES = [
    "crates/jftrade-kernel",
    "crates/jftrade-engine",
    "crates/jftrade-calendar",
    "crates/jftrade-settings",
    "apps/desktop/src-tauri",
    "scripts/quality",
]

ENGINE_ROUTING = {
    # If a test matches these terms in crates/jftrade-engine, route to the respective domain
    "futu_opend": ["futu", "opend"],
    "marketdata_quotes": ["market_data", "marketdata", "quote", "candle", "catalog"],
    "trading_broker": ["execution", "trade", "broker", "order", "position", "reconciliation"],
    "strategy_pine": ["strategy", "pine"],
    "backtest_calendar": ["backtest", "calendar"],
    "assistant_workflow": ["assistant", "workflow", "adk", "approval"],
    "storage_sqlite": ["sqlite", "store", "database", "migration"],
    "settings_watchlist": ["settings", "watchlist"],
    "api_transport": ["api", "transport", "wire", "route", "http"],
}

HIGH_RISK_KEYWORDS = [
    "pagination", "page", "nextreqkey", "cache", "session", "dst", "timezone",
    "reconcil", "fallback", "retry", "timeout", "cancel", "rollback",
    "deadlock", "race", "recovery", "disconnect", "reconnect", "null", "empty"
]

def classify_go_domain(file_path: str) -> str:
    """Route a Go source path to the migration domain that owns it.

    The **longest** matching prefix wins, so a nested family such as
    ``pkg/market/calendar`` lands in ``backtest_calendar`` even though the
    broader ``pkg/market`` belongs to ``marketdata_quotes``. Choosing by length
    rather than by declaration order keeps the result independent of where a
    domain sits in ``DOMAIN_MAPPING``; ties keep the earlier declaration. A
    path with no match is reported as ``other`` so that an unmapped family
    stays visible instead of being silently folded into a domain it does not
    belong to.

    A prefix matches only paths *inside* that directory. A bare ``startswith``
    would also swallow sibling packages whose directory names merely begin with
    the same text — ``internal/pine`` would claim the sibling asset package —
    and those asset-packaging families belong to tooling, not to the business
    domain that owns the shorter name.
    """
    domain = "other"
    matched_length = -1
    for d_key, _, prefixes, _ in DOMAIN_MAPPING:
        for prefix in prefixes:
            if (file_path.startswith(prefix + "/")) and len(prefix) > matched_length:
                domain, matched_length = d_key, len(prefix)
    return domain


def extract_go_tests():
    out = subprocess.run(['git', 'grep', '-n', '^func Test', 'go'], capture_output=True, text=True).stdout
    tests = []
    for line in out.splitlines():
        parts = line.split(':', 3)
        if len(parts) >= 4:
            file_path = parts[1]
            line_num = int(parts[2])
            func_sig = parts[3]
            test_name = func_sig.replace('func ', '').split('(')[0].strip()

            domain = classify_go_domain(file_path)
            
            is_high_risk = any(kw in test_name.lower() or kw in file_path.lower() for kw in HIGH_RISK_KEYWORDS)
            
            tests.append({
                "file": file_path,
                "line": line_num,
                "name": test_name,
                "domain": domain,
                "high_risk": is_high_risk
            })
    return tests

def _rust_source_files() -> list:
    """Every Rust file that can define a ``#[test]`` the mappings may cite.

    ``_workspace_rust_files`` already resolves references in the Tauri shell
    crate, so the headline test total must scan the same roots or the report
    understates the workspace and disagrees with the reference set.
    """
    files = glob.glob('crates/**/*.rs', recursive=True)
    files.extend(glob.glob('apps/desktop/src-tauri/**/*.rs', recursive=True))
    return sorted(files)


def _rust_crate_name(path: str) -> str:
    """Crate directory for a workspace Rust file, including the Tauri shell."""
    if path.startswith('apps/desktop/src-tauri/'):
        return 'apps/desktop/src-tauri'
    parts = path.split('/')
    return parts[1] if len(parts) > 1 else 'unknown'


def extract_rust_tests():
    tests = []
    # Count with the same regex the reference resolver uses, so the totals in
    # the report and the resolvable reference set cannot drift apart.
    test_pattern = _RUST_TEST_FN
    
    for f in _rust_source_files():
        crate_name = _rust_crate_name(f)
        with open(f, 'r', encoding='utf-8', errors='ignore') as fp:
            content = fp.read()
        for m in test_pattern.finditer(content):
            test_name = m.group(1)
            # assign domain
            domain = "other"
            if crate_name == "jftrade-engine":
                # examine file name or test name
                fname = os.path.basename(f).lower()
                for d_key, kws in ENGINE_ROUTING.items():
                    if any(kw in fname for kw in kws):
                        domain = d_key
                        break
            else:
                for d_key, _, _, crates in DOMAIN_MAPPING:
                    if any(c.replace("crates/", "") == crate_name for c in crates):
                        domain = d_key
                        break
            
            tests.append({
                "file": f,
                "crate": crate_name,
                "name": test_name,
                "domain": domain,
            })
    return tests

def main():
    print("Extracting Go tests from branch 'go'...")
    go_tests = extract_go_tests()
    print(f"Total Go tests: {len(go_tests)}")
    # Test names are not globally unique (different packages may reuse names).
    # Keep this visible in the audit instead of silently collapsing rows by name.
    duplicate_names = {}
    by_name = {}
    for test in go_tests:
        by_name.setdefault(test["name"], []).append(test)
    for name, entries in by_name.items():
        if len(entries) > 1:
            duplicate_names[name] = [f"{item['file']}:{item['line']}" for item in entries]
    if duplicate_names:
        print(f"WARNING: {len(duplicate_names)} duplicate baseline test names require file:line-scoped mappings")

    print("Extracting Rust tests from current workspace...")
    rust_tests = extract_rust_tests()
    print(f"Total Rust tests: {len(rust_tests)}")

    # Aggregate by domain
    domain_stats = {}
    for d_key, d_label, _, _ in DOMAIN_MAPPING + [("other", "Other / Tooling / Core", [], [])]:
        domain_stats[d_key] = {
            "label": d_label,
            "go_total": 0,
            "go_high_risk": 0,
            "rust_total": 0,
            "unported_samples": []
        }

    for t in go_tests:
        d = domain_stats[t["domain"]]
        d["go_total"] += 1
        if t["high_risk"]:
            d["go_high_risk"] += 1
            if len(d["unported_samples"]) < 10:
                d["unported_samples"].append(f"{t['file']}:{t['line']} {t['name']}")

    for t in rust_tests:
        if t["domain"] in domain_stats:
            domain_stats[t["domain"]]["rust_total"] += 1
        else:
            domain_stats["other"]["rust_total"] += 1

    # Print summary table
    print("\n" + "="*90)
    print(f"{'Domain':<40} | {'Go Tests':<10} | {'Go HighRisk':<12} | {'Rust Tests':<10} | {'Ratio':<8}")
    print("="*90)
    
    total_go = 0
    total_go_hr = 0
    total_rust = 0

    for d_key, s in domain_stats.items():
        ratio = f"{(s['rust_total'] / s['go_total'] * 100):.1f}%" if s['go_total'] > 0 else "N/A"
        print(f"{s['label']:<40} | {s['go_total']:<10} | {s['go_high_risk']:<12} | {s['rust_total']:<10} | {ratio:<8}")
        total_go += s['go_total']
        total_go_hr += s['go_high_risk']
        total_rust += s['rust_total']

    print("="*90)
    total_ratio = f"{(total_rust / total_go * 100):.1f}%"
    print(f"{'TOTAL':<40} | {total_go:<10} | {total_go_hr:<12} | {total_rust:<10} | {total_ratio:<8}")
    print("="*90)

    # Write detailed markdown report
    report_path = "docs/history/go-to-rust/test-parity-report.md"
    os.makedirs(os.path.dirname(report_path), exist_ok=True)
    with open(report_path, "w", encoding="utf-8") as fp:
        fp.write("# Go 与 Rust 测试用例全景对齐审计报告\n\n")
        fp.write("本报告由 `scripts/compatibility/audit_test_parity.py` 自动扫描生成。\n\n")
        fp.write("## 1. 总体概况\n\n")
        go_revision = branch_revision("go")
        current_revision = branch_revision("HEAD")
        fp.write(f"- **Go 分支（`go:{go_revision}`）测试总数**：{total_go}\n")
        fp.write(f"- **Go 高风险测试用例数**（涉及分页、缓存、时区、对账、断连重连等）：{total_go_hr}\n")
        fp.write(f"- **Rust 当前测试总数**：{total_rust}\n")
        fp.write(f"- **总体测试数量比（非覆盖率）**：{total_ratio}\n")
        fp.write(f"- **Rust 基线（`{current_revision}`）**：当前工作树\n\n")
        fp.write(f"- **同名 Go 测试组**：{len(duplicate_names)}（映射必须使用文件路径与行号，不能仅按测试名）\n")
        fp.write("## 2. 分领域对齐矩阵\n\n")
        fp.write("| 业务领域 | Go 测试数 | Go 高风险数 | Rust 测试数 | 测试数量比（非覆盖率） |\n")
        fp.write("| :--- | :--- | :--- | :--- | :--- |\n")
        for d_key, s in domain_stats.items():
            ratio = f"{(s['rust_total'] / s['go_total'] * 100):.1f}%" if s['go_total'] > 0 else "N/A"
            fp.write(f"| {s['label']} | {s['go_total']} | {s['go_high_risk']} | {s['rust_total']} | {ratio} |\n")
        fp.write("\n> 注意：测试数量比只表示数量关系，不证明行为等价；行为证据以逐项清单中的 `evidence_type` 为准。\n")
        
        fp.write("\n## 3. 高风险待对齐用例采样（各领域 Top 10）\n\n")
        for d_key, s in domain_stats.items():
            if s["unported_samples"]:
                fp.write(f"### {s['label']}\n\n")
                for item in s["unported_samples"]:
                    fp.write(f"- `{item}`\n")
                fp.write("\n")

    # Keep a machine-generated, one-row-per-test inventory separate from the
    # curated high-value checklist. This makes the full Go baseline auditable
    # without pretending that filename-level matching proves behavior parity.
    inventory_path = "docs/history/go-to-rust/test-parity-inventory.md"
    manual_path = "docs/history/go-to-rust/manual-test-mappings.json"
    manual_details = {}
    if os.path.exists(manual_path):
        import json
        manual_details = json.load(open(manual_path, encoding="utf-8"))

    mapping_values = list(manual_details.values())
    partial_count = sum(item.get("status") == "[~]" for item in mapping_values)
    malformed_keys = [key for key in manual_details if key.count(":") < 2]
    if malformed_keys:
        raise ValueError(
            f"manual mappings must use file:line:test composite keys; found {len(malformed_keys)} legacy keys"
        )
    expected_keys = {f"{item['file']}:{item['line']}:{item['name']}" for item in go_tests}
    actual_keys = set(manual_details)
    missing_keys = expected_keys - actual_keys
    extra_keys = actual_keys - expected_keys
    if missing_keys or extra_keys:
        raise ValueError(
            f"manual mapping key set mismatch: missing={len(missing_keys)} extra={len(extra_keys)}"
        )
    invalid_evidence = [
        key for key, item in manual_details.items()
        if item.get("status") == "[x]" and item.get("evidence_type") != "function_exact"
    ]
    if invalid_evidence:
        raise ValueError(f"[x] mappings require evidence_type=function_exact; found {len(invalid_evidence)}")
    required_fields = {"status", "rust_entry", "conclusion", "command", "evidence_type"}
    malformed_records = [
        key for key, item in manual_details.items()
        if not required_fields.issubset(item)
        or item.get("status") not in {"[x]", "[~]"}
        or item.get("evidence_type") not in {"function_exact", "partial", "boundary", "missing", "module_only"}
    ]
    if malformed_records:
        raise ValueError(f"invalid parity mapping records: {len(malformed_records)}")
    invalid_commands = [
        key for key, item in manual_details.items()
        if re.search(r"-p\s+(jftrade-[A-Za-z0-9_-]+)", item.get("command", ""))
        and not _is_known_cargo_package(
            re.search(r"-p\s+(jftrade-[A-Za-z0-9_-]+)", item["command"]).group(1)
        )
    ]
    print(f"WARNING: {len(invalid_commands)} mappings reference nonexistent -p crates")
    # Reference integrity: an approval must point at a test that exists today,
    # otherwise a rename or deletion silently keeps it marked as covered.
    broken_approvals, stale_references = unresolved_parity_references(manual_details)
    if stale_references:
        print(
            f"WARNING: {len(stale_references)} partial mappings cite no resolvable Rust test "
            "(acknowledged gaps, not failures)"
        )
    # Cross-check the two halves of a mapping claim: the inventory says which
    # Rust test proves a reference test, and that Rust test's own
    # `// Parity:` anchor says the same thing from the code side. Only the
    # existence check above is load-bearing; this one is advisory until the
    # anchoring convention is universal.
    anchors = _parity_anchor_index()
    if not anchors:
        print(
            "WARNING: no // Parity: anchors were found, so the anchor cross-check was "
            "skipped (run the audit from the repository root)"
        )
    else:
        unanchored = unanchored_approvals(manual_details, anchors)
        if unanchored:
            print(
                f"WARNING: {len(unanchored)} function_exact mappings cite a Rust test with no "
                "// Parity: anchor for that baseline test (code and inventory claims differ)"
            )
    if broken_approvals:
        listing = "\n".join(f"  - {key}\n      {detail}" for key, detail in broken_approvals)
        raise ValueError(
            "[x]/function_exact mappings must cite a Rust test that exists in the "
            f"workspace; {len(broken_approvals)} reference(s) do not resolve:\n{listing}"
        )
    approved = sum(1 for item in mapping_values if item.get("evidence_type") == "function_exact")
    print(f"OK: {approved} function_exact mappings cite existing workspace tests")
    assertionless = assertionless_approved_references(manual_details)
    if assertionless:
        listing = "\n".join(f"  - {key}" for key, _entry in assertionless)
        print(
            f"WARNING: {len(assertionless)} function_exact mappings cite tests whose "
            "bodies contain no assertion (helper-based or empty); review manually:\n"
            f"{listing}"
        )

    exact_entries = [item.get("rust_entry") for item in mapping_values if item.get("status") == "[x]"]
    duplicate_entries = len(exact_entries) - len(set(exact_entries))
    if duplicate_entries:
        raise ValueError(f"[x] mappings must use unique Rust test entries; duplicate references={duplicate_entries}")
    generic_proofs = sum(
        item.get("status") == "[x]" and item.get("evidence_type") != "function_exact"
        for item in mapping_values
    )
    print(f"WARNING: {generic_proofs} [x] mappings lack function_exact evidence_type")
    # The report header is written above; append this evidence summary after loading
    # mappings so it cannot accidentally claim unverified rows are covered.

    def manual_for(test):
        """Resolve a mapping by immutable Go file, line, and test name."""
        key = f"{test['file']}:{test['line']}:{test['name']}"
        return manual_details.get(key)
    rust_by_domain = {key: [crate for crate in crates] for key, _, _, crates in DOMAIN_MAPPING}
    with open(inventory_path, "w", encoding="utf-8") as fp:
        fp.write("# Go → Rust 全量测试索引\n\n")
        fp.write("本文件由 `scripts/compatibility/audit_test_parity.py` 生成，是 Go→Rust 全量逐测试核对索引。映射源为 `manual-test-mappings.json`；自动推导的业务域和 crate 仅是候选，不能视为已覆盖。只有映射源中的真实入口、差异结论和验证命令才计入证据；无法迁移的测试必须记录边界/不适用原因。\n\n")
        # Precalculate status per test to display accurate progress in summary
        test_statuses = {}
        for test in go_tests:
            key = f"{test['file']}:{test['line']}"
            detail = manual_for(test)
            if detail:
                test_statuses[key] = detail["status"]
            else:
                test_statuses[key] = "[ ]"

        confirmed = sum(s in ("[x]", "[~]") for s in test_statuses.values())
        fp.write(f"当前进度：已确认 `{confirmed}` / `{len(go_tests)}`，待核对 `{len(go_tests) - confirmed}`。\n\n")
        by_domain = {}
        for test in go_tests:
            key = f"{test['file']}:{test['line']}"
            bucket = by_domain.setdefault(test["domain"], [0, 0])
            bucket[0] += 1
            bucket[1] += (test_statuses.get(key) in ("[x]", "[~]"))
        fp.write("| 业务域 | 已确认 | 待核对 |\n|---|---:|---:|\n")
        for domain in sorted(by_domain):
            total, done = by_domain[domain]
            fp.write(f"| {domain} | {done} | {total - done} |\n")
        fp.write("\n")
        fp.write("| 状态 | Go 测试 | 业务域 | 风险 | Rust crate 候选 | Rust 测试/入口 | 差异结论 | 验证命令 |\n|---|---|---|---|---|---|---|---|\n")
        for test in sorted(go_tests, key=lambda item: (item["domain"], item["file"], item["line"])):
            risk = "高风险" if test["high_risk"] else "普通边界"
            crates = ", ".join(rust_by_domain.get(test["domain"], []))
            if not crates and test["domain"] == "other":
                crates = ", ".join(OTHER_CANDIDATE_CRATES)
            crates = crates or "候选待验证"
            source = f"`go:{go_revision}:{test['file']}:{test['line']}`<br>`{test['name']}`"
            c = manual_for(test)
            if c:
                status = c["status"]
                rust_entry = c["rust_entry"]
                conclusion = c["conclusion"]
                command = c["command"]
            else:
                status = "[ ]"
                rust_entry, conclusion, command = "", "未建立 Rust 函数级证据", ""
            fp.write(f"| {status} | {source} | {test['domain']} | {risk} | `{crates}` | {rust_entry} | {conclusion} | {command} |\n")
        evidence_counts = {}
        for item in manual_details.values():
            evidence = item.get("evidence_type", "unspecified")
            evidence_counts[evidence] = evidence_counts.get(evidence, 0) + 1
        fp.write("\n## 4. 证据类型汇总\n\n")
        fp.write("清单勾选表示已处理，不表示功能等价；`function_exact` 表示断言等价，`partial` 表示仅部分断言由真实 Rust 测试验证。\n\n")
        fp.write("| evidence_type | 数量 |\n|---|---:|\n")
        for evidence, count in sorted(evidence_counts.items()):
            fp.write(f"| `{evidence}` | {count} |\n")
        missing_with_entry = sum(
            1 for item in manual_details.values()
            if item.get("evidence_type") == "missing" and item.get("rust_entry")
        )
        partial_with_entry = sum(
            1 for item in manual_details.values()
            if item.get("evidence_type") == "partial" and item.get("rust_entry")
        )
        fp.write(
            f"\n> `partial` 真实入口条目：{partial_with_entry}；`missing` 中仍带候选入口的条目：{missing_with_entry}；均不构成完整行为等价。\n"
        )

    print(f"\nReport written to {report_path}")
    print(f"Inventory written to {inventory_path}")

if __name__ == '__main__':
    main()
