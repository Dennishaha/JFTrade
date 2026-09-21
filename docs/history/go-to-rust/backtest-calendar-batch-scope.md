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
