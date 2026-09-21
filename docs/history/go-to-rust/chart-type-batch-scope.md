# 图表类型归一化批次（第 115 批）

本文件记录 Go 基线 `pkg/chart/chart_type_test.go` 的 1 条测试迁移到 Rust 的逐条核对结论。本批结论是
`function_exact`：Go 的表驱动四行逐条落到 Rust 两处等价实现上——backtest 起点归一化
（`crates/jftrade-engine/src/product_production_ports_backtest_parse.rs::with_normalized_chart_type`）
与 Pine worker 请求归一化（`crates/jftrade-integration-pine/src/execution.rs::normalize_chart_type`）。
本批新增 2 条测试，`missing` 3 → 2 中的第 2 条收口（余 `scripts/archive_frontend_assets_test.go:11`）。
两个被探测的生产文件按字节回滚，回滚后 shasum 与探测前一致。

## 第一百一十五批：pkg/chart 归一化收口（1 条）

### 范围与分片

Go 侧 1 文件 1 条：

- P2 `pkg/chart/chart_type_test.go:5`：`TestNormalizeChartType`
  （原 `missing`；表驱动 4 行：`""`→`standard`、`"standard"`→`standard`、
  `"  HEIKINASHI "`→`heikinashi`、`"renko"`→`standard`）。

分类：1 条 `[x]`/`function_exact`；全仓 `missing` 3 → 2（余
`scripts/archive_frontend_assets_test.go:11`）。

### 关键事实（本批 recon 与实测）

- **Go 机制**（`pkg/chart/chart_type.go`）：包注释声明它只承载“chart 领域共享契约，不依赖渲染或
  行情”。`ChartType` 是字符串枚举，仅两个取值 `standard` / `heikinashi`；
  `NormalizeChartType` 先 `strings.TrimSpace` 再 `strings.ToLower`，命中 `heikinashi` 返回该值，
  其余（空、遗留、未知）一律回退 `standard`。注释明确“it never changes the standard OHLC execution
  source”——图表类型不影响标准 OHLC 成交源。
- **Rust 机制（两处，语义等价、各自独立实现）**：
  - backtest 起点：`with_normalized_chart_type`（`product_production_ports_backtest_parse.rs:243`）
    在 `product_production_ports_backtest_task.rs:106` 的起点装配链中被调用，`None | Some(Null)`
    → `standard`、字符串 `trim().eq_ignore_ascii_case("heikinashi")` → `heikinashi`、其余字符串
    → `standard`、**非字符串** → `BacktestsWritePortError::BadRequest("chartType must be a string")`
    （400，不静默降级）。
  - Pine worker 入参：`normalize_chart_type`（`execution.rs:761`）在 `execution.rs:595` 填充
    `chart_type`；同一 trim + 大小写不敏感规则，返回 `String` 透传到 worker wire。
- **前端同规则第三份拷贝**：`apps/web/src/charting/kline.ts:46` 的 `normalizeChartType` 用
  `trim().toLowerCase()` 做同样判定，`chart.ChartType` 在 `apps/web/src/generated/openapi.ts:1417`
  冻结为 `"standard" | "heikinashi"`；worker 侧 `workers/pineworker/src/types.ts:10` 也是同一规则。
  本批把 Rust 的两处拷贝都钉进测试，防止两侧漂移。
- **wire 不变**：Rust 用字符串而非重新引入枚举层，HTTP/worker 契约与 `contracts/openapi` 保持冻结。

### 逐条结论

- `pkg/chart/chart_type_test.go:5` →
  `crates/jftrade-engine/src/product_production_ports_backtest_strategy_time_range_tests.rs::time_range_tests::chart_type_normalization_matches_the_pkg_chart_table`
  与
  `crates/jftrade-integration-pine/src/execution/tests.rs::worker_chart_type_normalization_matches_the_pkg_chart_table`：
  两条测试各自对同一张 4 行表断言（engine 侧断言归一化后的 `chartType` 字段，pine 侧断言 worker
  入参字符串），并补固化既有边界——缺省 / `null` → `standard`（对应 Go 的空值回退），非字符串
  （数字等）→ `BadRequest("chartType must be a string")`，不允许静默降级为 `standard`。
  两条测试均带 `// Parity: go:452dea11:pkg/chart/chart_type_test.go:5 TestNormalizeChartType` 锚。

### 探针记录（破 → 红 → 按字节回滚）

- 探针 ①：同时去掉两处实现的 `trim()`（`product_production_ports_backtest_parse.rs`
  与 `execution.rs`）→ 两条测试同时转红，`"  HEIKINASHI "` 得到 `standard`。
- 探针 ②：把非字符串分支由报错改成静默回退 `standard` → 边界断言转红
  （`non-string chartType must keep the parse boundary`）。
- 两次探针均按字节回滚，回滚后 shasum 与探测前一致：
  - `crates/jftrade-engine/src/product_production_ports_backtest_parse.rs`：
    `6cc3cf2d68f27243e1104441930cfb2c1c819405c5aa1aa59be69e428e6678b8`
  - `crates/jftrade-integration-pine/src/execution.rs`：
    `aef970479948dcccadc319d74a3217de3589377d90c4fe684a73cad6ce7f93ac`
  - `crates/jftrade-engine/src/product_production_ports_backtest_strategy_time_range_tests.rs`：
    `334722d2dbe4e387cde6ac5b6cd53e4c29da0c668ccf58a4bef9dd41ce4c1ee5`
  - `crates/jftrade-integration-pine/src/execution/tests.rs`：
    `d7c79ddbc2682c046d91872a8ae26b171d9b27aae8976e90b64e1a6115a73486`

