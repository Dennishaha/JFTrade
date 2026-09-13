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
        fp.write(f"- **总体测试覆盖比率**：{total_ratio}\n")
        fp.write(f"- **Rust 基线（`{current_revision}`）**：当前工作树\n\n")
        fp.write("## 2. 分领域对齐矩阵\n\n")
        fp.write("| 业务领域 | Go 测试数 | Go 高风险数 | Rust 测试数 | 迁移比例 |\n")
        fp.write("| :--- | :--- | :--- | :--- | :--- |\n")
        for d_key, s in domain_stats.items():
            ratio = f"{(s['rust_total'] / s['go_total'] * 100):.1f}%" if s['go_total'] > 0 else "N/A"
            fp.write(f"| {s['label']} | {s['go_total']} | {s['go_high_risk']} | {s['rust_total']} | {ratio} |\n")
        
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
    rust_by_domain = {key: [crate for crate in crates] for key, _, _, crates in DOMAIN_MAPPING}
    with open(inventory_path, "w", encoding="utf-8") as fp:
        fp.write("# Go → Rust 全量测试索引\n\n")
        fp.write("本文件由 `scripts/compatibility/audit_test_parity.py` 生成；状态仅表示自动映射候选，行为等价性以高价值清单和回归证据为准。\n\n")
        fp.write("| 状态 | Go 测试 | 业务域 | 风险 | Rust crate 候选 |\n|---|---|---|---|---|\n")
        for test in sorted(go_tests, key=lambda item: (item["domain"], item["file"], item["line"])):
            risk = "高风险" if test["high_risk"] else "普通边界"
            crates = ", ".join(rust_by_domain.get(test["domain"], [])) or "待人工归类"
            source = f"`go:{go_revision}:{test['file']}:{test['line']}`<br>`{test['name']}`"
            fp.write(f"| [ ] | {source} | {test['domain']} | {risk} | `{crates}` |\n")

    print(f"\nReport written to {report_path}")
    print(f"Inventory written to {inventory_path}")

if __name__ == '__main__':
    main()
