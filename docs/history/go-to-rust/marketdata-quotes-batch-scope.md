# MarketData、Quote、Provider 领域对齐批次

本批范围固定为 Go 基线中的 `internal/marketdata/cache_test.go` 与
`internal/marketdata/broker_candles_test.go` 两个文件，共 10 条复合键
（Go 路径、起始行号、测试名）。逐条阅读 Go 断言并对照 Rust 实际测试函数后，
没有任何条目满足全断言等价，因此统一保留为 `[~]`/`partial`，不把“存在同名
Parity 函数”当作覆盖完成。

| Go 测试范围 | Rust 证据 | 结论 |
| --- | --- | --- |
| `cache_test.go:12` dedupe/promotion/inherit | `cache_boundaries::test_cache_deduplicates_promotes_and_inherits` | 同价 promotion 有证据；trade 字段继承、observedAt 保留等断言缺失 |
| `cache_test.go:76` freshness/retention/max | `cache_boundaries::test_cache_freshness_retention_and_maximum` | fresh/stale 有证据；retention 截断、最大容量和 AllFresh 缺失 |
| `cache_test.go:103` 跨交易日 extended session | `cache_extended_sessions_parity::test_cache_does_not_inherit_extended_sessions_across_trading_days` | trading_date/session/pre/after 有证据；ExtendedHours、overnight、book/volume 缺失 |
| `cache_test.go:138` after-hours regular-close promotion | `cache_extended_sessions_parity::test_cache_promotes_us_regular_close_when_after_hours_trade_arrives` | close promotion 有证据；JSON 序列化与后续 tick 保留缺失 |
| `cache_test.go:185` 同价新 extended quote | `cache_extended_sessions_parity::test_cache_retains_extended_quote_when_price_is_unchanged` | after-market quote 有证据；样本计数、时间及 close 上下文缺失 |
| `broker_candles_test.go:12` strict page projection | `product_market_data_candle_pagination_tests::test_broker_k_line_candles_response_projects_strict_page` | 页大小/分页标记有证据；具体 wire 值未锁定 |
| `broker_candles_test.go:51` terminal/bounded pages | `...::test_broker_k_line_candles_response_handles_terminal_and_bounded_pages` | 仅 terminal 分支，bounded 未覆盖 |
| `broker_candles_test.go:77` invalid provider rows | `...::test_broker_k_line_candles_response_rejects_invalid_provider_rows` | 仅 close 数值校验，Go 的其他错误矩阵缺失 |
| `broker_candles_test.go:109` helper classification | `...::test_broker_k_line_helpers_classify_sessions_and_numbers` | 时间/数字解析有证据；regular/after session 分类缺失 |
| `broker_candles_test.go:137` pagination metadata | `...::test_broker_k_line_pagination_rejects_invalid_bounded_and_paged_metadata` | 两个错误场景有证据；bounded cursor 等边界缺失 |

验证命令按单函数记录在 `manual-test-mappings.json`；本批使用
`jftrade-marketdata` 与 `jftrade-engine` 的 nextest wrapper。后续补测应先补齐
缺失断言，再将对应条目从 `partial` 升级为 `function_exact`/`[x]`。

## 第二批：runtime / sidecar / health + Futu candle session 标注

批次范围：`internal/app/apiserver/marketdataapp/` 的 `runtime_test.go`（22）、
`runtime_health_test.go`（11）、`sidecar_process_test.go`（9）、`market_http_test.go`（18）、
`provider_boundaries_test.go`（5）、`provider_test.go`（3）、`query_test.go` 等边界文件。
逐条对照后：18 条找到全断言等价的 Rust 测试（`[x]`/`function_exact`），14 条为
`[~]`/`partial`（有部分证据但断言集合不同），10 条为 `[~]`/`boundary`
（provider lease 引用计数旧实例、sidecar 进程清理重试、nil-receiver、deferred cleanup
等只在 Go/Wails 进程模型内成立的边界）。

### 真实功能差异与修复（P0：会话标注正确性）

- **复现条件**：Futu provider、US 市场、`period=1d`（或任意非 US 市场）请求
  `GET /api/v1/market-data/candles/US/AAPL?period=1d`；此前 Rust 会对每根 K 线
  固定写入 `"session": "regular"`。
- **Go 预期行为**：`ShouldAnnotateHistoricalKLineSession` 只在 US 且 intraday
  period 时标注 session；daily 与非 US 市场完全不输出 `session` 字段，且
  `meta.session` 仅在发生逐根标注时出现。
- **修复位置**：`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`
  （`read_candles` Futu 分支）新增 `annotate_session` 判定与
  `futu_candle_session` 辅助函数：用 `jiff` 解析 `at`，再经
  `CalendarManager::classify_session` 映射 `pre`/`regular`/`after`/`overnight`；
  未配置日历时返回 `None`（fail-closed，不臆造 session）。
- **回归测试**：
  `crates/jftrade-engine/src/product_market_data_candle_pagination_tests.rs::candle_route_only_annotates_sessions_for_us_intraday_history`
  与 `...::us_intraday_futu_candles_carry_calendar_resolved_session_labels`
  （断言 pre@13:00Z / regular@15:00Z / after@22:00Z 标签，daily 不输出 session）。

### 结构整理

`product_production_ports_market_data_quote_reads.rs` 因新增逻辑超过 800 行生产文件
上限，按既有 `#[path = ...] mod` 模式把 Futu `KLineQueryWindow` / `effectivePeriodSeconds`
/ `parseFutuTimeToTS` 辅助函数与其两个 `query_test.go` 回归测试拆分到
`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads_futu.rs`
（155 行），主文件回到 710 行；映射条目 `rust_entry` 同步更新到新路径。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`
（1110 passed）、`jftrade-marketdata`（51 passed）、
`jftrade-integration-marketdata-helper --all-targets`（19 passed）、
`pnpm run check:rust:architecture`、`python3 scripts/compatibility/audit_test_parity.py`。
