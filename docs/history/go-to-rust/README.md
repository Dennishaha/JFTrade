# Go 到 Rust 迁移历史

本目录保存迁移记录、行为审计、执行手册和逐路由 ledger；其中的历史结论不等同于当前完成声明，也不参与当前架构、路由所有权、门禁计划或发布资格计算。

当前产品事实以 [`../../architecture.md`](../../architecture.md)、[`../../architecture/quality-gates.md`](../../architecture/quality-gates.md) 和 [`../../architecture/release-qualification.md`](../../architecture/release-qualification.md) 为准。

相关深度审计与验证矩阵：

- [`2026-09-09-project-parity-audit.md`](2026-09-09-project-parity-audit.md)：固定 Go/main 基线的全项目能力盘点、F01–F05 修复后证据，以及 G01–G07 的限定验证；G05 真实旧发布包/四平台安装升级和 G08 live 验收仍未验证，审计中的结论不等同于迁移已完成或发布资格声明。
- [`2026-09-06-behavior-audit.md`](2026-09-06-behavior-audit.md)：本轮远端 Go / 本地 main 对比、已复现修复、实际验收范围与未闭环差异。
- [`go_to_rust_comprehensive_verification_matrix.md`](go_to_rust_comprehensive_verification_matrix.md)：Go 到 Rust 迁移全景深度验证矩阵与发布准入总览（主导航索引）
- [`high-value-test-mapping-checklist.md`](high-value-test-mapping-checklist.md)：高风险 Go 测试到 Rust 行为的样本核对清单（非全量）
- [`test-parity-inventory.md`](test-parity-inventory.md)：全量 Go 测试逐项勾选清单；每项都必须人工确认 Rust 测试映射，自动生成的 crate 候选仅作起点
- Watchlist/Futu 相关 `[~]` 项已按远程 reader、批量 snapshot、订阅配额和 session quote projection 分解，待对应 Rust adapter/port 建立后统一补测。
- [`verification-matrix/`](verification-matrix/)：十大核心领域代码级对比、边界失效推演与测试用例分卷目录

## 迁移期审计工具（临时）

`scripts/compatibility/` 下的 Python 三件套是迁移期人工审计工具，不是永久产品门禁：

- `audit_test_parity.py`：扫描冻结 Go 基线与工作树 Rust 测试，再生成 `test-parity-report.md` / `test-parity-inventory.md`；强制 `[x]` 条目引用真实存在且唯一的 Rust 测试函数，并对引用无断言测试的 `function_exact` 条目给出复核告警。
- `parity_anchor_reconcile.py`：对账 Rust 代码中的 `// Parity:` 锚点与清单（只读，不改清单）。
- `parity_gap_triage.py`：把 `missing` 条目保守三分类并给出名称候选（只读，候选不得直接升级为证据）。

三件套的单元测试经 `test:scripts -- compatibility` 接入 `check:policy`，每层门禁都会执行，防止脚本在两次人工批次之间腐化；但审计结论本身仍不构成门禁通过条件（门禁不使用迁移阶段作为调度或通过条件）。退役条件：迁移宣告完成（`missing` / `partial` 清零或全部转为确认的 `boundary`）后，三件套与本目录产物一并归档，不再维护。
