# Market 规则、Session 与日历领域对齐批次

本文件记录 Go 基线 `pkg/market`（市场归一化、交易时段、交易日历）迁移到 Rust
`jftrade-marketdata`（市场规则 SSOT 与仪器归一化）、`jftrade-calendar`
（交易日历、session 窗口、内置规则）与 `jftrade-engine`（会话分类投影）的
逐测试核对结论。

## 第九十批：`pkg/market` 全域收口（56 条）

### 范围与分片

`pkg/market/**` 剩余 **56 条 `missing`**（14 个文件），分三片：
P0 仪器/市场归一化与 session 校验 24 条（`market_normalization_test.go` 11、
`market_test.go` 11、`instrument_session_validation_test.go` 2）→ P1 交易日历与会话窗口
23 条（`calendar/builtin_test.go` 5、`calendar/helpers_boundaries_test.go` 5、
`session_window_test.go` 5、`session_boundaries_test.go` 4、`calendar/calendar_boundaries_test.go` 2、
`calendar/types_json_test.go` 1、`session_calendar_refresh_contract_test.go` 1）→
P2 分市场常量 9 条（`us/us_test.go` 3、`hk/hk_test.go` 2、`sh/sh_test.go` 2、`sz/sz_test.go` 2）。

### 结果

- 56 条全部给出结论：**1 条 `[x]`/`function_exact` + 52 条 `partial` + 3 条 `boundary`**。
- `pkg/market/**` 归零：56 条 = **1 `[x]` + 52 `partial` + 3 `boundary`，0 `missing`**。
- 全局：4451 = function_exact **1218** + partial **2420** + boundary **532** + module_only 4 +
  missing **277**（前批为 1217 / 2368 / 529 / 4 / 333），Rust 测试 2932 → **2933**。

### 新增 `[x]` 锚点

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `calendar/types_json_test.go:10 TestTradingDayScheduleJSONOmitsZeroUpdatedAt` | `jftrade-calendar/src/snapshot.rs::trading_day_schedule_json_omits_zero_updated_at` | `[x]`：零值 `updatedAt` 从 `TradingDaySchedule` JSON 省略、非零值输出 RFC3339 `2026-06-23T09:30:00Z`，与 Go 两条断言一致；本批新增回归测试与 Parity 锚点。 |

### 保留差异候选（保持 `partial` 的理由）

- **P0 市场归一化**：`ParseInstrument`/`ParseQualifiedInstrumentSymbol` 的市场匹配矩阵
  （CN/CNSH 接受 SH 限定符、CNSZ 拒绝）、SG 别名、`FormatProfile` 输出、全局
  `SwapCalendarResolver/Reset` 生命周期；Rust 以 `normalize_instrument` +
  不可变日历管理器实现，没有全局解析器与同名格式化函数。
- **P0 交易分钟与标签**：`TradingMinutesPerDay/TradingMinutesPerTradingDay`、
  `TradingDayKey`、`TradingPeriodKey`、`TradingPeriodLabelStart*`、
  `TradingDayBoundaryStart`、`SessionAwareIntradayBucketBounds` 这些 Go 辅助函数
  Rust 未提供同名 API，等价语义由 session context 的窗口/交易日字段与
  candle completion 的权威收盘边界承担。
- **P1 日历边界**：圣诞夜模板副本语义、跨年/月末内置回退、快照 helper 的
  from/to 覆盖窗口、覆盖率 98% 的延长窗口在刷新移除分类窗口后 fail-closed。
- **P2 分市场常量**：Go 断言 `us.Location()` 等 `*time.Location` 对象与
  `RegularWindows` 结构，Rust 以 `timezone` 字符串 + 规则表 + session context 表达。
- **已登记的 go-behavior quirk（不判等价）**：Go 对 `CN.600519` 与 `market=CN`+裸码返回
  “requires an exchange-qualified symbol”；Rust 明确按代码前缀推断 SH/SZ
  （`catalog_tests.rs` 注释标注为有意的产品边界改进），本批按差异保留为 partial。
- **boundary（3 条）**：`hk/sh/sz` 的 `TestLoadLocationFallsBackToUTC` —— Go 在时区库缺失时
  静默回退 UTC；Rust 使用 `jiff::tz::TimeZone::get` 并在失败时返回错误（fail-closed），
  保留该边界差异，不迁就静默降级。

验证：`cargo fmt --all`；`cargo clippy -p jftrade-calendar -p jftrade-marketdata --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar -p jftrade-marketdata -p jftrade-engine --all-targets --locked --no-fail-fast`（**1868 passed / 0 skipped**）；`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2933 Rust** / **1218 `[x]`**；missing 277、partial 2420、boundary 532、module_only 4；`OK: 1218 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 193、7 条 partial 无解析引用、2 条无断言为前批基线告警）；`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0）；`node scripts/check-zero-go.mjs`（2892 tracked files / 0 release artifact）；`pnpm run check:ai-context`；`git diff --check`。

`pnpm run check:quick` **未通过（EXIT=1）**，唯一失败项不在本批 diff 范围内，保留原始证据不记为通过：
`pnpm run check:rust:static` 的 `cargo-deny` advisories 仍因 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45）失败；
本批未改 `Cargo.lock`（根 `Cargo.toml` 精确锁定 `=0.23.44`，解除需一次显式的依赖升级批次并复核 `deny.toml` 许可例外）。
同一次 `check:quick` 中 `pnpm run check:rust:workspace` **3050 passed / 2 skipped**、
`check:rust:target-health` 通过，`check:web` 与 `check:python` 均通过。
