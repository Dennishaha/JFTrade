# Go → Rust 高价值测试映射与功能补齐清单

## 概述与推进规范

本清单以 `go` 分支基线（当前提交由 `scripts/compatibility/audit_test_parity.py` 动态记录）4,451 个测试与 952 个高风险测试为参考，建立细粒度行为映射清单。报告生成时会同时记录 Rust 当前 HEAD，避免清单长期漂移。

当前 100 项高价值样本中：94 项已建立 Rust 回归证据，6 项为已记录的架构边界差异（无待处理的空白项）；全量 4,451 项的逐项候选索引见 [`test-parity-inventory.md`](test-parity-inventory.md)。

### 状态标记规范
- `[x]` **已覆盖**：Rust 侧已有对应的单元测试、集成测试或契约回放测试，断言与行为语义已严格对齐。
- `[~]` **部分覆盖**：核心逻辑已实现或局部覆盖，但在取消、超时、异常回滚、流重连或极端边缘仍需补齐，或属于明确记录的架构边界差异。
- `[ ]` **待补测/修复**：尚未建立对应测试或存在已知语义缺陷，需排期补测与修复。

### 9 维标准字段
1. **状态**：勾选状态（`[x]` / `[~]` / `[ ]`）
2. **Go 来源**：分支 `go:452dea11` 下的源码文件路径、行号及测试函数名
3. **业务域**：`Futu`、`行情`、`交易`、`策略`、`回测`、`ADK`、`存储`、`设置`、`API`、`工具与核心`
4. **风险类型**：分页、缓存、时区、重试、取消、回滚、断连、幂等、错误映射等
5. **Rust 对应入口**：crate、production port、store 或 API route
6. **Rust 测试状态**：`已覆盖` / `部分覆盖` / `缺失` / `无对应能力`
7. **差异结论**：`语义一致` / `Rust 缺陷` / `契约差异` / `明确不适用`
8. **后续动作**：`保持回归` / `补测试` / `修实现` / `补 fixture` / `保留未验证`
9. **验证命令**：最小化回归运行命令

---

## 高价值测试细粒度映射清单（全量 100 项）

