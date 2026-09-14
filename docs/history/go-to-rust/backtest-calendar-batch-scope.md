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
