# Futu/OpenD 协议与集成领域对齐批次

本批范围固定在 `go` 基线中 `internal/integration/futu/` 与 `pkg/futu/` 的
高风险复合键（Go 路径、起始行号、测试名），逐条阅读 Go 断言并对照 Rust 实际
测试函数与生产 owner（`crates/jftrade-integration-futu`、`crates/jftrade-broker`）。
只有覆盖全部分支与断言时才标记 `[x]`/`function_exact`。

## 本批结论

| Go 测试（文件:行号:测试名） | 风险 | Rust 证据 | 结论 |
| --- | --- | --- | --- |
| `internal/integration/futu/subscription_reconciler_test.go:346:TestSubscriptionReconcilerUsesDelayedFallbackForBasicQuoteAvailabilityFailures` | fallback/retry/超时 | `fake_framed_opend_runtime_tests::test_basic_quote_availability_rejection_enters_delayed_fallback_and_reconcile_succeeds`；`subscriptions_tests::basic_quote_availability_failure_enters_delayed_fallback_and_recovers` | `[x]` function_exact：配额拒绝不再让 reconcile 失败，进入 delayed fallback（fallbackCount=1、ownActiveCount=0、brokerState=fallback、lastError 保留），15s 窗口内延迟、到点恢复 active |
| `internal/integration/futu/subscription_reconciler_test.go:676:TestSubscriptionReconcilerDropsFailedRecordsReleasedBeforeRetry` | 生命周期/内存所有权 | `subscriptions_tests::released_never_established_records_are_dropped_without_fallback_leakage` | `[x]` function_exact：从未建立（订阅被拒或 fallback）的记录离开 demand 时立即删除，fallbackCount 同步回退 |
| `internal/integration/futu/marketdata_runtime_test.go:432:TestTickFromTickerReclassifiesUSRegularBoundary` | 时区/session 边界 | `basic_quote_tick::tests::snapshot_projection_reclassifies_us_regular_boundary_and_clears_extended_hours` | `[x]` function_exact：2026-01-07T16:00Z 投影为 regular，且清除 pre/after/overnight 扩展字段 |
| `internal/integration/futu/marketdata_runtime_test.go:318:TestTickFromTickerPreservesHKPreviousCloseDuringLunchBreak` | 时区/session 边界/空值 | `basic_quote_tick::tests::hk_lunch_snapshot_keeps_previous_close_and_does_not_mark_extended_hours` | `[x]` function_exact：HK 午休 closed 投影保留 698.9 上一收盘价且不被当前价覆盖 |
| `pkg/futu/basicqot_subscription_errors_test.go:12:TestClassifyBasicQotSubscriptionErrorOnlyMarksAvailabilityFailures` | provider error mapping | `subscription_executor::tests::qot_sub_availability_classifier_matches_go_keyword_matrix` | `[~]` partial：关键字分类（entitlement/quota/unsupported 合格；frequency/rate-limit/transport 不合格）已覆盖，但 Go 契约返回可 `errors.Is` 的 `*SnapshotAvailabilityError` 类型链，Rust 目前返回 `Option<SnapshotAvailabilityKind>`，nil/cancel/deadline 以结构性 `None` 表达，未做类型包装 |
| `internal/integration/futu/subscription_reconciler_test.go:287:TestSubscriptionReconcilerRetriesFailuresAndCancelsRetryOnReacquire` | retry/取消 | `subscriptions_tests::unsubscribe_retry_ladder_escalates_and_reacquire_clears_retry_state` | `[x]` function_exact：完整退避阶梯 5s/10s/20s/30s/30s、窗口内 reconcile 推迟、`retry_delay_ms` 边界（0 钳到 5s、99 饱和到 30s），以及 reacquire 取消待定 unsubscribe 重试（failures/retryAt/lastError 清空、状态回到 active、下次失败从 5s 重新起梯）；修复 `actions()` 未在记录重新 desired 且 active 时清理 retry 状态 |
| `internal/integration/futu/marketdata_runtime_test.go:613:TestMarketDataRuntimeFiltersFallbackInstrumentsFromPushStream` | fallback/流过滤 | `subscriptions_tests::fallback_instruments_are_filtered_from_the_push_stream` | `[x]` function_exact：新增 `SubscriptionReconciler::filter_push_instruments`（此前 Rust 无此能力）；fallback 标的移出 push 流、恢复后放行、trim+大写化、空串丢弃、不可解析标的保留、无 fallback 时不过滤 |
| `internal/integration/futu/subscription_reconciler_test.go:88:TestSubscriptionReconcilerSharesExactPhysicalSubscriptionsAndDefersFinalRelease` | 共享/延迟释放 | `subscriptions_tests::exact_physical_subscriptions_are_shared_and_final_release_is_deferred` | `[x]` function_exact：大小写/前缀变体归并为一个逻辑能力，最小保留期内只标记 pending，重新 desired 复用 pending，到期只释放不再 desired 的物理订阅 |
| `internal/integration/futu/subscription_reconciler_test.go:713:TestSubscriptionReconcilerKeepsThreeViewerCapabilitiesAndReleasesOnlyOldOrderBook` | 多 viewer/所有权 | `subscriptions_tests::viewers_share_capabilities_and_only_the_stale_order_book_is_released` | `[x]` function_exact：viewer 切换只订阅新能力，旧 order book 保持 pending；来回切换复用 pending 不产生 RPC；到期仅释放旧 ORDER_BOOK |
| `internal/integration/futu/subscription_reconciler_test.go:160:TestSubscriptionReconcilerConcurrentReconcileIsIdempotent` | 并发/幂等 | `subscriptions_tests::concurrent_reconcile_passes_are_idempotent_for_subscribe_and_release` | `[x]` function_exact：真实 `Arc<Mutex<_>>` 共享 owner 下 64 并发 reconcile，订阅与释放各只规划一次，终态计数全 0 |
| `internal/integration/futu/subscription_reconciler_test.go:191:TestSubscriptionReconcilerReplaysAllDesiredSubscriptionsWhenConnectionGenerationChanges` | 断线重连/fencing | `subscriptions_tests::stale_observed_generation_reports_pending_reconnect_instead_of_active` | `[x]` function_exact：观察代际与本地不一致时报 `pending_reconnect`、ownActiveCount=0（修复 `physical_snapshot` 忽略 observed_generation 把过期订阅算作 active） |
| `internal/integration/futu/subscription_reconciler_test.go:236:TestSubscriptionReconcilerFailsWhenConnectionKeepsChanging` | 断线重连/收敛 | `subscriptions_tests::generation_churn_fences_actions_without_stranding_subscriptions` | `[~]` boundary：Rust 不内联重试报错，而是 fence 旧代际动作并在下一 pass 重规划；测试断言代际 1→2→3 不丢需求、旧代际永不 active，故不上抛 Go 的错误消息 |
| `internal/integration/futu/subscription_reconciler_test.go:459:TestSubscriptionReconcilerMeasuresRetentionFromOpenDAcknowledgement` | 计时/保留期 | `subscriptions_tests::retention_is_measured_from_the_opend_acknowledgement` | `[x]` function_exact：subscribedAt/eligibleAt 取自 OpenD 确认时刻，60s 到点不释放、ack+60s 才释放 |
| `internal/integration/futu/subscription_reconciler_test.go:499:TestSubscriptionReconcilerMeasuresRetryFromOpenDFailureAcknowledgement` | 重试/计时 | `subscriptions_tests::retry_is_measured_from_the_opend_failure_acknowledgement` | `[x]` function_exact：失败重试窗口从失败确认时刻起算（74_999 推迟、75_000 重试） |
| `internal/integration/futu/subscription_reconciler_test.go:697:TestDesiredPhysicalSubscriptionsRejectsIncompleteRefsAndNormalizesSymbols` | 空值/非法输入 | `subscriptions_tests::desired_physical_subscriptions_reject_incomplete_refs_and_normalize_symbols` | `[x]` function_exact：空引用/缺 interval/未支持 channel 丢弃，限定符号补 market，无法限定则拒绝且不产出物理订阅 |
| `internal/integration/futu/subscription_reconciler_test.go:394:TestSubscriptionReconcilerProviderSwitchDefersPhysicalReleaseUntilOpenDEligible` | provider 切换/延迟释放 | `subscriptions_tests::provider_switch_defers_physical_release_until_opend_eligible` | `[x]` function_exact：停用 Futu 在最小保留期内不释放（ownActiveCount=3、pendingReleaseCount=3、brokerState=pending_unsubscribe），窗口内重新激活复用 pending 不重复订阅，到期释放三条；修复 `ownActiveCount` 漏算 pending 释放与 `pending_release` 拼写不被前端契约接受两处生产缺陷 |
| `internal/integration/futu/subscription_reconciler_test.go:587:TestSubscriptionReconcilerPendingProviderCleanupDropsClosedConnectionOwnership` | provider 切换/所有权 | `subscriptions_tests::pending_provider_cleanup_drops_closed_connection_ownership` | `[x]` function_exact：连接替换后旧代际记录被丢弃，不会通过新连接误退订；replay 为空、计数全 0 |
| `internal/integration/futu/subscription_reconciler_test.go:623:TestSubscriptionReconcilerHandlesQuotaExchangeReplacementResetAndNilBoundaries` | 配额/连接替换 | `subscriptions_tests::connection_replacement_clears_quota_ownership_and_reset_is_idempotent` | `[x]` function_exact：连接替换清空旧连接配额诊断与检查时间、配额失败仅作诊断、close 幂等；修复 `replay_actions` 保留死连接配额总数/检查时间并投射到 UI 的缺陷 |
| `internal/integration/futu/subscription_reconciler_test.go:548:TestSubscriptionReconcilerMeasuresQuotaRefreshFromOpenDAcknowledgement` | 配额/计时 | `session_coordinator::tests::refresh_quota_uses_the_authenticated_opend_protocol_and_preserves_last_success` | `[~]` boundary：Rust 的检查时刻取 post-RPC now 并保留上次成功值，但由 poll loop 的 `quota_refresh_pending` 标志驱动，无 Go `subscriptionQuotaRefresh=1min` 时间节流，故「同一分钟内不重复查询」不成立 |
| `internal/integration/futu/marketdata_runtime_test.go:712:TestTickFromSnapshotMapsExtendedQuoteFields` | 时区/session 边界/空值 | `basic_quote_tick::tests::extended_quote_blocks_project_go_field_and_window_contract`；`basic_quote_tick::tests::extended_block_without_a_calendar_window_stays_unannotated` | `[~]` partial：extended 块字段与 session window 标注逐项对齐，并修复 calendar 缺少该 session 窗口时仍标注 tradingDate/timezone 的缺陷；Go 还断言 `Kind`/`Source`/`ExtendedHours`/`QuoteAt`/`Market`/`Symbol`（bbgo Tick DTO 字段），Rust 用 broker-neutral 模型 + LiveHub envelope，属模型差异 |
| `internal/integration/futu/marketdata_runtime_test.go:196:TestMarketDataRuntimeNilAndClosedLifecycleBoundaries` | 空值/生命周期 | `product_runtime_composition::tests::dynamic_opend_adapter_reports_no_snapshot_without_a_live_runtime` | `[~]` boundary：Rust 无 Ensure/Reset/OwnsBroker 门面（nil runtime 指针在类型层不可表达）；可迁移断言为未组合 OpenD runtime 时物理订阅端口返回 `None` |
| `internal/integration/futu/marketdata_runtime_test.go:655:TestMarketDataRuntimeUnavailableQueryHelpers` | 空值/不可用端口 | `product_runtime_composition::tests::dynamic_opend_adapter_reports_no_snapshot_without_a_live_runtime`；`product_production_ports_unavailable` | `[~]` boundary：Rust 以「未安装端口 → fail-closed 错误」表达配置禁用，类型层不允许返回 nil 指针；已迁移可断言部分为 None 快照语义 |
| `internal/integration/futu/marketdata_runtime_test.go:591:TestMarketDataRuntimeReplacesAnExchangeWhenItsConfigKeyChanges` | 配置热替换 | `product_runtime_composition::opend_provider_config`；`product_active_provider_state` | `[~]` boundary：Rust 不在运行时按配置键热替换 OpenD 物理连接；连接在 composition root 一次性组合，之后切换只在 router/demand 层激活，OpenD 会话存活到 shutdown supervisor 关闭 |
| `internal/integration/futu/marketdata_runtime_test.go:452:TestMarketDataRuntimeExchangeResetAndStreamLifecycle` | Reset/流生命周期 | `product_runtime_supervisor`；`runtime_task::OpenDSessionRuntime` | `[~]` boundary：以 ordered shutdown supervisor（reverse-of-construction + JoinHandle await）取代进程内 `Reset()`；Go 的「Reset 后新 exchange/broker adapter 与 generation 前进」在 Rust 中属进程级重启，broker ownership 断言不可迁移 |
| `internal/integration/futu/marketdata_runtime_test.go:27:TestMarketDataRuntimeCloseWaitsForEnsureAndDoesNotRevive` | 关闭/并发 | `tests/provider_runtime_recovery.rs::latest_demand_replaces_stale_replay_while_reconnect_is_pending`；`provider_runtime::shutdown` | `[~]` boundary：`start()` 内同步连接，不存在「Close 等待进行中 Ensure」窗口；等价保证由 shutdown 消费 self + Drop 兜底与重连期过期 replay 不复活覆盖 |
| `internal/integration/futu/marketdata_runtime_test.go:85:TestMarketDataRuntimeCloseReturnsActiveExchangeFailureIdempotently` | 关闭/幂等 | `provider_runtime::shutdown`；`provider_runtime::tests::release_and_deactivate_clears_bridge_owned_router_state` | `[~]` boundary：以 take/consume（`shutdown(self)`）取代 Go 的幂等 Close 计数，重复关闭在类型层不可表达，释放后 router/demand 收敛为空 |
| `internal/integration/futu/marketdata_runtime_test.go:116:TestMarketDataRuntimeCloseReturnsInflightExchangeFailure` | 关闭/回滚 | `provider_runtime::start`；`product_runtime_supervisor` | `[~]` boundary：`start()` 同步连接，失败即回滚（release_demand + deactivate），不存在可被 Close 竞争的 in-flight 句柄 |
| `internal/integration/futu/marketdata_runtime_test.go:163:TestMarketDataRuntimeDoesNotPublishExchangeWhenConfigChangesDuringCreate` | 并发/发布栅栏 | `product_active_provider_state`；`product_runtime_provider_activation` | `[~]` boundary：OpenD 配置仅在 composition root 一次性读取；provider 激活走串行 transition，激活成功后才发布 snapshot，shutdown 后拒绝激活，故不会发布过期 exchange |
| `internal/integration/futu/marketdata_runtime_test.go:220:TestTickFromTradeProducesBrokerNeutralPushTick` | 推送/中立模型 | `product_runtime_opend_listener` | `[~]` boundary：Rust 没有 bbgo Trade 中间类型，OpenD BasicQot 直接投影为中立 Tick 并经 LiveHub 发布 `market-data.tick`，Go 的 Trade→Tick 转义无对应生产路径 |
| `internal/integration/futu/marketdata_runtime_opend_test.go:154:TestMarketDataRuntimeQueryAndSubscriptionWrappers` | 查询/订阅包装 | `tests/fake_framed_opend_runtime_tests.rs`；`basic_quote_query` | `[~]` boundary：查询/订阅包装分散到各领域 adapter 与 fake framed OpenD 测试，错误为结构化枚举而非 `errors.As` 链，不迁移为单一等价测试 |
| `internal/integration/futu/marketdata_runtime_opend_test.go:275:TestTranslateSubscriptionRequiredErrorPreservesBrokerNeutralLeaseDetails` | 错误映射/lease | `basic_quote_query::BasicQuoteQueryError::SubscriptionRequired` | `[~]` boundary：Rust 已把 SubscriptionRequired 作为有界 query error 枚举成员并在各 adapter 内规范化，但无 Go 的 sentinel 包装与 interval 回退优先级链，属错误模型差异 |