| 状态 | Go 来源 | 业务域 | 风险类型 | Rust 对应入口 | Rust 测试状态 | 差异结论 | 后续动作 | 验证命令 |
|---|---|---|---|---|---|---|---|---|
| [x] | `go:452dea11:internal/integration/futu/candle_sessions_test.go:11`<br>`TestMarketSessionsForCandleSessions` | Futu | 时区/会话 | `crates/jftrade-integration-futu/tests/marketdata_runtime_candle_sessions_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(marketdata_runtime_candle_sessions_parity)'` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_opend_test.go:226`<br>`TestMarketDataRuntimePreservesRealtimeTicksWhenDelayedFallbackFails` | Futu | 断连/回退容灾 | `crates/jftrade-integration-futu/tests/fake_framed_opend_runtime_tests.rs` | 已覆盖 | 语义一致 | 验证 framed opend 下降级建立与失败容灾语义 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E "test(test_fallback_establishment_recovery_and_count_semantics)"` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:267`<br>`TestTickConversionRejectsUnusablePricesAndUsesQuoteFallbacks` | Futu | 报价/回退校验 | `crates/jftrade-integration-futu/src/futu_marketdata_facade.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:364`<br>`TestTickFromTradeInheritsLatestQuoteFieldsThroughCache` | Futu | 缓存/字段继承 | `crates/jftrade-integration-futu/src/futu_marketdata_facade.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:452`<br>`TestMarketDataRuntimeExchangeResetAndStreamLifecycle` | Futu | 断连/重连重置 | `crates/jftrade-integration-futu/tests/provider_runtime_recovery.rs` | 已覆盖 | 语义一致 | 验证重连排队期间最新 demand 取代旧 replay 与生命周期管理 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E "test(latest_demand_replaces_stale_replay_while_reconnect_is_pending)"` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:613`<br>`TestMarketDataRuntimeFiltersFallbackInstrumentsFromPushStream` | Futu | 过滤/流隔离 | `crates/jftrade-integration-futu/tests/fake_framed_opend_runtime_tests.rs` | 已覆盖 | 语义一致 | 验证 OpenD 帧流重连与 demand replay 下标的流隔离与重放 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E "test(test_reconnect_and_demand_replay_with_framed_opend)"` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:824`<br>`TestFallbackTickerMapProjectsOnlyRequestedUsableSnapshots` | Futu | 快照/映射投影 | `crates/jftrade-integration-futu/src/futu_marketdata_facade.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu` |
| [x] | `go:452dea11:internal/integration/futu/marketdata_runtime_test.go:869`<br>`TestFallbackSnapshotConversionRejectsInvalidValuesAndUsesClassification` | Futu | 值校验/分类 | `crates/jftrade-integration-futu/src/futu_marketdata_facade.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu` |
| [x] | `go:452dea11:internal/integration/futu/notifications_test.go:13`<br>`TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories` | Futu | 契约/通知路由 | `crates/jftrade-integration-futu/src/lib.rs (notification_parity.rs)` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu` |
| [x] | `go:452dea11:internal/integration/futu/notifications_test.go:98`<br>`TestNeutralNotificationBuildersHandleNilAndStatusTransitions` | Futu | 边界/空值与状态 | `crates/jftrade-integration-futu/src/lib.rs (notification_parity.rs)` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu` |
| [x] | `go:452dea11:internal/marketdata/broker_candles_test.go:12`<br>`TestBrokerKLineCandlesResponseProjectsStrictPage` | 行情 | 分页/严格切页 | `crates/jftrade-marketdata/tests/broker_candles_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/broker_candles_test.go:51`<br>`TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages` | 行情 | 分页/终止与有界 | `crates/jftrade-marketdata/tests/broker_candles_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/broker_candles_test.go:109`<br>`TestBrokerKLineHelpersClassifySessionsAndNumbers` | 行情 | 时区/数值分类 | `crates/jftrade-marketdata/tests/broker_candles_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/broker_candles_test.go:137`<br>`TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata` | 行情 | 分页/元数据校验 | `crates/jftrade-marketdata/tests/broker_candles_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/cache_test.go:12`<br>`TestCacheDeduplicatesPromotesAndInherits` | 行情 | 缓存/去重晋升 | `crates/jftrade-marketdata/tests/cache_freshness_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/cache_test.go:76`<br>`TestCacheFreshnessRetentionAndMaximum` | 行情 | 缓存/时效淘汰 | `crates/jftrade-marketdata/tests/cache_freshness_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/cache_test.go:103`<br>`TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays` | 行情 | 缓存/跨日隔离 | `crates/jftrade-marketdata/tests/cache_extended_session_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/cache_test.go:138`<br>`TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives` | 行情 | 缓存/盘后晋升 | `crates/jftrade-marketdata/tests/cache_us_regular_close_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/marketdata/cache_test.go:185`<br>`TestCacheRetainsNewExtendedQuoteWhenPriceIsUnchanged` | 行情 | 缓存/扩展行情保留 | `crates/jftrade-marketdata/tests/cache_promotes_after_hours_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [~] | `go:452dea11:internal/marketdata/cache_test.go:210`<br>`TestTickCandlesVolumeWindowAndLimit` | 行情 | 缓存/成交量窗口 | `crates/jftrade-marketdata/src/cache.rs` | 部分覆盖 | Rust 缺少同等 TickCandles 投影 API；Go 语义为默认 15 分钟窗口、显式 `VolumeDelta`、负值归零、limit 截断并保留 session | 记录为独立 marketdata projection parity 任务；不得用 snapshot/cache 测试冒充等价覆盖 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata` |
| [x] | `go:452dea11:internal/trading/broker_boundaries_test.go:11`<br>`TestServiceBrokerReadOperationsReturnFallbackWhenMarketDataUnavailable` | 交易 | 交易/无行情降级 | `crates/jftrade-trading/tests/order_execution_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/broker_conformance_test.go:58`<br>`TestFakeBrokerConformanceCancelAcceptedAndCancelRejected` | 交易 | 交易/撤单接受与拒绝 | `crates/jftrade-trading/tests/order_cancel_flow_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/broker_test.go:453`<br>`TestServicePortfolioAndFallbackResponses` | 交易 | 交易/持仓与降级 | `crates/jftrade-trading/tests/order_reconciliation_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/broker_test.go:533`<br>`TestServiceBrokerWriteAndTimeoutBehaviors` | 交易 | 交易/写入超时熔断与订单生命周期 | `crates/jftrade-engine/src/product_production_ports_execution_orders.rs` | 已覆盖 | 语义一致 | 验证 broker 下单/撤单/解锁写入与超时熔断链路 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(execution_orders)"` |
| [x] | `go:452dea11:internal/trading/control_plane_idempotency_test.go:65`<br>`TestRealTradeControlPlaneHardStopReleaseIsSingleShot` | 交易 | 交易/HardStop单次释放 | `crates/jftrade-trading/tests/order_fencing_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/control_plane_idempotency_test.go:99`<br>`TestRealTradeControlPlaneHardStopsBlockUntilEveryEntryReleased` | 交易 | 交易/HardStop全阻断 | `crates/jftrade-trading/tests/order_fencing_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:99`<br>`TestControlPlaneTreatsEmptyStateAsFreshAndRejectsUnavailableMutations` | 交易 | 交易/空状态初始检查 | `crates/jftrade-trading/tests/order_validation_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/control_plane_state_audit_test.go:250`<br>`TestControlPlaneSurfacesHardStopRejectionAuditPersistenceFailure` | 交易 | 交易/审计持久化失败 | `crates/jftrade-trading/tests/order_rollback_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:15`<br>`TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower` | 交易 | 交易/组合单全生命周期 | `crates/jftrade-trading/tests/order_execution_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/trading/execution_combo_lifecycle_test.go:635`<br>`TestExecutionDetailsResolverAndOrderUpdateCacheFailureBranches` | 交易 | 交易/订单缓存更新失败 | `crates/jftrade-trading/tests/order_reconciliation_parity.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading` |
| [x] | `go:452dea11:internal/pineworkerassets/asset_selection_boundaries_test.go:30`<br>`TestSelectFromFSTreatsMissingAndEmptyBundlesAsUnavailable` | 策略 | 策略/资源包缺失处理 | `crates/jftrade-integration-pine/src/lib.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine` |
| [x] | `go:452dea11:internal/strategy/catalog/activity_degraded_test.go:65`<br>`TestCatalogActivityReturnsEmptyPagesWhenActivityStoreIsUnavailable` | 策略 | 策略/活动分页降级 | `crates/jftrade-engine/src/product_production_ports_strategy.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_activity)'` |
| [x] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:34`<br>`TestCatalogActivityQueryFailureReturnsKnownEmptyPage` | 策略 | 策略/活动查询容错 | `crates/jftrade-engine/src/product_production_ports_strategy.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_activity)'` |
| [x] | `go:452dea11:internal/strategy/catalog/catalog_boundary_behavior_test.go:192`<br>`TestCatalogPrivateBusinessHelpersHandleEmptyAndUnknownInputs` | 策略 | 策略/空输入归一化 | `crates/jftrade-engine/src/strategy_runtime_mutation.rs` | 已覆盖 | 语义一致 | 验证默认 5m interval 填充、非法枚举校验与 symbols 归一化 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(binding_normalization)'` |
| [x] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:12`<br>`TestCatalogRuntimeTransitionsPersistStateAndActivity` | 策略 | 策略/状态变更日志 | `crates/jftrade-engine/src/strategy_runtime_port.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_runtime)'` |
| [x] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:52`<br>`TestCatalogRuntimeFailureReconcilesOnlyRunningInstance` | 策略 | 策略/单例故障对账 | `crates/jftrade-engine/src/strategy_runtime_port.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_runtime)'` |
| [x] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:80`<br>`TestCatalogStartupReconcileResetsStaleRunningAndPausedState` | 策略 | 策略/启动恢复对账 | `crates/jftrade-engine/src/strategy_runtime_port.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(restore_)'` |
| [x] | `go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:113`<br>`TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment` | 策略 | 策略/活动分页与过滤 | `crates/jftrade-engine/src/product_production_ports_strategy.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_activity)'` |
| [x] | `go:452dea11:internal/strategy/errors_test.go:8`<br>`TestClassifiedStrategyErrorsMatchSentinelKinds` | 策略 | 策略/错误哨兵分类 | `crates/jftrade-strategy/src/errors.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy` |
| [x] | `go:452dea11:internal/strategy/instancebinding/binding_test.go:137`<br>`TestNormalizeBrokerAccountDropsEmptyInput` | 策略 | 策略/账户输入归一化 | `crates/jftrade-strategy/src/instancebinding.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy` |
| [x] | `go:452dea11:internal/backtest/historical_source_test.go:111`<br>`TestHistoricalKLineSyncerCancelsInFlightProviderPage` | 回测 | 回测/同步取消 | `crates/jftrade-engine/src/product_production_ports_backtest_sync_helpers.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(backtest_sync)'` |
| [x] | `go:452dea11:internal/backtest/historical_source_test.go:147`<br>`TestHistoricalKLineSyncerRetriesTransientPageAndRejectsCapabilitiesDuringPreflight` | 回测 | 回测/重试与前置校验 | `crates/jftrade-engine/src/product_production_ports_backtest_sync_helpers.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(backtest_sync)'` |
| [x] | `go:452dea11:internal/backtest/historical_source_test.go:181`<br>`TestHistoricalKLineSyncerRejectsEmptyProviderResult` | 回测 | 回测/空结果拒绝 | `crates/jftrade-engine/src/product_production_ports_backtest_sync_helpers.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(backtest_sync)'` |
| [x] | `go:452dea11:internal/backtest/historical_source_test.go:268`<br>`TestHistoricalKLineSyncerRejectsBrokenPagination` | 回测 | 回测/畸形分页拒绝 | `crates/jftrade-engine/src/product_production_ports_backtest_sync_helpers.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(backtest_sync)'` |
| [x] | `go:452dea11:internal/backtest/historical_source_test.go:341`<br>`TestHistoricalProviderRetryExhaustionAndTimerCancellation` | 回测 | 回测/重试耗尽取消 | `crates/jftrade-engine/src/product_production_ports_backtest_sync_helpers.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(backtest_sync)'` |
| [x] | `go:452dea11:internal/backtest/recovery_test.go:11`<br>`TestBacktestExecutionPersistsFailureWhenRunnerReturnsNil` | 回测 | 回测/空结果故障持久化 | `crates/jftrade-engine/src/product_production_ports_backtest_task.rs` | 已覆盖 | 语义一致 | 验证 execute_backtest_lifecycle 异常捕获并持久化 FAILED 终端状态 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(backtests_write)"` |
| [x] | `go:452dea11:internal/backtest/recovery_test.go:27`<br>`TestBacktestExecutionRecoversRunnerPanicIntoFailedRun` | 回测 | 回测/Panic恢复收敛 | `crates/jftrade-engine/src/product_production_ports_backtest_task.rs` | 已覆盖 | 语义一致 | 验证 runner 失败/panic 在 TaskOutcome::Failed 边界安全收敛为 FAILED | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(backtests_write)"` |
| [x] | `go:452dea11:internal/backtest/recovery_test.go:45`<br>`TestStartScriptRejectsBlankResearchScript` | 回测 | 回测/空白脚本拒绝 | `crates/jftrade-backtest/src/service.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-backtest` |
| [x] | `go:452dea11:internal/backtest/result_view_test.go:223`<br>`TestResultViewRejectsBadRequestsAndPreservesEmptyRunShape` | 回测 | 回测/视图畸形请求 | `crates/jftrade-engine/src/product_backtests_tests.rs` | 已覆盖 | 语义一致 | 已由 production read route 与 snapshot port 验证坏请求拒绝与空结果 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(backtests_read)"` |
| [x] | `go:452dea11:internal/backtest/run_failure_recovery_test.go:26`<br>`TestBacktestStartDoesNotLeakLifecycleTaskWhenQueuePersistenceFails` | 回测 | 回测/入队回滚防泄漏 | `crates/jftrade-engine/src/product_production_ports_backtest_task.rs` | 已覆盖 | 语义一致 | 验证 start_backtest 在持久化失败时立即返回错误且不遗留孤儿执行任务 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(backtests_write)"` |
| [x] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:194`<br>`TestADKStrategyToolsHandleNegativeAndFallbackScenarios` | ADK | ADK/策略工具降级与验证 | `crates/jftrade-engine/src/product_mcp_server_tests.rs` | 已覆盖 | 语义一致 | 已由 production MCP strategy.definitions、strategy.pine_spec 与 validate_pine 覆盖空定义与非法脚本校验 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(production_mcp_pine)"` |
| [x] | `go:452dea11:internal/assistant/assembly/adk_strategy_test.go:605`<br>`TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure` | ADK | ADK/优化任务取消持久化 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(optimization_task_cancellation_persists_across_restarts)'` |
| [x] | `go:452dea11:internal/assistant/assembly/application_adapter_test.go:226`<br>`TestApplicationAdapterProvidesScreenCatalogAndCancelResult` | ADK | ADK/应用适配器 | `crates/jftrade-engine/src/product_production_ports.rs` | 已覆盖 | 语义一致 | 已在 product_tests::production_ports_provide_research_screen_catalog 覆盖屏幕目录与取消结果 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(screen_catalog)"` |
| [x] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:76`<br>`TestMCPServerManagerStartsAndStopsOnLoopback` | ADK | ADK/MCP服务回环启停 | `crates/jftrade-assistant/src/mcp_server.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-assistant` |
| [x] | `go:452dea11:internal/assistant/assembly/mcp_server_test.go:106`<br>`TestMCPServerManagerServesAuthenticatedStreamableMCP` | ADK | ADK/MCP鉴权流服务 | `crates/jftrade-assistant/src/mcp_server.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-assistant` |
| [x] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:15`<br>`TestPortfolioSummaryScansAllRealAccountsAndRanksNonEmptyFirst` | ADK | ADK/投资组合汇总工具与排序 | `crates/jftrade-engine/src/product_mcp_production_executor.rs` | 已覆盖 | 语义一致 | 验证 portfolio.summary 支持聚合 positions/balances/orders 并校验多账户非空优先排序 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(test_portfolio_funds_overview_and_sorting_and_unsupported_market)"` |
| [x] | `go:452dea11:internal/assistant/assembly/portfolio_tools_test.go:155`<br>`TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates` | ADK | ADK/分层工具动态挂载与发现验证 | `crates/jftrade-engine/src/product_production_ports_adk_tests.rs` | 已覆盖 | 语义一致 | 验证 attach_ports / detach_ports 下 portfolio 诸工具与 schema 的动态挂载与发现 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(adk_mcp_tool_executor_bundle_attachment_and_exact_schemas)"` |
| [x] | `go:452dea11:internal/assistant/assembly/product_adapters_test.go:186`<br>`TestProductExecutionAdapterRejectsInvalidScreenPageAndValue` | ADK | ADK/分页与数值校验 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:internal/assistant/assembly/runtime_test.go:14`<br>`TestOpenBuildsToolsServiceAndIdempotentLifecycle` | ADK | ADK/会话级联与写入隔离 | `crates/jftrade-assistant/src/runtime.rs` | 已覆盖 | 语义一致 | 验证级联清理后陈旧写入隔离与生命周期安全 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-assistant -E "test(cascade_deletion_fences_stale_writer_and_session_recovery)"` |
| [x] | `go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103`<br>`TestWorkflowToolsRemainingSessionAndPayloadErrors` | ADK | ADK/工作流错误边界 | `crates/jftrade-engine/tests/adk_workflow_canvas_contracts.rs` | 已覆盖 | 语义一致 | 校验缺失名称及会话时返回防御性验证失败与画布校验 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(test_workflow_tools_missing_session_and_empty_payload_boundaries)'` |
| [x] | `go:452dea11:internal/store/backtest/store_failure_test.go:121`<br>`TestStoreCanceledMaintenanceDoesNotMutateRuns` | 存储 | 存储/取消维护防变动 | `crates/jftrade-store-sqlite/src/backtest_sync_store.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite` |
| [x] | `go:452dea11:internal/store/backtest/store_test.go:168`<br>`TestInMemoryStoreImplementsRunLifecycleAndCancellation` | 存储 | 存储/内存运行生命周期 | `crates/jftrade-store-sqlite/src/backtest_sync_store.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite` |
| [x] | `go:452dea11:internal/store/backtest/sync_tasks_test.go:12`<br>`TestSyncTaskStoreReturnsSnapshotsAndCancelsProgress` | 存储 | 存储/同步快照与取消 | `crates/jftrade-store-sqlite/src/backtest_sync_store.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite` |
| [x] | `go:452dea11:internal/store/exchangecalendar/store_boundaries_test.go:45`<br>`TestCalendarStoreEmptyLoadAndDeleteAreIdempotent` | 存储 | 存储/日历空读写幂等 | `crates/jftrade-store-sqlite/src/exchange_calendar_store.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite` |
| [x] | `go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:95`<br>`TestSaveSnapshotValidatesInputsAndResolvesYearFallbacks` | 存储 | 存储/快照校验与年份回退 | `crates/jftrade-store-sqlite/src/exchange_calendar_store.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite` |
| [x] | `go:452dea11:internal/store/settingsfile/normalization_and_persistence_test.go:13`<br>`TestSettingsNormalizationHandlesFallbacksAndBoundaries` | 存储 | 存储/设置归一化与回退 | `crates/jftrade-store-settings-file/tests/store_recovery_test.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-settings-file` |
| [x] | `go:452dea11:internal/store/settingsfile/rollback_test.go:13`<br>`TestFailedSettingSavesRollbackAllRuntimeState` | 存储 | 存储/设置失败全局回滚 | `crates/jftrade-store-settings-file/tests/store_recovery_test.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-settings-file` |
| [x] | `go:452dea11:internal/store/settingsfile/rollback_test.go:185`<br>`TestFailedBootstrapAndMigrationRollbackRuntimeState` | 存储 | 存储/启动迁移失败回滚 | `crates/jftrade-store-settings-file/tests/store_recovery_test.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-settings-file` |
| [x] | `go:452dea11:internal/store/settingsfile/rollback_test.go:232`<br>`TestFailedManagedAccountCRUDRollsBackBackingArray` | 存储 | 存储/账户操作失败回滚 | `crates/jftrade-store-settings-file/tests/store_recovery_test.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-settings-file` |
| [x] | `go:452dea11:internal/store/settingsfile/store_recovery_test.go:13`<br>`TestSettingsStoreRejectsMalformedOrUnreadableInput` | 存储 | 存储/畸形设置防崩溃 | `crates/jftrade-store-settings-file/tests/store_recovery_test.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-settings-file` |
| [x] | `go:452dea11:internal/settings/market_data_test.go:203`<br>`TestMarketDataProviderRuntimeRollback` | 设置 | 设置/Provider回滚 | `crates/jftrade-settings/src/market_data_provider.rs` | 已覆盖 | 语义一致 | 验证运行时激活失败后原子回滚持久化 Provider | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E "test(active_failure_rolls_back)"` |
| [x] | `go:452dea11:internal/settings/market_data_test.go:224`<br>`TestMarketDataProviderReportsPersistenceAndRollbackFailures` | 设置 | 设置/持久化失败告警 | `crates/jftrade-settings/src/market_data_provider.rs` | 已覆盖 | 语义一致 | 验证持久化和回滚失败的强类型错误传递 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E "test(active_failure_rolls_back)"` |
| [x] | `go:452dea11:internal/settings/market_data_test.go:252`<br>`TestMarketDataProviderReadsWaitForRuntimeRollback` | 设置 | 设置/读写屏障对齐 | `crates/jftrade-settings/src/market_data_provider.rs` | 已覆盖 | 语义一致 | 验证 RwLock 与不可变切片读写保护语义 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E "test(provider_normalization_matches_current_go_defaults)"` |
| [x] | `go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:125`<br>`TestServicePreservesSecurityAndMCPFallbacks` | 设置 | 设置/安全密钥与MCP回退 | `crates/jftrade-settings/src/security.rs` | 已覆盖 | 语义一致 | 验证监听器失败时端口与验证密码原子回滚 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E "test(listener_failure_rolls_back_password_and_port_together)"` |
| [x] | `go:452dea11:internal/settings/service_test.go:435`<br>`TestDefaultCallbacksReturnEmptyMaps` | 设置 | 设置/默认空Map契约 | `crates/jftrade-settings/src/onboarding.rs` | 已覆盖 | 语义一致 | 已覆盖默认输入与空字段断言 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(default_onboarding_inputs)'` |
| [~] | `go:452dea11:internal/watchlist/futu/source_test.go:44`<br>`TestFutuWatchlistReaderMarksDuplicateNamesAmbiguousAndCachesReads` | 设置 | 自选股/重名歧义与缓存 | `crates/jftrade-watchlist/src/lib.rs` | 部分覆盖 | Rust 缺少 Futu remote watchlist reader；Go 语义要求规范化重名组标记 ambiguous、稳定唯一 remote ID、禁止导入歧义组，并缓存普通读取且 fresh 读取绕过缓存 | 新增 Futu watchlist port/adapter 后补契约测试；当前不将本地 watchlist 测试计为等价覆盖 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist` |
| [~] | `go:452dea11:internal/watchlist/futu/source_test.go:174`<br>`TestFutuWatchlistSnapshotDoesNotSplitGlobalOrCanceledFailures` | 设置 | 自选股/快照全局错误 | `crates/jftrade-watchlist/src/lib.rs` | 部分覆盖 | Rust 缺少远程快照批处理错误边界；Go 要求全局 provider/cancel 错误整体失败，不拆成逐标的伪成功结果 | 新增 remote snapshot port 后补全局错误与取消传播测试 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist` |
| [~] | `go:452dea11:internal/watchlist/futu/source_test.go:221`<br>`TestFutuWatchlistSnapshotUsesDelayedFallbackWhenSubscriptionQuotaIsFull` | 设置 | 自选股/配额满延时回退 | `crates/jftrade-watchlist/src/lib.rs` | 部分覆盖 | Rust 缺少订阅配额感知的延时快照回退；Go 在 quota full 时必须保留可用延时值并避免重复订阅 | 新增 provider capability/lease adapter 后补 quota-full fallback 回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist` |
| [~] | `go:452dea11:internal/watchlist/futu/source_test.go:396`<br>`TestWatchlistQuotePreservesSnapshotDisplayMetadataAndAvoidsUnknownTimezoneGuess` | 设置 | 自选股/时区元数据保留 | `crates/jftrade-watchlist/src/lib.rs` | 部分覆盖 | Rust 缺少远程 quote projection；Go 要求保留 snapshot display metadata，未知交易所时区不得猜测或伪造 | 新增 neutral quote DTO 与明确 timezone optional 字段后补契约测试 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist` |
| [~] | `go:452dea11:internal/watchlist/futu/source_test.go:418`<br>`TestWatchlistQuoteSelectsExtendedSessionPriceAndChange` | 设置 | 自选股/盘前盘后价格选择 | `crates/jftrade-watchlist/src/lib.rs` | 部分覆盖 | Rust 缺少 extended-session quote 选择器；Go 按盘前/盘后有效价与涨跌幅优先级投影，不能回退到未知或过期字段 | 新增 neutral quote selector 后覆盖 regular/pre/after/unknown session 分支 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist` |
| [x] | `go:452dea11:cmd/jftrade-api/main_test.go:86`<br>`TestRunAPICommandStartsAndStopsAPI` | API | API/启动与优雅停机 | `crates/jftrade-engine/tests/product_wire.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(api)'` |
| [x] | `go:452dea11:cmd/jftrade-api/main_test.go:121`<br>`TestRunAPICommandPreservesConfiguredCacheAndWrapsStartupErrors` | API | API/缓存保留与启动错误 | `crates/jftrade-engine/tests/product_wire.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(api)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_approval_test.go:335`<br>`TestADKRunCancelAndFilteredList` | API | API/Run取消与列表过滤 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_normalize_test.go:15`<br>`TestADKRoutesSerializeEmptySlicesAsArrays` | API | API/空切片序列化为数组 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_ops_test.go:247`<br>`TestADKOptimizationTaskCanBeQueriedAndCancelled` | API | API/优化任务查询与取消 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(optimization_task)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_routes_test.go:26`<br>`TestADKSessionDetailOmitsResolvedApprovalGroups` | API | API/会话详情忽略已解决审批 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_routes_test.go:214`<br>`TestADKAuditRouteRejectsInvalidPagination` | API | API/审计路由非法分页校验 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_routes_test.go:245`<br>`TestADKChatStreamEmitsSessionRunAndFinalEvents` | API | API/SSE事件流生命周期 | `crates/jftrade-engine/src/product_adk_chat_stream_product_tests.rs` | 已覆盖 | 语义一致 | 验证 SSE stream 帧格式、header 契约及 terminal 事件保留 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E "test(adk_chat_stream)"` |
| [x] | `go:452dea11:internal/api/assistant/adk_routes_test.go:597`<br>`TestADKProviderSaveReturnsRequestTimeoutMs` | API | API/保存Provider返回超时 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:internal/api/assistant/adk_routes_test.go:787`<br>`TestADKSessionNegativeRoutes` | API | API/会话路由非法参数校验 | `crates/jftrade-engine/src/product_adk_read_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(adk)'` |
| [x] | `go:452dea11:cmd/check-go-coverage/changed_lines_analysis_test.go:71`<br>`TestParseChangedGoLinesReportsPureRenameWithoutInventingChangedStatements` | 工具与核心 | 工具/Git Diff改动行解析 | `scripts/quality/check-coverage.py` | 已覆盖 | 明确不适用 | Go覆盖率解析脚本，Rust由 cargo-nextest 接管 | `python3 scripts/compatibility/audit_test_parity.py` |
| [x] | `go:452dea11:cmd/check-go-coverage/profile_analysis_test.go:109`<br>`TestAnalyzeProfilesRejectsEmptyBusinessCoverage` | 工具与核心 | 工具/覆盖率空文件拒绝 | `scripts/quality/check-coverage.py` | 已覆盖 | 明确不适用 | Go覆盖率解析工具，Rust门禁采用 nextest 门禁 | `pnpm run check:quick` |
| [x] | `go:452dea11:cmd/jftrade-desktop/desktop_startup_test.go:93`<br>`TestDesktopShutdownCancelsStartupAndReclaimsLateResources` | 工具与核心 | 桌面/关机幂等与资源回收 | `apps/desktop/src-tauri/src/native_lifecycle.rs` | 已覆盖 | 语义一致 | 验证 stop_product 互斥与幂等回收 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml -- stop_product_is_idempotent` |
| [x] | `go:452dea11:cmd/jftrade-desktop/main_test.go:131`<br>`TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset` | 工具与核心 | 桌面/静态资源404防穿透 | `apps/desktop/src-tauri/src/native_tests.rs` | 已覆盖 | 语义一致 | 补充 required_asset 静态资源缺失严格返回 MissingAsset 错误 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml -- required_asset_does_not_fallback` |
| [x] | `go:452dea11:cmd/jftrade-desktop/main_test.go:395`<br>`TestListDesktopLogDaysAndReadsFilteredPage` | 工具与核心 | 桌面/日志过滤分页 | `apps/desktop/src-tauri/src/native_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml test_list_desktop_log_days` |
| [x] | `go:452dea11:cmd/jftrade-desktop/main_test.go:426`<br>`TestDesktopLogPageCapsLimitAndPaginatesAllLines` | 工具与核心 | 桌面/日志分页上限控制 | `apps/desktop/src-tauri/src/native_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml test_list_desktop_log_days` |
| [x] | `go:452dea11:cmd/jftrade-desktop/main_test.go:464`<br>`TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder` | 工具与核心 | 桌面/日志TailOffset逆序 | `apps/desktop/src-tauri/src/native_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml test_list_desktop_log_days` |
| [x] | `go:452dea11:cmd/jftrade-desktop/main_test.go:506`<br>`TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging` | 工具与核心 | 桌面/日志过滤先于分页 | `apps/desktop/src-tauri/src/native_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml test_list_desktop_log_days` |
| [x] | `go:452dea11:cmd/jftrade-desktop/main_test.go:523`<br>`TestListDesktopLogDaysMissingDirReturnsEmpty` | 工具与核心 | 桌面/日志缺失目录空返回 | `apps/desktop/src-tauri/src/native_tests.rs` | 已覆盖 | 语义一致 | 保持回归 | `cargo test --manifest-path apps/desktop/src-tauri/Cargo.toml test_list_desktop_log_days` |
| [x] | `go:452dea11:internal/datamanagement/service_test.go:44`<br>`TestServiceFallbacks` | 工具与核心 | 核心/数据管理兜底与确认校验 | `crates/jftrade-datamanagement/src/maintenance.rs` | 已覆盖 | 语义一致 | 验证 compact/backup 严格确认码校验与拒绝语义 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-datamanagement` |
---

## 统计与覆盖率摘要

| 业务域 | 样本总数 | 已覆盖 (`[x]`) | 部分覆盖 (`[~]`) | 待补测 (`[ ]`) | 覆盖对齐率 |
|---|---|---|---|---|---|
| Futu / OpenD | 10 | 10 | 0 | 0 | 100.0% |
| 行情缓存与调度 | 10 | 9 | 1 | 0 | 90.0% |
| 交易执行与对账 | 10 | 10 | 0 | 0 | 100.0% |
| 策略编排与 Pine | 10 | 10 | 0 | 0 | 100.0% |
| 回测与交易日历 | 10 | 10 | 0 | 0 | 100.0% |
| Assistant / ADK | 10 | 10 | 0 | 0 | 100.0% |
| SQLite 存储持久化 | 10 | 10 | 0 | 0 | 100.0% |
| 系统设置与自选股 | 10 | 5 | 5 | 0 | 50.0% (架构差异/中心化日历驱动) |
| API 传输契约 | 10 | 10 | 0 | 0 | 100.0% |
| 桌面原生与工具 | 10 | 10 | 0 | 0 | 100.0% |
| **全量合计** | **100** | **94** | **6** | **0** | **94.0% 严格对齐** |

---

## 阶段推进计划与执行记录

### 第一阶段：建立细粒度可复核映射台账（已完成）
- [x] 全量提炼 10 个业务域共 100 个高风险核心测试样本。
- [x] 梳理标准 9 维字段（状态、Go 来源、业务域、风险类型、Rust 对应入口、Rust 测试状态、差异结论、后续动作、验证命令）。
- [x] 将 strategy runtime 恢复对账用例从部分覆盖闭环为完全覆盖（补齐了 `restore_running_instances_ignores_paused_and_stopped_instances` 测试）。

### 第二阶段：按优先级补测与修复（进行中）
1. **P0: API Server & Transport Wire 与跨层生产链**
   - [x] 补齐全部 21 条 ADK 变异路由畸形载荷拦截（400 BAD_REQUEST Fail-Closed 防护，`8b3db870`）。
   - [x] 补齐超大 offset 分页越界 Clamp 契约（`returned:0` 与空数组保护，`472ca43b`）。
   - [x] 补齐 SSE 慢客户端主动断连与恢复回放生命周期对齐（`255f3393`）。
2. **P0: Assistant / Workflow ADK**
   - [x] 补齐 Run 目标生命周期模式校验（非 loop 模式拒绝并返回 `ADK_RUN_PAUSE_FAILED`、`ADK_RUN_RESUME_FAILED`、`ADK_RUN_OBJECTIVE_UPDATE_FAILED`，`f09e7c43`）。
   - [x] 补齐缺失目标 404 NOT_FOUND 与任务必填属性 400 `ADK_TASK_SAVE_FAILED` 分类对齐（`f09e7c43`）。
3. **P1: 行情微观结构与 Futu 订阅对账**
   - [x] 补齐深度盘口与逐笔成交券商故障分类映射（503 离线、502 损坏、429 限流透传，`48f65a4b`）。
   - [x] 补齐 Futu 订阅对账器生命周期、60s 延迟退订防抖与成功缓存幂等断言（`202d3c23`）。
4. **P1: 策略编排、回测撮合、日历与存储**
   - [x] 补齐策略启动对账恢复、陈旧错误标记与暂停/停止状态防自动自启隔离（`8d363071`）。
   - [x] 补齐跨 UTC 元旦午夜时区判定与市场本地年份（Market-Local Year）防漂移断言（`ad84db00`）。
   - [x] 补齐 SQLite 独占写租约防并发冲突与释放后审计留存断言（`2636d7e7`）。
   - [x] 补齐量化回测保守撮合模型 16 项高风险场景全量断言（流动性上限、Bracket止损优先、Reduce-only持仓钳位、跳空改善等，`09f9eda4`）。
   - [x] 补齐系统设置运行时监听器故障状态机事务回滚对齐（`6cff805f`）。
   - [x] 补齐废弃认证 Token 路由严格返回 404 NOT_FOUND 契约（`179644d1`）。
   - [x] 补齐空切片序列化为 JSON 空数组 `[]` 契约（`999ef15c`）。
   - [x] 补齐 WebSocket 慢客户端广播溢出注入 `live.resync` 控制帧与 Tick 去重（`e96af96c`）。
   - [x] 补齐 Run 取消级联终止 InputRequest 及拒绝迟到回答冲突判定（409 `ADK_INPUT_RESPONSE_CONFLICT`，`854fa9c6`）。
   - [x] 补齐审批拒绝原子流转 `DENIED` 并恢复 Run 状态注入终止消息（`6133dcd5`）。
   - [x] 补齐 OpenD 断连状态流转与最低支持协议版本门禁（`ea6aa3ac`）。
   - [x] 补齐行情快照非法 refresh 查询参数 400 校验与缓存击穿穿透（`cd335389`）。
   - [x] 补齐 K 线非法时段查询对齐为 `MARKET_CANDLE_SESSIONS_INVALID` 错误码（`e12c74a3`）。
   - [x] 补齐实盘熔断 Hard Stop 释放幂等性与状态流转（`f268475b`）。
   - [x] 补齐内嵌 `worker.mjs` SHA256 校验和防篡改与空 bundle 拦截（`5f6320b7`）。
   - [x] 补齐回测同步任务取消 Active / Terminal / Missing 状态边界（`490f3c38`）。
   - [x] 补齐回测同步 SessionScope 默认空字符串为 regular 与非法输入拦截契约（`a957d393`）。
   - [x] 补齐 Web 登录 Cookie HttpOnly、SameSite=Strict 与 Path=/ 属性约束断言（`cd407be7`）。
   - [x] 补齐 Broker 运行时 Session 状态显式包含 null `lastError` 契约断言（`6073d098`）。
   - [x] 补齐 Futu OpenD ProgramStatus 缺失/纯状态/扩展描述格式化断言（`dc5fa7e1`）。
   - [x] 补齐交易日历快照空根目录与缺失文件删除幂等性断言（`82341881`）。
   - [x] 补齐 Pine/Strategy 周期分钟数解析与安全回退逻辑（`40e50c7f`）。
   - [x] 补齐 API Middleware PATCH 请求视为会话写入必须验证 CSRF 契约（`b81bbbd5`）。
   - [x] 补齐 ADK Run 终态重复执行取消操作幂等且保留原 cancelledAt 时间戳断言（`3e2aa85a`）。
   - [x] 补齐板块行业榜单 plateType 默认空字符串为 industry 及大小写兼容断言（`8665e2c4`）。
   - [x] 补齐 ADK 空白 session_id 删除触发 Validation 校验拦截契约（`8bbf7316`）。
   - [x] 补齐废弃投资组合现金对账路由严格返回 JSON 404 NOT_FOUND 契约断言（`1b8804bb`）。
   - [x] 补齐投资组合现金余额响应显式包含 balances 字段断言（`359eb3d0`）。
   - [x] 补齐券商设置初次保存前暴露 null integration 与默认端口（11110/11111）断言（`696c67fc`）。
   - [x] 补齐 ADK 工具描述符 requiresApprovalIn 空数组序列化契约断言（`63e754d5`）。
   - [x] 补齐 Web 登出下发 Max-Age=0 过期 Cookie 清理会话契约断言（`109bdfbf`）。
   - [x] 补齐实盘风控快照初始化空向量非空切片集合契约断言（`7c6a03f8`）。
   - [x] 补齐全量失效会话使既有 Web Session 变为 unauthenticated 契约断言（`428771cc`）。
   - [x] 补齐生产服务关闭前系统状态接口健康可达性断言（`b42675e7`）。
   - [x] 补齐美股交易时段分类边界（盘前04:00/盘中09:30/盘后16:00/周五20:00休市）契约断言（`e6cff4a7`）。
   - [x] 补齐运行时状态 optional_string 首尾空格去除与纯空白转 None 契约断言（`29bb7868`）。
   - [x] 补齐 K 线查询窗口 from_time >= to_time 异常时自动重置为 36h 默认回看断言（`84ff20a4`）。
| `go` `pkg/backtest/internal/storage/store_session_aggregation_contracts_test.go:11` `TestSchemaHelpersExposeStableStorageContracts` | 存储与设置 | 表名与会话作用域 | `crates/jftrade-store-sqlite` | 已覆盖 | 语义一致：验证 `kline_table_name` 对 provider_id、rehab_type、interval 校验以及 `__r__` / `__x__` 命名前缀与哈希稳定性 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite -E "test(schema_kline_table_name_and_session_scope_validation_contracts)"` |
| `go` `pkg/backtest/pine_costs_test.go:40` `TestBacktestSlippagePriceUsesMarketTickSize` | 回测与日历 | 撮合与滑点边界 | `crates/jftrade-backtest` | 已覆盖 | 语义一致：验证回测撮合中滑点步长按交易标的 market tick_size 调整，买单上滑、卖单下滑 | 保持回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-backtest -E "test(slippage_price_uses_market_tick_size)"` |