### 保留差异

- **无单一共享契约 crate**：Go 的 `pkg/chart` 被多处 import，是唯一实现；Rust 没有对应的图表领域
  crate，同一归一化规则分布在 engine（backtest 起点）与 integration-pine（worker 入参）两处，本批
  用两条测试同时钉住两侧语义。若后续新增第三处调用点，需评估是否抽到共享 kernel/domain crate，
  而不是再复制一份。
- **非字符串入参更严格**：Go 侧签名是 `NormalizeChartType(value string)`，类型系统在编译期排除了
  非字符串；Rust 的 engine 入口面对 JSON `Value`，因此把“非字符串”显式判为 400 而不是照旧回退，
  这是比 Go 更强的失败关闭，已在测试中固化。
- **枚举层差异**：Go 用 `ChartType` 强类型承载两个取值；Rust 全程以字符串 `"standard"` /
  `"heikinashi"` 透传（API 响应体与 worker wire），未新增枚举层，wire 契约不变。
- **前端与 worker 的同类归一化不在本行断言范围**：`apps/web/src/charting/kline.ts`、
  `workers/pineworker/src/types.ts` 各自重复同一 trim + 小写规则，本批只覆盖 Rust 两侧；其对齐
  证据由前端/worker 各自测试域承担。

### 后续待办

- 下一批（第 116 批）：`scripts/archive_frontend_assets_test.go:11`
  （`TestArchiveFrontendAssetsPreservesRelativePathsAndTimestamp`，`missing` 2 → 1 → 0 的最后一条）。
  完成后全仓 `missing` 归零，转入 2544 条 `partial` 与 573 条 `boundary` 的逐域收口。
- 本批登记的独立项（承第 114 批）：为 Rust 桌面补“周期性更新检查”（对应 Go
  `startDesktopUpdateChecks` 的 24 小时节奏），复用同一签名 updater 与
  `jftrade:desktop-update:available` 事件，需要可注入时钟/间隔的调度接缝。
- 既有独立项：审计脚本生成的 markdown 含尖括号占位符，`build:docs:generated` 在
  markdown-it/Vue 判为未闭合 HTML 标签处失败（与本批无关）。

验证：
- `cargo fmt --all --check` EXIT=0；`cargo clippy -p jftrade-engine -p jftrade-integration-pine
  --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-pine
  --all-targets --locked --no-fail-fast`：1801 passed / 1 skipped / 0 failed；
  单独 `-p jftrade-integration-pine`：40 passed / 1 skipped / 0 failed；
  定向 `-E 'test(worker_chart_type_normalization_matches_the_pkg_chart_table)'` 1 passed。
- 探针转红并已按字节回滚（去 trim、非字符串静默回退），回滚后 4 个文件 shasum 与探测前一致。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3067 Rust、`[x]`
  1327 → 1329、`missing` 3 → 1、0 破坏引用；未锚定 function_exact 202（既有基线）、
  partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1330 唯一引用（已记账 1275、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:compatibility` EXIT=0（provider-runtime 14 market-data / 9 Pine lifecycle /
  3 OpenD subscriptions / 3 health probes；trading-strategy 10 statuses / 7 transitions /
  6 command plans / 7 update events / 5 position refreshes / 3 strategy scenarios；assistant-runtime
  9 statuses / 12 transitions / 3 rejected input prompts / 2 durable claims / 3 workflow tasks /
  2 artifact versions / 3 stream deltas；api-transport 278 OpenAPI operations / 18 route groups /
  19 concrete route probes；desktop-runtime 3 platform profiles / 6 link cases / 10 facade
  commands / 4 events）。
- `node scripts/check-zero-go.mjs` EXIT=0（2928 tracked files）；`pnpm run check:ai-context` EXIT=0
  （6 modules / 8 instruction files）；`git diff --check` 干净。
- `pnpm run check:quick`：首轮在 `check:rust:test` 阶段因**与本批无关的信号竞态**失败——
  `api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 在 freshly-clean
  构建的负载下判 `left: None / right: Some(0)`（“operator stop 必须走 shutdown 路径而不是杀 sidecar”）；
  隔离重跑 3/3 通过，随后完整重跑 `check:quick` EXIT=0（含 pineworker 10 files / 98 tests），
  本批未触碰 launcher 代码。首轮 `check:rust:target-health` 曾报 `.rcgu.o ≥ 50000`，确认无 Cargo
  进程后 `pnpm run clean:rust:artifacts`（Removed 108451 files / 31.5GiB）并复跑通过。
- `pnpm run check:rust` EXIT=1：`check:rust:target-health` / `:architecture` / `:production-policy`
  均通过，唯一失败阶段 `check:rust:policy`（`cargo deny`）报 RUSTSEC-2026-0285 与陈旧 advisory
  告警，与本仓既有基线同源。**不记为通过**；等价测试面由 `check:quick` 内的 nextest 结果覆盖。
