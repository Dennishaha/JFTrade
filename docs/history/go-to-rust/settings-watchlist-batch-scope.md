# Settings、Watchlist 领域对齐批次

本批按 `Go 文件路径:行号:测试名` 复合键核对设置服务与 Futu/watchlist
边界，优先处理 provider 选择回滚、唯一字段所有权、输入校验和速率限制。
证据只引用真实 Rust 测试函数；测试装配或断言范围不同的条目保持
`[~]`/`partial`，不把相近 helper 当成完整覆盖。

| 状态 | Go 测试 | Rust 证据 | 差异结论 | 精确验证 |
| --- | --- | --- | --- | --- |
| [~] | `internal/settings/market_data_test.go:56:TestMarketDataProviderSettingsNormalizeAndApply` | `jftrade-settings::tests::provider_normalization_matches_current_go_defaults` | Rust 覆盖 provider 规范化、默认值、保存和非法输入；缺 Go 的 side-effect callback/idempotent restart 断言。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(provider_normalization_matches_current_go_defaults)'` |
| [~] | `internal/settings/market_data_test.go:203:TestMarketDataProviderRuntimeRollback` | `jftrade-settings::tests::active_failure_rolls_back_but_backtest_failure_never_persists` | Rust 覆盖 activation failure 后 active/backtest 持久值恢复；Go 只测 active provider，错误类型/返回值组合不同。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(active_failure_rolls_back_but_backtest_failure_never_persists)'` |
| [~] | `internal/settings/service_managed_accounts_test.go:13:TestServiceCreateManagedAccountNormalizesClientFields` | `jftrade-settings::managed_account_validation::create_account_clears_client_owned_identity_and_timestamps` | Rust 断言 server-owned id/timestamps 清空和 accountId trim；Go 还检查 backing array 与 fake store 形状。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(create_account_clears_client_owned_identity_and_timestamps)'` |
| [x] | `internal/settings/service_managed_accounts_test.go:111:TestServiceCreateManagedAccountRejectsBlankAccountID` | `jftrade-settings::managed_account_validation::blank_account_id_is_rejected_before_persistence` | 函数级等价：空 accountId 返回同一错误消息，且不会触发持久化。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(blank_account_id_is_rejected_before_persistence)'` |
| [~] | `internal/watchlist/futu/source_boundaries_test.go:137:TestFutuSnapshotRemainingProviderRateLimitAndSplitPaths` | `jftrade-integration-futu::test_opend_trade_read_client_burst_and_10th_call_preemption` | Rust 覆盖真实 framed OpenD rate-limit preemption；Go 还覆盖 split、nil provider 与 invalid item。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --test trade_session_gcra_rate_limit_challenge -E 'test(test_opend_trade_read_client_burst_and_10th_call_preemption)'` |
| [x] | `internal/watchlist/quote_preview_boundaries_test.go:37:TestQuoteCacheAndImportHelpersKeepAbsentDataExplicit` | `jftrade-watchlist::tests::test_quote_cache_and_import_helpers_keep_absent_data_explicit` | 已有函数级证据：limit、group trimming/dedup 和 absent data 边界逐项断言。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist -E 'test(test_quote_cache_and_import_helpers_keep_absent_data_explicit)'` |

## 未完成边界

Futu remote watchlist reader 的重名组 ambiguous/cache/fresh 读取、订阅配额满时
delayed fallback、extended-session quote 选择及时区未知值保护，目前没有独立 Rust
测试入口，继续保持 `missing`。Settings 的并发 security save、listener 失败后
完整 rollback 也不能由 provider rollback 证据替代。

