# Backtest、Calendar 领域对齐批次

本批逐项核对 Backtest/Calendar 的分页、失败恢复、取消生命周期与 DST 边界，
共 7 条 Go 复合键：历史 K 线空结果/分页/字段转换 3 条、Calendar 市场本地年
跨 UTC 新年 1 条、SyncTask 取消 1 条，以及 backtest run 删除/in-memory
生命周期 2 条。

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `internal/backtest/historical_source_test.go:181` | `product_production_ports_backtest_sync_helpers::test_historical_k_line_syncer_rejects_empty_provider_result` | `[~]`/`partial`：Rust 是 helper 空页校验，缺真实 Sync 错误文案与 store/provider 语义 |
| `internal/backtest/historical_source_test.go:268` | `...::test_historical_k_line_syncer_rejects_broken_pagination` | `[~]`/`partial`：缺 missing/forward/boundary cursor 错误矩阵 |
| `internal/backtest/historical_source_test.go:309` | `...::test_historical_candle_conversion_rejects_invalid_fields_and_defaults_volume` | `[~]`/`partial`：缺窗口过滤、默认 volume、五类非法字段及 extended session 展开 |
| `internal/exchangecalendar/manager_test.go:876` | `fetch_window_timezone::probe_uses_market_local_year_when_us_crosses_utc_new_year` | `[x]`/`function_exact`：固定 US 本地年与 EST 窗口切换 |
| `internal/store/backtest/sync_tasks_test.go:12` | `backtest_sync_task_store_contracts::sync_task_cancel_distinguishes_missing_active_and_terminal` | `[~]`/`partial`：缺快照副本、busy reason、取消回调、UpdatedAt |
| `internal/store/backtest/store_failure_test.go:121` | `backtest_run_store_contracts::test_store_canceled_maintenance_does_not_mutate_runs` | `[~]`/`partial`：仅覆盖不存在删除不变，缺取消 maintenance context |
| `internal/store/backtest/store_test.go:168` | `backtest_run_store_contracts::test_in_memory_store_implements_run_lifecycle_and_cancellation` | `[~]`/`partial`：仅覆盖取消回调，缺完整 run lifecycle 与缺失任务边界 |

验证命令按测试逐条记录于 `manual-test-mappings.json`。本批未将任何 helper
级或内存级部分证据升级为完整等价；下一步应优先补齐 HistoricalKLineSyncer
retry/cancel 和 SyncTask 快照隔离测试。

## 第八十批：`pkg/backtest` 全域收尾（237 条，回测域归零）

### 范围与结果

- 范围：`pkg/backtest` 剩余 237 条 `missing`（45 个文件），按 P0（内部存储不变量、失败恢复、
  连接/租约语义、进度与编解码边界）→ P1（pineworker 命令执行/适配/重放、session 分页与聚合读源）→
  P2（保守 bar 执行器、交易成本、结果收集与统计、shadow 参考实现）顺序分 5 片逐条核对。
- 结果：`missing` 1752 → **1515**（本批结清 237 条）、`partial` 1147 → **1337**、`boundary` 352 → **397**、
  `[x]` 1196 → **1198**。本批 237 条 = **2 `[x]` + 190 partial + 45 boundary**。
- **`pkg/backtest/**` 全域归零**：域内 237 条 = **2 `[x]` + 190 partial + 45 boundary，0 `missing`**。

### 分片执行

- **P0（36 条）**：`internal/storage` 的 `store_runtime_invariants` 14、`codec_progress_boundaries` 6、
  `store_failure_boundaries` 5、`store_connection` 4、`codec_test` 2、`query_failure_empty_recovery` 2、
  `stream_query_failure_sorting` 2、`aggregate_corruption_errors` 1。证据面为
  `crates/jftrade-store-sqlite/tests/{backtest_market_data_aggregation,backtest_market_data_failure_recovery,backtest_market_data_session_scope,backtest_market_data_session_dst_aggregation,backtest_sync_task_store_contracts,maintenance_backup_and_rebuild_contracts}.rs`
  与 `crates/jftrade-engine/src/product_production_ports_backtest_sync_helpers.rs`。
- **P1a（61 条）**：pineworker 适配/命令执行/重放/来源/运行器与原子边界。证据面为
  `crates/jftrade-integration-pine/src/{backtest_tests.rs,execution/tests.rs}`、
  `crates/jftrade-integration-pine/tests/pine_backtest_matcher_intents_e2e.rs`、
  `crates/jftrade-engine/src/strategy_runtime_execution_tests.rs`、
  `crates/jftrade-backtest/tests/conservative_bar_execution.rs` 与
  `crates/jftrade-broker/tests/market_rules_snapshot_errors.rs`。
- **P1b（38 条）**：运行器失败边界 + session 分页/合成/聚合读源。证据面为
  `backtest_market_data_session_scope.rs`、`backtest_market_data_session_dst_aggregation.rs`、
  `backtest_market_data_calendar_aggregation.rs`、`backtest_market_data_aggregation.rs`。
- **P2a（45 条）**：保守 bar 执行器 19、`store_test` 14、`store_business_aggregation` 6、
  `store_query_aggregation_contracts` 3、`sync_progress` 1、`runner_hardcut` 1、`runner_helpers` 1。
- **P2b（57 条）**：结果收集/交易统计、交易成本、回放 sizer、短回放、来源市场、shadow 参考与语料/冒烟。
  证据面为 `crates/jftrade-backtest/tests/{trading_cost_behavior,result_reporting_behavior,execution_cost_result_boundaries,backtest_compatibility,pine_indicator_compatibility,conservative_bar_execution}.rs`、
  `crates/jftrade-backtest/src/fees.rs`、`crates/jftrade-integration-pine/tests/real_worker_smoke.rs`。

### 新增批准项（2 `[x]`，新增 `// Parity:` 锚点）

- `pkg/backtest/pine_ts_shadow_reference_test.go:8 TestPinetsShadowEMAUsesSMAInitialization` →
  `crates/jftrade-backtest/tests/pine_indicator_compatibility.rs::pine_ema_seeds_from_sma_after_nan_warmup`：
  同输入 `[1,2,3,4,5]`/period 3 下，前两个值 NaN、其后 `2/3/4` 完全一致。
- `pkg/backtest/pine_ts_shadow_reference_test.go:21 TestPinetsShadowMACDSkipsNaNValuesForSignalInitialization` →
  `...::pine_macd_delays_signal_until_valid_macd_values_exist`：macd[..4] 与 signal/hist[..5] 为 NaN、
  索引 4 起 macd=1、索引 5/6 为 1/1/0，与 Go 断言逐项一致。

### 新增缺口登记（保留，均为 P2）

- **回测结果的订单观测面**：`conservative_bar_execution.rs` 中 16 条用例已带 `// Parity:` 锚点且成交明细
  （分笔数量/价格）与终态资金/持仓与 Go 一致，但 Rust 结果模型只导出终态订单行，没有 Go stream 的
  `PARTIALLY_FILLED → FILLED` 逐次订单状态更新，因此本批按 partial 记录；若需与 Go 观测面完全一致，
  属公开结果契约变更，需单独确认后补 `result.view` 订单状态序列。
- **运行期错误计数与样本**：Go `internal/runmodel` 导出 runtime error 计数与样本上限/去重；Rust 结果模型
  只有 `warnings`，无 `runtimeErrors` 聚合。
- **会话过滤包装层与流式 API**：Go 的 session-filtered store（Verify/Sync 委托、通道流式读取、自定义
  扩展时段区间路由、多标的稳定排序）在 Rust 不存在——会话作用域是存储参数、查询一次性返回单标的结果。
- **港股/大陆会话聚合断言缺失（测试缺口，非功能缺口）**：Rust 已实现 HK `570-720/780-960` 与 CN 时段
  窗口，但 store 会话聚合语料只覆盖美股；2 小时与日线跨港股午休、跨港股交易时段等 Go 断言无 Rust 覆盖，
  需要补美股以外的会话聚合回归用例。
- **费用规则生效日期**：Go 费用规则支持生效日期闭区间；Rust 费用规则模型没有该字段（规则按市场/合约
  静态生效）。
- **命令/重放中间层**：Go 的 command 转换、replay planner、replay pump、短回放委托层在 Rust 不存在
  （worker 意图直接经执行端口/确定性撮合执行）；报价货币默认表与初始资金优先级链无对应断言。
