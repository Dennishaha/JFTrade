# Go 到 Rust 迁移历史

本目录保存迁移记录、行为审计、执行手册和逐路由 ledger；其中的历史结论不等同于当前完成声明，也不参与当前架构、路由所有权、门禁计划或发布资格计算。

当前产品事实以 [`../../architecture.md`](../../architecture.md)、[`../../architecture/quality-gates.md`](../../architecture/quality-gates.md) 和 [`../../architecture/release-qualification.md`](../../architecture/release-qualification.md) 为准。

相关深度审计与验证矩阵：

- [`2026-09-09-project-parity-audit.md`](2026-09-09-project-parity-audit.md)：固定 Go/main 基线的全项目能力盘点、F01–F05 修复后证据，以及 G01–G07 的限定验证；G05 真实旧发布包/四平台安装升级和 G08 live 验收仍未验证，审计中的结论不等同于迁移已完成或发布资格声明。
- [`2026-09-06-behavior-audit.md`](2026-09-06-behavior-audit.md)：本轮远端 Go / 本地 main 对比、已复现修复、实际验收范围与未闭环差异。
- [`go_to_rust_comprehensive_verification_matrix.md`](go_to_rust_comprehensive_verification_matrix.md)：Go 到 Rust 迁移全景深度验证矩阵与发布准入总览（主导航索引）
- [`high-value-test-mapping-checklist.md`](high-value-test-mapping-checklist.md)：高风险 Go 测试到 Rust 行为的样本核对清单（非全量）
- [`test-parity-inventory.md`](test-parity-inventory.md)：全量 Go 测试逐项勾选清单；每项都必须人工确认 Rust 测试映射，自动生成的 crate 候选仅作起点
- [`parity-progress-summary.md`](parity-progress-summary.md)：当前数量、已完成核对、真实功能修复、未闭环缺口和单队列调度规则摘要
- Watchlist/Futu 相关 `[~]` 项已按远程 reader、批量 snapshot、订阅配额和 session quote projection 分解，待对应 Rust adapter/port 建立后统一补测。
- [`verification-matrix/`](verification-matrix/)：十大核心领域代码级对比、边界失效推演与测试用例分卷目录

## 迁移期审计工具（临时）

`scripts/compatibility/` 下的 Python 三件套是迁移期人工审计工具，不是永久产品门禁：

- `audit_test_parity.py`：扫描冻结 Go 基线与工作树 Rust 测试，再生成 `test-parity-report.md` / `test-parity-inventory.md`；强制 `[x]` 条目引用真实存在且唯一的 Rust 测试函数，并对引用无断言测试的 `function_exact` 条目给出复核告警。
- `parity_anchor_reconcile.py`：对账 Rust 代码中的 `// Parity:` 锚点与清单（只读，不改清单）。
- `parity_gap_triage.py`：把 `missing` 条目保守三分类并给出名称候选（只读，候选不得直接升级为证据）。

三件套的单元测试经 `test:scripts -- compatibility` 接入 `check:policy`，每层门禁都会执行，防止脚本在两次人工批次之间腐化；但审计结论本身仍不构成门禁通过条件（门禁不使用迁移阶段作为调度或通过条件）。退役条件：迁移宣告完成（`missing` / `partial` 清零或全部转为确认的 `boundary`）后，三件套与本目录产物一并归档，不再维护。

## 批次执行约定（迁移期）

逐批对齐的调度方（自动化 / 人工）按本节执行，不再把约定复制进调度 prompt：

