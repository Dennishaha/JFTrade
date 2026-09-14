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
        ["internal/marketdata", "pkg/market", "internal/productfeatures/marketdata"],
        ["crates/jftrade-marketdata", "crates/jftrade-integration-marketdata-helper"]
    ),
    (
        "trading_broker",
        "Trading & Broker Execution",
        ["pkg/broker", "pkg/trading", "internal/trading", "internal/app/trading"],
        ["crates/jftrade-trading", "crates/jftrade-broker"]
    ),
    (
        "strategy_pine",
        "Strategy & Pine Runtime",
        ["pkg/strategy", "pkg/pine", "internal/strategy", "internal/pine"],
        ["crates/jftrade-strategy", "crates/jftrade-integration-pine"]
    ),
    (
        "backtest_calendar",
        "Backtest & Exchange Calendar",
        ["pkg/backtest", "pkg/market/calendar", "internal/backtest"],
        ["crates/jftrade-backtest", "crates/jftrade-calendar"]
    ),
    (
        "assistant_workflow",
        "Assistant & Workflow ADK",
        ["internal/assistant", "pkg/adk", "internal/adk", "internal/workflow"],
        ["crates/jftrade-assistant"]
    ),
    (
        "storage_sqlite",
        "Storage & SQLite Persistence",
        ["pkg/database", "pkg/storage", "internal/store", "internal/database", "internal/storage"],
        ["crates/jftrade-store-sqlite", "crates/jftrade-store-settings-file", "crates/jftrade-owner-lock"]
    ),
    (
        "settings_watchlist",
        "Settings & Watchlist",
        ["pkg/settings", "internal/settings", "internal/watchlist", "pkg/watchlist"],
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
            
            # assign domain
            domain = "other"
            for d_key, _, prefixes, _ in DOMAIN_MAPPING:
                if any(file_path.startswith(p) for p in prefixes):
                    domain = d_key
                    break
            
            is_high_risk = any(kw in test_name.lower() or kw in file_path.lower() for kw in HIGH_RISK_KEYWORDS)
            
            tests.append({
                "file": file_path,
                "line": line_num,
                "name": test_name,
                "domain": domain,
                "high_risk": is_high_risk
            })
    return tests

def extract_rust_tests():
    tests = []
    test_pattern = re.compile(r'#\[(?:tokio::)?test\](?:\s*#\[[^\]]+\])*\s*(?:pub(?:\([^\)]+\))?\s+)?(?:async\s+)?fn\s+([a-zA-Z0-9_]+)')
    
    for f in glob.glob('crates/**/*.rs', recursive=True):
        crate_name = f.split('/')[1]
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
        and not os.path.isdir("crates/" + re.search(r"-p\s+(jftrade-[A-Za-z0-9_-]+)", item["command"]).group(1))
    ]
    print(f"WARNING: {len(invalid_commands)} mappings reference nonexistent -p crates")
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
