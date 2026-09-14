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