- **基线**：行为基线是冻结的 `go:452dea11`；Go 工具链为 go1.26.6，命令形如 `GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./<pkg>/ -count=1`，先取整包基线再按文件 `-run` 过滤。
- **映射源**：`manual-test-mappings.json`（key 为 `file:line:TestName`，当前 4451 条）。写回用 Python `json.load` → 改本批 key → `json.dumps(ensure_ascii=False, indent=2)` + 换行；写完复核 key 数仍为 4451、无 U+FFFD、本批净增 `[x]` 用集合差确认。
- **证据口径**：`[x]` 必须 `evidence_type=function_exact`，`rust_entry` 能被 `audit_test_parity.py` 解析到真实 `#[test]`/`#[tokio::test]` 且全局唯一；其余用 `partial`/`boundary`/`module_only`/`missing` 并写清“Rust 已覆盖什么 + 差异为何”。新 `[x]` 顺手补 `// Parity:` 锚点；不能只按文件名或测试名相似就判等价。
- **先红后改**：真实差异先写复现测试，再改所属领域 crate；用探针（改坏实现 → 跑测试确认转红 → 按字节回滚）证明测试是真实守卫，探针结论写进批次小结。
- **批次小结**：写入对应领域的 `*-batch-scope.md`（助理/ADK 用 `assistant-workflow-adk-batch-scope.md`，Futu/OpenD 用 `futu-opend-batch-scope.md`，其余领域按 `system-status-`、`productfeatures-`、`observability-batch-scope.md` 这类领域名新建或追加），最后一个小节是本批、末行是“验证：”。
- **锚点例外**：退休包路径 `internal/frontendassets`、`internal/marketdataassets`、`internal/pineworkerassets` 不得出现在活动根
  （`.github/`、`apps/`、`crates/`、`scripts/`、`workers/`）的文本中（`check-zero-go` 会失败）。这类 Go 测试的 `[x]`
  只保留 `manual-test-mappings.json` 的 `rust_entry` 与批次小结证据表，代码侧不写 `// Parity:` 锚点；审计“无锚点”告警按批次登记增量。
- **验证**：`cargo fmt --all`、`cargo clippy -p <changed-crates> --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p <changed-crates> --all-targets --locked --no-fail-fast`、`python3 scripts/compatibility/audit_test_parity.py`、`python3.12 scripts/compatibility/parity_anchor_reconcile.py`、`pnpm run check:compatibility`、`pnpm run check:zero-go`、`pnpm run check:rust:architecture`、`pnpm run check:ai-context`、`git diff --check`，最后 `pnpm run check:quick` 与 `pnpm run check:rust`。
  - `check:rust:target-health` 报 `target/debug/deps` 超过 5 万个 `.rcgu.o` 时：确认无 cargo/rustc 在跑，再 `pnpm run clean:rust:artifacts`（`cargo clean`）或删除该目录下的 `*.rcgu.o`。
  - `check:rust:static` 的 advisories 阶段在干净 HEAD 上同样失败（既有 RUSTSEC-2026-0285 / rustls 0.23.44）；不要改 `Cargo.lock` 规避，如实记录。
  - `check:zero-go` 会拦截源码注释里相邻出现的 `go test`/`go build` 字样与已退役资产目录名，英文注释写 “reference fixture” 之类措辞。
  - 工具输出通道会对 `webhookSecret`/`apiKey`/`token`/`[FUNC]`/`unauthorized` 等字面量做脱敏改写；核对源码真实字节用 `python repr()`，不要在 `apply_patch` 里直接抄显示文本。
- **其他长期约束**：新增 crate 必须登记 `scripts/quality/workspace-architecture-policy.json`，并加入根 `Cargo.toml` members 与 engine allowlist；fixture 的 `InitConnect` 必须 `serverVer=1009`；并发测试用 Gate/Condvar 不用 sleep 且线程必须 join；网络夹具在 `server.join()` 前 `drop(reader)` / `session.close()`（用 `pkill -f cargo-nextest` 清理），同步 reqwest 夹具用 `StdTcpListener` + 独立 `std::thread`；私有 `mod tests` 不能被其他模块 import；不要用 `git checkout --` 撤销生产改动；Go 行为即使看起来是 bug 也要冻结并记成 go-behavior quirk。
- **提交纪律**：按批次一次提交，提交前 `git status --short` 只含本批文件，审计产物 `test-parity-report.md`/`test-parity-inventory.md` 一并提交；每批结束后更新调度 prompt 与线程 goal 到下一批目标。