- **真实 PineTS 冒烟**：Go 的 shadow 语料报告与真实 PineTS 冒烟在 Rust 分别由冻结语料回放与 opt-in 的
  `real_worker_smoke::rust_client_executes_bundled_pinets_worker` 承担，默认门禁不跑外部 worker。

### 跨批 follow-up 汇总

- P0 无新增。P1 候选 1 条：回测结果订单状态观测面（是否需要与 Go stream 对齐，取决于结果契约决策）。
- P2 = 前批清单 + 本批登记项：运行期错误聚合、会话流式/多标的排序、港股与大陆会话聚合断言、费用规则
  生效日期、报价货币与初始资金优先级断言。

### 仍未结清（下一批）

- 下一批（第八十一批）范围：按域余量排序的下一块 **`internal/store` 206 条**，先按文件分组 recon 再按
  P0 → P1 → P2 分片；其后：`internal/api` 180、`internal/strategy` 169、`pkg/bbgo` 145、
  `internal/integration` 141、`internal/marketdata` 112、`pkg/futu` 86、`internal/trading` 80、
  `internal/backtest` 63、`pkg/market` 56、`internal/marketdataassets` 36，直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-backtest -p jftrade-store-sqlite -p jftrade-integration-pine -p jftrade-engine -p jftrade-broker -p jftrade-strategy --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-backtest -p jftrade-store-sqlite -p jftrade-integration-pine -p jftrade-engine -p jftrade-broker -p jftrade-strategy --all-targets --locked --no-fail-fast`（**2037 passed / 1 skipped**）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1198 `[x]`**；missing 1515、partial 1337、boundary 397、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）、`pnpm run check:compatibility`、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（98 passed）、`pnpm run check:ai-context`。

## 第八十九批：`internal/backtest` 全域收口（63 条）

### 范围与分片

`internal/backtest/**` 本批处理剩余 **63 条 `missing`**（11 个文件），分三片：
P0 运行生命周期与恢复 22 条（`service_test.go` 15、`run_failure_recovery_test.go` 3、
`recovery_test.go` 2、`service_pineworker_test.go` 2）→ P1 分页/同步/重试与时间边界 21 条
（`sync_test.go` 12、`historical_source_test.go` 6、`time_test.go` 3）→ P2 输入与结果视图 20 条
（`input_and_readiness_validation_test.go` 10、`result_view_test.go` 6、
`result_view_aggregation_test.go` 2、`business_test.go` 2）。

### 结果

- 63 条全部给出结论：**6 条 `[x]`/`function_exact` + 57 条 `partial`**。
- `internal/backtest/**` 归零：67 条 = **7 `[x]` + 60 `partial`，0 `missing`**。
- 全局：4451 = function_exact **1217** + partial **2368** + boundary 529 + module_only 4 +
  missing **333**（前批为 1211 / 2311 / 529 / 4 / 396），Rust 测试 2927 → **2932**。

### 新增 `[x]` 锚点

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `service_test.go:196 TestPrepareResolvedBacktestNormalizesChartType` | `product_production_ports_backtest_strategy_time_range_tests::backtest_start_normalizes_chart_type` | `[x]`：空/未知(renko)→standard、HeikinAshi→heikinashi；Start 持久化前新增 `with_normalized_chart_type` 归一化（此前 Rust 原样透传）。 |
| `sync_test.go:367 TestPlanSyncIntervals` | `product_backtest_sync_start_tests::sync_request_plans_intervals_like_go` | `[x]`：四条用例逐项一致；修复 US extended 下 `3d`/`2w` 未再降级为 `1h` 的差异。 |
| `sync_test.go:412 TestParseSessionScope` | `product_backtest_sync_start_tests::sync_request_session_scope_parity_with_go` | `[x]`：放行/拒绝集合与 Go 一致（既有 Parity 锚点）。 |
| `time_test.go:8 TestResolveBacktestTimeRangeUsesMarketDateAndDST` | `..._time_range_tests::backtest_start_resolves_market_dates_with_dst` | `[x]`：US 市场日按本地午夜解析（DST 日 23h），startDate/endDate 标签与 `America/New_York` 归一化。 |
| `time_test.go:33 TestResolveBacktestTimeRangeUsesHongKongCalendarDay` | `..._time_range_tests::backtest_start_resolves_hong_kong_calendar_day` | `[x]`：HK 市场日 16:00Z 起点与 `Asia/Hong_Kong` 归一化。 |
| `time_test.go:55 TestResolveBacktestTimeRangeNormalizesLegacyTimestamps` | `..._time_range_tests::backtest_start_normalizes_legacy_offset_timestamps` | `[x]`：偏移时间戳归一化 UTC，日期标签保持缺省。 |

### 本批修复（先红后改）

1. **市场日期 → 市场本地午夜**：Rust 原先把 `startDate`/`endDate` 当 UTC 零点
   （`parse_start_timestamp`），US/HK 回测区间整体偏移 5/8 小时且 DST 日时长错误。
   新增 `resolve_backtest_time_range` / `with_normalized_time_range`（jiff 时区 + 本地午夜、
   结束为次日本地午夜 − 1ns），并在 Start 持久化前写回 `startTime`/`endTime`/
   `startDate`/`endDate`/`marketTimezone`。红测：`backtest_start_resolves_market_dates_with_dst`
   原值 `1772928000000`（00:00Z）vs 期望 `1772946000000`（05:00Z）。
2. **US 扩展时段周期规划**：`plan_sync_intervals` 在多日降级后未再套用 extended 规则，
   `3d` 残留 `1d`；探针（把 extended 分支改为恒假）复现 `["1d","1w"] != ["1h"]` 后按字节回滚。
3. **chartType 归一化缺失**：Start 路径此前原样透传 `chartType`，现按 Go
   `chart.NormalizeChartType` 归一化（空/未知 → standard，heikinashi 保留）。

### 唯一引用约束

- `service_test.go:680 TestRunStoreDelegation` 与 `run_failure_recovery_test.go:81`、
  `service_test.go:732` 复用了 store 层既有证据：`test_in_memory_store_implements_run_lifecycle_and_cancellation`
  已被 `internal/store/backtest/store_test.go:168` 作为 `function_exact` 占用，本批保持 `partial`
  并在结论中写明引用来源。

### 保留差异候选（保持 `partial` 的理由）

- **P0 服务编排**：`service_test.go` 的多断言矩阵（符号/周期/定义版本/初始资金/日期标签/可观测性字段/
  DBPath/执行模型）、`Close` 后拒绝新 Start、`FinishRun` 的内存兜底、runner 返回 nil/panic 的错误文本；
  Rust 以端口注入 + 单一写入所有者实现，缺少同粒度断言或本就不存在等价路径。
- **P1 同步与历史源**：provider 隔离缓存、adapter 关闭时序、“受理前拒绝不支持组合”、
  在途分页取消、瞬时分页重试与 preflight 能力校验、重试耗尽/计时器取消。
- **P2 输入与结果视图**：非法日期/单侧日期拒绝矩阵、provider 覆盖优先级、结果视图游标分页、
  损坏 K 线丢弃与成交量守恒、summary 最新诊断选择、空运行形状的 wire 断言。

验证：`cargo fmt --all`；`cargo clippy -p jftrade-engine -p jftrade-backtest --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-backtest -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**1952 passed / 0 skipped**）；`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2932 Rust** / **1217 `[x]`**；missing 333、partial 2368、boundary 529、module_only 4；`OK: 1217 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 193、7 条 partial 无解析引用、2 条无断言为前批基线告警）；`pnpm run check:rust:architecture`（文件长度门禁触发后把新增测试拆到 `product_production_ports_backtest_strategy_time_range_tests.rs`）；`pnpm run check:compatibility`；`node scripts/check-zero-go.mjs`（2891 tracked files / 0 release artifact）；`pnpm run check:ai-context`；`git diff --check`。

`pnpm run check:quick` **未通过（EXIT=1）**，唯一失败项不在本批 diff 范围内，
保留原始证据不记为通过：`pnpm run check:rust:static` 的 `cargo-deny` advisories
仍因 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45）失败；本批未改
`Cargo.lock`，该依赖由根 `Cargo.toml` 的 `rustls = "=0.23.44"` 精确锁定，
解除需要一次显式的依赖升级批次（含 `deny.toml` 许可例外复核）。同一次
`check:quick` 中 `pnpm run check:rust:workspace` **3049 passed / 2 skipped**、
`check:web` 2435 passed、`check:python` 337 passed，目标健康告警按
`docs/history/go-to-rust/README.md` 的处置流程执行 `cargo clean` 后消失。

## 第 130 批 07 切片一：internal/backtest 首片 32 条 recon（2026-09-24）

范围：`internal/backtest/business_test.go`（2）+ `historical_source_test.go`（9）+
`input_and_readiness_validation_test.go`（10）+ `recovery_test.go`（3）+
`result_view_aggregation_test.go`（2）+ `result_view_test.go`（6），共 32 条
（[x] 1 + partial 31）。

方法：Go 体逐条核对结论；[x] 确认 Rust 测试存在、短式 Parity 锚点在位、断言等价；
全部 32 条 Rust 引用逐一 rg 命中（0 缺失）；高风险项（在途取消时序、重试计数与
preflight 能力、向后分页与跨 provider 隔离、游标分页 nextCursor、panic 恢复语义、
成交量守恒）抽查 Go 体，确认缺口描述与 Go 断言一致。

结论：32 条 verdict 全部成立——1 [x]（空白脚本拒绝，文案 script is required，
校验层等价）成立，唯一打磨是把该行薄结论补写为显式等价说明（v2_writer，
[x] 1565 不变，anchored=1）；31 partial 缺口诚实（含 :111 在途取消时序、:147 重试
计数与 preflight、:59 跨 provider 隔离、:34 游标分页、:27 panic 负载不捕获、
:84 成交量守恒、:12 provider 元数据逐字段矩阵）。
本片除该行结论补全外无判定与代码变更。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（engine research/backtest 位 + backtest 兼容）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（service/service_pineworker/sync/time 等）。

## 第 130 批 07 切片二：service/sync/time 32 条 recon（2026-09-24）

范围：`internal/backtest/service_test.go`（15：[x] 1 + partial 14）、
`internal/backtest/service_pineworker_test.go`（2：partial 2）、
`internal/backtest/sync_test.go`（12：[x] 2 + partial 10）、
`internal/backtest/time_test.go`（3：[x] 3），共 32 条（[x] 6 + partial 26）。

方法：Go 体逐条核对结论；6 [x] 确认 Rust 测试存在、Parity 锚点在位、断言逐项等价
（DST 23 小时与毫秒值、HK 日界、legacy 偏移归一化、区间规划四矩阵、会话作用域
八值集合、图表类型三分支均与 Go 期望同值）；全部 32 条 Rust 引用逐一 rg 命中
（0 缺失，组合引用逐段命中）；[x] 全文唯一性成立（会话作用域用例被路由行复用，
账本以组合形式区分；历史源行 partial 复用允许）；高风险项抽查 Go 体
（:652 关闭取消与关后拒绝、:628 错误文本透传、:680 投影粒度）确认缺口诚实。

结论：32 条 verdict 全部成立，无判定变更、无代码变更——
6 [x] 均为前期修复后新增的回归用例，断言与 Go 同值；
service:680 正确维持 partial（被引测试已作他行 [x] 证据，唯一引用约束）；
partial 缺口具体（含 :652 关后拒绝新 Start、:628 错误文本透传、:191 任务 ID 唯一性、
:332 未知 rehab 回退、:16 启动矩阵、:138 定义派生细节）。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（engine backtest 起止/同步位）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（run_failure_recovery/result_view 系 + pkg/backtest 执行器系）。

## 第 130 批 07 切片三：run_failure_recovery/保守执行器/成本/会话查询 37 条 recon（2026-09-24）

范围：`internal/backtest/run_failure_recovery_test.go`（3）+
`pkg/backtest/conservative_bar_executor_test.go`（19）+
`pkg/backtest/cost_account_failure_boundaries_test.go`（4）+
`pkg/backtest/filter_store_session_queries_test.go`（11），共 37 条
（partial 28 + boundary 9）。

方法：Go 体逐条核对结论；全部 37 条 Rust 引用逐一 rg 命中（0 缺失，组合引用逐段命中）；
保守执行器 16 个 Parity 锚点在位；高风险项深查 Go 体与 Rust 用例体
（:89 部分成交三段订单流与逐笔成交、:147 父括号止损优先、:26 队列持久化失败不泄漏、
:81 终态保持、费用预设与按单计费、错误分类器双片段）。

结论：37 条 verdict 全部成立，无判定变更、无代码变更——
曾存疑的“结果模型只导出终态订单行”表述经核实属实：Go :89 断言 NEW→PARTIALLY_FILLED
（exec 10）→FILLED（exec 50）三段订单流与 10@101、40@102 逐笔成交，Rust 同场景用例
断言终态单行 FILLED + totalFills 2 + 资金 4910/持仓 50（数值一致），中间态流式观测
确无对应断言，P2 观测面差异成立；其余 partial/boundary 缺口（队列失败 Close 行为、
warmup 拒绝矩阵、终态保持注入、预设意图表、通道/流式 API 不迁移、自定义区间回退）
均与 Go 体一致。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（backtest 保守执行/费用/结果上报 + store-sqlite 会话/失败恢复）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（runmodel/storage 编解码与聚合系）。

## 第 130 批 07 切片四：runmodel/storage 编解码与聚合 31 条 recon（2026-09-24）

范围：`pkg/backtest/internal/runmodel/result_test.go`（6）+
`pkg/backtest/internal/storage/` 6 文件（25：codec 2、codec_progress 6、
query_failure 2、store_aggregation 8、store_business 6、corruption 1），
共 31 条（partial 25 + boundary 6）。

方法：Go 体逐条核对结论；全部 31 条 Rust 引用逐一 rg 命中（0 缺失）；
结构性 claim 抽查 Rust 生产代码（Decimal 文本存取、无定点编解码层、
ON CONFLICT upsert、损坏 schema 打开即拒）；高风险项核对 Go 体
（损坏表须报 schema 错误而非覆盖缺失、同 bar 替换、空表空集、会话桶合并）。

结论：31 条 verdict 全部成立，无判定变更、无代码变更——
6 boundary 属实（Rust 以 Decimal 文本存取，无 fixedpoint 编解码/上游兜底层；
结果快照可复现面由语料逐字节确定性覆盖）；
partial 缺口具体（警告样本 cap、归组键文案、运行期错误计数、多标的与通道形态、
消毒兜底、nil 快照别名、句柄计数、逐列短行分类、反解析矩阵、替换专项断言等）。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（store-sqlite 聚合/失败恢复/会话 + backtest 告警/兼容）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（store 连接/失败/查询/运行时系）。

## 第 130 批 07 切片五：store 连接/失败/查询/运行时 32 条 recon（2026-09-24）

范围：`pkg/backtest/internal/storage/store_connection_test.go`（4）+
`store_failure_boundaries_test.go`（5）+ `store_query_aggregation_contracts_test.go`（3）+
`store_runtime_invariants_test.go`（14）+ `store_session_aggregation_contracts_test.go`（4）+
`stream_query_failure_sorting_test.go`（2），共 32 条（partial 23 + boundary 9）。

方法：Go 体逐条核对结论；全部 32 条 Rust 引用逐一 rg 命中（0 缺失）；
并发与结构性 claim 抽查 Rust 生产代码（Mutex 单连接 + busy_timeout 10s、
无 WAL 配置、WriterLease 拒绝第二写入者、TransactionBehavior::Immediate、
每次读 sqlite_master 无存在性缓存）与 Go 体（连接池 8 连接、双实例串行写、
WAL 并行读、排队写可见性、触发器中途失败回滚）。

结论：32 条 verdict 全部成立，无判定变更、无代码变更——
9 boundary 属实（连接池/WAL/排队写/通道流式/多标的排序/存在性缓存/逐不变量助手
在 Rust 无对应对象，升级路径已登记）；
partial 缺口具体，含 :208 批量中途失败无残留行的专项断言缺失（已登记回归要求，
P0 恢复类）、:500 会话分页缺口上报、:58 港股跨时段聚合 P2、:59 首个存在表回退选择。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（store-sqlite 全量 + backtest 全量）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（pine/costs/collector/runner/result 系）。

## 第 130 批 07 切片六：pineworker 命令执行/适配/原子边界 37 条 recon（2026-09-24）

范围：`pkg/backtest/pineworker_command_executor_test.go`（23）+
`pkg/backtest/pineworker_atomic_boundaries_test.go`（2）+
`pkg/backtest/pineworker_adapter_test.go`（12），共 37 条（partial 37）。

方法：Go 体逐条核对结论；全部 37 条 Rust 引用逐一 rg 命中（0 缺失）；
核心结构 claim 实证：Rust 全仓无 CommandFromOrderIntent/command 转换层
（rg 零命中），worker 意图经执行端口/确定性撮合直接执行；
适配器拒绝非法意图在位（requires long/short）；
Go WorkerOrderCommand DTO 形态（Kind/ID/Side/OrderType/Quantity/GTC）抽查确认。

结论：37 条 verdict 全部成立，无判定变更、无代码变更——
统一结论线与 strategy 域 s5/s6 一致：Go 的 command 转换层不迁移，
方向归一/数量解析/失败关闭/撤单派发落在执行层与撮合层，
逐 command 字段（方向字段、GTC、跟踪回退、畸形矩阵、港股整手、 sizing 文案）
无同形断言，partial 诚实。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（integration-pine 适配/e2e + engine 意图执行 + backtest 全量）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（replay/sizer/runner/collector/result 系）。

## 第 130 批 07 切片七：pine 成本/语料 + replay/runner 35 条 recon（2026-09-24）

范围：`pkg/backtest/pine_costs_test.go`（4）+ `pine_ts_corpus_test.go`（1）+
`pine_ts_shadow_reference_test.go`（2）+ `pine_ts_smoke_test.go`（1）+
`pineworker_replay_pump_test.go`（4）+ `pineworker_replay_source_test.go`（3）+
`pineworker_replay_test.go`（6）+ `pineworker_runner_boundaries_test.go`（4）+
`pineworker_runner_failure_boundaries_test.go`（3）+
`pineworker_runner_test.go`（7），共 35 条（[x] 2 + partial 23 + boundary 10）。

方法：Go 体逐条核对结论；2 [x] 确认 Rust 测试存在、Parity 锚点在位、
同输入同断言逐项等价（EMA 前 2 NaN + [2,3,4]、MACD NaN 分布与 1/1/0）；
全部 35 条 Rust 引用逐一 rg 命中（0 缺失）；冒烟用例确认 opt-in ignore 形态，
结论已注明默认门禁不跑。

结论：35 条 verdict 全部成立，无判定变更、无代码变更——
2 [x]（EMA SMA 初始化、MACD NaN 跳过）为纯数值语义等价；
partial/boundary 缺口诚实（pump/planner/收集器/归一化层不迁移、
报价货币默认表、缓存清理助手、逐 bar 完整性矩阵、港股整手等）。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（backtest 指标兼容 + integration-pine 全量）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（sizer/collector/costs/result 余量系）。

## 第 130 批 07 切片八：sizer/collector/short-replay/session-filter 31 条 recon（2026-09-24）

范围：`pkg/backtest/replay_sizer_bounds_test.go`（4）+
`result_collector_test.go`（8）+ `result_collector_trade_stats_test.go`（6）+
`run_result_test.go`（2）+ `runner_hardcut_test.go`（1）+
`runner_helpers_test.go`（1）+ `session_filter_store_boundaries_test.go`（3）+
`session_filter_store_test.go`（1）+ `short_replay_bounds_test.go`（3）+
`short_replay_test.go`（2），共 31 条（partial 22 + boundary 9）。

方法：Go 体逐条核对结论；全部 31 条 Rust 引用逐一 rg 命中（0 缺失）；
关键语义抽查 Rust 用例体（fills 数组逐笔 realizedPnl、终态 PARTIALLY_FILLED 导出、
部分平仓撤单终结已成交分段）与 Go 体（:198 四段增量更新与去重、累计账本）；
澄清与 s3 的关系：s3 的“终态订单行”指保守执行器 harness 的 orders 导出，
本片 reporting harness 导出 fills 明细与终态部分成交态，两者结论各自精确、无矛盾。

结论：31 条 verdict 全部成立，无判定变更、无代码变更——
:198 增量跟踪语义一致（分笔累计），增量更新 API 的去重/累计账本无直接断言，
partial 诚实；9 boundary 属实（合成订单委托层、包装 store、流式 API、
旧入口 hardcut、runmodel 计数样本等不迁移，升级路径已登记）。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（backtest 结果上报/费用/撤单 + store-sqlite 会话）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按文件行号继续（source/store/costs 余量 37 条）。

## 第 130 批 07 切片九：source/store/costs 余量 37 条 recon（2026-09-24）

范围：`pkg/backtest/source_exchange_business_test.go`（2）+
`store_session_synth_test.go`（8）+ `store_test.go`（14）+
`sync_progress_test.go`（1）+ `trading_cost_replay_boundaries_test.go`（4）+
`trading_costs_test.go`（8），共 37 条（partial 35 + boundary 2）。

方法：Go 体逐条核对结论；全部 37 条 Rust 引用逐一 rg 命中（15 个唯一入口，0 缺失）；
关键语义抽查：`trading_costs:275` 按单最低费分笔增量（Go total 12 + breakdown 12
与 Rust `totalBrokerFees "12"` + breakdown amount "12" 同值）；
`store:532` 1 分钟合成 5 分钟 OHLCV 精确断言（Go high 102.5/low 99/close 101.5/
volume 510 语义与 Rust open/high/low/close/volume 精确断言一致，API 形态不同故
partial 诚实）；2 boundary（sync 快照别名隔离归因所有权系统、费用生效日期区间
Rust 模型无字段）属实，升级路径已登记。

结论：37 条 verdict 全部成立，无判定变更、无代码变更。本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（backtest 费用行为 + store-sqlite 聚合/会话/日历）、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按域余量继续（exchangecalendar 约 59 条）。

## 第 130 批 07 切片十：exchangecalendar manager 系 35 条 recon（2026-09-24）

范围：`internal/exchangecalendar/manager_test.go`（21）+
`manager_boundaries_test.go`（9）+ `manager_probe_test.go`（1）+
`manager_runtime_test.go`（4），共 35 条（原 35 [x]）。

方法：Go 体逐条核对结论；35 条 Rust 引用逐一 rg 命中（0 缺失），`[x]` 唯一性
无重复；其余 32 条结论带探针/回滚证据或逐字段断言，成立。3 条收回过宽 `[x]`→partial：

- `:33` 失败退避：Go 只断言 NextRefreshAt 状态计算（1h 起步、24h 封顶，
  refresh 路径从不查阅该值）；Rust 状态值一致，但把该时间兼作退避门
  （提前 refresh 计 skipped_backoff 并跳过抓取），调度语义不等价。
- `:50` 预热超时：Go 断言常量不等式 60s>=3*15s（多源串行窗口）；
  Rust 无远端 provider、无 warmup 常量，只断言单次调用受 probe 预算约束。
- `:876` 市场本地年：US 侧规则等价（EST 窗口固定），HK 侧年份推导与键格式无断言。

结论：35 条 verdict = **32 `[x]` + 3 partial**（`[x]` 1565→1562，
partial 2248→2251）。有代码变更需求（退避门裁决、HK 窗口断言、warmup 预算关系）
已记入各条回归要求，本片仅台账修正 + 文档。

验证：`v2_writer.py --check` 过后落库；`audit_test_parity.py --write-report` exit 0；
`parity_anchor_reconcile.py` 过（1693/0/0/46）；`cargo fmt -p jftrade-engine -- --check`；
定向 nextest（jftrade-calendar 89 + jftrade-integration-calendar 23 全过）；
`check:ai-context`、`check:migration-manifest`、`check:zero-go`、
`check:quick`（单实例）、`git diff --check`。

下一片：backtest_calendar 按域余量继续（http_source 系 21 + snapshot/health 3 +
pkg/market/calendar 13 = 37 条）。

## 第 130 批 07 切片十一：http_source/健康/calendar 37 条 recon，backtest_calendar 收官（2026-09-24）

范围：`internal/exchangecalendar/http_source_test.go`（14）+
`http_source_boundaries_test.go`（7）+ `source_health_status_test.go`（2）+
`source_json_test.go`（1）+ `pkg/market/calendar/`（13：builtin 5、
calendar_boundaries 2、helpers_boundaries 5、types_json 1），共 37 条。

方法：Go 体逐条核对结论；37 条 Rust 引用逐一 rg 命中（0 缺失），`[x]` 无重复；
parser/validator/注册表/超时/快照元数据/状态错误 21 条 [x] 均为 fixture 级逐项
等价（含真实修复与探针证据），成立；12 partial + 2 JSON `[x]` 口径诚实。1 条收回
过宽 `[x]`→partial：

- `http_source_boundaries:298` nil 管理器安全：Go 整条断言 nil receiver guard、
  nil-clock 回退、nil-resolver 拒绝；Rust 无 nil 语义，现有用例断言空 registry
  生命周期安全。空集合安全等价，后三者在 Rust 无对应断言。
- `source_health:17` 的 `[x]` 复核保留：其断言主体是分源健康真实性（单次
  refresh/probe，退避门不触发），与 Go 逐项一致；退避门差异仅作上下文说明，
  与 `:33`（断言主体即 NextRefreshAt 调度语义）性质不同。

结论：37 条 verdict = **24 `[x]` + 13 partial**（`[x]` 1562→1561，
partial 2251→2252）。**backtest_calendar 域 376 条全部 recon 完毕**；
本片仅台账修正 + 文档，无代码变更。

验证：`v2_writer.py --check` 过后落库；`audit_test_parity.py --write-report` exit 0；
`parity_anchor_reconcile.py` 过（1693/0/0/46）；`cargo fmt -p jftrade-engine -- --check`；
定向 nextest（jftrade-calendar 89 + jftrade-integration-calendar 23 全过）；
`check:ai-context`、`check:migration-manifest`、`check:zero-go`、
`check:quick`（单实例）、`git diff --check`。

下一域：按余量排序的下一块（`internal/store`，recon 口径待定）。

## 第 130 批 09 切片 c：backtest [x] 第 45–64 条二次复核（2026-09-24）

范围：`internal/exchangecalendar/manager_test.go` 14 条（:291/:346/:393/:444/
:488/:543/:613/:670/:736/:751/:779/:811/:890/:917）+ `source_health_status_test.go`
2 条 + `source_json_test.go` 1 条 + `pine_ts_shadow_reference_test.go` 2 条，
共 20 条。Go 体全读，Rust 引用 22/22 可解析且 1:1 名匹配。

结论：19 条维持，1 条收回过宽 `[x]`→partial，1 条结论措辞修正（verdict 不变）：

- `manager_test:613` 超时变体去重：Go 标题断言是三轮变体失败只产生一条告警
  （`len(alerts)==1`）+ 指纹归一；Rust 用例只锁定指纹归一（每轮 fingerprint/
  status 断言 + 探针回滚证据），三轮 `last_alert_at` 仅收 3/3 非空、未如 :543
  那样断言去重不变。“引用存在 ≠ 断言等价”，按口径降为 partial；回归要求已记
  入 uncovered（补同指纹重复失败不刷新 `last_alert_at` 的断言，或登记设计决策）。
- `manager_test:291` 结论把 Rust 侧多加的 validUntil 隔离段误写成 Go 原文两段，
  已修正措辞（Go 本体一段 + Rust 加强一段），`[x]` 不变。

`[x]` 1477→1476。本片仅台账 + 报告/库存再生，无 Rust 代码变更。

验证：`audit_test_parity.py --write-report` exit 0（1476 exact，dup 0）；
`parity_anchor_reconcile.py` 1742/1696/0/0/46；定向 nextest（jftrade-calendar
89/89，backtest pine 9/9）；`git diff --check` 干净。
## 第 130 批 11 切片一：backtest 域 partial 第 1–20 行，19 行维持、1 行措辞锐化（2026-09-24）

范围：backtest 域 partial 第 1–20 行（business 2、historical_source 9、input_validation 9，
按文件加行号升序）。审计口径 backtest_calendar 域 376 行 = 63 exact + 313 partial。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（nil store 安全降级改 fail-closed、覆盖与归一辅助函数边界、向后分页加跨源隔离、
在途分页取消时序、瞬时重试加 preflight 能力、空结果 helper 层拒绝、可选校验器放行、
生命周期拒绝路径、三场景分页、转换五字段加会话展开、重试耗尽加计时器取消、provider
覆盖优先级、启动前队列状态、缺覆盖不持久化、adapter 构造失败、动态断言辅助、分辨率
拒绝矩阵、就绪终态矩阵，均与账本缺口一致）。

1 行措辞锐化（verdict 不变，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- input_validation:14：Rust 引用覆盖非法 since、非法周期、区间倒置三处 BadRequest，
  原结论误写成两类；Go 7 命名加零填充与不可能日期矩阵的其余项仍无覆盖。

抽核要点：sync_helpers 三用例逐行核实（空页拒绝、period 失配拒绝、合法单 candle 通过）；
游标推进由 Futu 夹具两页 walk 覆盖，跨源隔离缺口成立；重试 helper 有实现但零用例驱动
瞬时失败序列，preflight 能力校验缺口成立。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（engine backtest 族 66/66、jftrade-backtest 57/57）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片二，backtest 域 partial 第 21–40 行。
## 第 130 批 11 切片二：backtest 域 partial 第 21–40 行，20 行维持、零修改（2026-09-24）

范围：backtest 域 partial 第 21–40 行（input_validation 余 1、recovery 2、
result_view_aggregation 2、result_view 6、run_failure 3、service 4、
service_pineworker 2，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

20 行维持（provider 单次解析固定、nil 结果与 panic 负载不区分归类、警告与序列
组合过滤、损坏 K 线丢弃与成交量守恒、provider 与执行元数据逐字段矩阵、窗口游标
分页、解析辅助非法输入、非法时间过滤、空运行形状 wire 断言、最新诊断选取、
持久化失败后 Close 行为、低周期 warmup 拒绝矩阵、running 迁移失败注入终态保持、
默认 runner 错误文本分类、PineTS 配置透传、启动完整请求矩阵、adk 前缀派生细节、
coverage 调用参数边界、任务复用与同步参数，均与账本缺口一致）。

抽核要点：reap 用例锁定非终态 worker 标 failed 加句柄清零，不区分 nil 与 panic；
readiness 生命周期用例经工具分发覆盖，不计数 provider 解析次数；
store 契约无写失败注入，终态保持缺口成立。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（engine backtest/result_view/research 族 73/73、store-sqlite backtest 8/8）全过；
cargo fmt --check 与 git diff --check 干净；账本与 Rust 代码零改动（纯复核片）。

下一片：130-11 切片三，backtest 域 partial 第 41–60 行。
## 第 130 批 11 切片三：backtest 域 partial 第 41–60 行，18 行维持、2 行结论纠正（2026-09-24）

范围：backtest 域 partial 第 41–60 行（service 10、sync 10，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

18 行维持（失败不同重启、终态枚举与停止重试、任务清理时机、最大 warmup 取值、
窗口游标与聚合数值、校验矩阵分散、错误文本透传、关闭态拒绝新 Start、内存兜底
另一半语义、进度委托、适配器转换矩阵、失败关闭顺序、取消关闭时序、任务 ID 唯一、
缺存储清理路径、受理前拒绝顺序、构造后校验清理，均与账本缺口一致）。

2 行结论纠正（verdict 不变，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- service:680：原结论称同一 Rust 测试被 :168 行作 function_exact 证据占用，
  与事实不符（:168 行同为 partial）；partial 共用引用不受 [x] 唯一性约束，
  本行维持 partial 并保留引用，理由已改写。
- sync:298：Rust 引用覆盖非法 since、非法周期、区间倒置三处 BadRequest，
  原结论误写成两类；无效符号与非法 until 仍无覆盖。

抽核要点：未知 rehabType 在 Rust 解析器归一为 forward（request.rs），行为在位、
显式回退断言缺失，缺口成立；store 契约无写失败注入。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（engine backtest 族 66/66、store-sqlite backtest 8/8）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片四，backtest 域 partial 第 61–80 行。
## 第 130 批 11 切片四：backtest 域 partial 第 61–80 行，19 行维持、1 行换引用改写（2026-09-24）

范围：backtest 域 partial 第 61–80 行（exchangecalendar 6、conservative_bar_executor 14，
按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（nil 管理器安全、日历恢复路径 provider 校验扩展点、退避门调度语义、
warmup 预算多源串行、告警计数去重、HK 年份推导、输入校验文案、12 个执行器场景
的终态一致加 P2 观测面差异，均与账本缺口一致）。

1 行换引用改写（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、
entry_changed 等于 1，证据与复用键已按算法重建）：

- conservative:21：原结论两处失实——Go 对未知模型名是精确报错而非回退；
  归一行为已迁移到解析层 n_name（空缺省、裁剪加大小写折叠、未知名拒绝三簇
  俱有测试），并非未迁移。引用换为 execution_model_defaults_and_normalizes_
  ascii_case，残余缺口收敛为报错文案空白保留差异与助手单元改解析入口的层位差。

抽核要点：:89 的 10@101、40@102、终态 4910/50 与仅终态订单行逐项核实；
:21 的三簇断言与 n_name 回退分支逐行核实；6 个日历引用全部命中。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest 57/57、jftrade-calendar 89/89、
engine execution_model 4/4）全过；cargo fmt --check 与 git diff --check 干净；
无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片五，backtest 域 partial 第 81–100 行。
## 第 130 批 11 切片五：backtest 域 partial 第 81–100 行，20 行维持、零修改（2026-09-24）

范围：backtest 域 partial 第 81–100 行（conservative 余 5、cost_account 4、
filter_store 11，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

20 行维持（止损触发语义加 P2 观测面、警告去重口径、流动性阈值来源、助手白盒
分支口径、pending 内部状态边界、预设意图字段、非计费与不重复收费、权益点抑制
另一半语义、字符串片段分类器、包装委托计数、逐页过滤 nil 语义、通道回退路径、
自定义区间路由、后向窗口裁剪、通道扩展行、通道常规过滤、通道错误形态、区间
回退路径、流式扩展行、包装助手函数，均与账本缺口一致）。

抽核要点：:652 的 Rust 引用为场景级滑点用例，与 Go 白盒助手分支口径不同，
缺口成立；:771 的 pending 切片断言在 Rust 无同形对象，边界成立；
:26 的不重复收费由双用例共担（后者逐行核实存在）；:56 的 engine 侧对应
用例逐行核实存在，权益点抑制仍缺直接断言。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest 57/57、store-sqlite 市场数据族 28/28）全过；
cargo fmt --check 与 git diff --check 干净；账本与 Rust 代码零改动（纯复核片）。

下一片：130-11 切片六，backtest 域 partial 第 101–120 行。
## 第 130 批 11 切片六：backtest 域 partial 第 101–120 行，19 行维持、1 行结论纠正（2026-09-24）

范围：backtest 域 partial 第 101–120 行（runmodel 6、storage 14，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（快照独立拷贝与运行期错误模型缺席、空计数省略、样本去重上限、
警告样本上限、归组键文案、边界记账、损坏聚合等价性、进度状态机与别名保护、
定点编解码层、行扫描分类、schema 兜底消毒、空路径与 legacy 拒绝、只读库连接
释放、上游解析兜底、多标的与通道形态、周期值映射、反解析矩阵、作用域归一，
均与账本缺口一致）。

1 行结论纠正（verdict 不变，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- query_failure:120：原结论称 Rust 空表用例覆盖前向/后向双路径，与事实不符
  （Rust 只有单时间区间读取 API，无方向变体）；改写为方向语义无对应断言。

抽核要点：警告用例确无 100 样本上限与归组断言；read_candles 签名确无方向参数；
周期值映射与反解析矩阵逐项读过 Go 体。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest 57/57、jftrade-store-sqlite 整轮 188/188）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片七，backtest 域 partial 第 121–140 行。
## 第 130 批 11 切片七：backtest 域 partial 第 121–140 行，19 行维持、1 行结论纠正（2026-09-24）

范围：backtest 域 partial 第 121–140 行（store_aggregation_boundaries 5、store_business_aggregation 6、
store_connection 4、store_failure_boundaries 5，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（交易周期助手与周期间隔换算、基周期集合与扩展日线优先级、扩展时段基区间边界、
纯聚合空未知越界、会话桶内合并、upsert 替换与确定性分页、缺失区间与开放窗口、周线前后向聚合、
小时线聚合扩展日线、覆盖缺失消息与日线回退、无标签行跳过、8 并发读连接池边界、双写串行化、
WAL 并行读后来写边界、读等写队列边界、关闭库错误矩阵、助手层穿透边界、紧凑 schema 拒旧表、
作用域优先级表名，均与账本缺口一致）。

1 行结论纠正（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- store_failure_boundaries:207：原结论尾句称空符号列表由同步器层拒绝，与事实相反
  （Go 断言空输入静默 no-op 成功；引用的同步器用例断言空结果页报错，语义相反）；
  改写为空符号列表短路语义无对应断言（同步器层只拒绝空结果页，而非空输入）。

抽核要点：纯 Go 助手（tradingPeriodUnit、aggregationBaseIntervals、dailyAggregationBaseRange、
aggregateDailyKLinesFromBase 等）在 Rust 无同形对象，行为仅由集成用例侧写覆盖，缺口成立；
连接池与 WAL 与写队列三行确为单连接互斥结构差异，引用仅作租约与目录证据；关闭后错误矩阵
确无显式 closed 标记；:188 的未知作用域回退 regular 在 Rust 无 SetReadSessionScope 对应物。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-store-sqlite 整轮 188/188、engine backtest 族）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片八，backtest 域 partial 第 141–160 行。
## 第 130 批 11 切片八：backtest 域 partial 第 141–160 行，18 行维持、2 行结论收紧（2026-09-24）

范围：backtest 域 partial 第 141–160 行（store_query_aggregation 3、store_runtime 14、
store_session_aggregation 3，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

18 行维持（1 分钟聚合 5 分钟、日线与周线合成、压缩四形态与租约互斥、表存在缓存边界、
首表回退缺席、日历聚合助手分支、limit 归一化缺席、作用域隔离读、批量回滚缺专项断言、
覆盖选择边界助手、边界助手窗口外分支、覆盖解析器关闭库分支、损坏基数据与不可用存储、
不变量助手边界、空窗缺表坏表三态、部分批次缺口上报、港股跨时段缺席、美股扩展区间合成，
均与账本缺口一致）。

2 行结论收紧（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- store_query_aggregation:12：原结论称往返矩阵按表维度展开，含糊且高估覆盖
  （Rust 仅断言直接读取隔离与缺区间空结果）；改写为前向/后向/流式/通道形态
  与覆盖确认语义无对应断言。
- store_session_aggregation:11：原结论称 Rust 断言同样的稳定契约，与事实不符
  （Rust 只覆盖表名生成与非法输入拒绝）；改写为列清单与区间数值往返矩阵无逐条断言。

抽核要点：insert_candles 确为 Immediate 事务内建表加 upsert（backtest_market_data.rs），
失败即整体回滚，:208 缺专项断言成立；HK 分段窗口在生产代码存在但语料只覆盖美股；
:156 的缺表恢复与坏表 fail-closed 引用逐项存在。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-store-sqlite 整轮 188/188、jftrade-backtest 57/57）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片九，backtest 域 partial 第 161–180 行。
## 第 130 批 11 切片九：backtest 域 partial 第 161–180 行，19 行维持、1 行结论收紧（2026-09-24）

范围：backtest 域 partial 第 161–180 行（store_session 排序 1、stream 排序失败 2、
pine 成本 4、pine 语料与冒烟 2、pineworker 适配 11，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（多标的混合周期排序边界、损坏流多形态边界、多标的稳定排序键边界、
佣金百分比换算、初始资金优先级链边界、滑点 tick 双向、滑点执行与撤单分担、
语料报告与字节确定性、真实 worker 双 opt-in 冒烟、意图转命令三类、空头方向保持、
空头退出买入映射、作用域数量存活、条件单类型映射、OCO 拒绝时机、OCO 双腿展开、
缺省数量、拒绝分类、适配器 Run 职责，均与账本缺口一致）。

1 行结论收紧（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- pineworker_adapter:208：原结论称规范化规则一致，与事实不符
  （Rust 仅断言空头平仓映射为买且带 reduceOnly）；改写为 sell 入场规范化为
  short 无对应断言，归一化位于执行层而非适配层。

抽核要点：171/172/177 共用同一 Rust 用例属 partial 共用（非 [x] 唯一性约束），
其中 171/172 已显式限定覆盖范围，仅 177 高估一致性；166 的 100.03/99.97 双向数值
在 Rust 为同值端口用例，保持 partial（助手直测与执行链路层位差）；165 的初始资金
仅随请求传递、无优先级链断言，维持 boundary；169 双边均为 opt-in（Go 跳过、Rust 忽略）。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest 57/57、jftrade-integration-pine、engine 执行族）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十，backtest 域 partial 第 181–200 行。
## 第 130 批 11 切片十：backtest 域 partial 第 181–200 行，18 行维持、2 行结论收紧（2026-09-24）

范围：backtest 域 partial 第 181–200 行（adapter 错误映射 1、atomic 边界 2、
command 执行器 17，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

18 行维持（worker 错误三形态映射、原子组 11 形态矩阵、原子执行失败边界、提交与审计链路、
缺 sizing 拒绝文案、缺数量拒绝层位、权益与持仓百分比基数、无参 close 展平、参数less 跳过、
步长与碎股与规则缺失三行、告警归组键文案、自动平仓触发、撤单与 cancel_all 直接对应、
原子 bracket 先拒后零提交，均与账本缺口一致）。

2 行结论收紧（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- command_executor:158：原结论称订单标记由订单记录结构表达，未经证实
  （Rust 证据无任何 Tag 断言）；改写为 Tag 标记无对应断言。
- command_executor:361：原结论称显式平仓与告警开关由意图字段控制，含糊且掩盖三个子分支
  （空头标记、无告警收集器、全平 sizing 错误）；改写为三者均无对应断言。

抽核要点：Rust 无空头回放 Tag 概念（全仓 grep 无对应）；:361 的 Rust 用例只覆盖多头
多入场展平归零，无空头、无告警、无 sizing 分支；192/193 共用 lot-size 用例属 partial
共用，193 的 board-lot 缺口已显式登记；181 的三形态错误只覆盖不可用一支，维持 partial。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、integration-pine、engine 执行族）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十一，backtest 域 partial 第 201–220 行。
## 第 130 批 11 切片十一：backtest 域 partial 第 201–220 行，18 行维持、2 行结论纠正（2026-09-24）

范围：backtest 域 partial 第 201–220 行（command 执行器余 6、replay pump 4、
replay source 3、replay 规划 6、runner 边界 1，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

18 行维持（畸形 bracket 形态矩阵、执行器错误双通道传播、业务边界三分支、
撤单四边界、pump 消费执行顺序、pump 形状校验、pump 缺 bar 检测、pump 错误传播、
收集器过滤与错误映射、分块容量细节、请求组装助手、请求字段校验、planner 分组、
bar 越界拒绝、同 bar 归一化、worker 错误传播、运行时数据不可用，均与账本缺口一致）。

2 行结论纠正（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- command_executor:452：原结论只写原子提交，漏掉 Go 后半段
  （逻辑 OCO 整体撤销产生 2 条 cancelled）；补上该半段无对应断言。
- command_executor:579：原结论把止损价（Stops）误读为失败停止并虚构 ID 生成失败分支
  （Go 实际断言 ID 前缀格式、市价止损单字段与跟踪回退）；按 Go 体重写缺口。

抽核要点：:452 的 Rust 用例覆盖三腿原子与 stop-first，不管 OCO 整体撤销；
:579 的 Rust 引用只覆盖按 ID 撤销，与 ID 生成格式无关，维持 partial 已是宽容口径；
其余 replay pump/source/planner 的 Rust 引用逐项存在，结构差异如实记录为 boundary。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、integration-pine、engine 执行族）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十二，backtest 域 partial 第 221–240 行。
## 第 130 批 11 切片十二：backtest 域 partial 第 221–240 行，18 行维持、2 行结论收紧（2026-09-24）

范围：backtest 域 partial 第 221–240 行（runner 边界 3、runner 失败边界 3、
runner 主流程 7、sizer 边界 4、result 收集 3，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

18 行维持（来源与覆盖前置校验、报价货币默认表边界、缓存清理边界、非法配置两分支、
完整性三分支、无效命令 fail-closed、端到端订单簿交易、warmup 继承与种子保留、
quantity pct 数量基数、来源回退不拒单、worker 错误映射、sizer 非法输入与持仓维护
与权益边界与精度矩阵、heikinashi 种子、交易统计、部分成交累计，均与账本缺口一致）。

2 行结论收紧（verdict 仍为 partial，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- pineworker_runner:234：原结论称告警由运行时通知路径表达，含糊且未经证实
  （Rust 只有 INTENT_SKIPPED 审计，无计数文案）；改写为忽略计数、告警文案、
  空订单簿三者无对应断言。
- pineworker_runner:293：原结论未提告警计数与文案缺口
  （Rust 只断言约束合并语义）；补上告警计数、文案与空订单簿无对应断言。

抽核要点：:234 的 Rust 审计断言逐行核实（INTENT_SKIPPED 加 no open position）；
:293 的 Rust 用例逐行核实（仅约束合并，无告警无订单簿）；其余 runner/sizer/result
引用逐项存在， verdict 均为诚实 partial/boundary。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、integration-pine、engine 执行族、broker 市场规则）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十三，backtest 域 partial 第 241–260 行。
## 第 130 批 11 切片十三：backtest 域 partial 第 241–260 行，18 行维持、2 行结论纠正（2026-09-24）

范围：backtest 域 partial 第 241–260 行（result 收集 5、trade stats 6、run result 2、
runner 硬切与助手 2、session 过滤 4、short 回放 1，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

18 行维持（回撤指标、非正收盘告警、订单身份三级回退、终结与 warmup 标记、
加权成本三行、部分平仓聚合、撤单终结、warmup 平仓、防御边界、运行时错误聚合、
旧 runner 硬切、助手族、会话过滤三行、美股自定义聚合、短回放委托，均与账本缺口一致）。

2 行结论纠正（verdict 不变，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- result_collector:256：原结论描述的是 Rust 侧（每单只收一次），不是 Go 侧
  （Go 实际断言空交易、零订单号、空条目应用均为无操作）；按 Go 体重写缺口。
- run_result:8：原结论误贴 253 行的运行时错误文案（计数/样本不导出）
  （Go 实际断言 Snapshot 深拷贝独立副本）；按 Go 体重写，维持 boundary。

抽核要点：两处均为结论与 Go 体错位、非 verdict 错误；:256 的 Rust 用例逐行核实
为按单计费场景；:8 的 Rust 引用为字节确定性回放，无 Snapshot API；
其余 18 行引用逐项存在。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、integration-pine、strategy 族）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十四，backtest 域 partial 第 261–280 行。

## 第 130 批 11 切片十四：backtest 域 partial 第 261–280 行，19 行维持、1 行结论收紧（2026-09-24）

范围：backtest 域 partial 第 261–280 行（短回放合成校验与价格回退 2、短回放入场回补与撤单过滤 2、
来源市场固定规则 2、美股 2h/前向 2h/后向分页/日线/周线/月线合成与港股缺口 8、紧凑 schema/维度分表/
作用域版本隔离/读写作用域优先与回退 6，按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（合成校验与价格回退边界、短回放入场回补与撤单过滤、合约规格保守默认、
美股 2h 与前向 2h 合成、后向分页一致性、美股日线与周月合成语义、港股午休 P2 缺口三行、
紧凑 schema、维度分表、作用域版本隔离、读写作用域优先与不回退，均与账本缺口一致）。

1 行结论收紧（verdict 不变，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- source_exchange:9：原结论“规则来源为快照、不触达实盘”易被误读为引用用例的断言；
  按 Go 体重写，明确引用用例仅断言去空白匹配与覆盖顺序，存活操作拒绝链无对应断言。

抽核要点：20 条 Rust 引用逐项存在；265 的 Rust 用例逐行核实仅覆盖 trim 与覆盖顺序；
276 的别名共享与维度分表经核实相容（别名归一、维度缀名）。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、jftrade-broker、jftrade-store-sqlite、jftrade-engine 执行族）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十五，backtest 域 partial 第 281–300 行。

## 第 130 批 11 切片十五：backtest 域 partial 第 281–300 行，19 行维持、1 行结论收紧（2026-09-24）

范围：backtest 域 partial 第 281–300 行（store 紧凑往返/复权过滤/覆盖 Verify/分钟合成/
周期存储值 8、同步进度快照 1、交易成本与回放边界 4、费用预设与费用引擎 7，
按文件加行号升序）。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

19 行维持（紧凑行存储编码往返、复权过滤查询、重叠接受与缺失报告的 Verify、
1m→5m 与 5m→15m 合成及 5m 源优先、周期存储值往返、快照别名隔离边界、
成本助手归一化矩阵、费用原语与回放批处理边界、pump 失败契约、市场预设矩阵、
报价币解析、自定义规则克隆归一、生效区间边界、港股/美股/按单最低费用语义，
均与账本缺口一致；其中 298/299/300 的 Rust 用例与 Go 数值逐项对应，
295 的报价币矩阵缺口已显式登记）。

1 行结论收紧（verdict 不变，[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- trading_cost_replay_boundaries:150：原结论“回放边界由请求校验与执行身份校验覆盖”
  引用了条目之外的用例；按 Go 体重写，明确引用用例仅覆盖方向校验，
  回放请求构建、参数隔离、数量归一、方向映射表无对应断言。

抽核要点：20 条 Rust 引用逐项存在；298/299/300 与 285 的 Rust 数值断言逐行核实
与 Go 一致；292 的 Rust 用例逐行核实仅覆盖非法方向拒绝。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、jftrade-integration-pine、jftrade-store-sqlite
视触及范围）全过；cargo fmt --check 与 git diff --check 干净；
无 Rust 生产代码改动、无新增测试。

下一片：130-11 切片十六，backtest 域 partial 第 301–313 行（末片）＋下一域开片。

## 第 130 批 11 切片十六：backtest 域 partial 第 301–313 行（末片），12 行维持、1 行 verdict 纠正（2026-09-24）

范围：backtest 域 partial 第 301–313 行（脚本佣金映射 1、日历 builtin/边界/helper 12，
按文件加行号升序）。本片后 backtest 域 partial 313/313 核对完毕。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

12 行维持（US 节假日与提前收盘、HK 工作日模板窗口、大陆节假日关闭与原因、
工作日开放、别名共享节假日回退、年/月/周末 schedule 边界、helper 非法输入边界、
快照市场日期与覆盖窗口、交易日会话业务状态、圣诞提前收盘与模板副本、解析器
nil/零值/未知市场回退、会话归一化与时区回退，均与账本缺口一致；
302/311 的 Rust 用例逐行核实覆盖 black_friday/christmas_eve/independence 三日
与延长窗口保留，303–306 的 HK 分钟窗口与端午原因逐项对应）。

1 行 verdict 纠正（[x] 1472 不变，b83 dry-run 先行、entry_changed 等于 0）：

- trading_costs:301：由 partial 改为 boundary。Go 断言 Pine 脚本佣金只映射到
  broker 费用；经核实 Rust 无脚本佣金概念（backtest 与 integration-pine 均无
  commission 映射，fees.rs 的 commission 仅为单测规则 ID），引用用例仅旁证
  费用组分离。原结论“映射由费用规则解析承担”属概念错位，按 Go 体与 Rust
  体重写。

抽核要点：13 条 Rust 引用逐项存在；7 个日历引用用例名逐项核实；
integration-pine 无 commission 引用（rg 空）。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-backtest、jftrade-calendar 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无 Rust 生产代码改动、无新增测试。

下一域：strategy_pine（459 partial，为最大关键域缺口，见 automation s17 首片）。

## 2026-09-27 P1：同步分页 cursor 生产路径补测

范围：`internal/backtest/historical_source_test.go:268` / `TestHistoricalKLineSyncerRejectsBrokenPagination`。

本批新增 `production_helper_sync_rejects_broken_pagination_cursors`，使用真实
`ProductionBacktestPort` 与 HTTP helper 夹具逐项覆盖 missing `nextBefore`、向前
cursor、以及 cursor 到达 `since` 边界三种路径；前两者必须将 durable task 置为
failed 并保留 pagination 错误，边界路径成功且落库一根 candle。修复前后未发现
生产实现差异，故映射保持 `partial`：Go 使用可注入 `HistoricalKLineSyncer/source`
与 futu provider，Rust 生产路径使用 yfinance helper 与 durable task，无法声称
同形 provider/progress seam。

映射、reuse 与 Parity anchor 已同步；未填充 receiptDigest。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(production_helper_sync_rejects_broken_pagination_cursors)'`（1/1 passed）；`audit_test_parity.py --write-report`、`parity_anchor_reconcile.py`、JSON 校验、`cargo fmt --all -- --check` 与 `git diff --check` 通过；strict 审计仍保留历史 evidence/receipt 缺口。

## 2026-09-27 P1：Calendar health strict evidence 收口

本批不改生产实现，复核 3 条已有 `function_exact` 的高风险 calendar 映射：
`TestManagerProbeMarksEmptyParsesUnhealthy`、`TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts`
与 `TestManagerProbeRecoveryClearsCurrentFetchError`。逐项对照 Go 的 zero-schedule
structure-changed、health/alert fingerprint、失败后成功恢复清空 error/failure state
断言，Rust `manager_lifecycle` 对应测试的断言范围一致；三条均已有单引用 reuse 与
Parity anchor，补齐 `assertionCoverage.source=reviewed` 及 receipt。

验证：`env NEXTEST_EXPERIMENTAL_LIBTEST_JSON=1 node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar --test manager_lifecycle --locked --message-format libtest-json-plus --no-fail-fast -E 'test(probe_treats_an_empty_parse_as_structure_changed_unhealthy) or test(refresh_treats_an_empty_parse_as_structure_changed_failure) or test(successful_probe_recovery_clears_the_recorded_fetch_failure)'`（3/3 passed）；receipt `sha256:27d4c070d9bbff94444923e57d51a73b4f62069b70fc6dd5fe357382c0e87bcc`；`audit_test_parity.py --write-report`、`parity_anchor_reconcile.py`、JSON 校验、`cargo fmt --all -- --check` 与 `git diff --check` 通过。strict gap 由 3927 降至 3921，仍不宣称严格审计完成。

## 2026-09-27 P1：Provider retry exhaustion 与 timer cancellation 对齐

范围：`internal/backtest/historical_source_test.go:341` / `TestHistoricalProviderRetryExhaustionAndTimerCancellation`。

先红后修：新增 helper HTTP 夹具，固定 transient 503 在四次尝试后返回最终错误，并在第一次退避期间取消 durable sync task；修复前取消测试因固定 `sleep` 等待完整退避而超时。生产 Futu/helper 两条重试路径改用共享的取消感知退避轮询，取消在退避窗口内返回 `sync cancelled`，不改变四次尝试与重试计数语义。新增 Rust 证据为 `helper_retry_backoff_honors_cancelled_task_promptly` 与 `helper_retry_exhaustion_attempts_four_times`，两条均写入同一 Go `Parity:` anchor。

映射从 `partial` 升为 `function_exact`；两个 Rust evidence、单引用 reuse、reviewed assertion 与 receipt 已同步。定向 nextest 2/2 通过，receipt `sha256:26dc42121e439058156a76cfe41acd1d0c041eb7104b2f320d99861e119019dc`。本批最新只读审计为 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；anchor `1792/1746/0/0/46`；strict 仍真实失败 3921 个历史 evidence/receipt gaps，未宣称整体严格审计完成。

验证：`cargo fmt --all -- --check`、`node scripts/quality/check-workspace-architecture.mjs`、定向 `cargo-nextest` 2/2、`pnpm run check:rust`（workspace nextest 3510/3510 passed、2 skipped，7 类 compatibility replay 全部 passed）、`audit_test_parity.py --write-report`、`parity_anchor_reconcile.py` 与 JSON 校验均通过；`audit_test_parity.py --strict` 继续保留 3921 个历史 evidence/receipt 缺口。