## 本轮（市场数据运行时生命周期批量）修复的功能差异

1. **扩展时段缺少 calendar 窗口时被部分标注（P1）**
   - 复现：标的的 `PreAfterMarketData` 块存在，但交易所日历没有对应
     session 窗口（例如 HK 没有 pre-market 窗口）。
   - 修复前：`extended_session_metadata` 在找不到窗口时仍返回
     `trading_date`/`exchange_timezone`，给一个交易所根本不开的时段标注了
     交易日期。
   - 修复后：四个字段整体返回 `None`，与 Go 的
     `ResolveTradingDaySessionWindow` 失败后
     `attachFutuSessionWindow` 整体跳过一致。
   - 回归：`extended_block_without_a_calendar_window_stays_unannotated`、
     `extended_quote_blocks_project_go_field_and_window_contract`。

## 本轮结论边界

Go `marketdata_runtime.go` 的 `Ensure`/`Reset`/`OwnsBroker`/`Close` 门面是
Wails 时代的进程内热替换层。Rust 把它替换为：composition root 一次性组合
OpenD 连接、`ActiveProviderState` 串行 provider 切换、以及
`ProductShutdownSupervisor` 的 reverse-of-construction 关闭。因此这批 Go
测试中与「进程内 runtime 指针」「幂等 Close 计数」「in-flight 创建竞争」
相关的断言属于**边界保留**，不是待补测试；可迁移的行为断言（未组合 runtime
返回 `None`、扩展时段字段与 session window 投影）已补 Rust 回归。

## 本批修复的功能差异

1. **BasicQot 可用性失败未进入 delayed fallback（P0/P1）**
   - 复现：OpenD 对 BasicQot 订阅返回配额/权限/不支持类拒绝。
   - 修复前：`record_failure` 无条件递增 `fallback_count`，却没有任何把记录
     标记为 `fallback` 的路径；`fallbackCount` 与 `brokerState` 因此长期漂移，
     `is_fallback_instrument` 恒为 false，延迟快照路径不可达。
   - 修复后：`subscription_executor` 在 Qot_Sub 拒绝边界按 Go 关键字矩阵分类，
     `execute_action` 对合格 BasicQot 失败调用新增的 `record_fallback_failure`
     （15s 窗口、failures 归零、不计入错误返回），并补齐
     `is_fallback_instrument`/`has_fallback_subscriptions`。
   - 回归：`test_basic_quote_availability_rejection_enters_delayed_fallback_and_reconcile_succeeds`、
     `qot_sub_availability_classifier_matches_go_keyword_matrix`。

2. **从未建立的订阅记录离开 demand 后泄漏（P1）**
   - 修复：`actions` 在每次 reconcile 时删除 `subscribed_at_ms` 为空且不再被
     desired 的记录，并把 `fallback` 记录同步从 `fallback_count` 扣除。
   - 同时把 `subscribed_at_ms` 改为 `Option<i64>`，区分“从未建立”与
     “在时间 0 成功建立”，避免误删真实订阅。

3. **普通订阅失败被错误计入 fallback**
   - 修复：`record_failure` 不再递增 `fallback_count`，只有
     `record_fallback_failure` 计入，并在成功/释放时对称回退。

## 验证命令

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-broker --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

本批实测：`jftrade-integration-futu` 227 项通过、1 项跳过；
`jftrade-broker` 8 项通过。逐条命令与 state 记录在
`manual-test-mappings.json`。
