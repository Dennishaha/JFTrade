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

## 第二批：order_updates / security_details / probe / notifications（Go `internal/integration/futu` 剩余缺失项）

本批把 `manual-test-mappings.json` 中 `internal/integration/futu/` 与
`internal/app/apiserver/servercore/settings_broker_futu_health_test.go` 的最后
13 条 `evidence_type=missing` 全部闭环，并修正 probe 的 2 条 `partial`。

| Go 测试 | 风险标签 | Rust 入口 | 状态 |
| --- | --- | --- | --- |
| `internal/integration/futu/notifications_test.go:135:TestNotificationLabelsCoverEverySupportedProgramAndGatewayState` | 空值/非法输入、错误映射 | `tests/futu_notifications_parity.rs::test_notification_labels_cover_every_supported_program_and_gateway_state` | `[x]` function_exact：12 个 ProgramStatus 状态 + 17 个 GtwEvent 状态逐行断言 level/title/label，含未知状态回退 |
| `internal/integration/futu/notifications_test.go:207:TestNotificationAndQuoteRightLabelsRemainStable` | 错误消息稳定性 | `tests/futu_notifications_parity.rs::test_notification_and_quote_right_labels_remain_stable` | `[x]` function_exact：NotifyType（6+未知）与 QotRight（6+未知）标签冻结 |
| `internal/integration/futu/probe_test.go:22:TestProbeOpenDReportsProtocolOutcomesWithoutARealOpenD` | 超时/断线、错误映射 | `src/health.rs::tcp_probe_reports_protocol_outcomes_without_a_real_opend` | `[~]` partial：四个协议分支已断言；Go 折叠为 `LastError` 文本，Rust 用类型化 `OpenDTcpProbeError` |
| `internal/integration/futu/probe_test.go:96:TestProbeOpenDMapsHealthyProtocolFixture` | 健康 fixture | `src/health.rs::tcp_probe_maps_login_global_state_and_market_readiness` | `[x]` function_exact：connected/healthy/10.9.7000/Ready/quoteLoggedIn/markets=4 |
| `internal/integration/futu/probe_test.go:114:TestProbeOpenDReportsClosedPortAsDisconnected` | 断线 | `src/probe.rs::test_probe_opend_disconnected_and_version_enforcement_parity` | `[x]` function_exact：offline/disconnected/lastError + readiness 失败 |
| `internal/integration/futu/probe_test.go:137:TestProbeFromGlobalStateEnforcesMinimumVersionAndMapsNeutralState` | 版本门禁、空值 | `src/probe.rs::test_probe_opend_disconnected_and_version_enforcement_parity` | `[x]` function_exact：nil state、1008.6708 拒绝、健康映射三段 |
| `internal/integration/futu/order_updates_test.go:59:TestOrderUpdatesAdapterConvertsPushesAndStopsOnce` | 并发/生命周期、幂等 | 无 push adapter；对账 worker + `map_order_update` | `[~]` boundary：Rust 功能缺失，轮询对账替代推送 adapter |
| `internal/integration/futu/order_updates_test.go:105:TestOrderUpdatesAdapterRefreshRepeatsAccountPushWithoutReregisteringHandlers` | 断线重连、幂等 | `execution_reconciliation_push_worker_tests::test_tc_d5_04_...` | `[~]` boundary：仅可断言断连降级与自愈 |
| `internal/integration/futu/order_updates_test.go:137:TestOrderUpdateSubscriptionNilAndNoAccountPaths` | 空值 | `trade_session.rs::subscribe_trade_accounts` | `[~]` boundary：Go interface nil 语义无 Rust 对应 |
| `internal/integration/futu/order_updates_test.go:159:TestOrderUpdatesAdapterSubscribeReturnsNoOpOrErrorsCleanly` | 回滚/清理、空值 | `trading.rs::protocol_ids_match_go_opend_and_shadow_forbids_writes` | `[~]` boundary：保留协议号 2008/2208/2218 与 shadow 写禁止 |
| `internal/integration/futu/security_details_test.go:12:TestSecurityDetailsMapPreservesCompleteBrokerNeutralWireShape` | wire 契约、精度 | `product_market_data_quote_read_tests::futu_securities_route_projects_broker_neutral_envelope_boundary` | `[~]` boundary：权威 OpenAPI 仅要求 9 字段，研究块不伪造 |
| `internal/integration/futu/security_details_test.go:126:TestSecurityDetailsMapKeepsMissingOptionalAndProductBlocksNull` | 空值 | 同上 | `[~]` boundary：缺失研究块不出现 |
| `internal/integration/futu/security_details_test.go:160:TestSecurityRefMapUsesCanonicalIdentity` | 身份规范化 | 同上 | `[~]` boundary：身份由路由派生，无 `SecurityRef` 类型 |
| `internal/app/apiserver/servercore/settings_broker_futu_health_test.go:16:TestFutuRuntimeAndHealthDiagnoseEnabledButUnreachableOpenD` | 诊断/恢复 | `product_production_assembly_tests::production_opend_health_diagnoses_unreachable_and_unsupported_opend` | `[x]` function_exact（修复后）：offline + `OPEND_API_CONNECTIVITY` + restart 建议 |
| `internal/app/apiserver/servercore/settings_broker_futu_health_test.go:66:TestFutuOpenDHealthRejectsOldBuildAndGuidesUpgrade` | 版本门禁/诊断 | 同上 | `[x]` function_exact（修复后）：`OPEND_VERSION_UNSUPPORTED` + serverVersion 10.8.6708 + 不重启 |

### 本批修复的功能差异

1. **OpenD 健康投影丢弃 typed issue code（P1）**
   - 复现：OpenD 可达但版本低于 10.9.6908（例如 1008/6708）。
   - 修复前：`futu_opend_snapshot` 在 probe 分支硬编码 `diag_code="NONE"`、
     `manualRetry=false`，并把 `quoteLoggedIn` 缺失伪造成 `true`；任何 probe
     结果都落入同一分支，导致 `OPEND_VERSION_UNSUPPORTED` 永远不会出现在
     诊断中，`restartOpenDRecommended` 也恒为 false。
   - 修复后：probe 分支采用 `probe.issue_code`（缺省回退
     `OPEND_API_CONNECTIVITY`），`manualRetryRequired` 由 lastError 决定，
     `restartOpenDRecommended` 只在无 typed code 且错误文本含 dial/connection
     refused 时为 true；`quoteLoggedIn` 不再伪造。
   - 同时区分 `offline/disconnected`（dial 失败）与 `degraded/degraded`
     （协议被拒、解码失败、对端关闭），并在版本不受支持时生成包含
     `MINIMUM_OPEND_VERSION` 的 summary。
   - 回归：`production_opend_health_diagnoses_unreachable_and_unsupported_opend`
     （内置 framed OpenD fixture + 关闭端口两种场景）。

### 本批边界结论

- **Order update push adapter**：Go 的 `OrderUpdatesAdapter` 是 push 订阅
  adapter；Rust 的 `subscribe_trade_accounts`（trd_sub_acc_push）没有调用方，
  `trd_update_order`/`trd_update_order_fill`/`trd_notify` 仅声明未解码，
  engine 改由 `ExecutionReconciliationWorker` 轮询对账，并在 OpenD ready /
  reconnect 时 `wake_execution_reconciliation`。在补齐 push 解码与 handler
  注册面之前，这 4 条保持 `[~]` boundary，不计入功能等价。
- **Security details research model**：Go 的 `SecurityDetailsMap` 转换完整
  `pkg/futu.SecurityDetails`（含 warrant/option/index/plate/future/trust 等
  研究块）。Rust 的权威契约 `marketdata.SecurityDetailsPayload` 只要求
  `instrumentId`/`market`/`name`/`symbol`，并允许 provider 追加研究字段；
  Rust 只投影 9 个规范字段并从 live snapshot 补 equity pe/pb。已用
  `futu_securities_route_projects_broker_neutral_envelope_boundary` 冻结该边界，
  不伪造未迁移的研究块。

### 验证命令

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

本批实测：`jftrade-integration-futu` 244 项通过、1 项跳过；`jftrade-engine`
1070 项通过（含 2 项新增）。逐条命令与 state 记录在
`manual-test-mappings.json`。

## 批次：opend new_methods 与 exchange 协议常量收口

本批把 `pkg/futu/exchange_test.go:434` 与 `pkg/futu/opend/new_methods_test.go`
中可证明等价的项转为 `[x]`/`function_exact`，其余记录边界。

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `pkg/futu/exchange_test.go:434 TestTradeProtocolConstantsMatchOfficialIDs` | `jftrade-integration-futu::trading::tests::protocol_ids_match_go_opend_and_shadow_forbids_writes` | `[x]`：本轮给 `TradeProtocol` 补齐 `GetMaxTradeQuantity=2111`、`GetMarginRatio=2223`、`GetCashFlowSummary=2226`，连同既有 2211/2222 与 Go 五个官方 ID 完全一致。 |
| `pkg/futu/exchange_test.go:152 TestFutuSecurityFromSymbolUsesMarketParser` | `jftrade-engine::...::test_futu_security_from_symbol_uses_market_parser` | `[x]`：HK./HK:/US./SH./SZ. 解析矩阵与 `CN.600519` 拒绝一致；`normalize_instrument` 提升为 `pub(super)` 以供证据测试。 |
| `pkg/futu/exchange_test.go:47 TestQueryMarketsReturnsBootstrapMarket` | `jftrade-integration-futu::provider::tests::futu_descriptor_is_static_and_valid_without_connecting_to_opend` | `[~]`/partial：Rust 用静态 ProviderDescriptor 表达市场/币种，不提供 bootstrap 符号行情档案。 |
| `pkg/futu/exchange_test.go:125 TestInferMarketUsesMarketProfiles` | 同上（前置解析测试） | `[~]`/boundary：Go 的 tickSize/pricePrecision 档案在 Rust 无对应模型。 |
| `pkg/futu/opend/new_methods_test.go:313 TestSubscribeQuotes` | `subscription_executor::tests::executor_sends_subscribe_and_unsubscribe_over_one_framed_session` | `[x]`：`isSubOrUnSub=true` 经 Qot_Sub(3001) 帧发送，market/code 与 Go 一致。 |
| `pkg/futu/opend/new_methods_test.go:352 TestUnsubscribeQuotes` | `subscription_executor::tests::unsubscribe_request_sets_is_sub_or_un_sub_false_and_keeps_security` | `[x]`：本轮新增专属退订分支测试。 |
| `pkg/futu/opend/new_methods_test.go:499 TestRequestHistoryKL` | `history::tests::history_wire_frame_keeps_protocol_and_serial_for_mock_opend` | `[x]`：响应经 frame 往返保留 nextReqKey 与 s2c。 |
| `pkg/futu/opend/new_methods_test.go:578 TestGetSecuritySnapshot` | `security_snapshot_query::tests::maps_security_snapshot_bbo_and_equity_metrics_without_defaults` | `[x]`：Qot_GetSecuritySnapshot(3203) 解码 BBO/equity 指标且不补默认值。 |

### 本批发现

- `TradeProtocol` 枚举此前缺少 `GetMaxTradeQuantity`/`GetMarginRatio`/`GetCashFlowSummary`
  三个官方协议 ID；Go 的 `TestTradeProtocolConstantsMatchOfficialIDs` 直接断言这些常量，
  属于 Rust 功能缺失（不是仅缺测试），本轮补齐枚举成员。
- Go 的 `pkg/futu/opend` 客户端封装（连接复用、call 计数、错误包装、
  `GetGlobalState`/`GetKL`/`UnlockTrade`/`LockTrade` 等）在 Rust 由
  `managed_session`/`session_coordinator`/各 typed query 模块承担，二者
  结构不同；这些条目的逐项等价仍需后续批次按具体断言继续核对。

### 剩余高缺口文件

`pkg/futu/opend/new_methods_test.go` 尚有 20 项、`pkg/futu/exchange_test.go`
尚有 23 项、`pkg/futu/adapter_new_methods_test.go` 22 项、
`pkg/futu/exchange_kline_test.go` 16 项保持 `[~]`，将在后续批次逐条处理。

## 批次：US 历史 K 线会话路由（RTH/ETH/ALL）

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `pkg/futu/exchange_kline_test.go:50 TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults` | `jftrade-engine::product::tests::...::us_intraday_history_fans_out_across_opend_session_routes` | `[x]`：RTH/ETH/ALL 三路由扇出 + 重复 bucket 由更具体路由覆盖。 |
| `pkg/futu/exchange_kline_test.go:106 TestQueryKLinesForSessionsFiltersUSHistoricalRoutes` | `...::us_regular_only_history_uses_a_single_rth_route` | `[x]`：regular-only 只发 RTH。 |
| `pkg/futu/exchange_kline_test.go:131 TestHistoricalKLineSessionHelpersFilterAndPlanExplicitSelections` | `jftrade-integration-futu::history_session_plan::tests::session_planner_selects_explicit_routes_and_keep_sets` | `[x]`：pre-only → 单条 ETH，keepSessions 对齐。 |
| `pkg/futu/exchange_kline_test.go:178 TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight` | `jftrade-integration-futu::history_session_plan::tests::routed_sessions_override_the_clock_classification` | `[x]`：路由强制 regular/overnight。 |
| `pkg/futu/exchange_kline_test.go:192 TestQueryKLinesFallsBackToSessionAllWhenHistoricalRouteUnsupported` | `...::us_history_falls_back_to_session_all_when_a_route_is_rejected` | `[x]`：ETH 被拒后回退 Session_ALL 并返回完整窗口。 |
| `pkg/futu/exchange_kline_test.go:228 TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage` | `jftrade-integration-futu::history_session_plan::tests::chinese_supported_session_message_triggers_the_all_fallback` | `[x]`：中文会话提示触发回退（含负例）。 |
| `pkg/futu/exchange_kline_test.go:17 TestQueryTickersBatchesBasicQotRequests` | `jftrade-integration-futu::history_session_plan::tests::non_us_and_daily_requests_stay_unsegmented` | `[~]`/partial：仅覆盖分段前提，批量报价合并由 `basic_quote_query` 承担。 |

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(history_session_plan)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(us_intraday_history_fans_out_across_opend_session_routes)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(us_regular_only_history_uses_a_single_rth_route)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(us_history_falls_back_to_session_all_when_a_route_is_rejected)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
```

### 发现并修复的真实功能差异

- 复现：`GET /api/v1/market-data/candles/US/AAPL?period=1m`（无 `sessions` 参数）。
  修复前：Rust 只发一次未分段的 `Qot_RequestHistoryKL`，US 盘前/盘后/隔夜 bar 拿不到，
  也没有 `Session_ALL` 回退路径。
  预期（Go `pkg/futu/exchange_kline.go::queryHistoricalKLinesAcrossPlans`）：US 小时级及以下窗口
  按 RTH→ETH→ALL 扇出，合并去重后由调用方按请求会话过滤；单条路由被 OpenD 以
  “不支持/无效/仅支持”类消息拒绝时整体回退到 `Session_ALL`。
- 修复位置：
  - `crates/jftrade-integration-futu/src/history_session_plan.rs`（新增）：
    `MarketSession`、`HistoricalKlineRequestPlan`、`build_request_plans`、
    `should_fallback_to_all`、`HistoricalKlineRouteError`。
  - `crates/jftrade-engine/src/product_trade_runtime_candles.rs`：
    `historical_klines_window_routed` 执行多路由扇出、按路由 keep-session 过滤与回退。
  - `crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`：
    Futu 历史读取改为调用路由版本，keep-filter 使用 `jftrade-calendar` 的
    `classify_session`（DST/假日/隔夜由日历拥有），日历缺失或无法分类时不静默丢 bar。
  - `crates/jftrade-engine/src/product_wire_helpers.rs`：
    `kline_route_sessions` 把 API `sessions` 值（regular/extended/pre/after/overnight）
    映射到路由集合，缺省即全会话。

### 边界与未迁移项

- 本次未改动非 US 市场、日线及以上窗口的行为（保持 unsegmented），
  与非 US/日线相关的既有测试全部通过。
- `pkg/futu/exchange_kline_test.go` 其余项（ticker 批量、日内标签归一化、
  9 页以上分页、扩大页大小、当前未闭合 bucket 合并、stream 重建）仍在清单中待办。
- 已知 flake：`jftrade-integration-futu` 的
  `health::tests::tcp_probe_reports_protocol_outcomes_without_a_real_opend`
  在基线工作树同样失败（本地端口竞争），与本批改动无关，定向重跑通过。

## 批次：历史 K 线分页、页大小与标签归一化

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `exchange_kline_test.go:291 TestQueryKLinesFollowsHistoryPaginationAndKeepsLatestLimit` | `jftrade-integration-futu::history::window_tests::history_window_follows_forward_pages_and_keeps_the_latest_limit` | `[x]`：跟随 nextReqKey 连续 3 页。 |
| `exchange_kline_test.go:326 TestQueryKLinesAllowsMoreThanEightHistoryPages` | `...::history_window_allows_more_than_eight_pages` | `[x]`：9+1 页全部跟随。 |
| `exchange_kline_test.go:356 TestQueryKLinesUsesLargerHistoryPageSizeThanRequestedLimit` | `...::history_window_uses_a_larger_upstream_page_size_than_the_limit` | `[x]`：2/500/5000 → 200/500/1000。 |
| `exchange_kline_test.go:241 TestQueryKLinesNormalizesIntradayHistoryLabelToBucketStart` | `...::history_window_normalizes_intraday_history_label_to_bucket_start` | `[x]`：1m/5m/60m 标签位移到桶起点。 |
| `exchange_kline_test.go:269 TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart` | `...::history_window_keeps_daily_history_label_as_bucket_start` | `[x]`：日线标签保持桶起点。 |
| `exchange_kline_test.go:385 TestQueryKLinesIncludesCurrentRealtimeBucketFromGetKL` | `jftrade-engine::...::candle_route_keeps_latest_history_after_all_forward_pages_and_current_bar` | `[~]`/partial：当前 bucket 合并已覆盖，订阅前置与 closed 标签细节未逐条断言。 |
| `exchange_kline_test.go:428 TestStreamConnectEmitsBasicQotPushAsBBGOEvents` | `jftrade-integration-futu::quote_push_tests::decodes_basic_kline_and_order_book_pushes_with_go_field_semantics` | `[~]`/partial：push 解码已覆盖；Rust live 事件为 market-data.tick/orderbook envelope，成交量差分语义未实现。 |
| `exchange_kline_test.go:479 TestStreamConnectRebuildsClosedCachedOpenDClient` | `jftrade-integration-futu::session_coordinator::tests::reconnect_replay_contains_only_subscriptions_and_resets_retry_state` | `[x]`：会话关闭后重建并只重放订阅（generation 递增）。 |

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(history_window)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(quote_push)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(reconnect_replay_contains_only_subscriptions_and_resets_retry_state)'
```

### 说明

- 本批为证据映射与边界确认：分页预算、页大小放大、标签归一化在 Rust 读取层
  （`history.rs::query_window`、`kline_query.rs::adjust_kline_time`）已与 Go 行为一致，
  新增 4 条测试把此前仅有实现、没有逐项证据的契约补齐。
- 仍未实现/未迁移（保留在清单）：
  - stream 层的成交量差分（`nextTradeQuantity`：跨日基线、回退、负值、超大整数）
    与 `VolumeDelta`/`CumulativeVolume` 字段；
  - 当前未闭合 bucket 的 `closed=false` 与 `GetKL` 订阅前置的逐条断言。

## 批次：OpenD client 传输边界、codec 帧守卫与 keep-alive

本批覆盖 `pkg/futu/opend/client_transport_boundaries_test.go`（13 项）与
`pkg/futu/codec/*`（6 项），共 19 项全部转为 `[x]` / `function_exact`。
其中 3 处是真实功能差异修复，其余为缺失行为证据补齐。

| Go 测试 | Rust 证据入口 | 状态 | 说明 |
| --- | --- | --- | --- |
| `codec/frame_test.go:8 TestEncodeDecodeRoundTrip` | `jftrade-integration-futu::frame::tests::encode_decode_round_trip_matches_opend_wire` | `[x]` | 44 字节头 + body 往返，protoID/serial/body 逐项断言。 |
| `codec/frame_test.go:29 TestDecodeRejectsCorruptedBody` | `...::frame::tests::decode_rejects_corrupted_body_hash` | `[x]` | body 翻转 → `BadBodyHash`。 |
| `codec/frame_test.go:39 TestDecodeRejectsBadMagic` | `...::frame::tests::decode_rejects_bad_magic` | `[x]` | magic 破坏 → `BadMagic`。 |
| `codec/frame_test.go:49 TestDecodeRejectsShortFrame` | `...::frame::tests::decode_rejects_short_frame` | `[x]` | 短于 44 字节 → `TooShort`。 |
| `codec/frame_test.go:56 TestDecodeRejectsLengthMismatch` | `...::frame::tests::decode_rejects_length_mismatch` | `[x]` | 截断 body → typed `LengthMismatch`。 |
| `codec/frame_size_guards_test.go:9 TestFrameSizeGuardsCoverEncodeAndDecode` | `...::frame::tests::frame_size_guards_cover_encode_and_decode` | `[x]` | encode/decode 双向 32MiB 守卫，decode 在分配前拒绝。 |
| `client_transport_boundaries_test.go:69 TestConnectSurfacesInvalidAddressAndClosedClient` | `...::managed_session_tests::closed_session_rejects_connect_like_rpcs_before_touching_the_transport` | `[x]` | 关闭会话拒绝 InitConnect/GetGlobalState 且不写 socket。 |
| `client_transport_boundaries_test.go:107 TestClientCloseWaitsForReadWorkerAfterClosingTransport` | `...::managed_session_tests::close_shuts_down_the_transport_before_joining_the_reader` | `[x]` | 先 shutdown 再 join reader；peer 观察 EOF 后才返回。 |
| `client_transport_boundaries_test.go:143 TestClientCloseDrainsPendingRequestsAfterBestEffortCloseFailure` | `...::managed_session_tests::close_drains_pending_requests_when_the_peer_closes` | `[x]` | pending RPC 收到 `Closed(PeerClosed)`，close 幂等。 |
| `client_transport_boundaries_test.go:166 TestKeepAliveLoopHalvesLongIntervalsAndStopsForClosedClient` | `...::managed_session_tests::keep_alive_worker_sends_frames_and_stops_on_close` | `[x]` | **功能补齐**：Rust 新增 keep-alive worker；1004 帧发出、间隔离散折半、close 后停止。 |
| `client_transport_boundaries_test.go:182 TestStartKeepAliveRejectsClosedClientWithoutOwningWorker` | `...::managed_session_tests::start_keep_alive_rejects_a_closed_session` | `[x]` | 已关闭会话不注册 worker。 |
| `client_transport_boundaries_test.go:202 TestSubscribeNotifySkipsNilAndMalformedPush` | `...::subscriptions_tests::quote_push_ingestion_drops_malformed_and_stale_frames_without_registering_handlers` | `[x]` | malformed push 丢弃、无 stream failure；stale generation 在 decode 前拒绝。 |
| `client_transport_boundaries_test.go:217 TestCallFrameRejectsMarshalAndOversizedPayloads` | `...::managed_session_tests::call_rejects_oversized_payload_before_touching_the_socket` | `[x]` | >32MiB payload 被 encode 守卫拒绝，未写 socket（Rust 无反射 marshal 失败路径，该半支保留为语言边界）。 |
| `client_transport_boundaries_test.go:230 TestCallFrameReturnsWriteAndCloseFailures` | `...::managed_session_tests::call_reports_write_failure_and_closed_session_to_the_waiter` | `[x]` | 写失败/对端关闭/等待中 close 三种 waiter 结果。 |
| `client_transport_boundaries_test.go:263 TestReadLoopClosesConnectionOnOversizedFrame` | `...::managed_session_tests::reader_closes_on_oversized_declared_body` | `[x]` | 超长声明 → `Closed(InvalidFrame(BodyTooLarge))`。 |
| `client_transport_boundaries_test.go:290 TestReadLoopDiscardsInvalidFramesAndClosesOnTruncation` | `...::managed_session_tests::reader_discards_undecodable_frames_and_closes_on_truncation` | `[x]` | **功能修复**：坏 magic/hash 的完整帧丢弃继续（原先直接关会话），截断才 `PeerClosed`。 |
| `client_transport_boundaries_test.go:315 TestDispatchDropsDuplicateResponseWhenPendingBufferIsFull` | `...::managed_session_tests::dispatch_drops_a_duplicate_response_when_the_pending_buffer_is_full` | `[x]` | 匹配 pending 先移除，重复帧降级为 unsolicited。 |
| `client_transport_boundaries_test.go:327 TestSubscribeQuotesReportsTransportFailure` | `...::managed_session_tests::closed_session_reports_subscribe_transport_failure` | `[x]` | 关闭会话的 `PROTO_QOT_SUB` 返回 `Closed(Local)`。 |
| `client_transport_boundaries_test.go:334 TestProgramStatusLabelIncludesServerDescription` | `...::health::tests::program_status_label_includes_server_description` | `[x]` | **功能修复**：programStatus 由短标签改为 Go 的 `ProgramStatusType_*` 枚举名 + trimmed 描述。 |

### 本批发现与修复

1. **keep-alive 完全缺失**（功能缺失 → 已修复）
   - Go `pkg/futu/opend/client.go::StartKeepAlive` 从 `InitConnect.S2C.keepAliveInterval`
     启动心跳，`>1s` 折半；Rust 此前没有该功能，长连接可能在 OpenD
     侧仍接受 TCP 但实际无响应。
   - 修复位置：`crates/jftrade-integration-futu/src/managed_session.rs`
     （`start_keep_alive` / `run_keep_alive` / `keep_alive_tick`，含 `KeepAlive` 1004 proto），
     接线在 `crates/jftrade-integration-futu/src/health.rs::connect_with_push_notifications`
     （仅长连接角色启动，健康探测不启动），proto 注册在
     `crates/jftrade-integration-futu/build.rs` 与 `src/trade_proto.rs::keep_alive`。
   - 失败/超时心跳按 Go 语义终止会话，后续调用 fail-closed。

2. **reader 对坏帧处理与 Go 不一致**（功能差异 → 已修复）
   - Go `readLoop` 对 payload 长度为界、但 decode 失败的完整帧 `continue`；
     Rust 原先一律关闭会话，导致一个坏帧断开整条行情连接。
   - 修复：`managed_session.rs::run_reader` 仅对 `BadMagic`/`BadBodyHash` 丢弃继续，
     I/O 失败与超长/截断仍关闭。

3. **programStatus 标签与前端契约不一致**（功能差异 → 已修复）
   - Go `programStatusLabel`/`ProgramStatusString` 返回 protobuf 枚举名
     （`ProgramStatusType_Ready: <desc>`），Rust 之前返回 `"Ready"`；
     前端 `FUTU_PROGRAM_STATUS_LABELS` 只匹配枚举名前缀，会退化为原文显示。
   - 修复：`health.rs::program_status_type_label`；同步
     `health::tests::tcp_probe_maps_login_global_state_and_market_readiness`、
     `probe::tests::probe_from_global_state_enforces_minimum_version_and_maps_neutral_state`
     的断言到 Go 期望值。

4. **既有 flake 修复（非本批新增）**
   - `health::tests::tcp_probe_reports_protocol_outcomes_without_a_real_opend`
     的 "connection closes during init" 分支原先让服务端 accept 后立即 drop，
     与客户端写入竞态，间歇性得到 `Transport` 而非断言的 `PeerClosed`。
     夹具改为先读走 InitConnect 再关闭（与 Go `startProbeProtocolServer` 一致），
     连续 6 次全量 lib 运行通过。

未迁移边界（保留，不伪装等价）：

- `TestCallFrameRejectsMarshalAndOversizedPayloads` 的 protobuf 反射 marshal
  失败半支：Rust 编码路径没有可注入的失败 marshal，只保留 payload 大小守卫。
- workspace 尚无 Rust 等价的 `Client.Subscribe(protoID, handler)` 回调注册表；
  nil handler 语义由 lifecycle 的 push 解码边界承担。

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --lib --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(managed_session)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(frame)'
python3 scripts/compatibility/audit_test_parity.py
```

## 批次：OpenD 交易写入前置条件、业务拒绝与读取错误边界

覆盖 `pkg/futu/opend/trading_methods_test.go`、`trading_write_boundaries_test.go`、
`trading_error_boundaries_test.go` 的 P0 写入/读取路径，共 15 项。
其中 1 处为真实功能差异修复。

| Go 测试 | Rust 证据入口 | 状态 | 说明 |
| --- | --- | --- | --- |
| `trading_methods_test.go:64 TestPlaceOrderRequiresRequestAndConnID` | `trade_session_tests::place_order_requires_an_authenticated_conn_id` | `[x]` | 未认证客户端如实投影 `conn_id=0`，不伪造。 |
| `trading_methods_test.go:83 TestPlaceOrderAndModifyOrderEncodeTradeWrites` | `...::place_order_encodes_packet_conn_id_and_projects_server_order_identity`、`...::modify_order_encodes_packet_conn_id_and_returns_server_identity` | `[x]` | packetID.connID=42、code/qty/price 转发、order_id/order_id_ex 投影。 |
| `trading_methods_test.go:182 TestHistoryOrderReadersPreserveFiltersAndEmptyResponses` | `...::history_order_call_uses_history_protocol_and_forwards_filters` | `[x]` | filter/status 保留，空列表返回空集合。 |
| `trading_methods_test.go:244 TestSubscribeAccountPushAndTradePushDecoding` | `...::subscribe_trade_accounts_forwards_every_account_id` | `[x]` | 请求侧逐项转发 [11,22] 且接受无 S2C 的成功 ack；推送回调解码半支保留为边界。 |
| `trading_methods_test.go:334 TestSubscribeAccountPushPropagatesTradeErrors` | `...::subscribe_trade_accounts_propagates_opend_rejection` | `[x]` | **功能修复**：原先误判无 S2C 成功 ack 为 `MissingS2c`，现按 Go 语义只校验 retType。 |
| `trading_methods_test.go:351 TestPlaceOrderUsesPresetPacketIDWhenProvided` | `...::place_order_encodes_packet_conn_id_and_projects_server_order_identity` | `[~]` | Rust packetID 由客户端自增，无 preset 入口（旧 owner 边界）。 |
| `trading_methods_test.go:381 TestTradeWriteWrappersSurfaceCallErrors` | `...::trade_write_wrappers_surface_call_errors` | `[x]` | 无连接时 typed Session 错误。 |
| `trading_write_boundaries_test.go:17 TestTradeWriteMethodsEnforcePrerequisitesAndDisconnectedState` | `...::trade_write_methods_enforce_prerequisites_and_disconnected_state` | `[x]` | place/unlock/subscribe 断开态全部失败。 |
| `trading_write_boundaries_test.go:58 TestPlaceOrderPropagatesOpenDBusinessRejection` | `...::place_order_propagates_opend_business_rejection` | `[x]` | `retType=-1/errCode=201` 投影为 typed ReturnCode。 |
| `trading_write_boundaries_test.go:82 TestModifyOrderReturnsStableEmptyResult` | `...::modify_order_returns_stable_identity_for_an_empty_success_payload` | `[x]` | 空成功载荷返回零值身份，不伪造 id。 |
| `trading_write_boundaries_test.go:102 TestTradePushSubscribersIgnoreMalformedAndUnsuccessfulUpdates` | `trading::tests::protocol_ids_match_go_opend_and_shadow_forbids_writes` | `[~]` | Rust 无推送订阅 adapter，订单更新走 engine 轮询对账，边界保留。 |
| `trading_error_boundaries_test.go:26 TestTradingReadMethodsPropagateOpenDBusinessErrors` | `...::trading_reads_propagate_opend_business_errors` | `[x]` | read_funds 的 ReturnCode 细节逐项断言。 |
| `trading_error_boundaries_test.go:102 TestTradingReadMethodsRejectDisconnectedSession` | `...::trading_reads_reject_a_disconnected_session` | `[x]` | funds/positions/orders/history 四路径全部 fail-closed。 |
| `trading_error_boundaries_test.go:138 TestHistoryTradingReadsReturnStableEmptyCollections` | `...::history_trading_reads_return_stable_empty_collections` | `[x]` | 空 S2C 列表→空集合。 |

### 本批发现与修复

- **`SubscribeAccountPush` 误判成功 ack**（功能差异 → 已修复）
  - Go `pkg/futu/opend/trading_writes.go::SubscribeAccountPush` 只要求 `retType==0`
    并接受缺失 S2C；Rust 经 `trade_command_proto!` 宏走 `decode_response`，
    无条件要求 S2C，导致 OpenD 成功订阅确认被报 `MissingS2c`。
  - 修复位置：`crates/jftrade-integration-futu/src/trade_session.rs::subscribe_trade_accounts`
    （改为只校验 retType，保留 errCode/retMsg 细节）。
  - 回归：`subscribe_trade_accounts_forwards_every_account_id`（接受无 S2C 成功）与
    `subscribe_trade_accounts_propagates_opend_rejection`（拒绝细节）。

### 边界保留（不迁移为等价测试）

- `SubscribeOrderUpdate` / `SubscribeOrderFillUpdate` 回调 adapter：Rust 无此 API，
  trd_update_order/_fill 只声明未解码；engine 用轮询对账 + 影子 mapper。
- preset `packetID`：Rust 由客户端自增，调用方不预设。

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --lib --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(trade)'
python3 scripts/compatibility/audit_test_parity.py
```

## 批次：`pkg/futu/opend/new_methods_test.go` 全量收口

本批把 `go:452dea11:pkg/futu/opend/new_methods_test.go` 的 24 条测试全部落成
`[x]`/`[~]` 结论，并修复 2 处真实功能差异。

### 本批结论

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `new_methods_test.go:272 TestGetGlobalState` | `health::tests::global_state_maps_market_login_and_version_fields` | `[x]` |
| `new_methods_test.go:313 TestSubscribeQuotes` | `subscription_executor::tests::executor_sends_subscribe_and_unsubscribe_over_one_framed_session` | `[x]`（前批） |
| `new_methods_test.go:352 TestUnsubscribeQuotes` | `subscription_executor::tests::unsubscribe_request_sets_is_sub_or_un_sub_false_and_keeps_security` | `[x]`（前批） |
| `new_methods_test.go:391 TestGetBasicQot` | `basic_quote_query::tests::basic_quote_query_maps_a_single_security_success_response` | `[x]` |
| `new_methods_test.go:419 TestSubscribeBasicQot` | `quote_push_tests::decodes_basic_kline_and_order_book_pushes_with_go_field_semantics` | `[~]`/partial |
| `new_methods_test.go:466 TestGetKL` | `kline_query::tests::get_kl_maps_name_and_klines_for_a_valid_response` | `[x]` |
| `new_methods_test.go:499 TestRequestHistoryKL` | `history::tests::history_wire_frame_keeps_protocol_and_serial_for_mock_opend` | `[x]`（前批） |
| `new_methods_test.go:534 TestGetStaticInfo` | `instrument_search_query_tests::static_info_lookup_maps_every_security_static_basic_field` | `[x]` |
| `new_methods_test.go:578 TestGetSecuritySnapshot` | `security_snapshot_query::tests::maps_security_snapshot_bbo_and_equity_metrics_without_defaults` | `[x]`（前批） |
| `new_methods_test.go:627 TestUnlockTrade` | `trade_session_tests::unlock_trade_omits_security_firm_when_the_caller_does_not_request_one` | `[x]` |
| `new_methods_test.go:654 TestLockTrade` | `trade_session_tests::lock_trade_sends_unlock_false_without_a_security_firm` | `[x]` |
| `new_methods_test.go:683 TestGetBasicQotError` | `basic_quote_query::tests::basic_quote_query_propagates_negative_ret_type_with_err_code_and_message` | `[x]` |
| `new_methods_test.go:700 TestGetGlobalStateError` | `health::tests::tcp_probe_preserves_opend_rejection_and_unsupported_version` | `[x]` |
| `new_methods_test.go:716 TestSubscribeQuotesError` | `subscription_executor::tests::qot_sub_rejection_surfaces_ret_type_err_code_and_message` | `[x]` |
| `new_methods_test.go:734 TestUnlockTradeError` | `trade_session_tests::unlock_trade_propagates_opend_rejection` | `[x]` |
| `new_methods_test.go:752 TestGetBasicQotEmptyS2C` | `basic_quote_query::tests::basic_quote_query_returns_an_empty_list_when_the_success_s2c_is_absent` | `[x]` |
| `new_methods_test.go:768 TestGetKLNullS2C` | `kline_query::tests::get_kl_missing_s2c_returns_an_empty_result` | `[x]` |
| `new_methods_test.go:791 TestGetGlobalStateAdvancedFields` | `health::tests::global_state_maps_market_login_and_version_fields` | `[~]`/boundary |
| `new_methods_test.go:848 TestRequestHistoryKLPagination` | `history::tests::history_pagination_round_trips_the_next_req_key_across_pages` | `[x]` |
| `new_methods_test.go:919 TestGetBasicQotMultipleSecurities` | `basic_quote_query::tests::basic_quote_query_maps_multiple_securities_in_order` | `[x]` |
| `new_methods_test.go:954 TestUnlockTradeWithSecurityFirm` | `trade_session_tests::unlock_trade_encodes_unlock_flag_and_security_firm` | `[x]` |
| `new_methods_test.go:986 TestGetSecuritySnapshotIndex` | `security_snapshot_query::tests::maps_security_snapshot_bbo_and_equity_metrics_without_defaults` | `[~]`/boundary |
| `new_methods_test.go:1023 TestSubscribeQuotesAllOptions` | `subscription_executor::tests::qot_sub_all_options_match_go_quote_sub_request_encoding` | `[x]` |
| `new_methods_test.go:1082 TestSubscribeQuotesUnsubAll` | `subscription_executor::tests::qot_sub_unsub_all_matches_go_all_flag_encoding` | `[x]` |

### 本批发现与修复

- **GetKL 把缺失 S2C 判成协议错误**（已修复）
  - Go `pkg/futu/opend/kline.go::GetKL` 在 `retType==0` 且 S2C（或 klList）缺失时
    返回空 `KLines`；Rust 之前直接 `MissingS2c`，会把正常的“无数据”放大成读取失败。
  - 修复位置：`crates/jftrade-integration-futu/src/kline_query.rs::decode_get_kl_response`。
  - 回归：`get_kl_missing_s2c_returns_an_empty_result`（缺失 S2C 与空 klList 两条分支）
    与 `get_kl_rejects_a_protocol_error_with_typed_details`（拒绝仍保持 typed）。
- **UnlockTrade 要求 S2C 才能确认成功**（已修复）
  - Go `UnlockTrade` 只校验 `retType==0`，接受缺失 S2C 的成功 ack；Rust 走
    `decode_response` 宏时无条件要求 S2C，导致解禁被误报 `MissingS2c`。
  - 修复位置：`crates/jftrade-integration-futu/src/trade_session.rs::unlock_trade`。
  - 回归：`unlock_trade_encodes_unlock_flag_and_security_firm`、
    `unlock_trade_omits_security_firm_when_the_caller_does_not_request_one`、
    `unlock_trade_propagates_opend_rejection`。
- **Qot_Sub 可选字段缺失**（已补齐 wire 契约）
  - 新增 `regPushRehabTypeList`(5)/`isFirstPush`(6)/`isUnsubAll`(7)/
    `isSubOrderBookDetail`(8)/`extendedTime`(9)/`session`(10)，并对齐
    Go 的“未指定则不发送”语义：Basic 保留既有 push 注册（`None`），
    K 线显式 `isRegOrUnRegPush=false`，`isUnsubAll` 总是显式 `false`。
  - 回归：`qot_sub_all_options_match_go_quote_sub_request_encoding`、
    `qot_sub_unsub_all_matches_go_all_flag_encoding`。

### 边界保留（不迁移为等价测试）

- `TestGetSecuritySnapshotIndex`：Go 的 `indexExData`（raiseCount/fallCount/equalCount）
  在 Rust 的 broker-neutral `TradeQuoteSnapshot` 中没有中立字段，
  `internal/integration/futu/security_details.go` 对应的 `index`/`plate` 研究块
  也未迁移；Rust 路由显式不伪造这些块。
- `TestGetGlobalStateAdvancedFields`：`marketHKFuture/marketUSFuture/marketSGFuture/
  marketJPFuture`、`localTime`、`qotSvrIpAddr`、`trdSvrIpAddr`、`connID` 只存在于
  Go 的 `opend.Client.GetGlobalState` 投影中。Rust 的 `WireGlobalState` 只保留
  产品路径真正消费的 HK/US/SH/SZ 四市场、登录位、版本与 programStatus。
- `TestSubscribeBasicQot` 回调 adapter：Rust 没有 `SubscribeBasicQot(callback)` API，
  推送经 `OpenDSessionCoordinator` → `QuotePush` → LiveHub 事件；解码契约由
  `quote_push_tests` 覆盖，成交量差分语义仍待补（见下批）。

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-integration-futu --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：`pkg/futu/stream_connection_quote_boundaries_test.go` 推送与成交量语义

本批处理 stream 层 12 条测试，修掉 2 处真实功能缺口：Rust 直播流从未注册
OpenD 推送，以及累计成交量从未换算成每笔增量。

### 本批结论

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `stream_connection_quote_boundaries_test.go:22 TestStreamCloseCancelsAndJoinsOwnedWorkers` | `runtime_task::tests::runtime_task_updates_dynamic_demand_and_shuts_down_its_coordinator` | `[~]`/partial |
| `:78 TestStreamConnectionAndSubscriptionBoundaries` | `subscription_executor::tests::executor_rejects_invalid_instrument_and_unsupported_interval` | `[~]`/partial |
| `:115 TestStreamPushHandlersRejectInactiveMalformedAndEmptyQuotes` | `product_runtime_opend_listener_tests::basic_quote_pushes_drop_rows_without_a_usable_security_or_price` | `[x]` |
| `:143 TestStreamConvertsCumulativeQuoteVolumeToIncrementalTradeQuantity` | `trade_volume::tests::first_sample_is_a_baseline_and_decreases_or_negatives_report_zero` | `[x]` |
| `:174 TestStreamPreservesFractionalCumulativeVolumeDelta` | `trade_volume::tests::fractional_and_out_of_fixedpoint_counters_keep_their_exact_delta` | `[x]` |
| `:188 TestStreamMarketTradeCarriesDeltaAndCumulativeVolume` | `product_runtime_opend_listener_tests::basic_quote_pushes_publish_delta_and_cumulative_volume` | `[x]` |
| `:215 TestStreamMarketTradePreservesVolumeBeyondLegacyFixedpointRange` | `product_runtime_opend_listener_tests::basic_quote_pushes_keep_exact_volume_beyond_fixedpoint_range` | `[x]` |
| `:239 TestStreamRejectsNegativeSnapshotVolume` | `tick_candles::tests::tick_candles_default_to_a_fifteen_minute_window_and_clamp_negative_volume` | `[~]`/partial |
| `:255 TestBasicQuotePushSubscriptionErrorsAndIdempotency` | `subscription_executor::tests::basic_subscribe_registers_push_delivery_like_go_stream` | `[x]` |
| `:292 TestOrderBookStreamConnectionBoundaries` | `subscription_executor::tests::hk_order_book_subscribe_requests_detail_and_registers_push` | `[~]`/partial |
| `:317 TestStreamReconnectAndClientWatcherExitPaths` | `session_coordinator::tests::peer_close_advances_generation_replays_subscriptions_and_accepts_push` | `[~]`/partial |
| `:369 TestStreamConnectReportsPhysicalSubscriptionFailures` | `subscription_executor::tests::executor_maps_qot_sub_rejection_without_reporting_success` | `[x]` |

### 本批发现与修复

- **BasicQot/盘口推送从未注册**（P0 功能缺口 → 已修复）
  - Go 的 stream 层在订阅 Basic 行情时调用 `subscribeBasicQotPush`
    （`IsRegOrUnRegPush=true`、`IsFirstPush=true`），盘口走
    `ensureOrderBookPushSubscriptions`（`IsRegPush=true`）；不注册推送时
    OpenD 不会下发 `Qot_UpdateBasicQot`(3005) / `Qot_UpdateOrderBook`(3013)，
    而 `product_runtime_opend_listener` 却在消费这两种推送。
  - Rust 的 `OpenDSubscriptionExecutor` 既拥有唯一 OpenD 会话、又承担 stream
    角色，此前 Basic 传 `None`、盘口传 `Some(false)`，等于显式不注册。
  - 修复位置：`crates/jftrade-integration-futu/src/subscription_executor.rs::qot_sub_request`。
    Basic 订阅 → `Some(true)`+`isFirstPush=Some(true)`；Basic 退订 →
    `Some(false)`；盘口订阅 → `Some(true)`（HK 同时带
    `isSubOrderBookDetail=true`，对齐 Go 的 HK 分批）；K 线保持 `Some(false)`。
  - 回归：`basic_subscribe_registers_push_delivery_like_go_stream`、
    `basic_unsubscribe_unregisters_push_delivery_like_go`、
    `hk_order_book_subscribe_requests_detail_and_registers_push`。
- **累计成交量从未换算为每笔增量**（P0 功能缺口 → 已修复）
  - Go `Stream.nextTradeQuantity` 用 `(symbol, tradingDay, session)` 记录上一笔
    累计量，首样本/跨日/跨时段/计数回退/负值一律返回 0，其余返回精确差值，并在
    `TickEventDTO.JSON` 里同时发布 `cumulativeVolume` 与 `volumeDelta`。
    Rust 的 tick 一直 `volume_delta: None`，直播事件也只发
    `volume`/`price`，前端 `marketDataRealtimeVolumeSequence` 拿不到增量。
  - 修复位置：
    - `crates/jftrade-marketdata/src/trade_volume.rs`（新增 `TradeVolumeTracker`，
      基于 `DecimalText` 任意精度做差，避免 2^53 以上丢位）；
    - `crates/jftrade-marketdata/src/cache.rs` 暴露 `trading_day_key` 作为
      共享的交易日定义；
    - `crates/jftrade-integration-futu/src/basic_quote_tick.rs` 新增
      `quote_session_label`，让流与快照共用同一时段判定；
    - `crates/jftrade-engine/src/product_runtime_opend_listener.rs` 按 Go
      `TickEventDTO.JSON` 发布 `at`/`brokerId`/`source`/`cumulativeVolume`/
      `volumeDelta` 与 `instrument`/`snapshot` 块，并复用 `cur_price == 0`
      早退。
    - `crates/jftrade-api/src/websocket.rs::event_instrument_id` 学会解析
      `payload.instrument.instrumentId`，否则新 payload 会被订阅过滤丢弃。
  - 回归：`trade_volume::tests::*`（4 条）与
    `product_runtime_opend_listener_tests::*`（5 条）。

### 边界保留（不迁移为等价测试）

- `TestStreamCloseCancelsAndJoinsOwnedWorkers`：Rust 用
  `OpenDSessionRuntime` 的 stop channel + `JoinHandle` 关闭并 join 工作线程，
  不存在 Go 的 `Stream.workerWG`/`startWorker(generation)` 代际 worker 语义。
- `TestStreamConnectionAndSubscriptionBoundaries` / `TestOrderBookStreamConnectionBoundaries`：
  “不可用 OpenD”与“空订阅”报错在 Rust 由 provider runtime 注入与
  `OpenDSessionCoordinator` 承担，用例分散，未合并成单条断言。
- `TestStreamReconnectAndClientWatcherExitPaths`：Go 的 `ReconnectC` watcher
  在 Rust 对应运行时任务的重连循环，取消语义通过 stop channel 表达。
- `TestStreamRejectsNegativeSnapshotVolume`：负值已在 candle 与 tracker 层钳为
  0；Go 额外要求“完全不产生事件”，Rust 目前仍会下发 `volumeDelta=0` 的 tick。

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata -p jftrade-engine -p jftrade-api -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-marketdata -p jftrade-engine -p jftrade-api -p jftrade-integration-futu --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：`pkg/futu/opend/` 用户安全组、搜索与版本边界

本批处理 `user_security_test.go`(6)、`search_quote_test.go`(3)、
`protocol_ids_test.go`(1)、`version_test.go`(2)，共 12 条，修掉 3 处真实差异。

### 本批结论

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `protocol_ids_test.go:5 TestStaticInfoAndKLineUpdateProtocolIDsDoNotOverlap` | `trading::tests::market_read_protocol_ids_match_go_and_do_not_overlap` | `[x]` |
| `user_security_test.go:148 TestUserSecurityProtocolIDs` | `trading::tests::user_security_protocol_ids_match_go` | `[x]` |
| `user_security_test.go:15 TestGetUserSecurityGroupsEncodesAllAndReturnsCustomAndSystemGroups` | `user_security_protocol::groups_encode_group_type_all_and_project_custom_and_system` | `[x]` |
| `user_security_test.go:51 TestGetUserSecuritiesEncodesTrimmedGroupName` | `user_security_protocol::members_encode_a_trimmed_group_name_and_project_static_info` | `[x]` |
| `user_security_test.go:93 TestUserSecurityMethodsRejectInvalidInputBeforeEncoding` | `user_security_protocol::a_blank_group_name_is_rejected_before_any_rpc` | `[x]` |
| `user_security_test.go:103 TestUserSecurityMethodsPropagateBusinessErrorsAndEmptyResults` | `user_security_protocol::a_rejected_group_query_surfaces_the_opend_message` | `[x]` |
| `user_security_test.go:125 TestUserSecurityMethodsNormalizeMissingGroupsAndReportSecurityErrors` | `user_security_protocol::a_missing_group_payload_is_an_empty_list_but_member_errors_stay_typed` | `[x]` |
| `search_quote_test.go:14 TestGetSearchQuoteSendsKeywordAndReturnsCandidates` | `instrument_search_query_tests::search_request_preserves_chinese_name_and_requests_full_candidate_window` | `[x]` |
| `search_quote_test.go:48 TestGetSearchQuoteValidatesInputAndPreservesOpenDErrors` | `instrument_search_query_tests::search_failures_and_malformed_responses_are_not_empty_successes` | `[x]` |
| `search_quote_test.go:77 TestGetSearchQuoteNormalizesMissingPayload` | `instrument_search_query_tests::a_success_without_a_payload_normalizes_to_an_empty_candidate_list` | `[x]` |
| `version_test.go:8 TestFormatVersion` | `health::tests::version_support_matches_go_minimum_version_rule` | `[x]` |
| `version_test.go:17 TestValidateMinimumVersion` | `health::tests::minimum_version_validation_matches_go_boundaries` | `[x]` |

### 本批发现与修复

- **搜索 `maxCount` 被写死为 100**（功能差异 → 已修复）
  - Go `GetSearchQuote(keyword, maxCount)` 校验 `1..=100` 后把调用方数值原样下发，
    `QuerySecuritySearch` 也把请求的 `limit`（默认 100）传进去。
  - Rust 之前无条件发送 `max_count=100`，调用方较小的 limit 只在本地截断，
    OpenD 仍按 100 检索并计费。
  - 修复位置：`crates/jftrade-integration-futu/src/instrument_search_query.rs`
    （新增 `search_with_limit`、`MAX_SEARCH_QUOTE_COUNT`、`InvalidMaxCount`）与
    `crates/jftrade-engine/src/product_production_ports_market_data_catalog_futu.rs`
    （把请求 limit 传入）。
  - 回归：`search_request_preserves_chinese_name_and_requests_full_candidate_window`。
- **搜索缺失 S2C 被当成协议错误**（功能差异 → 已修复）
  - Go 在 `retType==0` 且 S2C 缺失时返回空 slice；Rust 报
    `MissingField("s2c")`，把“无匹配”放大成 502。
  - 修复位置：`instrument_search_query.rs::decode_response`。
  - 回归：`a_success_without_a_payload_normalizes_to_an_empty_candidate_list`。
- **最低版本判断只覆盖 10.9 一条线**（潜在功能差异 → 已修复并提取）
  - Go `ValidateMinimumVersion` 接受 `serverVer > 1009` 的任意 build；
    Rust 的内联表达式把 `1010` 及更高 minor 的 `build_no==0` 情形交给 `>` 分支，
    逻辑正确但无法单独验证，且与 `FormatVersion` 分离。
  - 修复位置：`crates/jftrade-integration-futu/src/health.rs` 提取
    `version_supported(server_ver, build_no)`，探针改为调用它。
  - 回归：`minimum_version_validation_matches_go_boundaries`（逐项覆盖 Go 的 6 个用例）。

### 说明

- 用户安全组/自选股读取此前只有实现、没有逐项证据；本批新增
  `tests/user_security_protocol.rs`，用真实 loopback 帧覆盖
  `Qot_GetUserSecurityGroup(3222)` 与 `Qot_GetUserSecurity(3213)` 的
  请求编码、拒绝映射与缺载荷归一化。
- 协议 ID 常量从 `customization.rs` 的私有常量提升为 crate 级公开常量
  （`PROTO_GET_USER_SECURITY`、`PROTO_GET_USER_SECURITY_GROUP`），并新增
  `PROTO_GET_USER_INFO`、`PROTO_GET_STATIC_INFO`、`PROTO_GET_PLATE_SET`、
  `PROTO_GET_PLATE_SECURITY`、`PROTO_GET_SEARCH_QUOTE` 以便逐项断言。

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/adapter_earnings_calendar_test.go（10 项）

基线：`go:8a78fc78`。本批 10 项全部 `[x]`（`function_exact`）。

### 功能差异

Rust 此前完全没有 Futu 财报日历能力：`Qot_GetEarningsCalendar.proto`
未纳入 build.rs，integration 层没有 typed reader，engine 没有
`/api/v1/research/calendars?operation=earnings` 的 OpenD 分支，
研究路由在 `read_market_calendar` 里直接对 Futu 返回 unavailable。
本批按 Go 基线补齐完整链路，而不是只补测试。

### 行为映射

| Go 测试 | Rust 入口 |
| --- | --- |
| `:12 TestTranslateEarningsCalendarParamsMapsBusinessSemantics` | `crates/jftrade-engine/src/research_earnings_calendar_query_tests.rs::tests::translate_earnings_calendar_params_maps_business_semantics` |
| `:51 TestTranslateEarningsCalendarParamsRejectsUnsupportedMarketConditions` | `...::translate_earnings_calendar_params_rejects_unsupported_market_conditions` |
| `:61 TestEarningsCalendarDateChunksLimitEveryOpenDCallToSevenDays` | `...::earnings_calendar_date_chunks_limit_every_opend_call_to_seven_days` |
| `:79 TestEarningsCalendarDateChunksSupportsThirtyFiveDayGridAndRejectsLongerThanFortyTwo` | `...::earnings_calendar_date_chunks_supports_thirty_five_day_grid_and_rejects_longer_than_forty_two` |
| `:96 TestDeduplicateEarningsCalendarEntriesUsesDateAndSecurity` | `...::deduplicate_earnings_calendar_entries_uses_date_and_security` |
| `:110 TestCollectEarningsCalendarChunksFailsTheWholeRangeWhenOneChunkFails` | `...::collect_earnings_calendar_chunks_fails_the_whole_range_when_one_chunk_fails` |
| `:137 TestCollectEarningsCalendarChunksUsesEveryExactSegmentInOrder` | `...::collect_earnings_calendar_chunks_uses_every_exact_segment_in_order` |
| `:164 TestEarningsCalendarParameterValidationEdges` | `...::earnings_calendar_parameter_validation_edges` |
| `:190 TestEarningsCalendarDateValidationEdges` | `...::earnings_calendar_date_validation_edges` |
| `:217 TestDeduplicateEarningsCalendarEntriesFallsBackForAnonymousRows` | `...::deduplicate_earnings_calendar_entries_falls_back_for_anonymous_rows` |

### 实现与新增证据

- `crates/jftrade-integration-futu/src/earnings_calendar_query.rs`：协议 3401
  的 typed reader，负责 C2S protobuf 编码、拒绝/缺失 s2c/非法响应映射、
  `MARKET:CODE` 身份解析和逐字段投影；`validate_query` 对 market、
  日期和 filter 形状做边界校验。
- `crates/jftrade-engine/src/research_earnings_calendar_query.rs`：公开查询
  到 OpenD 请求的唯一翻译 owner，包含 7 天分块、42 天上限、
  `eventDate+instrumentId+symbol` 去重、匿名行回退、整批失败不合并部分
  结果，以及 `OPEND_EARNINGS_CALENDAR_FAILED` 502 错误映射。
- `crates/jftrade-integration-futu/tests/earnings_calendar_protocol.rs`：
  真实 loopback framed OpenD 测试，断言 proto_id 3401 的 C2S
  字段与 C2S filter 区间，并验证响应投影。
- `crates/jftrade-engine/src/product_production_ports_research.rs`：
  Futu + `/api/v1/research/calendars` + 缺省/earning 操作走 OpenD
  reader；其余日历操作保持原有边界结论；`SharedTradeReadRuntime`
  在 provider 激活/重置时安装或清空 reader，保持唯一写入所有权。
- 额外修复多字节日期输入会触发 UTF-8 切片 panic：现在按 Go 基线返回
  非法日期错误；同时移除分块循环里 `mem::take` 造成的过滤器状态隐患。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(earnings_calendar)' --all-targets --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -E 'test(earnings_calendar)' --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

## 批次：pkg/futu/exchange_orderbook_test.go（11 项）

基线：`go:452dea11`。本批 11 项全部 `[x]`（`function_exact`），发现并修复 1 处真实功能差异。

### 真实功能差异（已修复）

1. **order-book 推送要求 server 接收时间，缺失即丢弃整条推送**
   - 依据：`go:452dea11:pkg/futu/stream_orderbook.go:48`
     `handleOrderBookPush`。Go 只要求 `futuSymbolFromSecurity` 可解析且
     `Buy`/`Sell` 至少一侧非零，从不读取 `SvrRecvTimeBid`/`SvrRecvTimeAsk`。
   - 差异：`crates/jftrade-engine/src/product_runtime_opend_listener.rs` 的
     `QuotePush::OrderBook` 分支此前要求 bid/ask 接收时间至少一个非空，
     否则直接 `return`；OpenD 在部分行情下不回填该字段时，整个 depth 推送
     会静默消失。同时缺少「两侧均无价位就不推送」的 Go 语义。
   - 修复位置：`crates/jftrade-engine/src/product_runtime_opend_listener.rs`
     （缺失时间戳回落 `current_utc_rfc3339()`；bids/asks 均为空时丢弃）。
   - 回归：`product_runtime_opend_listener_tests::order_book_pushes_no_longer_require_server_receive_times`
     （时间戳缺失仍发布完整 depth）与
     `order_book_pushes_without_any_price_are_dropped`（全空不发布）。

### 行为映射

| Go 测试 | Rust 证据 |
|---|---|
| `:20 TestSubscribeOrderBookRequestConstruction` | `subscription_executor::tests::order_book_request_construction_keeps_market_code_and_subtype_identity` |
| `:47 TestIsHKMarket` | `subscription_executor::tests::order_book_detail_follows_each_instruments_market_not_list_order` |
| `:99 TestSubscriptionRegistryOrderBook` | `subscriptions_tests::order_book_registry_marks_stay_independent_and_reset_clears_them` |
| `:129 TestSubscriptionRegistryOrderBookReset` | `subscriptions_tests::order_book_registry_reset_clears_marks_for_a_replacement_connection` |
| `:144 TestSubscriptionRegistryOrderBookEnsure` | `subscriptions_tests::order_book_registry_ensure_lazily_yields_an_order_book_plan_entry` |
| `:156 TestSubscriptionRegistryQuoteAndKLineFamiliesAreIndependent` | `subscription_executor::tests::order_book_reconciler_isolates_quote_and_kline_families` |
| `:178 TestGroupOrderBookRequestsForPushSplitsHKAndNonHK` | `subscription_executor::tests::order_book_reconcile_plan_splits_hk_and_non_hk_requests` |
| `:214 TestGroupOrderBookRequestsForPushSingleHKBatchNeedsDetail` | `subscription_executor::tests::hk_order_book_subscribe_requests_detail_and_registers_push` |
| `:237 TestEnsureOrderBookPushSubscriptionsSplitsDetailsAndDeduplicates` | `tests/fake_framed_opend_runtime_tests.rs::test_order_book_reconcile_splits_hk_detail_and_deduplicates_replay` |
| `:281 TestOrderBookSubscriptionLifecycleRequiresLeaseAndUnsubscribes` | `subscription_executor::tests::order_book_lifecycle_leases_hk_detail_and_releases_idempotently` |
| `:363 TestHandleOrderBookPushEmitsSingleCompleteBookTicker` | `product_runtime_opend_listener_tests::order_book_pushes_no_longer_require_server_receive_times` |

### 架构差异说明

- Go 用 `subscriptionRegistry.orderBook` / `orderBookPush` 两张标记表加
  `groupOrderBookRequestsForPush` 的 HK/非 HK 批量拆分。Rust 由
  `SubscriptionReconciler` 的 generation 化记录 + 每 instrument 一条物理
  订阅动作承接，因此不再存在“批次里任一 HK 就让整批带 detail”的耦合；
  HK detail 由每条订阅自身市场推导。旧 getter 语义（`isHKMarket` 空列表
  false、任一 HK true）在逐条路径上等价，已用顺序无关的断言固定。
- Go `UnsubscribeOrderBook` 对不存在的订阅直接返回 nil；Rust reconciler
  对「从未建立」的记录在离开 demand 时本地丢弃，不产生 unsubscribe action。
- 新增 framed harness 字段 `is_reg_or_un_reg_push`(tag 4) 与
  `is_sub_order_book_detail`(tag 8)，与生产 `QotSubC2s` 的 tag 一致。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

---

## 批次：`pkg/futu/exchange_test.go`（27 项）

本批处理 `pkg/futu/exchange_test.go` 全部 27 个 Go 测试。结果：`[x]` 21 项、
`[~]` 6 项（boundary 4、partial 2）。本批发现并修复两个真实功能差异。

### 本批发现与修复

1. **实时委托缺少“仅活动单”过滤**（功能差异 → 已修复）
   - Go 依据：`pkg/futu/exchange_trade_read.go:273 brokerOrderSnapshotsFromProto(...,
     workingOnly=true)`；控制台 `GET /brokers/{id}/orders`（CURRENT）经
     `QueryOrders → QueryBrokerOrders` 只返回未终态委托。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_trade.rs`
     （CURRENT 分支过滤）+ `crates/jftrade-engine/src/trade_projection.rs::active_order_status`。
   - 回归：`broker_current_orders_hide_terminal_statuses_while_history_keeps_them`
     （CURRENT 丢弃 `FILLED_ALL`，`scope=history` 保留）。

2. **默认 OpenD TCP 端口误用 WebSocket 端口**（功能差异 → 已修复）
   - Go 依据：`pkg/futu/exchange.go:45 DefaultOpenDAddr = "127.0.0.1:11110"`。
   - 修复位置：`crates/jftrade-engine/src/product_runtime_composition.rs`
     （`11111 → 11_110`，提取 `default_opend_port`）。
   - 回归：`opend_provider_config_defaults_to_the_go_tcp_api_port`
     （默认 11110、已持久化端口优先、非法端口报错）。

### 本批新增 Rust 证据（21 项 `[x]`）

| Go 测试 | Rust 入口 |
| --- | --- |
| `:23 TestRegistration` | `provider.rs::tests::futu_descriptor_is_static_and_valid_without_connecting_to_opend` |
| `:36 TestConstructorFallsBackToDefaultAddress` | `product_runtime_composition.rs::tests::opend_provider_config_defaults_to_the_go_tcp_api_port` |
| `:47 TestQueryMarketsReturnsBootstrapMarket` | `catalog_tests.rs::bootstrap_market_rule_is_available_without_opend` |
| `:62 TestEnsureMarketWithContextAppliesBrokerLotSize` | `market_rules_snapshot_errors.rs::broker_lot_size_initializes_minimum_and_step_quantity` |
| `:125 TestInferMarketUsesMarketProfiles` | `catalog_tests.rs::inferred_market_profiles_match_go_market_rules` |
| `:198 TestQueryTickerReusesSingleOpenDConnection` | `basic_quote_query.rs::tests::repeated_basic_quote_reads_reuse_one_opend_connection` |
| `:265 TestDiscoverAccountsReusesSingleOpenDConnection` | `trade_session_tests.rs::repeated_account_reads_reuse_one_opend_connection` |
| `:322 TestQueryAccountBalancesUsesOpenDFundsSnapshot` | `trade_session_tests.rs::funds_snapshot_projection_preserves_available_and_withdrawable_cash` |
| `:367 TestQueryOpenOrdersReturnsActiveOrders` | `product_production_ports_trade_tests.rs::broker_current_orders_hide_terminal_statuses_while_history_keeps_them` |
| `:452 TestQueryBrokerHistoryOrdersReturnsHistoricalOrders` | `trade_session_tests.rs::history_order_read_projects_external_id_and_filters_status` |
| `:514 TestQueryBrokerHistoryOrderFillsReturnsHistoricalFills` | `trade_session_tests.rs::history_fill_read_projects_fill_identity` |
| `:561 TestQueryBrokerOrderFeesReturnsFeeBreakdown` | `trade_session_tests.rs::order_fee_read_projects_amount_and_item_breakdown` |
| `:609 TestQueryBrokerMarginRatiosReturnsMarginData` | `trade_session_tests.rs::margin_ratio_read_projects_permit_fee_and_tier_ratios` |
| `:654 TestQueryBrokerMarginRatiosSkipsUnknownStock` | `trade_session_tests.rs::margin_ratio_read_retries_without_unknown_stock_and_keeps_known_rows` |
| `:691 TestQueryBrokerMarginRatiosUsesCacheWithinTTL` | `product_production_ports_trade_tests.rs::margin_ratios_reuse_a_recent_success_within_the_ttl` |
| `:729 TestQueryBrokerCashFlowsReturnsFlowSummary` | `trade_session_tests.rs::cash_flow_read_encodes_header_and_projects_neutral_snapshot` |
| `:774 TestQueryBrokerMaxTradeQuantityReturnsSnapshot` | `trade_session_tests.rs::max_trade_quantity_read_projects_cash_and_margin_buying_power` |
| `:834 TestSubmitOrderPlacesViaOpenD` | `product_production_ports_execution_preview_tests.rs::submit_order_uses_client_order_id_as_the_opend_remark` |
| `:885 TestCancelOrdersUsesModifyOrderCancel` | `product_production_ports_execution_preview_tests.rs::cancel_order_uses_modify_order_cancel_operation` |
| `:762`（历史/成交/费用/最大可买） | 见上表相应条目 |

### 边界与 partial（6 项 `[~]`）

- `:89 TestEnsureMarketWithContextFallsBackToSecuritySnapshotLotSize`（boundary）：
  Rust 不暴露 GetStaticInfo→snapshot 两段式回退与 warning 列表。
- `:114 TestEnsureMarketWithContextReturnsInferredMarketWhenStaticInfoUnavailable`（boundary）：
  Rust provider 不可用时 fail-closed，不返回带错误的推断档案。
- `:237 / :251 TestConnectRejectsOpenDBelowMinimumVersion/Build`（partial）：
  Rust 在 health/ProviderRouter 投影层以 `OPEND_VERSION_UNSUPPORTED` 拒绝激活，
  `OpenDInitializedSession::connect` 本身不抛版本异常。
- `:927 TestEnsureSystemNotificationsBindsSystemPushHandler`（boundary）：
  Rust 以 `connect_with_push_notifications` + `UnsolicitedFrame` 暴露推送，
  没有 Go 式 `OnSystemNotify` 回调注册 API。
- `:974 TestSubscribeTradeAccountPushReplaysOnReconnectedClient`（boundary）：
  Rust 生产运行时没有 Go order-update push adapter；订单推送由对账 worker
  取代（见 `2026-09-09-project-parity-audit.md` G02/G08）。

### 说明

- `TradeOrderSnapshot` 历史/实时读取在 Rust 侧统一走 `TradeReadPort`，
  投影函数据此拆分出 7 个独立集成测试；`FakeTradeRead` 保持空订单夹具，
  新增 `OrderFixtureRead` 专用于活动单过滤断言，避免污染既有
  broker/portfolio 投影测试。
- `product_production_ports_execution_preview_tests.rs` 新增
  `RecordingTradeWriter`，用于同时验证下单 remark 与撤单 ModifyOrder 操作。
- 引擎测试新增 `CountingMarginRead`，以调用计数证明 TTL 缓存命中。

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine -p jftrade-marketdata -p jftrade-broker --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine -p jftrade-marketdata -p jftrade-broker --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：`pkg/futu/adapter_new_methods_test.go`（22 项）

基线：`go:452dea11`。本批 14 项 `[x]`（`function_exact`）+ 8 项 `[~]`
（boundary 4 / partial 4）。逐项复核 Go 行为后发现并修复 **资金摘要 wire 契约漂移**，
并补齐 securities/order-book 的独立行为证据。

### 本批发现与修复

1. **资金摘要字段名与枚举类型偏离 OpenAPI 契约**（真实功能差异 → 已修复）
   - 依据：`go:452dea11:pkg/futu/trade_read_proto.go:31`、`internal/trading/responses.go:139`
     与 `contracts/openapi/openapi.json`（`trading.BrokerFundsSummary`）。
   - 差异：Rust `trade_projection::funds_value` 输出 `power`（应为 `purchasingPower`）、
     `currency` 为整数（应为字符串 `HKD/USD/...`）、`exposureLevel`/`dtStatus`/`riskStatus`
     为整数（应为 `NORMAL/UNLIMITED/LEVEL1` 枚举名）、缺少 `shortSellingPower`、
     缺少 `riskStatus`/`dtStatus`。前端 `AccountAssetStrip.vue` 读取 `purchasingPower`、
     `shortSellingPower`、`riskStatus`，`AccountMoreSection.vue` 读取 `dtStatus`，
     缺失字段会让账户页静默显示空值。
   - 修复位置：`crates/jftrade-engine/src/trade_projection.rs::funds_value`，
     新增 `risk_status_label` / `dt_status_label` / `exposure_level_label`，
     严格按 Go `enumName`（截掉枚举类型前缀）+ `normalizeRuntimeEnum`（大写、
     Unknown→null）映射；字段名对齐 OpenAPI。
   - 回归：`funds_projection_preserves_full_margin_pdt_and_exposure_fields`
     断言新字段名与新枚举字符串，并显式断言旧 `power` 键已不存在。
2. **周期别名映射**（`period_to_kl_type`）：`1M` 月线特例、`1min/5min/1hour/2h/3h/4h/daily/weekly/monthly/quarter/1y` 等别名与 Go 全表对齐；
   非法周期保留 daily 兜底但由 route 层 `normalize_candle_period` 拒绝。

### `[x]`（14 项 function_exact）

| Go 测试 | Rust 证据 |
| --- | --- |
| `:14 TestConvertFundsSnapshotFullMarginFields` | `jftrade-engine::...trade_tests::funds_projection_preserves_full_margin_pdt_and_exposure_fields` |
| `:78 TestConvertFundsSnapshotNilMarginFields` | `...::funds_projection_keeps_missing_margin_fields_absent` |
| `:112 TestConvertFundsSnapshotCurrencyBalances` | `...::funds_projection_preserves_currency_and_market_asset_arrays` |
| `:140 TestSecuritiesFromSymbols` | `basic_quote_query::tests::normalized_instruments_trims_uppercases_dedupes_and_rejects_invalid` |
| `:175 TestSecuritySymbol` | `basic_quote_tick::tests::security_projection_builds_market_qualified_symbol` |
| `:182 TestSecuritySymbolNil` | `basic_quote_tick::tests::security_projection_rejects_nil_or_unknown_market` |
| `:191 TestFutuKLTypeFromIntervalStringAll` | `kline_query::tests::period_to_kl_type_matches_go_interval_aliases` |
| `:257 TestBrokerFundsSnapshotFromProtoFullMargin` | `trade_session::tests::funds_read_maps_full_margin_pdt_and_exposure_proto_fields` |
| `:344 TestBrokerFundsSnapshotFromProtoNilFunds` | `trade_proto::tests::funds_missing_s2c_normalizes_to_an_empty_snapshot` |
| `:431 TestOrderBookLevelFromPb` | `market_microstructure_query::tests::order_book_levels_project_price_volume_count_and_details` |
| `:465 TestOrderBookLevelFromPbNil` | `...::order_book_levels_return_empty_for_empty_input` |
| `:475 TestOrderBookLevelFromPbEmptyDetails` | `...::order_book_levels_omit_detail_list_when_absent` |
| `:492 TestOrderBookSnapshotFromOpendResult` | `...::depth_read_projects_name_times_and_levels_from_opend_s2c` |
| `:559 TestOrderBookSnapshotFromOpendResultEmptyResult` | `...::depth_read_returns_empty_arrays_for_empty_s2c_lists` |

### 边界与 partial（8 项 `[~]`）

- `:103 TestConvertFundsSnapshotNilInput`（boundary）：Rust 拥有所有权的
  `TradeFundsSnapshot` 不存在 nil 指针输入；缺失 S2C 归一到 `Funds::default()`。
- `:156/:163 TestSecuritiesFromSymbolsInvalid/Empty`（partial）：Rust 入口是
  `normalized_instruments` + route 层订阅校验，不存在 Go 的
  `securitiesFromSymbols` proto builder；已覆盖 trim/大写/去重/非法拒绝/空返回。
- `:228 TestFutuKLTypeFromIntervalStringInvalid`（partial）：Go 返回 error，
  Rust 保留 daily 兜底并显式记录差异。
- `:237/:248 TestInt64AsFloat64Ptr(+Nil)`（boundary）：Rust 用 `Option<f64>` 强类型，
  无运行期 int64→float64 指针 helper。
- `:358 TestBrokerFundsSnapshotRoundTripNoMargin`（partial）：Rust 无 Go 的双结构
  round-trip，已覆盖无 margin 数据时 debt/isPdt/exposure 保持 null。
- `:552 TestOrderBookSnapshotFromOpendResultNil`（boundary）：Rust 缺失 S2C 时
  `decode_missing(Qot_GetOrderBook, s2c)` fail closed，不制造 nil/空成功。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1492 passed / 1 skipped
cargo fmt --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py   # keys OK, 0 invalid [x]
git diff --check
```

## 批次：`pkg/futu/exchange_business_boundary_test.go`（12 项）

基线：`go:452dea11`。本批 8 项 `[x]`（`function_exact`）+ 4 项 `[~]`
（boundary 3 / partial 1）。逐项复核后修复 **两个真实功能差异**，并把三处
Go 遗留模型明确记录为架构边界。

### 本批发现与修复

1. **portfolio cash-balances 丢失 market 默认币种回落**（真实功能差异 → 已修复）
   - 依据：`go:452dea11:pkg/futu/trade_read_convert.go:38`
     `balanceMapFromBrokerFunds` → `defaultFundsCurrencyForMarket`（:301）。
   - 差异：Go 在 funds 无 per-currency 行时按 market 默认币种回落
     （US→USD、CN→CNH、SG→SGD、JP→JPY、MY→MYR、CA→CAD、AU→AUD，其余 HK→HKD）；
     Rust `portfolio_cash_balance_values` 仅当 `funds.currency` 存在时才回落，
     currency 缺失时直接返回空 `balances`，账户页现金行静默消失。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_trade.rs`
     新增 `default_funds_currency_for_market`，并按 Go 语义丢弃币种枚举未知的
     cash 行（不再输出 null currency）。
   - 回归：`portfolio_cash_balances_fall_back_to_market_currency_when_summary_currency_is_absent`、
     `portfolio_cash_balances_prefer_currency_rows_over_summary_fallback`。
2. **orders/fills 缺少 Go 的倒序排序**（真实功能差异 → 已修复）
   - 依据：`go:452dea11:pkg/futu/exchange_trade_read.go:285/:306`
     `brokerOrderSnapshotsFromProto` / `brokerOrderFillSnapshotsFromProto`
     按 `brokerOrderSortKey`（UpdatedAt，回落 SubmittedAt；timestamp 字段优先）
     与 `brokerOrderFillSortKey`（FilledAt）倒序，同值按 broker id 倒序。
   - 差异：Rust `orders_projection` / `fills_projection` 直接透传 OpenD 顺序，
     历史/当前订单与成交列表顺序依赖上游，不是契约。
   - 修复位置：`crates/jftrade-integration-futu/src/trade_snapshots.rs`
     （`order_sort_key`/`fill_sort_key`/`time_sort_key`/`parse_order_time`）。
   - 回归：`orders_projection_sorts_newest_updated_first_with_id_tiebreak`、
     `orders_projection_falls_back_to_submitted_at_and_applies_timestamp_fields`、
     `fills_projection_sorts_filled_at_desc_with_id_tiebreak`。
3. **`MARKET:CODE` 分隔符缺失**（真实功能差异 → 已修复）
   - 依据：`go:452dea11:pkg/futu/exchange_trade_write.go:268`
     `tradeSecurityInfoFromSymbol` 同时接受 `.` 与 `:`。
   - 差异：Rust `securities()` 只识别 `.`，`US:AAPL` 被当作裸 code 并套用请求
     market，导致市场静默错配。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_trade_requests.rs`。
   - 回归：`trade_security_parsing_accepts_every_go_prefix_and_rejects_invalid_symbols`。

### `[x]`（8 项 function_exact）

| Go 测试 | Rust 证据 |
| --- | --- |
| `:18 TestBalanceMapFromBrokerFundsUsesCurrencyRowsBeforeAccountFallback` | `...trade_tests::portfolio_cash_balances_prefer_currency_rows_over_summary_fallback` |
| `:46 TestBalanceMapFromBrokerFundsFallsBackToMarketCurrencyAndLockedCash` | `...::portfolio_cash_balances_fall_back_to_market_currency_when_summary_currency_is_absent` |
| `:92 TestBalanceMapFromFundsAndBrokerOrderSortBoundaries` | `trade_snapshots::tests::orders_projection_sorts_newest_updated_first_with_id_tiebreak`（+ fill 排序测试） |
| `:128 TestBrokerOrderMappingCoversOrderLifecycleEnums` | `...trade_tests::broker_order_status_labels_cover_lifecycle_enums` |
| `:179 TestBrokerOrderTypeAndTimeInForceMappingsCoverTradingVariants` | `...::broker_order_enum_labels_cover_trading_variants` |
| `:216 TestBrokerOrderMarketAndCurrencyBoundaries` | `...::market_and_currency_authority_tables_match_go_boundaries` |
| `:277 TestExchangeLocalMarketAndOrderBookHandlerBoundaries` | `product_runtime_opend_listener.rs::tests::order_book_pushes_publish_depth_for_the_subscribed_instrument` |
| `:349 TestTradeSecurityInfoAndRuntimeMarketAuthorityBoundaries` | `...::trade_security_parsing_accepts_every_go_prefix_and_rejects_invalid_symbols` |

### 边界与 partial（4 项 `[~]`）

- `:326 TestExchangeInvalidateClientClearsReadyStateAndSubscriptions`（partial）：
  Rust 无 `Exchange.invalidateClient`；由 `OpenDSessionCoordinator` 走 close/断线重建
  generation 并重放 desired 订阅，已引用 generation fencing 测试作为等价证据。
- `:433 TestKLineSessionRegistryResolvesExactRecordAndQuoteSamples`（boundary）：
  Go 的 `klineSessions` exact-record 缓存不存在；Rust 由注入的 `QuoteSessionResolver`
  （calendar）与投影层 session 字段解析，不再维护交易所内缓存。
- `:461 TestKLineSessionSamplePruningAndWindowFallback`（boundary）：
  Go 的 12h TTL / 256 上限 / ±interval 窗口回落无 Rust 等价缓存。
- `:487 TestMergeStaticInfoIntoSecurityDetailsFillsMissingFieldsWithoutClobberingSnapshot`
  （boundary）：Go 把 `GetStaticInfo` 合并进既有 `SecurityDetails` 且不覆盖非空字段；
  Rust 直接构建 snapshot 投影，不存在合并层，避免复活遗留模型。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

### 收尾修正

- `product_production_ports_trade_tests::margin_ratios_reuse_a_recent_success_within_the_ttl`
  原先断言整个响应相等，但缓存只保存 margin ratio 快照，响应外壳的 `checkedAt`
  是每次读取的墙钟毫秒值，跨毫秒即抖动失败（全量并发运行时命中）。Go 基线
  `TestQueryBrokerMarginRatiosUsesCacheWithinTTL` 只断言缓存结果与 `Trd_GetMarginRatio`
  调用次数，故改为比较 `marginRatios` 负载并保留调用次数断言。

## 批次：pkg/futu/adapter_stock_screen_test.go（11 项）

基线：`go:452dea11`。本批 11 项全部 `[x]`（`function_exact`），无功能差异。

### 行为映射

| Go 测试 | Rust 证据 |
|---|---|
| `:15 TestTranslateResearchScreenParamsBuildsStrictStockScreenRequest` | `crates/jftrade-integration-futu/tests/stock_screen_protocol.rs::stock_screen_request_encodes_typed_filters_retrieve_sort_and_resolves_mainland_identity` |
| `:105 TestTranslateResearchScreenParamsValidatesStableKeysAndMarket` | `crates/jftrade-engine/src/product_research_screen_write_port_tests.rs::research_screen_definition_rejects_unsupported_market_and_stable_keys` |
| `:142 TestStockScreenFeatureResultNormalizesIdentityCellsAndOffset` | `...product_production_ports_research_tests.rs::futu_stock_screen_projects_exact_mainland_rows_and_omits_combined_total` |
| `:188 TestNormalizeStockScreenRowPreservesParameterizedInstanceIdentity` | `...::futu_stock_screen_selects_parameterized_columns_and_derives_counter_currency` |
| `:228 TestStockScreenFeatureResultUsesPerRowMainlandIdentityAndFiltersExactMarkets` | `...::futu_stock_screen_filters_exact_mainland_markets_and_maps_rate_limit` |
| `:316 TestStockScreenMainlandIdentityMustBeAuthoritative` | `crates/jftrade-integration-futu/tests/stock_screen_protocol.rs::stock_screen_identity_resolution_fails_closed_without_static_info` |
| `:327 TestResearchScreenQuoteCurrencyUsesSecurityCounterIdentity` | `...::stock_screen_quote_currency_uses_security_counter_identity` |
| `:367 TestResearchScreenLimiterAllowsTenPerThirtySeconds` | `...stock_screen_protocol.rs::stock_screen_reader_rejects_unsupported_market_and_throttles_after_ten_calls` |
| `:385 TestResearchScreenRateLimitErrorRoundTrip` | `...::futu_stock_screen_rate_limit_maps_to_retry_after_seconds` |
| `:395 TestResearchScreenTranslationEdges` | `crates/jftrade-integration-futu/src/stock_screen_query.rs::tests::rejects_unknown_factors_and_market_mismatch_during_encoding` |
| `:607 TestStockScreenNormalizationEdges` | `crates/jftrade-integration-futu/src/stock_screen_query.rs::tests::decodes_every_value_type_and_keeps_property_unit_semantics` |

### 新增证据

- `crates/jftrade-integration-futu/tests/stock_screen_protocol.rs`：三个真实
  loopback framed OpenD 测试，覆盖 `Qot_StockScreen`(3252) 的 C2S 严格校验
  （隐式市场过滤、typed filterList、retrieveList、sortList、pageFrom/pageCount、
  watchlistStockIds）、`Qot_GetStaticInfo` 双市场候选身份解析、A 股身份不可解析时
  fail closed，以及 10 次/30 秒本地限流。
- `stock_screen_query.rs` 模块测试新增三类因子全表编码、全类别非法输入拒绝、
  全部 valueType 解码与未知 provider id 丢弃。
- 引擎侧新增参数化列实例绑定、HK 人民币柜台币种、exact SH/SZ 过滤与
  `Retry-After` 秒数向上取整映射。

### 结论

Go 的 `adapter_stock_screen.go` / `stock_screen_normalization.go` 行为在 Rust
由 `jftrade-integration-futu::stock_screen_query`（协议与身份解析）与
`jftrade-engine::product_production_ports_research_screen`（投影、币种、限流映射）
两处 owner 承接，未在 API handler 复制业务逻辑。老 GET `/api/v1/research/screens`
仍走同一 typed reader，wire 字段 `nextOffset` 与前端 `useStockScreenerController`
一致。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/subscription_lifecycle_test.go（10 项）

基线：`go:8a78fc78`。本批 10 项全部 `[x]`（`function_exact`）。

### 功能差异与修复

1. **US 盘中 K 线订阅缺少 extended session 路由**（P1）。
   复现：对 `US.AAPL` 订阅 1m K 线，捕获 Qot_Sub 请求。
   预期（Go `makeKLineSubscriptionRequest`）：`ExtendedTime=true`、
   `Session=Session_ALL`、`IsRegOrUnRegPush=false`。
   实际（修复前）：Rust 只发送 subtype 11，两个 session 字段均为空，OpenD
   仅回 RTH 数据，盘前/盘后 bar 静默缺失。
   修复位置：`crates/jftrade-integration-futu/src/subscription_executor.rs`
   `qot_sub_request`（复用 `kline_query::period_duration_seconds` 与
   `history_session_plan::SESSION_ALL`，仅 US 且 <=1h 生效）。
   回归：`subscription_pairs_are_exact_idempotent_and_use_all_session_for_us_intraday_kline`、
   `executor_sends_us_intraday_kline_session_all_on_the_wire`。

2. **订阅额度把 optionUsedQuota 误当自有连接额度**（P0，资金/额度安全相邻）。
   复现：Qot_GetSubInfo 返回 `ConnSubInfoList` 与 `optionUsedQuota` 时读取
   `ownUsedQuota`。
   预期（Go `Exchange.QuerySubscriptionQuota`）：OwnUsed 只累加
   `IsOwnConnData=true` 的 `UsedQuota`；S2C tag 4 是 `optionUsedQuota`。
   实际（修复前）：Rust 直接把 tag 4 当 OwnUsed，且 S2C 根本没有解析
   ConnSubInfoList，导致订阅编排用错误的额度做决策。
   修复位置：`crates/jftrade-integration-futu/src/session_coordinator.rs`
   （新增 `ConnSubInfo`、`own_used_quota()`，并保持
   `IsReqAllConn=true`）；同步修正
   `crates/jftrade-integration-futu/tests/fake_framed_opend_runtime_tests.rs`
   与 `crates/jftrade-engine/tests/market_data_production_compatibility.rs`
   两处使用错误 tag 语义的 fixture。
   回归：`quota_separates_own_and_other_connections_without_reading_option_quota`
   及真实 composition runtime 用例。

### 行为映射

| Go 测试 | Rust 入口 |
| --- | --- |
| `:19 TestExchangeSubscriptionMethodsArePairedExactAndIdempotent` | `crates/jftrade-integration-futu/src/subscription_executor.rs::tests::subscription_pairs_are_exact_idempotent_and_use_all_session_for_us_intraday_kline` |
| `:80 TestExchangeSubscriptionCacheUpdatesOnlyAfterOpenDConfirmation` | `crates/jftrade-integration-futu/src/subscriptions_tests.rs::failed_or_replayed_subscriptions_are_not_active_until_opend_confirms` |
| `:123 TestQueryKLinesWithoutLeaseReturnsExplicitErrorBeforeRealtimeRead` | `crates/jftrade-engine/src/product_market_data_quote_read_tests.rs::candle_read_without_a_kline_lease_fails_before_realtime_provider_access` |
| `:142 TestBasicQuoteReadRequiresExplicitLeaseAndNeverLeaksRawOpenDError` | `crates/jftrade-engine/src/product_market_data_quote_read_tests.rs::basic_quote_read_requires_a_lease_and_never_leaks_the_raw_opend_error` |
| `:162 TestConnectionGenerationInvalidatesClosedSessionAndItsSubscriptions` | `crates/jftrade-integration-futu/src/subscriptions_tests.rs::closed_session_generation_invalidates_its_subscriptions_and_requires_replay` |
| `:194 TestExchangeCloseIsTerminalAndPreventsOrphanedReconnect` | `crates/jftrade-integration-futu/src/session_coordinator.rs::tests::coordinator_close_is_terminal_and_prevents_orphaned_reconnect` |
| `:215 TestSubscriptionRequiredErrorFormattingAndNilConnectionGeneration` | `crates/jftrade-engine/src/product_production_ports_market_data_quote_lease.rs::tests::subscription_required_error_formats_channel_instrument_and_interval` |
| `:236 TestFailedConnectionDoesNotAdvanceEstablishedSessionGeneration` | `crates/jftrade-integration-futu/src/session_coordinator.rs::tests::failed_connection_does_not_advance_the_established_session_generation` |
| `:246 TestExchangeQuerySubscriptionQuotaSeparatesOwnAndOtherConnections` | `crates/jftrade-integration-futu/src/session_coordinator.rs::tests::quota_separates_own_and_other_connections_without_reading_option_quota` |
| `:270 TestSubscriptionMethodsValidateSymbolsAndIntervals` | `crates/jftrade-integration-futu/src/subscription_executor.rs::tests::subscription_methods_reject_invalid_symbols_and_intervals_before_qot_sub` |

### 边界说明

- Go 的 `Exchange.SubscribeKLine` 只发送 KL subtype；Rust 的物理订阅计划为
  `KLINE` 同时持有 BASIC + KLINE 两条物理记录（K 线读取需要 Basic 快照）。
  因此成对/幂等断言覆盖 3 条物理订阅，而不是 Go 的 2 条；这是既有架构
  owner 差异，不改变 Go 的 wire 语义（subtype、push 标志、session 均逐字段对齐）。
- `:215` 的 Go nil receiver 分支（`var nilExchange *Exchange`）在 Rust 没有对应
  类型：`OpenDSessionCoordinator` 不是指针可空对象，无 session 时返回
  `Closed`。该分支已在同批 `coordinator_close_is_terminal_...` 用 post-close
  行为覆盖，故仍记 `function_exact` 而非 boundary。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/adapter_kline_pagination_test.go（9 项）

基线：`go:320084a5`。本批 9 项全部 `[x]`（`function_exact`）。

### 功能差异与修复

**反向时间窗口被静默接受**（P1，分页/取消相邻）。
复现：`GET /api/v1/market-data/candles/HK/00700?period=5m&from=2026-07-02&to=2026-07-01`。
预期（Go `queryBrokerKLines`）：`fromTime must be earlier than or equal to toTime`，400，且不触碰 provider。
实际（修复前）：Rust 让窗口收敛成 lookback，返回了与请求无关的 K 线，掩盖调用方错误。
修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`
（在 `before` 组合校验之后、provider 解析之前前置校验解析后的 `from`/`to`）。
回归：`broker_kline_query_rejects_cursor_and_time_boundary_errors`。

其余 8 项在 Rust 已有等价行为，本批补齐独立行为证据：分页 cursor 精确秒 +
本地排他过滤、HK 市场时区窗口与 UTC 投影、latest/earliest 窗口选择与重复
bucket 取最新、inclusive range、声明周期同时具备历史与实时映射、列表日期
解析边界。

### 行为映射

| Go 测试 | Rust 入口 |
| --- | --- |
| `:15 TestBrokerKLinesReturnLatestPageAndUseExclusiveBeforeCursor` | `crates/jftrade-engine/src/product_market_data_candle_pagination_tests.rs::candle_pagination_tests::broker_klines_return_latest_page_and_use_exclusive_before_cursor` |
| `:66 TestFutuDeclaredCandlePeriodsMapToHistoricalAndRealtimeTypes` | `crates/jftrade-integration-futu/src/kline_query.rs::tests::declared_candle_periods_map_to_historical_and_realtime_types` |
| `:92 TestBrokerKLineQueryFormatsOpenDWindowInMarketTimeAndReturnsUTC` | `...candle_pagination_tests::broker_kline_query_formats_opend_window_in_market_time_and_returns_utc` |
| `:121 TestBrokerKLineCursorPreservesExactSecondWindowAndExcludesBoundaryLocally` | `...candle_pagination_tests::broker_kline_cursor_preserves_exact_second_window_and_excludes_boundary_locally` |
| `:152 TestNormalizeBrokerKLinePageDeduplicatesSortsAndKeepsLatest` | `...candle_pagination_tests::normalize_broker_kline_page_deduplicates_sorts_and_keeps_latest` |
| `:198 TestNormalizeBrokerKLineRangeKeepsInclusiveBoundaries` | `...candle_pagination_tests::normalize_broker_kline_range_keeps_inclusive_boundaries` |
| `:223 TestBrokerKLineQueryRejectsCursorAndTimeBoundaryErrors` | `...candle_pagination_tests::broker_kline_query_rejects_cursor_and_time_boundary_errors` |
| `:284 TestBrokerKLinePaginationHelpersCoverSessionsBoundsAndListingDates` | `crates/jftrade-engine/src/product_production_ports_market_data_quote_reads_futu.rs::tests::broker_kline_pagination_helpers_cover_sessions_bounds_and_listing_dates` |
| `:342 TestFutuCandlePeriodCatalogSkipsMissingAndUnmappableIntervals` | `crates/jftrade-integration-futu/src/kline_query.rs::tests::candle_period_catalog_rejects_missing_and_unmappable_intervals` |

### 边界说明

- Go 的 `normalizeBrokerKLinePage` 是 broker 层自由函数；Rust 的对应 owner 是
  `jftrade_integration_futu::kline_query::merge_klines_by_time`（bucket 去重取后到值 +
  升序），窗口截取由 `read_candles` 决定。测试按同一 owner 组合断言，未复制实现。
- Go fixture 的 `testHistoryKLine` 由 OpenD 原始 label 经 `futuHistoryKLineStartTime`
  转为 bucket start；Rust 的 loopback `HistoricalKlineReadPort` fixture 直接提供
  已转换的 bucket start。OpenD 原始 label 的位移在
  `kline_query::adjust_kline_time` 的单测中覆盖，本批不重复。
- Go 支持 `Adjustment`（`split-only` 报错）；Rust 路由不接受该参数，该子项由
  既有 API 校验测试覆盖，未在本批重复。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/adapter_option_fix_test.go（9 项）

基线：`go:320084a5`（上一批提交后）。本批把 Futures/期权事件族的
`injectAdvancedDefaults` + `opend.ValidateAdvancedC2S` + drilldown 归一化逐项映射到
Rust 的 typed OpenD reader 与 engine 翻译层。

### 发现的功能差异（已修复）

1. **earnings screener 拒绝 Go 允许的 index 市场**（P1，provider error mapping）
   - Go `injectOptionEventDefaults` → `futuOptionMarket` 对
     `underlyingProductClass=index` 返回 2（US index）/4（HK index），
     `opend.ValidateAdvancedC2S` 对 1..4 全部通过。
   - 修复前：`ProductMarketDataOptionsPort` 的 earnings 分支只接受
     `US/HK` 的 `equity|option`，`operation=earnings&market=HK&underlyingProductClass=index`
     返回 400 `earnings screener supports US/HK security options only`；
     typed reader `validate_query` 也只接受 1|3。
   - 复制条件：`GET /api/v1/market-data/options/events?operation=earnings&market=HK&underlyingProductClass=index`。
   - 预期（Go）：`optionMarket=4`，请求送达 provider。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_options_events.rs::parse_screener_common`、
     `crates/jftrade-integration-futu/src/option_earnings_screener_query.rs::validate_query`。
   - 回归测试：`option_event_operation_queries_pass_strict_opend_validation`、
     `option_earnings_screener_query::tests::accepts_security_and_index_option_markets`。

2. **0DTE screener 公开 entry 泄漏原始 `chainInfo`**（P1，wire 契约）
   - Go `optionZeroDteFeatureResult` 先把 `chainInfo` 投影为 `drilldownContext`
     （owner → `chainInfo.underlying` 回退），然后 `delete(entry, "chainInfo")`。
   - 修复前：Rust 写入 `drilldownContext` 但保留 `chainInfo`，公开 entry 同时含
     OpenD 原始结构与中立结构；且 underlying 只取 owner，不回退嵌套
     `chainInfo.underlying`。
   - 复制条件：`GET /api/v1/market-data/options/events?operation=zero_dte&market=US&underlying=US.AAPL`
     （owner 缺失时 `chainInfo.underlying` 被忽略）。
   - 修复位置：`product_production_ports_market_data_options_events.rs::project_zero_dte_drilldown`。
   - 回归测试：`zero_dte_drilldown_context_prefers_owner_and_falls_back_to_nested_underlying`。

### 行为映射

| Go 测试 | Rust owner / 测试 | 状态与结论 |
| --- | --- | --- |
| `:12 TestFutuRootAdapterForwardsBatchSnapshotsThroughProtocol3203` | `product_production_ports_market_data_actions_tests::batch_snapshots_forward_each_instrument_through_the_snapshot_reader_once` | `[x]` function_exact：每个标的恰好一次 3203 owner 读取，转发标的逐字一致 |
| `:31 TestFutuDeclaredCapabilitiesHaveExecutableAdapterInterfaces` | `product_production_assembly_tests::every_declared_capability_resolves_to_an_executable_production_adapter` | `[x]` function_exact：catalog 每个 feature 的 adapterInterface/operations 都能解析到已安装 adapter |
| `:56 TestFutuOptionEventRequestsPassStrictOpenDValidation` | `product_production_ports_market_data_options_tests::option_event_operation_queries_pass_strict_opend_validation` | `[x]` function_exact：US equity/index、HK index earnings、covered call/CSP 全部通过严格校验；0DTE 保持 US-only |
| `:120 TestFutuZeroDteContractRebuildsBrokerNeutralChainContext` | `...options_tests::zero_dte_contract_query_rebuilds_chain_context_and_projects_drilldown` | `[x]` function_exact：locator→chainInfo 重建 + typed 校验，公开 entry 无原始 chainInfo |
| `:168 TestFutuOptionEventValidationRejectsUnsupportedInputs` | `option_seller_screener_query::tests::strict_validation_rejects_each_unsupported_seller_boundary` | `[x]` function_exact：非法 optionMarket/sellerType/sort/indicator 在 RPC 前拒绝 |
| `:206 TestFutuOptionRequestTranslationBoundaries` | `...options_tests::option_event_request_translation_boundaries_match_go_helpers` | `[x]` function_exact：不支持市场、非法 security 前缀、owner filter 形状 |
| `:261 TestFutuZeroDteContractTranslationRejectsEachInvalidBoundary` | `...options_tests::zero_dte_contract_translation_rejects_each_invalid_boundary`；`option_zero_dte_contract_query::tests::strict_validation_rejects_each_invalid_zero_dte_contract_boundary` | `[x]` function_exact：六类边界逐项拒绝 |
| `:331 TestFutuOptionNumericParameterBoundaries` | 不适用：Rust 无 untyped numeric parameter helpers | `[~]` boundary：强类型 serde 字段替代 `int64Param/floatParam/int32Param`，非法数字在反序列化阶段拒绝 |
| `:362 TestFutuZeroDteNormalizationHandlesAbsentAndNestedUnderlying` | `...options_tests::zero_dte_drilldown_context_prefers_owner_and_falls_back_to_nested_underlying` | `[x]` function_exact：owner 优先、嵌套 underlying 回退、无 chain 原样通过 |

### 边界说明

- Go 的 `injectAdvancedDefaults` 是"先注入默认值、再跑 protojson 严格反序列化"的两段式；
  Rust 把同一语义拆成 engine 翻译（`parse_screener_common` / `parse_seller_query` /
  `parse_zero_dte_contract_query`）+ typed reader 的 `validate()`，两处共同构成等价边界，
  因此同一 Go 测试可能映射到 engine 与 integration 两个 Rust 测试（见 `:261`）。
- 三个 screener/contract query 新增 `pub fn validate()`，把原本私有的
  `validate_query` 暴露为显式契约，便于调用方与测试在 RPC 前断言，不改变调用路径。
- Go 的 `int64Param` 接受 `int/int32/int64/float64/json.Number/string` 六种动态类型；
  Rust 的请求体通过 `serde` 直接反序列化为类型化字段，不存在运行期动态转换分支，
  故记 boundary 而不是伪造 helper。
- `:120` 的 Go 断言还包含 `sortType=2`（open_interest）；Rust 的
  `FixtureOptionZeroDteContractReader` 断言重建后的 `sort_type == Some(2)`，
  由同一 Rust 测试覆盖。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/quote_snapshot_test.go（8 项）

基线：`go:320084a5`。本批核对 `quoteSnapshotFromBasicQotAt` 的
previousClosePrice 会话条件、HpVolume 精度回退、HK 午休与节假日陈旧 extended block。

### 发现的功能差异（已修复）

1. **previousClosePrice 条件漏掉 unknown 会话**（P1，session 边界）
   - Go：`market.ShouldUseRegularCloseAsPreviousClose(symbol, session, regularClose)` =
     `IsUSSymbol(symbol) && session != SessionRegular && regularClose > 0`。
   - 修复前：Rust `product_production_ports_market_data_quote_snapshot.rs` 用
     allow-list `matches!(session, "pre" | "after" | "overnight" | "closed")`，
     US `unknown` 会话不会使用最近常规收盘价；同时未显式断言 `regularClose > 0`
     （零价只在 `decimal_value(Some(..))` 路径上间接表现）。
   - 复制条件：US 标的在日历解析失败/未覆盖时得到 `session=unknown`，
     `GET /api/v1/market-data/snapshots/US/AAPL` 返回 provider LastClosePrice 而非最近常规收盘价。
   - 预期（Go）：`session != regular` 的全部 US 会话（含 `unknown`）使用最近常规收盘价。
   - 修复位置：新增 `uses_regular_close_as_previous_close(market, session, regular_close)`，
     `project_cached_snapshot` 与 `project_fallback_snapshot` 共用。
   - 回归测试：`previous_close_condition_switches_on_session_type`、
     `previous_close_condition_does_not_rewrite_non_us_unknown_sessions`。

### 行为映射

| Go 测试 | Rust owner / 测试 | 状态与结论 |
| --- | --- | --- |
| `:14 TestQuoteSnapshotResolvesHighPrecisionVolumeWithoutLosingInt64Precision` | `basic_quote_tick::tests::high_precision_volume_never_loses_required_int64_precision` | `[x]` function_exact：1000.5 生效；0/NaN/缺失回退 int64，9_007_199_254_740_993 精度不丢失 |
| `:49 TestQuoteSnapshotPreviousClosePriceInClosedSession` | `quote_snapshot::tests::closed_us_session_reports_regular_close_as_previous_close` | `[x]` function_exact：closed 用最近常规收盘价，lastClosePrice 保留原值 |
| `:93 TestQuoteSnapshotHolidayRemainsClosedWithStaleExtendedBlocks` | `quote_snapshot::tests::holiday_snapshot_stays_closed_with_stale_extended_blocks` | `[x]` function_exact：节假日保持 closed，三个陈旧 block 保留且无独立 quoteTime |
| `:133 TestQuoteSnapshotPreviousClosePriceInAfterHours` | `quote_snapshot::tests::after_hours_projection_uses_todays_regular_close_and_keeps_block_time_empty` | `[x]` function_exact：盘后主价格取 after block，previousClosePrice=当日常规收盘 |
| `:164 TestQuoteSnapshotPreviousClosePriceZeroCurPrice` | `quote_snapshot::tests::zero_current_price_falls_back_to_provider_last_close` | `[x]` function_exact：CurPrice=0 回退 LastClosePrice |
| `:183 TestQuoteSnapshotPreviousClosePriceForHKLunchBreak` | `basic_quote_tick::tests::hk_lunch_break_keeps_provider_last_close_as_previous_close` | `[x]` function_exact：HK 午休 closed + lastClose 保留，price≠previousClose |
| `:217 TestPreviousClosePriceConditionBySessionType` | `quote_snapshot::tests::previous_close_condition_switches_on_session_type` | `[x]` function_exact：regular 用 LastClose，其余（含 unknown）用最近常规收盘；<=0 不改写 |
| `:258 TestPreviousClosePriceConditionDoesNotRewriteNonUSUnknownSession` | `quote_snapshot::tests::previous_close_condition_does_not_rewrite_non_us_unknown_sessions` | `[x]` function_exact：HK/SH/SZ/CN unknown/closed 均不改写 |

### 边界说明

- Go 的 `quoteSnapshotFromBasicQotAt` 同时产出 `Price`/`PreviousClosePrice`/`LastClosePrice`
  三个字段与 pre/after/overnight block；Rust 把它拆成
  `basic_quote_tick::basic_quote_ticks_with_resolver`（provider→neutral，session 与 block 归属）
  与 `product_production_ports_market_data_quote_snapshot::project_cached_snapshot`
  （neutral→公开 wire，previousClosePrice 决策）。同一 Go 断言按 owner 归属分别映射，
  未在两侧重复实现同一规则。
- Go 的 `SessionUnknown` 只在日历无法分类时出现；Rust 的 `normalized_session`
  把缺失 session 归一为 `regular`，因此 `unknown` 只会在日历显式返回该标签时出现，
  规则本身仍按 `!= regular` 判定。
- HK 午休用例在 Rust 需要显式注入日历 resolver（`StaticSessionResolver`），
  因为无 resolver 的 fallback 只按市场分钟区间判定；生产组合始终注入
  `RuntimeCalendarResolver`，与 Go 的 `CurrentCalendarResolver` 一致。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/advanced_product_adapter_contracts_test.go（8 项）

基线：`go:206a9dfd`。本批 8 项最终为 2 `[x]`、6 `[~]`（partial/boundary），发现并修复 2 处真实差异。

### 真实功能差异（已修复）

1. **Option combo 腿缺少 MARKET.CODE 前缀被静默接受（P0，订单写入边界）**
   - 复现：`POST /api/v1/execution/combos/previews`，legs 中传入 `{"instrumentId":"BAD",...}`。
   - 预期（Go）：`futuSecurityFromSymbol` -> `market.ParseInstrument` 拒绝无市场前缀的符号，
     `PreviewComboOrder`/`PlaceComboOrder` 返回错误。
   - 修复前：Rust `parse_combo_with_defaults` 用 `quote_market_from_trade_market(trd_market)`
     回退，把 `BAD` 当成交易市场的未限定代码接受并继续组合流程。
   - 修复后：非 event_parlay 腿必须含 `MARKET.CODE`，否则返回
     `combo leg instrumentId must be in MARKET.CODE form`；并删除不再使用的
     `quote_market_from_trade_market`。
   - 回归测试：`product_production_ports_execution_order_validation_tests::combo_intent_rejects_missing_kind_legs_and_account`。

2. **SecuritySnapshot securityType 缺 PlateSet/Forex/Crypto（P2，产品身份标签）**
   - 复现：Qot_GetSecuritySnapshot 返回 secType=9/11/12。
   - 修复前：`security_snapshot_query::security_type` 落 `UNKNOWN`，而 `instrument_search_query`
     已输出 `PLATESET`/`FOREX`/`CRYPTO`，同一 provider 内两套标签不一致。
   - 修复后：补齐 9/11/12 分支，并与搜索 reader 的枚举表逐值一致。
   - 回归测试：`security_snapshot_query::tests::security_type_mapping_matches_futu_proto_definitions`。

### 行为映射

| Go 测试 | Rust 入口 | 状态与结论 |
| --- | --- | --- |
| `:22 TestFutuAdvancedSpecializedReadersAndCustomizationSuccess` | `user_security_protocol.rs::remote_watchlist_modify_encodes_group_operation_and_security_list`；`prediction_category_protocol.rs::prediction_category_read_encodes_protocol_and_projects_entries`；`product_production_ports_market_data_catalog_tests.rs::futu_search_distinguishes_no_match_unsupported_market_and_runtime_failure` | `[~]` partial：按 owner 拆分；新增 3214 watchlist 写入与 3434 分类 loopback 证据，search/错误传播与 depth 已有独立测试 |
| `:150 TestFutuComboAdapterErrorPropagationBranches` | `product_production_ports_execution_order_validation_tests.rs::combo_intent_rejects_missing_kind_legs_and_account` | `[~]` partial：三条 parse 边界已断言并修复无前缀腿；place/cancel 缺账户两条适配器级断言仍缺 |
| `:216 TestFutuEventContractStatusBranches` | `product_production_ports_execution_preview_tests.rs::event_parlay_preview_rejects_inactive_contract_filtered_from_snapshot_list` | `[x]` function_exact：过滤不相关 ACTIVE 行后按 code 判定 CLOSED 并拒绝 |
| `:235 TestFutuWarrantsStayHKOnlyAndFuturesRemainDiscoverable` | `product_production_ports_trade_tests.rs::broker_capabilities_keep_warrants_hk_only_and_futures_discoverable_in_hk_us` | `[x]` function_exact：warrants 仅 HK、productClasses=[warrant,cbbc]，futures 仅 HK|US |
| `:262 TestSecurityDetailsProductIdentityFallbacks` | `product_market_data_quote_read_tests.rs::futu_securities_route_projects_broker_neutral_envelope_boundary` | `[~]` boundary：Rust 无 SecurityDetails/ProductClass/MarketSegment 类型，公开路由只投影 9 字段契约 |
| `:336 TestFutuSnapshotProductExtensionsAndSecurityTypeMapping` | `security_snapshot_query.rs::tests::security_type_mapping_matches_futu_proto_definitions` | `[~]` partial：securityType 枚举已与 Go 对齐并修复 9/11/12；SnapshotExData 子块与 ProductClass 无对应结构 |
| `:403 TestFutuTradeProductRequestAndReadLifecycleBranches` | `execution_order_validation_tests.rs::event_single_rejects_negative_amount_and_invalid_prediction_side`；`trade_session_tests.rs::event_contract_place_order_encodes_amount_and_prediction_side` | `[~]` partial：amount/predictionSide 校验与 Trd_PlaceOrder 编码已覆盖；持仓 ProductClass/OrderKind 属边界 |
| `:475 TestFutuComboProtocolTransportErrors` | `trade_session_tests.rs::combo_protocol_transport_errors_are_surfaced`；`execution_preview_tests.rs::event_parlay_preview_surfaces_snapshot_transport_failure` | `[~]` partial：combo max/place 会话关闭报错与 3445 Transport→Unavailable 分流均有回归 |

### 新增测试

- `crates/jftrade-integration-futu/tests/prediction_category_protocol.rs`：loopback 3434 分类读取，
  断言请求 c2s.category 与 categoryName/tags 投影。
- `crates/jftrade-integration-futu/tests/user_security_protocol.rs`：
  `remote_watchlist_modify_encodes_group_operation_and_security_list`（3214 group/op/securityList）。
- `crates/jftrade-integration-futu/src/trade_session_tests.rs`：
  `combo_protocol_transport_errors_are_surfaced`、
  `event_contract_place_order_encodes_amount_and_prediction_side`。
- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs`：
  `event_parlay_preview_rejects_inactive_contract_filtered_from_snapshot_list`、
  `event_parlay_preview_surfaces_snapshot_transport_failure`。
- `crates/jftrade-engine/src/product_production_ports_execution_order_validation_tests.rs`：
  `combo_intent_rejects_missing_kind_legs_and_account`、
  `event_single_rejects_negative_amount_and_invalid_prediction_side`。
- `crates/jftrade-engine/src/product_production_ports_trade_tests.rs`：
  `broker_capabilities_keep_warrants_hk_only_and_futures_discoverable_in_hk_us`。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/adapter_research_normalization_boundaries_test.go（8 项）

状态：8 项全部 `[x]` function_exact。该批次补齐了 Rust 缺失的 research 参数注入、payload 归一化和 quote-rights generation fencing 三个 owner，此前 Rust 侧完全没有对应实现。

### 真实功能差异与修复

1. **research 参数注入缺失（P1，公开 API 范围）**
   - 差异：Go `injectAdvancedProtocolDefaults`（`pkg/futu/adapter_advanced_defaults.go`）负责把公开 query 翻译为严格 OpenD C2S 参数：plateType→plateSetType、direction→sortDir、market→marketList、institutionId→int32、instrumentId→news keyword、secType=4 等。Rust 只有 typed port 校验，没有该翻译层，公开 query 可绕过 market/枚举/数值边界。
   - 修复：新增 `crates/jftrade-integration-futu/src/research_params.rs`，导出 `inject_advanced_research_defaults`、`inject_advanced_option_defaults`、`inject_advanced_protocol_defaults`、`translate_top_movers_direction`、`translate_heat_map_plate_type`、`translate_plate_set_type`、`bounded_research_enum`、`research_number_parity`、`ResearchQueryScope`。
   - 回归：`crates/jftrade-integration-futu/src/research_params_tests.rs` 4 个测试。

2. **research payload 归一化与本地分页缺失（P1）**
   - 差异：Go `normalizeResearchProtocolPayload` / `applyResearchLocalPagination` 为 rankings/calendar/institution 行补 instrumentId、symbol、name、productClass、changeRate、price、marketValue、dividendYield、calendarType、eventTimestamp 等 canonical 字段，并对 PlateSet/PlateSecurity/StaticInfo 做 `local:N` 本地分页。Rust 路由只透传 typed reader 结果。
   - 修复：新增 `crates/jftrade-integration-futu/src/research_normalization.rs`，导出 `normalize_research_protocol_payload`、`research_security_type`、`research_product_class`、`flatten_research_ipo`、`normalize_research_calendar_fields`、`normalize_research_institution_fields`、`apply_research_local_pagination`、`LocalResearchPage`。
   - 回归：`crates/jftrade-integration-futu/src/research_normalization_tests.rs` 4 个测试。

3. **quote-rights 状态机缺失导致能力矩阵谎报可用（P0 公开 API/权限安全）**
   - 差异：Go 维护 connect status、quote-rights snapshot、revision 与可重试失败缓存，并按 OpenD connection generation fencing；stale 通知不得覆盖新 generation，notification 可清除同 generation 失败，late query 不得覆盖 notification。Rust 原先仅用 `connection_ready` 直接输出 `QUOTE_RIGHT_AVAILABLE`，Socket 连接即被当作权限已验证。
   - 修复：
     - 新增 `crates/jftrade-integration-futu/src/quote_rights.rs`（`QuoteRightsState`、`QuoteRightField`、`QuoteRightSnapshot`、`QuoteRightState`、`QuoteRightsFetchOutcome`、3s `QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL`）。
     - 新增 `crates/jftrade-engine/src/product_trade_runtime_quote_rights.rs` 作为唯一 owner，并在 `product_trade_runtime_projection.rs` 暴露；`product_broker_capabilities_projection.rs` 改为按 market/product 读取已验证权限，未验证固定输出 degraded `QUOTE_RIGHT_UNVERIFIED`。
     - 复现条件：`/api/v1/brokers/capabilities` 在 OpenD 已连接但未收到 QotRight 推送时，旧实现返回 `QUOTE_RIGHT_AVAILABLE`/`RUNTIME_READY`；预期为 degraded `QUOTE_RIGHT_UNVERIFIED`/`RUNTIME_STATUS_PARTIAL`，只有 active generation 的已验证快照才可 available。
   - 回归：`crates/jftrade-integration-futu/src/quote_rights_tests.rs` 6 个测试；`crates/jftrade-engine/src/product_production_ports_trade_tests.rs::broker_capabilities_stay_degraded_until_a_generation_verifies_quote_rights`，并同步修正原先断言“连接即可用”的 `broker_capabilities_microstructure_and_research_runtime_ready`。

### 行为映射

| Go 测试 | Rust 入口 | 状态与结论 |
| --- | --- | --- |
| :16 TestAdvancedResearchDefaultsRejectIncompleteQueries | research_params_tests.rs::advanced_research_defaults_reject_incomplete_queries | `[x]` function_exact：7 条缺失/非法参数逐条拒绝 |
| :54 TestAdvancedResearchDefaultsTranslatePublicInputs | research_params_tests.rs::advanced_research_defaults_translate_public_inputs | `[x]` function_exact：plate/economic/institution/news/unknown 翻译 |
| :96 TestAdvancedResearchEnumTranslations | research_params_tests.rs::advanced_research_enum_translations_match_go_bounds | `[x]` function_exact：top movers/heatmap/plateSet/boundedResearchEnum 边界 |
| :178 TestResearchNormalizationCoversAlternateWireShapes | research_normalization_tests.rs::alternate_wire_shapes_keep_scalars_and_add_aliases | `[x]` function_exact：scalar 保留、字段别名、越界游标 |
| :208 TestResearchNormalizationCoversProductAndCalendarVariants | research_normalization_tests.rs::product_and_calendar_variants_match_go_projection | `[x]` function_exact：securityType/productClass/IPO/calendar/institution |
| :284 TestResearchNumberAcceptsSupportedScalarTypes | research_params_tests.rs::research_number_accepts_go_supported_scalar_types | `[x]` function_exact：标量与 " 1 " 文本接受、bool 拒绝 |
| :306 TestQuoteRightsRefreshStateEdges | quote_rights_tests.rs::notification_resolution_wins_over_inflight_query_result | `[x]` function_exact：stale generation fencing、notification 优先、fresh store |
| :368 TestQuoteRightsFailureAndExchangeGenerationEdges | quote_rights_tests.rs::fetch_failure_outcomes_follow_generation_and_notification_state | `[x]` function_exact：失败缓存/重试、generation 0 未验证、投影 fail-closed |

### 边界说明

- `Qot_GetOptionChain` 日期、`Qot_GetOptionMarketStatistic`、warrant/macro 默认值随参数注入 owner 一并实现，但本批次未新增独立断言；后续 option/macro 批次按各自 Go 测试补齐。
- quote-rights 的 OpenD 通知写入与 activation generation 接线仍需在 live OpenD 场景验证（`JFTRADE_FUTU_LIVE_TEST=1`），本批次以状态机与投影回归为主。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1609 passed
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/opend/market_read_boundaries_test.go（7 项）

状态：7 项全部 `[x]` function_exact。本批转入 OpenD 低层读写边界，新增 socket 级回归（真实 framing + 脚本化 OpenD 服务器），并修复两处“成功但缺 S2C 被当作错误”的语义偏差。

### 真实功能差异与修复

1. **Qot_RequestHistoryKL 缺 S2C 被误判为错误（P1 公开读取路径）**
   - 差异：Go `pkg/futu/opend/kline.go::RequestHistoryKL` 在 `retType==0` 且无 S2C 时返回空结果（`HistoryKLineResult{}`）。Rust `crates/jftrade-integration-futu/src/history.rs` 返回 `HistoricalKlineError::MissingS2c`，使“区间内无数据”被上层当成传输失败并触发重试。
   - 复现条件：OpenD 对 3103 回 `retType=0` 且无 s2c；预期空页（保留请求 identity、无 name/klines/nextReqKey），旧实现报错。
   - 修复：`history.rs` 改为返回带 `query_security` 的空 `HistoricalKlineResult`，并移除不再产生的 `MissingS2c` 变体；`history_window_tests`、`product_market_data_candle_pagination_tests`、`product_production_ports_backtest_sync::futu_error_retryable` 同步改为真实重试型错误（429）。
   - 回归：`crates/jftrade-integration-futu/tests/market_read_boundaries.rs::history_optional_fields_round_trip_and_missing_s2c_is_an_empty_result`。

2. **Qot_GetStaticInfo 缺 S2C 被误判为错误（P1 公开读取路径）**
   - 差异：Go `security_info.go::GetStaticInfo` 对缺 S2C 返回非 nil 空切片；Rust `instrument_search_query.rs::decode_lookup` 报 `MissingField("s2c")`。
   - 修复：改为返回 `Ok(Vec::new())`；`Qot_GetSecuritySnapshot` 的同类语义本已正确（`unwrap_or_default`）。
   - 回归：同批 socket 测试 `security_info_methods_return_empty_collections_for_a_payload_less_ack`。

### 行为映射

| Go 测试 | Rust 入口 | 状态与结论 |
| --- | --- | --- |
| :21 TestSubscribeQuotesEncodesAdvancedMarketDataOptions | tests/market_read_boundaries.rs::quote_subscribe_encodes_advanced_market_data_options_on_the_wire | `[x]` function_exact：Qot_Sub 高级字段（push/first/unsubAll/extendedTime/SESSION_ALL/orderBookDetail）|
| :77 TestRequestHistoryKLEncodesOptionalFieldsAndHandlesEmptyResult | tests/market_read_boundaries.rs::history_optional_fields_round_trip_and_missing_s2c_is_an_empty_result | `[x]` function_exact：可选字段上线 + 空页语义（本轮修复）|
| :129 TestMarketReadMethodsPropagateOpenDBusinessErrors | tests/market_read_boundaries.rs::market_read_business_errors_keep_opend_return_details | `[x]` function_exact：4 条业务错误保留 retType/errCode/retMsg |
| :192 TestSecurityInfoMethodsReturnEmptyCollectionsForEmptyOpenDResults | tests/market_read_boundaries.rs::security_info_methods_return_empty_collections_for_a_payload_less_ack | `[x]` function_exact：static info / snapshot 空集合（本轮修复 static info）|
| :212 TestGetKLReturnsEmptyResultWhenOpenDOmitsS2C | tests/market_read_boundaries.rs::get_kl_returns_empty_result_when_opend_omits_s2c | `[x]` function_exact：socket 级 3006 空结果；`kline_query.rs::tests::get_kl_missing_s2c_returns_an_empty_result` 保留给 new_methods_test.go:768 |
| :232 TestMarketReadMethodsRejectDisconnectedSession | tests/market_read_boundaries.rs::market_read_methods_reject_a_disconnected_session | `[x]` function_exact：未连接/已关闭 coordinator 失败关闭 |
| :284 TestMarketPushSubscribersIgnoreMalformedAndUnsuccessfulUpdates | tests/market_read_boundaries.rs::stale_or_malformed_push_updates_never_reach_the_lifecycle | `[x]` function_exact：拒绝推送丢弃、malformed typed error、空更新不产生 push |

### 边界说明

- Go 的 `QuoteSubRequest` 是通用低层 API，允许调用方任意组合 `RegPushRehabTypes` 等字段；Rust 按 demand/kind 生成 Qot_Sub，不暴露 rehab 列表，因此该字段以“Rust 按 kind 固定语义”断言，未强行迁移通用参数面。
- `:232` 的 BasicQot 分支由 `basic_quote_query` 既有 Session/SubscriptionRequired 测试覆盖，不在本文件重复造测试。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1615 passed
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/client_exchange_recovery_boundaries_test.go（7 项）

状态：4 项 `[x]` function_exact，3 项 `[~]`（partial / 边界保留）。该批次把 OpenD 客户端的错误分类、重连替换、通知发布与交易推送订阅四条边界补齐为 socket 级回归，并修复了三处真实缺陷：重连通知自锁、InitConnect 不校验最低版本、以及 recoverable 分类在多个读端口各自为政。

### 真实功能差异与修复

1. **重连期间发布通知会自锁（P1，重连/并发）**
   - 差异：Go 的 `TestReconnectDoesNotDeadlockWithInFlightNotification` 让 system-notify handler 在重连进行中调用 `exchange.ConnectionGeneration()`，要求重连与 handler 都不阻塞。Rust `runtime_task.rs::run_task` 在持有 coordinator `Mutex` 时调用 `OpenDSessionEventListener::on_event`，任何回读 coordinator（engine 的 `LiveHubOpenDEventListener` 就带 reconciliation wake 路径）的 listener 都会在同一线程自锁，重连永远无法完成。
   - 复现条件：`OpenDSessionRuntime` 配 event listener，其 `on_event` 调 `coordinator.lock()...generation()`；peer close 后运行状态下 `status().reconnects` 长时间为 0。
   - 修复：把事件发布移到 `drop(coordinator)` 之后（用 `published_outcome` 暂存本轮 outcome）；`on_error` 保持原有顺序。
   - 回归：`crates/jftrade-integration-futu/tests/client_recovery_boundaries.rs::reconnect_completes_while_a_notification_listener_reads_coordinator_state`（listener 内回读 generation，断言收到 generation 2）。

2. **InitConnect 未校验最低 OpenD 版本（P1，会话建立）**
   - 差异：Go 的 `validateInitConnectResponse` 在 InitConnect 阶段就用 `ValidateMinimumVersion(serverVer, nil)` 拒绝旧版本，因此 `TestOldOpenDVersionFailsSessionInitialization` 的 `Connect()` 直接失败。Rust 原先只在 health probe 的 GetGlobalState 阶段比较 `serverVer/serverBuildNo`，InitConnect 报 10.8 仍会建立可用会话，行情/交易路径可继续使用不受支持的协议面。
   - 复现条件：脚本化 OpenD 对 InitConnect 回 `retType=0, S2C.serverVer=1008`；旧实现 `connect_with_push_notifications` 返回 Ok。
   - 修复：新增 `health.rs::version_supported_without_build` 与 typed 变体 `OpenDTcpProbeError::UnsupportedVersion { server_version }`（消息带检测到的版本与 `MINIMUM_OPEND_VERSION`）；`initialize_session` 在返回前校验。`crates/jftrade-engine/src/product_production_ports_open_d_snapshot.rs` 将该变体并入 degraded 协议失败分支（升级引导而非重启）。
   - 回归：`client_recovery_boundaries.rs::below_minimum_version_fails_session_initialization`；同时修正多份缺失 `serverVer` 的测试 fixture（Go 的 `quoteOpenDServer` 始终上报 1009），避免 fixture 与真实 OpenD 行为不一致。

3. **recoverable 分类分散在多处（P1，重放策略唯一 owner）**
   - 差异：Go 用单一 `isRecoverableOpenDErr` 决定 `withClient`/`withRetryingClient` 是否重放。Rust 的 `basic_quote_query::is_recoverable_session_error` 自成一类判断，新增读端口极易写出更宽或更窄的重放策略。
   - 修复：新增 `crates/jftrade-integration-futu/src/recoverable_error.rs`（`OpenDRecoverableKind`、`classify_recoverable`、`classify_recoverable_io`、`is_recoverable_error`）作为唯一 owner，分类同时覆盖 Go 的 `opend: client closed` / `opend: request timed out` 哨兵文本；`is_recoverable_session_error` 改为委托该 owner。
   - 回归：`recoverable_error_tests.rs` 2 个测试 + 本批 socket 测试的 peer-close 重放断言。

### 行为映射

| Go 测试 | Rust 入口 | 状态与结论 |
| --- | --- | --- |
| :19 TestWithClientReplayPolicyForRecoverableErrors | tests/client_recovery_boundaries.rs::recoverable_error_policy_gates_replay_safe_reads | `[x]` function_exact（含功能修复）：分类表 + peer close 后 generation 2 的订阅重放 |
| :56 TestExchangeReconnectsClosedReadyClientAndCoversHandlerBoundaries | tests/client_recovery_boundaries.rs::closed_ready_session_is_replaced_on_peer_close | `[x]` function_exact（收窄到会话替换）：ready 会话关闭后替换为 generation 2 且可用 |
| :107 TestReconnectDoesNotDeadlockWithInFlightNotification | tests/client_recovery_boundaries.rs::reconnect_completes_while_a_notification_listener_reads_coordinator_state | `[x]` function_exact（含功能修复）：listener 在重连中回读 generation 不再自锁 |
| :198 TestTradeAccountPushNormalizationSubscriptionAndFactoryEnvironment | tests/client_recovery_boundaries.rs::trade_push_subscription_forwards_the_requested_accounts | `[~]` partial：Trd_SubAccPush 转发与成功 ack 等价；排序去重/幂等/重连重放与 bbgo env-prefix 工厂属旧 owner，边界保留 |
| :249 TestOldOpenDVersionFailsSessionInitialization | tests/client_recovery_boundaries.rs::below_minimum_version_fails_session_initialization | `[x]` function_exact（含功能修复）：InitConnect 阶段拒绝旧版本 |
| :260 TestInitResponseAndSessionTransportFailures | tests/client_recovery_boundaries.rs::init_response_and_session_transport_failures_stay_typed | `[x]` function_exact：retType 拒绝、缺 S2C、InitConnect/GetGlobalState 断线全部 typed |
| :282 TestReconnectTradePushFailureAndTradeHandlerBinding | tests/client_recovery_boundaries.rs::trade_push_subscription_failure_surfaces_a_transport_error | `[~]` partial：未 ack 的 2008 失败关闭；order/fill 推送绑定由 engine 轮询对账 owner 承担，边界保留 |

### 边界说明

- Rust 没有 Wails/bbgo exchange 工厂层：OpenD 地址来自 settings 的 `host`/`apiPort`，`NewWithEnvVarPrefix` 的 env 前缀回退在本仓库无对应入口，因此 :198 的工厂环境分支记录为边界而非移植。
- Rust 不注册 `OnSystemNotify`/`OnOrderBookUpdate`/`OnOrderUpdate`/`OnOrderFillUpdate` handler map；系统通知经 `notification.rs` 的中性分类，订单状态由 engine 的轮询对账与 push worker 拥有，handler 绑定语义不迁移。
- `OpenDTcpProbeError::UnsupportedVersion` 是新增公开变体；engine 健康投影已同步，live OpenD 的旧版本拒绝仍需在 `JFTRADE_FUTU_LIVE_TEST=1` 场景复核。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1625 passed, 1 skipped
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/exchange_mapping_boundaries_test.go（7 项）

基线：`go:452dea11:pkg/futu/exchange_mapping_boundaries_test.go`。该文件把 Go 的中性枚举、
符号与订阅契约打到 Futu wire 值上，其中 :158 暴露了一个真实的 P0 交易安全缺陷。

### 真实功能缺陷（P0：下单枚举写错 OpenD 协议）

1. **`Trd_PlaceOrder.orderType` 使用了私有编号（P0，交易安全）**
   - 差异：Go 的 `trdOrderTypeFromBBGOOrderType`（`pkg/futu/exchange_trade_write.go:324`）把中性订单类型映射到
     OpenD `Trd_Common.OrderType` 的 `Normal=1`、`Market=2`、`Stop=10`、`StopLimit=11`、
     `MarketifTouched=12`、`LimitifTouched=13`。Rust `parse_order_type`
     （`crates/jftrade-engine/src/product_production_ports_execution_order_parse.rs`）返回私有编号
     `STOP=3`/`STOP_LIMIT=4`，而 `ParsedOrder::to_trade_request()` 把这个值原封不动交给
     `trd_place_order::C2s.order_type`（`crates/jftrade-integration-futu/src/trade_session.rs::place_order_command`）。
   - 影响：止损单以 OpenD 未定义的 `3` 下发，`STOP_LIMIT` 被当成 `AbsoluteLimit`；同一错误编号还会经
     `order_type_label` 写进 execution ledger 的 `order_type`，而该字段随后被
     `product_production_ports_execution_reconciliation_recovery.rs::matches_safe_attributes`
     与 `execution_reconciliation_discovery.rs` 用来与 broker 快照比对，因此止损单的恢复/对账会静默失配，
     恢复路径可能把已提交的订单判为"非候选"或错误地保留错误类型。
   - 复现条件：POST `/api/v1/executions`（或 preview）携带 `orderType: "STOP"` / `"STOP_LIMIT"`；
     观察 `Trd_PlaceOrder.orderType` 得到 `3`/`4`（Go 为 `10`/`11`）。
   - 修复：`parse_order_type` 改为只接受 Go `normalizeExecutionOrderType` 允许的
     `LIMIT`/`MARKET`/`STOP`/`STOP_LIMIT`（保留既有 `NORMAL`/`STOP_MARKET`/`ABSOLUTE_LIMIT`/`AUCTION`/
     `AUCTION_LIMIT` 别名）并返回新增的 `ORDER_TYPE_*` wire 常量；价格门限改用 wire 常量比较，
     `fillOutsideRTH` 的适用集合同步为 `Normal|StopLimit`（对齐 Go `supportsFillOutsideRTH`）；
     `order_type_label` 改为同一组 wire→中性标签（`10→STOP`、`11→STOP_LIMIT`、`12→TAKE_PROFIT_MARKET`、
     `13→TAKE_PROFIT`、`14/15→TRAILING_*`），保证写入 ledger 与恢复路径的是中性词汇。
   - 回归：`product_production_ports_execution_order_validation_tests.rs::parsed_order_type_matches_the_opend_wire_enum`
     （同时断言 `parse_order().order_type` 与 `to_trade_request().order_type`，并断言 ICEBERG 等不在 Go 支持集合内的
     别名仍被拒绝）；既有 `test_normalize_execution_order_supports_stop_and_market_orders` 的
     `stop_order.order_type` 由错误的 `3` 修正为 `10`。

### 辅助能力补齐（P1：分页页大小唯一 owner）

2. **`resolveHistoricalKLinePageSize` 在 Rust 无独立 owner（P1，分页）**
   - 差异：Go 的 `resolveHistoricalKLinePageSize(limit)` 对 `limit<=0` 返回 `0`（调用方因此省略
     `MaxAckKLNum`，保住 OpenD 默认），`<200` 抬到 200，`>1000` 夹到 1000。Rust 只在
     `history.rs::query_window` 内联 `unwrap_or(1000).clamp(200,1000)`，`limit<=0` 这一 wire 语义无法被单独验证。
   - 修复：新增 `jftrade-integration-futu::resolve_historical_kline_page_size` 作为唯一 owner 并从 lib 导出；
     `query_window` 的既有预算保持不变（路由层在此之前已把 `<=0` 归一化为 200）。
   - 回归：`history_window_tests.rs::historical_page_size_preserves_the_unset_non_positive_budget`。

### 行为映射

| Go 测试 | Rust 入口 | 状态与结论 |
| --- | --- | --- |
| :19 TestFutuKLineIntervalMappingsCoverSupportedAndUnsupportedValues | kline_query.rs::tests::candle_interval_mappings_cover_supported_and_unsupported_values | `[x]` function_exact：interval 表在 Rust 拆为 `period_to_kl_type`（KLType）与 `kline_sub_type`（SubType）两套断言，别名全表一致，`2m`/`13m` fail-closed |
| :85 TestFutuKLineQueryWindowAndPreflightValidation | history_window_tests.rs::historical_page_size_preserves_the_unset_non_positive_budget | `[x]` function_exact（含功能补齐）：页大小 owner 覆盖 `<=0 → 0`；窗口/`shouldQueryCurrentKLine`/预检拒绝由既有 `futu_kline_query_window`、`test_should_query_current_kline`、`executor_rejects_invalid_instrument_and_unsupported_interval` 覆盖 |
| :127 TestFutuHistoricalKLineSessionFallbacksAndETHClassification | history_session_plan.rs::tests::fallback_requires_the_same_route_and_an_unsupported_marker | `[x]` function_exact：回落三分支逐条断言；ETH 归属分类 owner 为 calendar `calendar_session_for_route` + `resolve_market_session`（`Unknown` 语义差异见边界说明） |
| :158 TestFutuTradeEnumMappingsCoverBrokerAndBBGOBoundaries | product_production_ports_execution_order_validation_tests.rs::parsed_order_type_matches_the_opend_wire_enum | `[x]` function_exact（含 P0 修复）：wire 枚举 1/2/5/6/7/10/11 对齐 Go；side 与 TimeInForce 断言不变 |
| :237 TestFutuReadHelperMappingsAndPushSnapshots | product_production_ports_trade_tests.rs::generated_trade_enum_values_are_preserved | `[~]` partial：session/cash-flow/enum 投影等价；`isMarginRatioRateLimitedError` 文本分类器属边界保留（Rust 用本地 governor 前置限流 + RateLimited 缓存回落，不解析 OpenD 错误文本） |
| :316 TestFutuMarketAndSecuritySymbolBoundaries | security_snapshot_query.rs::tests::market_code_and_label_supports_all_standard_markets | `[x]` function_exact：HK/US/SH/SZ 双向映射、未知市场 None、`split_instrument` 归一化与拒绝；`inferMarket` 的 HKD 兜底与正 tick 由 `jftrade-marketdata` catalog 断言承接 |
| :353 TestFutuOrderBookSubscriptionRequestExtraction | subscription_executor.rs::tests::order_book_replay_deduplicates_already_active_subscriptions | `[x]` function_exact：canonical 去重、KLine 家族独立、非法 symbol 返回 `InvalidInstrument`、重复订阅不产生第二次 RPC |

### 边界说明

- `isMarginRatioRateLimitedError`（:237）在 Rust 无对应文本分类器，且不应移植：Rust 的限流是
  `trade_session.rs` 中 governor 的前置配额判定，超限直接产生 `TradeSessionError::RateLimited`，
  `product_trade_margin_route.rs` 再降级为缓存回落。Go 需要解析中英文错误文本，是因为它只能事后识别服务端限流。
- `MarketSession`（:127）没有 Go 的 `Unknown` 变体：零值/缺失时间在 Rust 由 route 校验拒绝，
  ETH 的 pre/after 归属由 exchange-calendar 的时钟分类给出，路由会话（RTH/OVERNIGHT）覆盖时钟分类。
- 本批次未改动 `contracts/openapi/openapi.json`、`proto/` 或冻结 fixture：`orderType` 在契约中是自由字符串，
  `tests/fixtures/compatibility/api-transport/execution-write.json` 只使用 `LIMIT`，因此无生成物变化。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine -E 'test(execution)' --all-targets --locked --no-fail-fast   # 178 passed
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1628 passed, 1 skipped
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：`pkg/futu/adapter_capability_runtime_test.go`（7 项，含 1 项 partial）

### 本批结论

P0/P1 高风险：连接世代 fencing、quote-right 单飞加载、失败缓存与重连刷新、legacy
`GetUserInfo` 不推断细分权限。7 项中 6 项 `[x]` function_exact，1 项 `[~]` partial。

### 本批发现与修复

1. **能力权限 owner 在生产路径没有写入方（P0 权限/公开 API）**
   - 差异：`quote_rights.rs` 早已实现世代 fencing 状态机，但生产组合从未写入它——
     OpenD `Notify`（协议 1003）推送没有解码入口，`GetUserInfo` 也没有 wire 读取，
     因此 `/api/v1/brokers/capabilities` 的 read 能力永远停在
     `QUOTE_RIGHT_UNVERIFIED`，而 Go 会在连接建立后加载一次并随推送更新。
   - 修复位置：新增 `crates/jftrade-integration-futu/src/open_d_quote_rights.rs`
     作为唯一 owner——`query_quote_rights` 拉取 `GetUserInfo`（fenced 到代际，
     RPC 前后各校验一次）、`OpenDQuoteRightsOwner` 持有状态/代际/单飞锁与失败缓存、
     `ingest_notification` 处理 `ConnStatus`/`QotRight` 推送。
   - 回归测试：
     `open_d_quote_rights::tests::entitlement_query_runs_once_per_connection`
     （20 并发仅 1 次 GetUserInfo）、
     `...::entitlement_failure_is_cached_and_refreshed_after_reconnect`、
     `...::cached_failure_expires_at_the_retry_interval`、
     `...::stale_snapshot_is_not_reused_for_a_new_generation`、
     `...::stale_notifications_and_failure_writes_are_fenced`、
     `...::user_info_conversion_does_not_infer_detailed_entitlements`、
     `...::notification_keeps_state_fenced_and_revision_observable`。

2. **通知推送未接入会话事件泵（P1）**
   - 差异：`OpenDSessionEventPump` 把非行情帧直接交给 `decode_quote_push`，协议
     1003 系统通知被静默丢弃，`ConnStatus` 登录态与 `QotRight` 权限无法到达 owner。
   - 修复位置：`session_event_pump.rs` 新增 `OpenDCapabilityNotificationSink`
     与 1003 分支（先于 quote push 解码）；`session_coordinator.rs` 增加
     `set_capability_sink` 并把 sink 传入 pump；`lib.rs` 导出
     `decode_capability_notification` 与 `PROTO_NOTIFY = 1003`。

3. **连接登录态未进入能力判定（P1）**
   - 差异：投影只按 socket/lib 就绪判定 connection，Go 会在 quote/trade 会话未登录时
     降级为 `OPEND_NOT_LOGGED_IN`（read 看 quote，交易看 trade），并使用 OpenD 观测时间。
   - 修复位置：`product_broker_capabilities_projection.rs` 的 `evaluation()` 读取
     `runtime.quote_rights.connect_status(read_access)`，并新增 RFC3339 时间格式化。
   - 回归测试：
     `product_production_ports_trade_tests.rs::broker_capabilities_login_gate_uses_the_opend_connect_status_push`。

4. **engine 侧 owner 缺少生产安装入口（P0 唯一写入所有权）**
   - 差异：engine 的 `QuoteRightsOwner` 只持有状态、无写入方；只有测试能塞值。
   - 修复位置：`product_trade_runtime_quote_rights.rs` 改为持有 activation 时创建的
     `OpenDQuoteRightsOwner` 句柄；新增
     `product_trade_runtime_quote_rights_projection.rs`（`install_quote_rights_owner` /
     `refresh_quote_rights` free impl，保持生产文件 ≤800 行）；
     `product_runtime_provider_activation.rs::activate_quote_rights` 在 provider 激活时
     安装 sink、发布代际并做首次拉取；`product_runtime_start.rs` 同样在启动路径接线。

### 边界说明

- Go `TestFutuCapabilityQuoteRightProductsAndStates` 的逐产品字段选择矩阵
  （prediction/option/future/index、SH/SZ 回落 CN）由 `QuoteRightField` 承载并通过
  `user_info_conversion_does_not_infer_detailed_entitlements` 断言的 SH/SZ/US option 分支
  部分覆盖，仍保留 `[~]` partial；其余产品字段的逐条断言待后续批次补齐。
- Go `context.Canceled`/`DeadlineExceeded` 文本过滤在 Rust 无对应实现：Rust 在
  `query_quote_rights` 用代际校验与 typed error 表达同一失败语义，因此不迁移文本分类器。
- `GetUserInfo` 的 `flag` 字段 Rust 传 `None`（OpenD 默认返回全部信息），与 Go 的
  `GetQuoteRights` 行为一致；请求体由 `proto/futu/GetUserInfo.proto` 生成。
- 本批次未改动 `contracts/openapi/openapi.json`、`proto/` 或冻结 fixture：capability wire
  形状不变，仅补上实际写入方与登录态判定。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1636 passed, 1 skipped
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/adapter_bridge_test.go（7 项，含 1 项 partial）

本批以 `go:452dea11:pkg/futu/adapter_bridge_test.go` 全量 7 条为基线，逐条核对 Rust 现有实现，
补齐了 3 处真实功能缺口（市场规则读取端口、持仓 preferred 成本/PnL 语义、账户分析读贯通断言），
其余 3 条复用既有回归入口。映射同步写入
`docs/history/go-to-rust/manual-test-mappings.json`。

### 逐项结果

| Go 测试 | 状态 | Rust 证据 |
| --- | --- | --- |
| `:19 TestBrokerAdapterDiscoverAccountsAndTradingBridge` | `[x]` | `product_production_ports_execution_preview_tests.rs::broker_adapter_place_and_cancel_keep_server_order_identity_and_submitted_status` |
| `:124 TestBrokerAdapterQueryMarketRulesUsesSecurityInfoLotSize` | `[x]` | `market_rules_query_tests.rs::market_rules_use_security_info_lot_size_without_warnings` |
| `:155 TestBrokerAdapterQueryMarketRulesFallsBackToSecuritySnapshotLotSize` | `[x]` | `market_rules_query_tests.rs::market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error` |
| `:191 TestBrokerAdapterMarketDataReaderTradingSnapshots` | `[x]` | `product_production_ports_trade_tests.rs::position_projection_prefers_diluted_cost_and_account_pnl_with_legacy_fallback` |
| `:465 TestBrokerAdapterMarketDataReaderAccountAnalytics` | `[x]` | `product_production_ports_trade_tests.rs::broker_account_analytics_project_fees_margin_cash_flow_and_buying_power` |
| `:608 TestBrokerAdapterQuoteKLinesSubscriptionsAndValidation` | `[~]` | 订阅幂等/非法输入拒绝/K 线窗口/409 lease 四组既有入口；reader 级错误字符串不迁移，保留 partial |
| `:709 TestBrokerAdapterUnlockTradeBridge` | `[x]` | `product_production_ports_execution_preview_tests.rs::broker_unlock_route_forwards_password_md5_and_unlock_flag_to_opend` |

### 修复的功能差异

1. **QueryMarketRules 端口缺失（P1，`[x]`）**
   - 差异：Go 的 `futuAdapter.QueryMarketRules`（`pkg/futu/adapter_marketdata_reader.go:634`）以
     `Qot_GetStaticInfo`（3202）的 `lotSize` 为主、`Qot_GetSecuritySnapshot`（3203）为回退，并把
     回退原因写入 warning；Rust 侧此前只有 `jftrade-broker` 的 `apply_market_rule` pure 函数和
     `MarketRuleItem` 类型，没有任何 OpenD 读取端口，也没有 lot 过滤与 warning 语义。
   - 修复位置：新增 `crates/jftrade-integration-futu/src/market_rules_query.rs`——
     `MarketRulesReadPort` 契约、`OpenDSecurityInfoReader`（3202 编码/解码，含 `PROTO_GET_STATIC_INFO`
     常量与 5s 超时）、`OpenDMarketRulesReader`（主读命中即返回；失败或无可用 lot 时回退快照并
     追加 `futu market rules loaded from QuerySecuritySnapshot fallback because <reason>`），
     以及 `market_rules_from_static_info` / `market_rules_from_snapshots` 的空白符号与非正 lot 过滤。
     组合入口通过 `with_ports` 注入两个读端口，便于回归测试。
   - 回归测试：`market_rules_query_tests.rs` 新增 8 条（主路径无 warning 且快照 0 次调用、回退路径
     1 次主读 + 1 次快照 + warning 含主失败文本、空符号先行拒绝且不触达 provider、非法行过滤、
     双源空结果 `NoRules`、仅快照失败保留原文、双源失败合并、快照行过滤）。

2. **持仓 preferred 成本/PnL 语义缺失（P0，`[x]`）**
   - 差异：Go `brokerPositionSnapshotFromProto`（`pkg/futu/trade_read_proto.go:146`）用
     `preferredFloat64Ptr` 解析 `dilutedCostPrice > costPrice`、`unrealizedPL > plVal`、
     `averagePlRatio > plRatio`；Rust `position_value` 只投影回退字段，`unrealized_pl` 与
     `average_pl_ratio` 完全未使用，证券账户的权威成本/PnL 在 `/api/v1/portfolio/{brokerId}/positions`
     上会丢失。
   - 修复位置：`crates/jftrade-engine/src/trade_projection.rs::position_value` 改为优先取
     `diluted_cost_price`/`unrealized_pl`/`average_pl_ratio`，缺失时回退旧字段。
   - 回归测试：`product_production_ports_trade_tests.rs::position_projection_prefers_diluted_cost_and_account_pnl_with_legacy_fallback`
     使用 Go 同款两行 fixture（腾讯走 diluted 分支、NVIDIA 走回退分支）并逐字段断言。

3. **账户分析读缺少贯通断言（P1，`[x]`）**
   - 差异：Go 在同一 adapter 上串起 fees/margin/cash-flow/max-qty 四类读；Rust 各字段有投影
     实现但缺少同源 fixture 的一次贯通断言。
   - 回归测试：`broker_account_analytics_project_fees_margin_cash_flow_and_buying_power` 断言
     `feeAmount=12.5` + 2 条 FeeItems、`shortFeeRate=1.25`、`cashFlowDirection=IN` + `88.8`、
     `maxCashBuy=1000` / `maxCashAndMarginBuy=2000` / `session=RTH`。

4. **下单/撤单与解锁写路径（P0，`[x]`）**
   - `broker_adapter_place_and_cancel_keep_server_order_identity_and_submitted_status`：断言
     `brokerOrderId=9001`、`brokerOrderIdEx=FT-9001`、`status=SUBMITTED`、clientOrderId 作为
     OpenD remark，以及 cancel 恰好一次 `Trd_ModifyOrder`（`order_id_ex` 透传）。
   - `broker_unlock_route_forwards_password_md5_and_unlock_flag_to_opend`：断言 unlock 路由恰好一次
     `Trd_UnlockTrade`，透传 `unlock=true` / `passwordMd5`，未提供 `securityFirm` 时保持 `None`。

### 边界说明

- `:608` 的 5 条 reader 级错误字符串（`futu: QueryQuote requires at least one symbol` 等）不迁移：
  Rust 在 market-data 路由层用 400 `BAD_REQUEST` / 409 `MARKET_DATA_SUBSCRIPTION_REQUIRED`
  表达同一无效输入与缺失 lease 语义，复制 reader 字符串会与既有 route 契约重复。
- 账户投影中的 `brokerId` 由 `/api/v1/brokers/{brokerId}/runtime` 路由承载，Rust 的账户对象
  按 OpenAPI `trading.BrokerRuntimeAccount` 不再重复输出该字段。
- 本批未改动 `contracts/openapi/openapi.json`、`proto/` 或冻结 fixture；新增读取端口复用既有
  3202/3203 协议与 `SecuritySnapshotReadPort`，wire 形状不变。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast   # 1648 passed, 1 skipped
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py   # 2164 Rust tests, Futu 域 86.1%
git diff --check
```

## 批次：pkg/futu/watchlist_reader_test.go（6 项，含 1 项顺带修复）＋ internal/watchlist/futu/source_test.go 别名项

行为基线：`go:452dea11:pkg/futu/watchlist_reader_test.go`（205 行，6 项）、
`go:452dea11:pkg/futu/watchlist_reader.go`（354 行）、
`go:452dea11:internal/watchlist/futu/source_test.go:83`。
本批把 Go 的 `futuWatchlistReader` 语义（30s TTL 缓存、10 次/30s 滚动配额门、规范化重名歧义、
canonical id 与 broker 别名分离）落到 Rust 的
`crates/jftrade-integration-futu/src/watchlist_reader.rs`，并在生产装配中拆开读写 owner。

### 逐项结果

1. `:14 TestFutuWatchlistReadGateEnforcesTenCallsPerRollingThirtySeconds` → `[x]`
   `crates/jftrade-integration-futu/src/watchlist_reader_tests.rs::tests::watchlist_read_gate_enforces_ten_calls_per_rolling_thirty_seconds`
   Rust `WatchlistReadGate::allow` 复刻 Go 的 `!call.After(cutoff)` 边界：前 10 次允许、t=29s 拒绝、
   t=30s 放行。
2. `:30 TestFutuWatchlistReaderCacheUsesTTLAndReturnsCopies` → `[x]`
   `watchlist_reader_cache_uses_ttl_and_returns_copies` + `watchlist_reader_cache_expires_at_the_ttl_boundary`
   调用方改写返回值后缓存命中仍返回深拷贝；`now < expiresAt` 才命中，`+30s` 边界即 miss。
3. `:72 TestConvertFutuWatchlistGroupsMarksEveryNormalizedDuplicateAmbiguous` → `[x]`
   `watchlist_group_conversion_marks_every_normalized_duplicate_ambiguous`：
   trim+lower 归一化重名**全部** ambiguous=true，system 组 type=system。
4. `:89 TestConvertFutuWatchlistSecuritiesPreservesCanonicalIDAndBrokerAlias` → `[x]`（**含真实修复**）
   `crates/jftrade-integration-futu/tests/user_security_protocol.rs::members_keep_canonical_id_and_broker_aliases_distinct`
   差异：Rust 3213 投影只输出 `instrumentId` + 原始整型 `securityType`，**完全缺失**
   `brokerCode`/`brokerSecurityId`；非法 market 还会投影成 `".BAD"`。
   修复位置：`crates/jftrade-integration-futu/src/customization.rs`
   （`watchlist_member_instrument_id` 复刻 `futuSymbolFromSecurity` 的 trim+upper 与市场白名单，
   `security_type_label` 复刻 `enumName(SecurityType_name)` 去前缀）。
   回归：新增 framed OpenD 测试断言 `JP.7203`/`7203`/`123456`/`Eqty` 且别名互不相同、非法行被丢弃；
   无修复时该测试失败（已用 stash 验证）。
5. `:113 TestFutuWatchlistFreshReadBypassesAndReplacesGroupAndMemberCaches` → `[x]`
   `watchlist_fresh_read_bypasses_and_replaces_group_and_member_caches`：计数 1→1（缓存）→2/2（fresh）
   →2/2（fresh 后复用新缓存）。
6. `:164 TestFutuWatchlistFreshMemberReadRechecksRemoteAmbiguity` → `[x]`
   `watchlist_fresh_member_read_rechecks_remote_ambiguity`：fresh 先重查 3222，重名即报 ambiguous
   且**不发起** 3213（member 计数保持 1）。

顺带条目 `internal/watchlist/futu/source_test.go:83
TestRemoteMembersKeepBrokerCodeAndSecurityIDAsSeparateAliases` → `[~] partial`：
Rust 已断言三个 id 互不相同，但该 Go 测试针对 `internal/watchlist` 的 `RemoteMember.securityId`
投影层，Rust 无同层类型，故保留 partial 而不冒充 function_exact。

### 边界说明

- Rust 读取走 `CachedRemoteWatchlistReader`（缓存+配额门+歧义），写入仍走裸
  `FutuRemoteWatchlistReader`；`product_runtime_start.rs` 通过
  `set_customization_readers` / `set_customization_writers` 分离两个 owner，避免缓存门被写路径绕过。
- 未改动 `contracts/openapi/`、`proto/` 或冻结 fixture：3213/3222 协议与既有 wire 形状不变，
  成员条目仅补齐 Go 已输出的 `brokerCode`/`brokerSecurityId` 并把 `securityType` 从整型改为 Go 的标签。
- 已知未闭环：`ProductionRemoteWatchlistPort::read` 仍把 reader 的 `Invalid`（空白组名、未知/歧义组）
  统一映射为 503 `WATCHLIST_UNAVAILABLE`，而 Go 的 `writeError` 对 `ErrValidation`/`ErrNotFound`/
  `ErrAmbiguousRemoteGroup` 分别返回 400/404/409。该差异属 API transport 层，需连同 OpenAPI 与
  route ledger 一起处理，本批不擅自变更既有 502/503 契约。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/security_snapshot_coordinator_test.go（6 项，全部新增功能）

行为基线：`go:452dea11:pkg/futu/security_snapshot_coordinator_test.go`（255 行，6 项）、
`go:452dea11:pkg/futu/security_snapshot_coordinator.go`（205 行）、
`go:452dea11:pkg/futu/security_snapshot.go:203-265`（direct 读取）。
本批是**功能缺失修复**：Rust 此前没有 3203 快照协调器，只有裸
`OpenDSecuritySnapshotReader`（按市场分组 + 固定 20 分块 + 失败逐标的回退，且无缓存、无单飞、
无调用预算）。

### 修复内容

新增 `crates/jftrade-integration-futu/src/security_snapshot_coordinator.rs`，
按 Go 一一承接：

| Go 常量/行为 | Rust |
| --- | --- |
| `securitySnapshotCacheTTL` = 3s | `SECURITY_SNAPSHOT_CACHE_TTL` |
| `securitySnapshotCallLimit` = 54 | `SECURITY_SNAPSHOT_CALL_LIMIT` |
| `securitySnapshotCallWindow` = 30s | `SECURITY_SNAPSHOT_CALL_WINDOW` |
| `securitySnapshotHKBatchSize` = 20 | `SECURITY_SNAPSHOT_HK_BATCH_SIZE` |
| `securitySnapshotOtherBatchSize` = 400 | `SECURITY_SNAPSHOT_OTHER_BATCH_SIZE` |
| `singleflight.Group` | `flights: Mutex<HashMap<key, Arc<Flight>>>` + `Condvar` |
| `context.Canceled` | `SecuritySnapshotCancelToken` + `query_with_cancel` |
| `broker.NewSymbolScopedSnapshotError` | `SecuritySnapshotCoordinatorError::InvalidInstrument` + `is_symbol_scoped()` |
| `classifySecuritySnapshotError` | `classify_security_snapshot_fetch_error`（四段限流文案） |
| `querySecuritySnapshotListDirect` | `OpenDSecuritySnapshotReader::query_batch`（单批一次物理读） |

关键设计：协调器拥有分批/缓存/单飞/预算，内层 reader 只做**一次** 3203 编解码
（`SecuritySnapshotBatchReader` 端口）。这样 400 标的非 HK 批次就是 1 次记账 + 1 次 socket
写入，与 Go 一致；旧实现里 reader 自己再按 20 分块会让预算记账失真。
生产装配（`product_runtime_provider_activation::install_security_catalog_readers`）已改为
`CachedSecuritySnapshotReader::new(OpenDSecuritySnapshotBatchReader::new(coordinator))`。

### 逐项结果（全部 `[x]` function_exact）

1. `:17` 缓存/深拷贝/市场分批 → `..._caches_clones_and_uses_market_batches`
   （TTL 边界单独由 `..._cache_expires_at_the_ttl_boundary` 断言）。
2. `:48` 并发合流 → `..._coalesces_concurrent_requests`（leader/follower 显式角色 +
   `coalesced_waiters()` 探针，无 sleep 会合；follower 的 fetch 一旦被调用即 panic）。
3. `:83` 滑动预算 + 失败不缓存 → `..._enforces_sliding_budget_and_does_not_cache_failures`。
4. `:120` 远端限流分类 + 取消 → `..._classifies_remote_rate_limit` 与
   `..._honors_cancellation_of_a_coalesced_wait`（取消后断言 leader 仍在飞行中）。
5. `:154` 非法/不可用输入 → `..._rejects_invalid_and_unavailable_inputs`
   （symbol-scoped 与空输入成功空结果）。
6. `:189` 空/异常结果 → `..._handles_empty_and_unexpected_results`
   （空结果仍占配额槽；缺 symbol 快照不入缓存）。

### 回归可证性

对实现注入临时探针验证测试确为回归守卫（探针已回滚，工作树经 diff 复核一致）：

- 移除 `store()`（即无缓存）→ 3 条测试失败（缓存、TTL 边界、取消后缓存）。
- 把 HK 分批改成单批 → 2 条失败：
  `security_snapshot_batches_split_hk_and_other_markets_by_size` 与
  **引擎级** `cached_security_snapshot_reader_batches_hk_and_serves_repeats_from_cache`
  （45 个 HK 标的必须恰好 3 次物理读、重复查询不再读、`admitted_calls` 保持 3），
  证明生产装配确实走协调器而非裸 reader。

### 边界说明

- 未改动 `contracts/openapi/`、`proto/` 或冻结 fixture；3203 协议与既有 wire 形状不变。
- Go 的 nil-receiver 分支（nil Exchange / nil coordinator / nil fetch）与 Rust 的
  `Option`/`Arc` 所有权模型不同，未伪造等价断言，已在对应条目的结论中写明。
- 保留既有 `OpenDSecuritySnapshotReader::query`（按市场分组 + 逐标的回退）给直接调用方；
  协调器路径经由新的 `query_batch` 单批入口，两者不重复分块。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

## 批次：pkg/futu/adapter_advanced_test.go（6 项：3 项 [x]，3 项 partial）

本批以 `go:452dea11:pkg/futu/adapter_advanced_test.go` 为行为基线，逐条核对
`go:452dea11:pkg/futu/adapter_advanced.go`（`injectAdvancedPageSize` /
`advancedPageSizeLimit` / `injectAdvancedCursor` / `injectAdvancedDefaults` /
`advancedProtocolReplaySafe`）在 Rust 的归属，并补齐此前完全没有 owner 的两组语义。

### 本批新增的 Rust owner（真实功能差异修复）

1. **分页注入与协议上限**（此前 Rust 完全没有等价物）：
   `crates/jftrade-integration-futu/src/research_params.rs` 新增
   `advanced_page_size_limit`（`Qot_GetIndustrialChainList` = 50，其余 100）、
   `advanced_page_size_field`（按 `count` → `pageCount` → `num` → `maxCount` →
   `maxRetNum` 探针顺序，58 个协议的表由 `proto/futu/*.proto` 的 C2S 定义逐一生成并复核）、
   `clamp_advanced_page_size` 与 `inject_advanced_page_size`（调用方已有值不被覆盖，
   协议未声明分页字段时不注入 `count`）。

   **真实修复**：`/api/v1/market-data/options/screens` 原先把 `pageSize` 直接透传
   （`OptionScreenQuery::validate` 允许 1..=1000），Go 的 `injectAdvancedPageSize`
   会先夹到 100 再写入 `pageCount`。已在
   `crates/jftrade-engine/src/product_production_ports_market_data_options_screen.rs`
   接入 `clamp_advanced_page_size("Qot_OptionScreen", …)`；同批把
   `product_production_ports_research_futu.rs` 的 institution `count` 与
   short-interest `num` 也改为按同一上限夹取。

2. **重放安全默认拒绝**：新增 `advanced_protocol_replay_safe`，与 Go 完全一致——
   `Qot_GetEventContractComboRfq` 保留 Get 前缀但会创建短生命周期报价，必须单次执行；
   `Set*`/`Modify*` 与未知协议默认不重放；仅 `Get*`/`Request*`/`Filter*` 及
   `OptionScreen`/`WarrantScreen`/`StockFilter`/`StockScreen`/`SubEventContract` 可重放。

3. **作用域默认值与游标注入**：新增 `inject_advanced_defaults`
   （仅当协议声明 `market`/`offset`/`pageFrom` 时注入；非法 market 只在调用方显式提供时报错）、
   `advanced_cursor_field` / `inject_advanced_cursor`（`nextPage` → `page` → `nextKey`
   探针顺序，调用方游标优先，空游标不写）、`advanced_has_field` /
   `advanced_has_market_field`（33 个 `market`、8 个 `offset`、3 个 `pageFrom` 协议）。

4. **协议 id 单一事实来源**：`crates/jftrade-integration-futu/src/trade_proto.rs`
   新增 `framed_protocol_id`（66 个 framed 协议 → 生成模块 `PROTOCOL_ID`），
   让能力目录的 id 可以被交叉校验，而不是只信任目录里的字面量。

### 逐条结论

| Go 测试 | 结论 | Rust 入口 |
|---|---|---|
| `:14 TestAdvancedFeatureDefaultsBuildStrictOpenDRequests` | `[~]` partial | `research_params_tests.rs::advanced_feature_defaults_build_strict_opend_requests` + `product_production_ports_market_data_options_tests.rs::option_screen_page_size_is_clamped_to_the_adapter_limit_before_the_reader` |
| `:80 TestIndustrialChainListPageSizeRespectsOpenDLimit` | `[~]` partial | `research_params_tests.rs::advanced_page_size_respects_protocol_limits` |
| `:92 TestEveryAllowlistedAdvancedProtocolMapsToCatalogFeature` | `[x]` function_exact | `product_broker_capabilities_projection.rs::tests::every_catalog_protocol_maps_to_a_feature_with_one_stable_id` |
| `:124 TestAdvancedProtocolReplaySafetyDefaultsToNoReplay` | `[x]` function_exact | `research_params_tests.rs::advanced_protocol_replay_safety_defaults_to_no_replay` + `prediction_category_protocol.rs::combo_rfq_creates_the_quote_once_and_never_replays_after_a_transport_failure` |
| `:150 TestEveryDefaultFeatureOperationBuildsGeneratedRequest` | `[~]` partial | `product_production_ports_research_tests.rs::futu_earnings_calendar_route_defaults_to_earnings_and_projects_event_identity` |
| `:212 TestHighDividendStateRejectsMisleadingMainlandScope` | `[x]` function_exact | `research_params_tests.rs::high_dividend_state_is_hk_only` |

### 保留的边界与未完成项（后续批次目标）

- **Rust 功能缺失**：`Qot_GetWarrant`/`Qot_WarrantScreen`、`Qot_GetTopMoversRank`
  等 rankings 协议、`Qot_GetMacroIndicatorList`、`Qot_GetIndustrialChainList/Detail/ByPlate`
  在 Rust 尚无 typed reader。`/api/v1/market-data/warrants` 目前返回
  `Futu warrants market-data reader is not ready`；Futu 的 rankings/industry 走
  akshare helper 分支并以 capability 拒绝；Futu macro 走 helper 分支。参数层规则
  （本批新增）已就位，补齐 reader 时可直接复用，故这 3 条 Go 测试保持 partial。
- **架构差异（有意保留）**：Go 的 `defaultFeatureOperations` 全局表在 Rust 由各路由
  显式拥有，未搬表以免出现第二个默认 operation owner；`defaultOperation(protocols)`
  的“多操作取字典序最小”规则同样没有等价物（Rust 每个 feature 的默认 operation 是
  显式的）。逐 feature 的默认请求证据留待后续批次。
- **Go 允许协议属于两个 feature 的判定在 Rust 不成立**：Rust 有意让 10 个协议服务两个
  feature（snapshot、staticInfo、option-strategy 家族、单腿/组合下单与查询协议），
  测试用显式集合冻结该共享集合。
- 未改动 `contracts/openapi/`、`proto/` 或冻结 fixture；wire 形状不变。

### 回归可证性（临时探针，均已回滚）

- 把 `advanced_page_size_limit("Qot_GetIndustrialChainList")` 改回 100 →
  `advanced_page_size_respects_protocol_limits` 失败（`Number(100)` ≠ `Number(50)`）。
- 移除 option screen 的 `clamp_advanced_page_size` → 引擎级
  `option_screen_page_size_is_clamped_to_the_adapter_limit_before_the_reader` 失败，
  实测记录到 `Some(1000)`，证明该测试捕获的是修复前的真实透传行为。
- 把目录里 `Qot_GetStaticInfo` 的 id 改成 9999 →
  `every_catalog_protocol_maps_to_a_feature_with_one_stable_id` 以
  `catalog id disagrees with the generated protocol id` 失败。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
```

结果：1675 passed / 1 skipped；clippy 无告警；architecture 通过；
parity 审计 Futu/OpenD 90.6%（本批 3 条 `[x]`、3 条 partial，总 `[x]` 436）。

## 批次：pkg/futu/adapter_advanced_protocol_test.go（6 项）

范围：`pkg/futu/adapter_advanced_protocol_test.go` 的 6 条 `Test*`，逐条对照 Go
实现（`adapter_advanced.go`、`adapter_advanced_defaults.go`、`adapter_advanced_helpers.go`、
`adapter_advanced_normalization.go`、`adapter_combo.go`、`adapter_prediction_stream.go`）核对
Rust owner。

| Go 测试 | 状态 | Rust 入口 |
|---|---|---|
| `:15 TestFutuAdvancedAdapterReaderSurfaceAndPredictionSubscriptions` | `[x]` function_exact | `prediction_category_protocol.rs::prediction_subscriptions_replay_after_reconnect_and_clear_on_unsubscribe` |
| `:128 TestBrokerKLineAdjustmentMapping` | `[x]` function_exact | `product_trade_runtime_broker_routes.rs::tests::candle_adjustment_maps_to_the_opend_rehab_enum` |
| `:149 TestFutuAdvancedAdapterProtocolValidationDefaultsAndPayloadHelpers` | `[~]` partial | `research_params_tests.rs::advanced_scope_defaults_and_cursor_injection_follow_the_protocol_payload + advanced_feature_defaults_build_strict_opend_requests` |
| `:296 TestFutuComboAdapterOptionAndEventLifecycle` | `[~]` partial | `product_production_ports_execution_order_validation_tests.rs::combo_intent_rejects_missing_kind_legs_and_account + product_production_ports_execution_preview_tests.rs::event_parlay_preview_rejects_inactive_contract_filtered_from_snapshot_list` |
| `:365 TestFutuComboAdapterProductRulesAndValidationFailures` | `[x]` function_exact | `product_production_ports_execution_preview_tests.rs::product_rule_denials_return_the_go_reason_code_matrix` |
| `:471 TestFutuAdvancedProtocolTransportFailureIsReturned` | `[x]` function_exact | `prediction_category_protocol.rs::prediction_category_transport_failure_is_returned_and_never_retried` |

### 本批真实功能修复

1. **预测订阅 generation 重放（P1 断线重连）**：Go 的 `predictionSubscriptions` +
   `ensurePredictionPushHandlers` 会在拿到新 client 时重放全部活跃事件合约订阅。
   Rust 此前只有单次 `Qot_SubEventContract` 调用，重连后租约静默丢失。现在
   `crates/jftrade-integration-futu/src/prediction.rs` 的 `OpenDPredictionMarketReader`
   持有 `prediction|sorted-types → dataTypes` 重放表，按 `coordinator.generation()` fencing：
   新 generation 的第一次调用先重放全部活跃订阅再发调用方请求；退订按 contract 前缀清除；
   重放失败不标记 attached，下一次调用重试而不是丢流。
2. **K 线复权映射收口**：把路由内联的 adjustment 匹配抽成
   `product_trade_runtime_broker_routes.rs::broker_kline_adjustment`，与 Go
   `brokerKLineRehabType` 一一对应（`""`/`forward`→1、`none`→0、`backward`→2）。
3. **产品规则拒绝码矩阵**：`product_rule_denials_return_the_go_reason_code_matrix`
   固化 8 条拒绝码，并用 reader 调用计数证明放行请求仍会到达 OpenD reader。

### 保留的边界与未完成项（后续批次目标）

- **`:149` 功能缺失**：`injectFeatureInstrument`（`protocolInstrumentField` 表、
  `securityList`/`ownerList`/`multi_legs` 赋值、非法 instrument 报错、已存在字段不覆盖）、
  `payloadEntries` 多列表选择、`featureResultFromPayload` 的 `nextPage`/`snapshotList` 丢弃
  `warning`/`quoteExpiresAt`、`normalizeOpenDMap`/`normalizeOpenDEnum`、
  `numberValue`/`stringValue`/`cloneMap`/`structMap` 在 Rust 已无同名生产入口（职责移交给
  强类型 `trade_proto` DTO 与 `research_normalization.rs`），因此记为 partial 而不是造假测试。
- **`:296` 缺口**：option combo `preview → place → cancel` 与 event parlay
  `place → cancel` 尚无引擎级端到端断言；Rust 的 `RecordingTradeWriter::place_combo_order`
  目前是 `unsupported()` fixture。生产路径已存在（`combo_preview` 校验策略合法性 +
  `read_combo_max_trade_quantity`；`place_combo` 走 preview 检查、
  `reserve_order_with_preview_checked`、`persist_external_success`；cancel 走
  `modify_order(operation=2)`）。**下次目标**：为 engine fixture 增加 combo writer 记录，
  按 3258/3445 wire 断言 preview/place/cancel 各一次、`SUBMITTED` 状态与 legs 数量。

### 回归可证性（临时探针，均已回滚）

- 移除 `replay_if_generation_changed` 的 generation 比较（`state.attached` 即返回）→
  `prediction_subscriptions_replay_after_reconnect_and_clear_on_unsubscribe` 以
  `left: 3434 / right: 3455` 失败，证明该测试真实守护重连重放。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
git diff --check
pnpm run check:quick
```

结果：1679 passed / 1 skipped；clippy 无告警；architecture 通过；parity 审计
Futu/OpenD 91.0%（本批 4 条 `[x]`、2 条 partial，总 `[x]` 440，全局 49.3%）。

## 批次：pkg/futu/advanced_product_adapter_contracts_test.go（8 项收敛）

范围：把上一轮遗留的 partial/boundary 收敛。本批新增 **2 条真实修复** 与 2 条引擎级生命周期/投影回归。

| Go 测试 | 状态 | Rust 入口 |
|---|---|---|
| `:22 TestFutuAdvancedSpecializedReadersAndCustomizationSuccess` | `[x]` function_exact | watchlist modify + prediction categories + `live_read_routes_require_a_logical_subscription_lease` + catalog search 失败分支 |
| `:150 TestFutuComboAdapterErrorPropagationBranches` | `[x]` function_exact | `combo_intent_rejects_missing_kind_legs_and_account` + `option_combo_preview_place_and_cancel_keep_server_identity` + `event_parlay_preview_rejects_inactive_contract_filtered_from_snapshot_list` |
| `:216 TestFutuEventContractStatusBranches` | `[x]` function_exact | 既有 |
| `:235 TestFutuWarrantsStayHKOnlyAndFuturesRemainDiscoverable` | `[x]` function_exact | 既有 |
| `:262 TestSecurityDetailsProductIdentityFallbacks` | `[~]` boundary | 公开 OpenAPI 契约无 SecurityDetails/ProductClass |
| `:336 TestFutuSnapshotProductExtensionsAndSecurityTypeMapping` | `[x]` function_exact | securityType 1..12 + research_security_type 表 + warrants=[warrant,cbbc] |
| `:403 TestFutuTradeProductRequestAndReadLifecycleBranches` | `[~]` partial | amount/predSide 编码 + 新增投影回归；ProductClass/OrderKind 由上层契约承接 |
| `:475 TestFutuComboProtocolTransportErrors` | `[x]` function_exact | combo max/place 会话错误 + 3445 Transport→Unavailable |

### 本批真实功能修复

1. **组合腿投影过滤（`crates/jftrade-integration-futu/src/trade_snapshots.rs`）**：Go 的
   `brokerOrderLegSnapshots` 会跳过 `security == nil` 或无法解析 market 的腿，并把事件合约
   market 101 映射成公开 `US.` 命名空间。Rust 此前原样保留任何 market（例如 999）并输出
   `market=999`，调用方会拼出非法 instrument。现新增 `combo_leg_snapshot`：空 code 丢弃、
   未知 market 丢弃、101 归一为 11，其余受支持 market 保留并 trim/大写 code。
2. **combo 生命周期可测性**：`RecordingTradeWriter::place_combo_order` 此前是
   `unsupported()`，任何 combo place 相关断言都无法成立。现改为记录请求并返回
   `COMBO-9001`，使 `option_combo_preview_place_and_cancel_keep_server_identity` 能覆盖
   preview 持久化→place 消费 previewId→幂等重放→cancel 走 `Trd_ModifyOrder(operation=2)`
   的完整链路（Go `adapter_advanced_protocol_test.go:296` 的同一断言集）。

### 回归可证性（临时探针，均已回滚）

- 把 `combo_leg_snapshot` 的未知 market 分支改成保留（不返回 `None`）→
  `trade_snapshot_projection_preserves_combo_event_and_leg_identity` 以
  `left: 2 / right: 1`（unresolvable legs are dropped）失败。
- 把 cancel 的 `operation: 2` 改成 `3` → `option_combo_preview_place_and_cancel_keep_server_identity`
  以 `left: 3 / right: 2`（cancel uses Trd_ModifyOrder）失败。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
pnpm run check:quick
```

结果：1681 passed / 1 skipped；fmt/clippy/architecture 通过；parity 审计 Futu/OpenD 91.2%，
总 `[x]` 444 / 4451，全局 49.4%。本批 8 项：5 `[x]`、2 partial/boundary 收敛为更精确结论、
1 边界保留。

---

## 批次：`pkg/futu/adapter_new_methods_test.go`（22 项，口径纠正）

此前小结曾把本文件写成“8 项”，实际该文件含 **22 个 `Test*`**；本次按行号全量逐项复核，
纠正统计口径并补齐差异。

### 结果：17 `[x]` / 5 `[~]`

| Go 测试 | 行号 | 状态 | 证据 / 结论 |
|---|---:|---|---|
| `TestConvertFundsSnapshotFullMarginFields` | :14 | `[x]` | engine funds 投影全 margin/PDT/exposure 字段 |
| `TestConvertFundsSnapshotNilMarginFields` | :78 | `[x]` | 缺省 margin 保持 null，currencyBalances 空数组 |
| `TestConvertFundsSnapshotNilInput` | :103 | `[~]` boundary | Rust 无 nil 指针输入；缺失 S2C 归一 `Funds::default()` |
| `TestConvertFundsSnapshotCurrencyBalances` | :112 | `[x]` | 2 币种 + 2 市场资产投影 |
| `TestSecuritiesFromSymbols` | :140 | `[x]` | 规范化 MARKET.CODE（trim/大写/去重） |
| `TestSecuritiesFromSymbolsInvalid` | :156 | `[x]` | 新增 `normalized_instruments_rejects_invalid_symbol_before_encoding` |
| `TestSecuritiesFromSymbolsEmpty` | :163 | `[x]` | 新增 `normalized_instruments_accepts_empty_symbol_list` |
| `TestSecuritySymbol` | :175 | `[x]` | market/code → HK.00700 |
| `TestSecuritySymbolNil` | :182 | `[x]` | 缺失/未知 market → None |
| `TestFutuKLTypeFromIntervalStringAll` | :191 | `[x]` | 全别名表 KLType 编码 |
| `TestFutuKLTypeFromIntervalStringInvalid` | :228 | `[x]` | 新增 `unsupported_interval_is_rejected_instead_of_encoding_a_daily_candle` |
| `TestInt64AsFloat64Ptr` | :237 | `[~]` boundary | Rust 保留 i64，无指针 helper |
| `TestInt64AsFloat64PtrNil` | :248 | `[~]` boundary | Option 表达缺失，nil 分支不存在 |
| `TestBrokerFundsSnapshotFromProtoFullMargin` | :257 | `[x]` | 真实 fake OpenD 解码全 margin 字段 |
| `TestBrokerFundsSnapshotFromProtoNilFunds` | :344 | `[x]` | 缺失 S2C/funds → 零值快照 |
| `TestBrokerFundsSnapshotRoundTripNoMargin` | :358 | `[~]` partial | `TradeFunds.debt_cash` 非可选 f64，缺省会投影成 0 |
| `TestOrderBookLevelFromPb` | :431 | `[x]` | level 全字段投影 |
| `TestOrderBookLevelFromPbNil` | :465 | `[x]` | 空输入返回空 levels |
| `TestOrderBookLevelFromPbEmptyDetails` | :475 | `[x]` | 无 detail_list 不注入空数组 |
| `TestOrderBookSnapshotFromOpendResult` | :492 | `[x]` | depth 名称/时间/买卖盘全投影 |
| `TestOrderBookSnapshotFromOpendResultNil` | :552 | `[~]` boundary | Rust 缺失 S2C fail closed，不静默空成功 |
| `TestOrderBookSnapshotFromOpendResultEmptyResult` | :559 | `[x]` | 空 S2C 列表 → 空 bids/asks |

### 真实功能修复 ①：未知 K 线周期不再静默降级为日线

- Go `futuKLTypeFromIntervalString`（`pkg/futu/adapter_new_methods.go:105`）对未知周期返回 error。
- Rust 旧实现 `period_to_kl_type` 的 `_ => 2` 会把 `"invalid"` 静默编码成
  `KLType_1Day`，返回错误数据而调用方无感知。
- 修复：新增 `period_to_kl_type_checked`（唯一的别名表 owner），返回
  `CurrentKlineError::InvalidQuery`；`encode_get_kl_request` 改为
  `Result<Vec<u8>, CurrentKlineError>`，`query_current_klines` 在发包前 fail closed。
- `period_to_kl_type` 保留为目录/订阅用的 infallible 视图，未知值返回 `0`
  （unspecified），不再伪装成日线。
- 新增回归 `kline_query::tests::unsupported_interval_is_rejected_instead_of_encoding_a_daily_candle`，
  同时断言 helper 错误消息与编码路径错误。

### 真实功能差异 ②：`TradeFunds.debt_cash` 丢失缺失语义

- Go 的 `BrokerFundsSnapshot.DebtCash` 是 `*float64`，proto 未给 `debtCash`
  时为 nil，`convertFundsSnapshot` 透传 nil。
- Rust `TradeFunds.debt_cash` 是非可选 `f64`（proto2 `required double debtCash = 6`
  解码为零值），投影后 `summary.debtCash` 会变成 `0.0`，与 Go 的 `null` 不一致。
- 修复位置：`crates/jftrade-integration-futu/src/trade_snapshots.rs`
  （`TradeFunds::from(trd_common::Funds)` / `debt_cash` 可选性）；
  回归要求：SIMULATE/无 margin 账户断言 `summary.debtCash` 为 JSON null。
- 本轮只记录差异与修复位置，不改 proto 解码契约，避免与并发批次冲突。

### 回归可证性（临时探针，已回滚）

- 把 `period_to_kl_type_checked` 的未知分支改回 `Ok(2)` →
  `period_to_kl_type_matches_go_interval_aliases` 与
  `candle_period_catalog_rejects_missing_and_unmappable_intervals` 立即失败，
  证明新增断言确实锁住 fail-closed 行为。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
```

结果：`jftrade-integration-futu` 464 passed / 1 skipped；`jftrade-engine`
1220 passed / 0 skipped；fmt 通过；parity 审计 0 条非法 `-p` crate、
0 条 `[x]` 缺 `function_exact`。本批 **17 `[x]` / 5 `[~]`**。

---

## 批次：`pkg/futu/exchange_test.go`（27 项）

该文件实际含 **27 个 `Test*`**（此前代办目标写的 6 项为域名级误计）。逐行复核后：

### 结果：23 `[x]` / 4 `[~]`（本批从 2 个 boundary/partial 收敛）

新收敛为 `function_exact`：

| Go 测试 | 行号 | Rust 证据 |
|---|---:|---|
| `TestEnsureMarketWithContextFallsBackToSecuritySnapshotLotSize` | :89 | `market_rules_query.rs::tests::market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error` |
| `TestConnectRejectsOpenDBelowMinimumVersion` | :237 | `tests/client_recovery_boundaries.rs::below_minimum_version_fails_session_initialization` |

`:89` 之前被记为 `[~] boundary`，属于基线口径过时：Rust 的 `OpenDMarketRulesReader::query`
已实现与 Go 相同的 `Qot_GetStaticInfo → Qot_GetSecuritySnapshot` 两段式回退，并有测试
断言两者各调用一次、`lot_size=100`、warning 同时含 `QuerySecuritySnapshot fallback`
与主错误文本。

`:237` 的 Go 语义是 InitConnect 只报 `serverVer` 时低版本直接失败；Rust
`connect_with_push_notifications` 在同一位置执行 `version_supported_without_build`
并返回 `OpenDTcpProbeError::UnsupportedVersion`（消息含检测到的版本），属同一 owner 与
同一失败语义。

### 保留边界 / 部分覆盖（4 项，均补上真实入口与命令）

| Go 测试 | 行号 | 状态 | Rust 入口与结论 |
|---|---:|---|---|
| `TestEnsureMarketWithContextReturnsInferredMarketWhenStaticInfoUnavailable` | :114 | `[~]` boundary | `product_market_data_catalog_read_tests.rs::market_data_catalog_read_routes_fail_closed_when_snapshot_port_is_unavailable`；Rust 产品契约在 catalog 不可用时 fail-closed `MARKET_DATA_CATALOG_UNAVAILABLE`，不构造“带错误的推断市场” |
| `TestConnectRejectsOpenDBelowMinimumBuild` | :251 | `[~]` partial | `health.rs::tests::minimum_version_validation_matches_go_boundaries`；Rust `version_supported(1009,6808)=false` 只在 health 投影为 degraded+OPEND_VERSION_UNSUPPORTED，会话初始化不因 build 号抛错，差异在拒绝边界 |
| `TestEnsureSystemNotificationsBindsSystemPushHandler` | :927 | `[~]` boundary | `tests/futu_notifications_parity.rs::test_live_notification_from_response_routes_protocol_payloads_to_neutral_categories`；Rust 无 Go 式 `OnSystemNotify` 回调 API，改用 `UnsolicitedFrame` + 中性通知投影 |
| `TestSubscribeTradeAccountPushReplaysOnReconnectedClient` | :974 | `[~]` boundary | `trade_session_tests.rs::subscribe_trade_accounts_forwards_every_account_id`；Rust 只保留协议编码，账户推送生命周期由对账 worker 取代，无 Go 式 push 订阅 owner |

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

结果：定向 5 项全部通过；parity 审计 0 条非法 `-p` crate、0 条 `[x]` 缺
`function_exact`；Futu/OpenD 91.8%；总 `[x]` 449 / 4451。本批 **23 `[x]` / 4 `[~]`**。

---

## 批次：`pkg/futu/opend/client_test.go`（7 项，全部 `missing` → `[x]`）

该文件此前 7 条全部是领域级 `missing`。逐行复核后全部收敛为 `function_exact`：

| Go 测试 | 行号 | Rust 证据 |
|---|---:|---|
| `TestCallFailureRecordsRequestCorrelation` | :77 | 新增传输层 observer + 生产组合接线 |
| `TestStartKeepAliveIgnoresNonPositiveIntervalsWithoutConsumingStart` | :104 | `managed_session_tests.rs::start_keep_alive_ignores_non_positive_intervals` |
| `TestCallRoundTrip` | :181 | `concurrent_rpc_responses_are_routed_by_protocol_and_serial` |
| `TestRequestTimeout` | :206 | `response_after_request_timeout_is_not_delivered_to_a_stale_waiter` |
| `TestKeepAliveFailureClosesClient` | :243 | `keep_alive_failure_closes_the_session` |
| `TestSubscribeNotifyReceivesSystemPush` | :280 | `health.rs::tests::initialized_probe_and_subscription_rpc_share_one_managed_reader` |
| `TestCallIgnoresMismatchedProtoOnSameSerial` | :405 | `same_serial_with_wrong_protocol_is_unsolicited_until_exact_response_arrives` |

### 真实功能缺失修复：OpenD 调用未记录 observability correlation

Go `Client.Call`（`pkg/futu/opend/client.go:283`）每次都调用
`observability.RecordOpenDCall(ctx, proto_<id>, latency, err)`，把失败与上下文里的
`RequestID`、`Source=opend`、`Importance=high` 关联起来，并在
`/api/v1/system/status` 上投影为 `observability.requests.openD`。

Rust 侧此前 `TransportMetrics::record_open_d_call` **没有任何生产调用方**：OpenD 传输层
不知道 observability，API 层的 `openD.totalCalls` 永远是 0。修复分三层：

1. `crates/jftrade-kernel/src/open_d_observer.rs`（新增）：`OpenDCallObserver` port +
   `OpenDCallRecord { operation, request_id, error }` + no-op 实现。放在 kernel 是因为
   `jftrade-integration-futu` 不能依赖 `jftrade-api`，而两者都已依赖 kernel。
2. `crates/jftrade-integration-futu`：`OpenDManagedSession` 持有可选 observer，
   `call_with_timeout` 在每次 RPC 结束后上报 `proto_<id>`、请求关联 ID 与错误文本；
   `OpenDTcpProbeConfig::with_open_d_call_observer/with_request_id` 让组合根注入。
3. `crates/jftrade-engine`：`start_product_runtime` 在启动 OpenD provider 之前创建唯一的
   `TransportMetrics`，注入 `provider.opend`，并把同一实例传给
   `prepare_product_with_runtime_state`，保证只有一个 observability writer。
   `crates/jftrade-engine/src/product_opend_call_observer.rs` 提供 composition-root 适配器，
   把 port 记录转写进既有的 `TransportMetrics::record_open_d_call`。
   适配器放在 engine 而非 api，是因为架构门禁禁止 `jftrade-api` 依赖
   `jftrade-kernel`（首版实现放 api 时 `check:rust:architecture` 直接失败，已按门禁归位）。

### 回归可证性（临时探针，已回滚）

- 移除 `OpenDManagedSession::call_with_timeout` 的 observer 上报 →
  `call_failure_records_request_correlation_against_the_observer` 以
  `left: 0, right: 1` 失败。
- 移除 `start_product_runtime` 中 `provider.opend` 的 observer 注入 →
  `product_runtime_records_opend_calls_in_transport_metrics` 在
  `totalCalls > 0` 断言处失败，证明端到端接线不是靠 API 层伪造。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-kernel -p jftrade-integration-futu -p jftrade-api -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
python3 scripts/compatibility/audit_test_parity.py
```

结果：1752 passed / 1 skipped；fmt 通过；parity 审计 0 条非法 `-p` crate、
0 条 `[x]` 缺 `function_exact`；Futu/OpenD 92.2%；总 `[x]` 456 / 4451。

---

## 批次：`pkg/futu/adapter_failure_boundaries_test.go`（5 项）

### 结果：4 `[x]` / 1 `[~]` boundary

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---:|---|---|
| `TestBrokerAdapterForwardsUnavailableOpenDErrors` | :19 | `[x]` | 新增 `broker_adapter_forwards_unavailable_opend_errors_on_every_read_route` |
| `TestAdapterSubscriptionParsingErrorsAreReturned` | :64 | `[x]` | `subscription_executor.rs::tests::executor_rejects_invalid_instrument_and_unsupported_interval` |
| `TestAdapterAndDecimalConversionBoundaries` | :77 | `[x]` | `trade_price` + `basic_quote_tick` + `security projection` 组合证据 |
| `TestKLineSessionAndPriceHelperBoundaries` | :115 | `[x]` | `history_session_plan` + `kline_query` + `trade_price` 组合证据 |
| `TestCanceledContextIsPreservedByUnavailableAdapter` | :165 | `[~]` boundary | Rust 端口无 ctx；等价保证是“不可用即失败” |

### 真实功能缺失修复：下单价格未按交易所 tick 归一化

Go `exchange_trade_write.go:177` 在编码 `Trd_PlaceOrder` 前调用
`normalizeSubmitOrderPrice`：US 市价 ≥1 用 0.01、<1 用 0.0001，其他市场保持原价，
非正价格不上送；结果再经 `fixedpoint.NewFromFloat` 量化到 8 位小数。Rust 的
`place_order_command` 此前把 `request.price`/`aux_price` 原样编码，等于把未对齐 tick
的价格直接发给 OpenD（可能被拒或错价成交）。

修复：新增 `crates/jftrade-integration-futu/src/trade_price.rs`（Go
`exchange_trade_price.go` 的 owner），提供 `submit_order_price_step`、
`round_price_to_step`、`step_rounded_unit`、`count_step_decimals`、
`normalize_submit_order_price`；`place_order_command` 对 price/aux_price 应用归一化并
过滤非正值，改单路径保持不变（与 Go 一致）。`quantize_eight_decimals` 复现
`fixedpoint.NewFromFloat` 的量化，避免 `123.456 → 123.46000000000001` 的二进制漂移
进入 protobuf double 字段。

### 覆盖面补强

`:19` 的 Go 测试用拒连 OpenD 驱动整个 broker adapter 面。Rust 的对应 owner 是
`ProductionBrokerPort` 的统一 fail-closed readiness 门，因此新增参数化测试遍历 12 条读
路径（accounts/funds/positions/orders/orders-history/fills/fills-history/order-fees/
margin-ratios/cash-flows/max-trade-qtys/securities）逐条断言返回 `Unavailable` 类错误
而非零值成功；history 变体使用 Rust 的 `scope=HISTORY` 路由约定。

### 回归可证性（临时探针，已回滚）

- 移除 `place_order_command` 中的价格归一化（保留原价直传）→
  `place_order_rounds_us_prices_to_the_venue_tick_before_encoding` 在 wire price 断言处失败
  （探针同时暴露 `Session(Closed(PeerClosed))`，因为断言先于响应写回触发）。

### 保留边界

`:165` 的 Go 断言依赖 `context.Context`。Rust 端口是同步 `Result` 且不含取消 token，
没有可传播的 ctx；同一失败保证（不可用 OpenD 绝不返回成功）已由 `:19` 那条参数化测试
覆盖，取消/超时语义由写端口的 `Canceled`/`Timeout` 变体与 runtime deadline 承担。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast
cargo fmt --all --check
python3 scripts/compatibility/audit_test_parity.py
```

结果：1693 passed / 1 skipped；fmt 通过；parity 审计 460 条 `function_exact` 全部解析到
真实测试、0 重复、0 非法 `-p` crate；Futu/OpenD 93.1%；总 `[x]` 460 / 4451。

---

## 批次：`pkg/futu/opend/orderbook_boundaries_test.go`（5 项）

### 结果：5 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---:|---|---|
| `TestGetOrderBookRejectsDisconnectedSession` | :14 | `[x]` | `market_microstructure_query.rs::tests::depth_read_rejects_a_closed_session_before_any_projection` |
| `TestParseOrderBookResponseRejectsMalformedTopLevelFields` | :22 | `[x]` | `depth_read_rejects_malformed_top_level_wire_fields`（10 组原始线格式） |
| `TestParseOrderBookResponseRejectsMalformedS2CFields` | :50 | `[x]` | `depth_read_rejects_malformed_s2c_wire_fields`（16 组原始线格式） |
| `TestParseOrderBookResponseSkipsValidUnknownFields` | :85 | `[x]` | `depth_read_skips_valid_unknown_fields_and_keeps_known_projection` |
| `TestParseOrderBookLevelAcceptsValidRequiredFields` | :101 | `[x]` | `depth_read_accepts_a_wire_level_ask_payload_with_required_fields` + `depth_read_rejects_levels_missing_proto2_required_fields` |

### 真实功能缺失修复：缺失 proto2 `required` 的摆盘档位被伪造成零值成功

Go `pkg/futu/opend/orderbook.go` 用 `proto.Unmarshal` 解码每层
`Qot_Common.OrderBook` 与 `Qot_Common.Security`，缺失 proto2 `required`
字段（`OrderBook.price/volume/orederCount`、`Security.market/code`）会返回
`RequiredNotSet` 错误。Rust 侧由 prost 解码，而 prost 无法观察 proto2
presence：探针确认一个空 level 会被投影成
`{"price":0.0,"volume":0.0,"orderCount":0}` 的**虚假深度档位**并以成功返回。

修复：新增 `crates/jftrade-integration-futu/src/order_book_wire.rs`（wire 层
required 校验 owner，提供 `require_fields`、`validate_order_book_s2c`、
`validate_order_book_response`），并在
`OpenDMarketMicrostructureReader::call` 中于 prost 解码前对
`Qot_GetOrderBook` 调用 `validate_order_book_response`，把失败映射为
`MarketMicrostructureError::Decode`。同时新增 `depth_reader_with_body`
测试辅助，使测试可以直接投递原始线格式 body。

### 回归可证性（临时探针，已回滚）

- 移除 `call` 中的 `validate_order_book_response` 调用 →
  `depth_read_rejects_levels_missing_proto2_required_fields` 失败，报错文本显示
  空 level 被成功投影为 `{"orderCount":0,"price":0.0,"volume":0.0}`。

### 保留边界

- Go 的手写 `protowire` 消费器与 Rust 的 prost 解码器是不同实现，因此与
  Go 错误消息文本一一对应不可行；本批映射的是行为等价：每一类畸形
  payload 都必须以 typed `Decode` 失败，合法未知字段必须被跳过，已知
  投影必须保留。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
cargo fmt --all --check
python3 scripts/compatibility/audit_test_parity.py
```

---

## 批次：`pkg/futu/transport_error_propagation_test.go`（5 项）

### 结果：5 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---:|---|---|
| `TestTradeReadMethodsPropagateTargetProtocolDisconnects` | :13 | `[x]` | `client_recovery_boundaries.rs::trade_read_methods_propagate_target_protocol_disconnects` |
| `TestQuoteKLineAndOrderBookPropagateTargetDisconnects` | :69 | `[x]` | `client_recovery_boundaries.rs::quote_kline_and_order_book_propagate_target_disconnects` |
| `TestTradeWriteMethodsPropagateAccountAndWriteDisconnects` | :125 | `[x]` | `product_production_ports_execution_preview_tests.rs::trade_write_methods_propagate_write_disconnects` |
| `TestTradeWritesAreNotReplayedWhenResponseIsLost` | :166 | `[x]` | `product_production_ports_execution_preview_tests.rs::trade_writes_are_not_replayed_when_the_response_is_lost` |
| `TestDirectSubscriptionCallsPropagateClosedClientErrors` | :207 | `[x]` | `client_recovery_boundaries.rs::direct_subscription_calls_propagate_closed_client_errors` |

### 覆盖的语义

Go 的 `quoteOpenDServer.setDropProto(protoID)` 在读走目标协议帧后**不再回复**，
客户端因此超时报错。本批在 Rust 侧新增等价夹具
`drop_protocol_server(drop_proto)`（完成 InitConnect 握手，其后只丢弃指定协议的
响应），以 300ms 超时驱动三条边界：

1. **读路径断连**：accounts(2001)/funds(2101)/positions(2102) 各自独立起一个
   drop 服务器，断言方法返回错误而不是默认值成功。
2. **订阅/深度断连**：丢弃 `Qot_Sub` 使 coordinator 的订阅回放失败；丢弃
   `Qot_GetOrderBook`(3012) 使深度读取返回 typed `Session` 错误。
3. **写路径断连与不重放（P0 资金安全）**：`DisconnectingTradeWriter` 与
   `CancelLosesResponseWriter` 在写端口层面返回
   `Session(Closed(PeerClosed))` 并计数调用次数，断言下单与撤单各自**恰好尝试
   一次**。

Rust 的 `OpenDManagedSession::call_state` 本身不含重试：写失败返回 typed 错误，
响应丢失（超时）返回 `RequestTimeout` 且移除 pending。engine 的写端口在此之上
持久化 `UNKNOWN` 并向上返回错误，因此不存在自动重放路径——本批用测试把该性质
固定下来。

### 回归可证性（临时探针，已回滚）

- 让 `get_account_list` 在 `call` 失败时返回 `S2c::default()` →
  `trade_read_methods_propagate_target_protocol_disconnects` 以
  `accounts must fail closed` 失败。
- 在 `place_order` 的 `execute_order_under_guard` 闭包内对失败结果重放一次 →
  `trade_write_methods_propagate_write_disconnects` 与
  `trade_writes_are_not_replayed_when_the_response_is_lost` 同时失败
  （`left: 2, right: 1`）。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --test client_recovery_boundaries --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
cargo fmt --all --check
cargo clippy -p jftrade-engine --all-targets --locked
```

---

## 批次：`pkg/futu/read_account_test.go`（5 项）

### 结果：5 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---:|---|---|
| `TestRecoverableOpenDErrClassifiesConnectionFailures` | :12 | `[x]` | `recoverable_error_tests.rs::recoverable_errors_match_go_is_recoverable_opend_err` + `io_error_kinds_are_classified_without_string_matching` |
| `TestResolveTradeMarketHonorsRequestedAuthorityAndFallbacks` | :37 | `[x]` | `product_production_ports_trade_tests.rs::resolve_account_honors_requested_authority_and_falls_back_like_go` |
| `TestCandidateTradeAccountFromProtoFiltersAndBuildsHeader` | :85 | `[x]` | `product_production_ports_trade_tests.rs::candidate_account_filters_and_card_identity_match_go` |
| `TestBrokerReadQueryNormalizationAndAccountPriority` | :140 | `[x]` | `product_production_ports_trade_tests.rs::broker_read_query_normalization_and_account_priority_match_go` |
| `TestTrdMarketFromNormalizedCoversSupportedMarkets` | :167 | `[x]` | `product_production_ports_trade_tests.rs::trd_market_codes_follow_go_normalized_mapping` |

### 真实功能缺失修复：无授权列表的账户无法直映射请求市场

Go `resolveTradeMarket`（`pkg/futu/trade_read_account.go:101`）有三个分支：

1. 授权列表非空 → 请求市场必须在列表内，否则返回 `ok=false`（不是错误）；
2. **授权列表为空 → 按归一化请求市场直接映射**，未支持的名称报
   `unsupported market`；
3. 请求为空 → 取第一个有效授权，全无则默认 `HK`。

修复前 Rust 的 `resolve_account_with_environment`
（`crates/jftrade-engine/src/product_production_ports_trade_requests.rs`）把市场匹配
写成“授权列表中存在该市场”，于是**授权列表为空的账户一律被过滤**，导致
`market=JP` 这类 Go 会直接映射的合法请求报
`no Futu trading account matched ... market=JP`。这是账户解析层面的行为差异，
会直接影响实盘/模拟盘下单前的账户选择。

修复：按 Go 的三分支重写市场过滤——空授权列表走 `market_code` 直映射
（失败即报错），非空列表保持授权校验；同时把 `header_market` 的回退从
`self.market_label()` 改为 `selected_market`，使直映射分支能产出正确的
`trd_market`。

### 回归可证性（临时探针，已回滚）

- 恢复“空授权列表即过滤”的旧分支 →
  `resolve_account_honors_requested_authority_and_falls_back_like_go` 在
  `no-authority JP` 断言处失败：
  `no Futu trading account matched account 42 for tradingEnvironment=REAL market=JP`。

### 保留边界

- Go 的 `candidateTradeAccountFromProto` 返回 `(candidate, ok, err)` 三元组；
  Rust 由 `resolve_account_with_environment` 直接产出 `ResolvedTradeRequest`，
  因此映射的是同样的过滤与归一化结果，而非同名函数式 API。
- Go 的 `resolvedTradeAccountPriority` 是独立排序函数；Rust 用
  “无环境请求时优先保留 `trd_env == 0`”实现同一优先级，测试固定该可观察结果。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
cargo fmt --all --check
cargo clippy -p jftrade-engine --all-targets --locked
```

---

## 批次：`pkg/futu/exchange_quote_request_boundaries_test.go`（5 项）

### 结果：5 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---:|---|---|
| `TestExchangeAccountPushMarketWarningAndEmptySymbolBoundaries` | :16 | `[x]` | `trade_session_tests.rs::subscribe_trade_accounts_propagates_opend_rejection` + `subscription_executor.rs::tests::executor_rejects_invalid_instrument_and_unsupported_interval` + `market_rules_query_tests.rs::market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error` |
| `TestMaxTradeQuantityInvalidSecurityAfterAccountResolution` | :42 | `[x]` | `product_production_ports_trade_tests.rs::max_trade_quantity_rejects_invalid_security_after_account_resolution` |
| `TestBasicQuoteMissingInvalidAndSubscriptionCacheBoundaries` | :50 | `[x]` | `basic_quote_query.rs::tests::basic_quote_query_requires_subscription_and_maps_success_rejection_and_empty` + `basic_quote_tick.rs::tests::maps_normalized_requested_rows_and_keeps_the_last_duplicate` + `basic_quote_query.rs::tests::basic_quote_query_returns_an_empty_list_when_the_success_s2c_is_absent` |
| `TestCurrentKLineExtendedErrorBlankAndInvalidRequestBoundaries` | :77 | `[x]` | `kline_query.rs::tests::blank_current_klines_are_filtered_before_merge_and_return` + `test_get_kl_request_encoding` + `period_to_kl_type_matches_go_interval_aliases` |
| `TestSecuritySnapshotInvalidRowsErrorsAndEmptyOptionMerge` | :110 | `[x]` | `security_snapshot_query.rs::tests::snapshot_reader_rejects_a_present_s2c_with_no_mappable_rows` + `snapshot_reader_keeps_mappable_rows_and_drops_invalid_ones` + `snapshot_reader_rejects_invalid_symbols_before_any_opend_call` + `snapshot_reader_keeps_a_payload_less_ack_as_an_empty_collection` |

### 真实功能缺失修复：全非法快照行被当成空成功

Go 有两层语义需要同时满足：

1. `opend.Client.GetSecuritySnapshot`
   （`pkg/futu/opend/market_read_boundaries_test.go:192`）在 OpenD 返回**没有 S2C**
   的成功包时返回 non-nil 空切片；
2. `Exchange.querySecuritySnapshotListDirect`
   （`pkg/futu/security_snapshot.go:224`）把每一行经安全投影映射，**当没有任何一行
   可用时返回 `errNoSecuritySnapshots`**；上层产品适配器再把该错误转成空结果。

Rust 的 `OpenDSecuritySnapshotReader::query_raw_securities` 此前用
`filter_map(map_snapshot).collect()`，因此“S2C 存在但所有行非法”会返回**空成功**，
调用方无法区分“OpenD 说没有”与“行全被丢弃”。修复后按上述分层实现：缺 S2C 仍是
空集合，S2C 存在但无可用行返回新增的
`SecuritySnapshotQueryError::NoSnapshots`。

### 覆盖补强

- 新增 `snapshot_reader_with_s2c` 测试辅助：完成 InitConnect 握手后投递调用方
  指定的 S2C，用于驱动响应形状边界；测试在 `join` 前显式 `drop(reader)`，避免
  服务端等待对端关闭造成的挂起。
- 新增 `blank_current_klines_are_filtered_before_merge_and_return`：固定
  `IsBlank` 占位 K 线既不进入合并结果、也不阻塞同时间戳的真实 K 线。
- 新增 `max_trade_quantity_rejects_invalid_security_after_account_resolution`：
  账户可解析时 `symbol=BAD` 仍必须在触网前被拒。

### 保留边界

- Go 的 `mergeStaticInfoIntoSecurityDetails` 会把 `optionExData` 的
  `type`/`strikePrice` 填进 `SecurityDetails.Option`。Rust 的 securities 公开契约
  （OpenAPI 已核对）不暴露该 option 块，`strikePrice` 仅作为下单预览的产品字段
  存在，因此该子断言属产品模型边界而非缺失；同族用例
  `TestMergeStaticInfoFillsMissingSecurityMetadataWithoutOverwritingSnapshot`
  在 Go 侧覆盖的字段级合并另有 owner。

### 回归可证性（临时探针，已回滚）

- 把 `query_raw_securities` 的 `NoSnapshots` 分支恢复为 `Ok(Vec::new())` →
  `snapshot_reader_rejects_a_present_s2c_with_no_mappable_rows` 以
  `an all-invalid snapshot list must fail closed: []` 失败。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
```

结果：1716 passed / 1 skipped。

## 批次：pkg/futu/trade_margin_ratio_boundaries_test.go（5 项）

### 结果：5 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---|---|---|
| `TestMarginRatioRecoveryAndErrorClassificationBoundaries` | :13 | `[x]` | `trade_session_tests.rs::margin_ratio_unknown_stock_code_extraction_matches_go_boundaries` + `margin_ratio_unknown_stock_recovery_keeps_nil_security_rows_and_code_forms` + `margin_ratio_server_throttling_maps_to_the_typed_rate_limit` + `margin_ratio_wire_throttling_reaches_the_typed_rate_limit_variant` |
| `TestMarginRatioCacheReturnsDefensiveFreshSnapshots` | :62 | `[x]` | `product_trade_margin_cache.rs::tests::cache_returns_cloned_snapshots_only_within_requested_age` + `product_production_ports_trade_tests.rs::margin_ratio_cache_returns_defensive_clones_and_ignores_empty_keys` |
| `TestBrokerMarginRatioFallsBackToRecentCacheAndSurfacesInputFailures` | :86 | `[x]` | `product_production_ports_trade_tests.rs::margin_ratios_fall_back_to_recent_cache_only_for_rate_limit_errors` + `margin_ratios_surface_invalid_symbol_and_missing_account_input_failures` + `margin_ratios_use_recent_cache_only_for_rate_limit_errors` |
| `TestBasicQuoteQueriesHandleEmptyDuplicateAndInvalidRequests` | :124 | `[x]` | `basic_quote_query.rs::tests::normalized_instruments_accepts_empty_symbol_list` + `basic_quote_query_requires_subscription_and_maps_success_rejection_and_empty` + `basic_quote_query_returns_an_empty_list_when_the_success_s2c_is_absent` + `basic_quote_tick.rs::tests::maps_normalized_requested_rows_and_keeps_the_last_duplicate` + `product_market_data_candle_pagination_tests.rs::tick_candles_report_an_empty_page_when_the_ticker_returns_no_sample` + `product_production_ports_trade_tests.rs::margin_ratio_empty_requests_and_duplicate_symbols_match_go` |
| `TestMarginRatioUncachedRecoveryAndConversionBoundaries` | :150 | `[x]` | `product_production_ports_trade_tests.rs::margin_ratios_surface_invalid_symbol_and_missing_account_input_failures` + `trade_session_tests.rs::margin_ratio_unknown_stock_code_extraction_matches_go_boundaries` + `trade_snapshots.rs::tests::margin_ratio_projection_sorts_by_symbol_and_reports_market_labels` + `product_production_ports_trade_tests.rs::margin_ratio_empty_requests_and_duplicate_symbols_match_go` |

### 真实功能缺失修复

1. `unknown_security_code` 的 code 提取不符合 Go 的 `extractUnknownStockCode`
   （`pkg/futu/exchange_trade_margin.go`）：

   - Go 按 marker 顺序 `未知股票` → `unknown stock` → `unknown security` 查找，
     取 marker 后第一个空白分隔 token，用 cut-set `"'.,;:()[]{}` 截断后大写；
     **首个 marker 存在但没有可用 token 时直接返回 `ok=false`**，不再尝试下一个 marker。
   - 修复前 Rust 只用 `trim_matches` 去掉 `:`/`"`/`,`/`;`/`"` 等少量字符，
     `未知股票 (00700)` 提取出 `(00700)`、`unknown stock 00700;` 提取出 `00700;`，
     导致 `remaining.retain` 永远匹配不到，未知股票恢复循环直接回传原错误。
   - 修复后按 Go 的 marker 顺序 + cut-set + 大写实现，并在首个 marker 无 token 时返回 `None`。
2. 服务端限流文本未映射为 `TradeSessionError::RateLimited`
   （`isMarginRatioRateLimitedError`）：

   - Go 除了本地 governor，还分类**服务端**拒绝文本
     （`频率太高` / `too high request frequency` / `每30秒最多10次` / `rate limit`），
     engine 的「30s 直读 TTL + 120s fallback 窗口」缓存回退只在 `RateLimited` 上生效。
   - 修复前 `get_margin_ratio` 把服务端限流一律当成普通 `ResponseError`，
     于是 OpenD 真的限流时回退永不触发，读直接失败。
   - 修复后新增单一 owner `margin_ratio_rate_limited_error`，`get_margin_ratio` 在解码
     失败时分类并提升为 `RateLimited`。

### 覆盖补强（新增回归）

- engine：`margin_ratio_cache_returns_defensive_clones_and_ignores_empty_keys`、
  `margin_ratios_fall_back_to_recent_cache_only_for_rate_limit_errors`、
  `margin_ratios_surface_invalid_symbol_and_missing_account_input_failures`、
  `margin_ratio_empty_requests_and_duplicate_symbols_match_go`；
  新增 `RecordingMarginRead` 夹具记录每次 provider 读取的 security 数，
  证明「两个拼法」折叠成一次请求且随后命中缓存。
- integration：`margin_ratio_unknown_stock_code_extraction_matches_go_boundaries`（10 行表）、
  `margin_ratio_unknown_stock_recovery_keeps_nil_security_rows_and_code_forms`（wire 级
  `未知股票 (07226)` 恢复）、`margin_ratio_server_throttling_maps_to_the_typed_rate_limit`（7 行文本表）、
  `margin_ratio_wire_throttling_reaches_the_typed_rate_limit_variant`（脚本化 OpenD wire 级限流）、
  `margin_ratio_projection_sorts_by_symbol_and_reports_market_labels`。
- 所有联网夹具都在 `server.join()` 前显式 `drop(reader)`/`session.close()`，避免服务端等待对端关闭而挂起。

### 保留边界

- Go 的 `removeUnknownMarginSecurity` 需要处理 `nil *qotcommonpb.Security`。Rust 的
  `read_margin_ratios` 接收 `Vec<TradeSecurity>`（值类型，无 nil 元素），该子断言没有同构 owner；
  等价保证是「cache key 非空 + 重复符号折叠为单次 provider 读取」，
  由 `margin_ratio_empty_requests_and_duplicate_symbols_match_go` 固定。
- Go 的 `basicQotForSymbol`（返回 map 中查不到请求符号即报错）没有同构 Rust 函数：
  Rust 的 ticker 读按 instrument 单查并返回 `Option<Tick>`，缺失即 `Ok(None)`，
  由 tick-candle 读 owner 决定语义，映射为
  `tick_candles_report_an_empty_page_when_the_ticker_returns_no_sample`（空页）
  与 `tick_candles_surface_the_ticker_error_when_no_candle_is_retained`（无缓存时 fail closed）。
- Go 的 `brokerMarginRatioSnapshotsFromProto` 跳过 nil 行。Rust 的 protobuf 投影
  （`margin_ratios_projection`）没有 nil 行可跳过，等价的不可用行是未知 market（投影出空 symbol），
  已在 `margin_ratio_projection_sorts_by_symbol_and_reports_market_labels` 中固定。

### 回归可证性（临时探针，已回滚）

1. 把 `unknown_security_code` 恢复为旧的宽松 `trim_matches` 实现 →
   `margin_ratio_unknown_stock_code_extraction_matches_go_boundaries` 失败
   （`message=Some("unknown stock 00700;")`，`left: Some("00700;")` vs `right: Some("00700")`），
   且 `margin_ratio_unknown_stock_recovery_keeps_nil_security_rows_and_code_forms` 失败
   （`Response(ReturnCode { ret_type: -1, err_code: 1, message: "未知股票 (07226)" })`）。
2. 把 `get_margin_ratio` 恢复为 `Ok(decode_response(&body)?)` →
   `margin_ratio_wire_throttling_reaches_the_typed_rate_limit_variant` 失败
   （`expected the typed rate limit, got Response(ReturnCode { ret_type: -1, err_code: 9, message: "rate limit: too high request frequency" })`）。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
```

## 批次：pkg/futu/adapter_research_contract_test.go（5 项）

### 结果：5 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
|---|---|---|---|
| `TestResearchCatalogOperationsBuildStrictOpenDRequests` | :12 | `[x]` | `research_normalization_tests.rs::catalog_operations_build_strict_opend_requests_like_go` |
| `TestResearchCatalogOperationsRejectMissingOrInvalidParameters` | :84 | `[x]` | `research_normalization_tests.rs::catalog_operations_reject_the_exact_go_parameter_matrix` + `research_params_tests.rs::advanced_research_defaults_reject_incomplete_queries` |
| `TestResearchProtocolPayloadAddsCanonicalFieldsWithoutDroppingOpenDFields` | :115 | `[x]` | `research_normalization_tests.rs::wire_security_rows_gain_canonical_fields_without_losing_opend_fields` + `open_d_enum_text_is_stripped_like_go` + `product_and_calendar_variants_match_go_projection` |
| `TestResearchCatalogLocalPaginationKeepsOpenDRequestUnchanged` | :237 | `[x]` | `research_normalization_tests.rs::local_pagination_never_leaks_into_the_opend_request` + `local_pagination_walks_plate_and_catalog_windows` |
| `TestEconomicCalendarPaginationHonorsExplicitHasMoreAndEmptyRows` | :314 | `[x]` | `research_normalization_tests.rs::economic_calendar_pagination_honors_explicit_has_more_and_empty_rows` + `payload_envelope_picks_the_first_sorted_entry_list_like_go` |

### 真实功能缺失修复

1. **wire 安全标志识没有 owner**（`adapter_advanced_normalization.go::normalizeOpenDValue` /
   `normalizeOpenDSecurity` / `normalizeOpenDEnum`）：

   - Rust 的 `research_entry_security` 只认「已存在的 `security["instrumentId"]`」，
     而真实 OpenD payload 里只有 `{market: "QotMarket_US_Security", code: "aapl"}`。
     实测把 `{"staticInfoList":[{"basic":{"security":{"market":"QotMarket_US_Security","code":"spy"},...}}]}`
     喂给 `normalize_research_protocol_payload("Qot_GetStaticInfo", …)` 会**原样返回**，
     `instrumentId`/`market`/`symbol`/`name`/`productClass` 一个都没补——
     也就是说该投影对任何真实 payload 都是空操作。
   - 修复后新增等价 owner：枚举前缀剥离（13 个前缀，含 `EC_` 的双段规则）、
     `market` 支持枚举文本与数值 `Qot_Common.QotMarket` 两种来源、
     五市场映射 + `future`→HK 与 `event`/`prediction`→US 的 `productClass`，
     并注入 `market`/`quoteMarket`/`tradeMarket`/`instrumentId`；
     在 `normalize_research_protocol_payload` 入口先套用。
2. **没有 Go `payloadEntries` + `setPagination` 的同构 owner**：

   - Go 由这两者把 payload 拆成 entries/metadata 并派生分页信封
     （`nextCursor = firstString(nextPage, nextKey)`、显式 bool `hasMore` **覆盖**派生值、
     `hasMore=false` 清空游标、`total` 取 `total`/`totalCount`/`allCount` 否则用行数）。
     Rust 此前只把 `PAGINATION_KEYS` 用于「只有分页元数据就原样返回」判定，
     `TestEconomicCalendarPaginationHonorsExplicitHasMoreAndEmptyRows` 的
     Entries/HasMore/NextCursor/Total/Metadata 组合断言没有归属。
   - 修复后新增 `research_payload_envelope`（entry 列表按键名升序取第一个对象数组、
     标量行丢弃、行状 payload 自成一条；metadata 剔除已被消费的分页键）。

### 只读侦察结论（本批前提）

- `research_params.rs` 的注入族（`inject_advanced_defaults` /
  `inject_advanced_research_defaults` / `inject_advanced_protocol_defaults` /
  `inject_advanced_cursor` / `inject_advanced_page_size` / `translate_*`）
  在 workspace 内除 `lib.rs` 的 `pub use` 外**没有任何生产调用方**，只有 `*_tests.rs` 在驱动。
  Go 侧这些函数由 `adapter_advanced.go::queryAdvancedFeatureWithProtocols` 统一编排
  （cursor → pageSize → injectFeatureInstrument → injectAdvancedDefaults → CallAdvanced →
  featureResultFromProtocolPayload → applyResearchLocalPagination）。
  本批按「测试 owner 已存在且契约被逐条固定」判 `function_exact`，
  但编排入口缺失属于独立的架构性缺口，未在本批扩大范围处理。

### 保留边界

- Go 的 `injectFeatureInstrument`（`protocolInstrumentField` 表：`securityList`/`ownerList`/
  `multi_legs` 三种赋值形态、事件合约 `US.` 前缀剥离）没有同构 Rust owner；
  本批只固定 `Qot_GetPlateSecurity` 的 `plate` 对象按 `{market, code}` 透传，
  其余形态已在 `adapter_advanced_protocol_test.go` 的既有 partial 结论中记录，不重复登记。
- Go 的 `opend.ValidateAdvancedC2S` 反射式严格校验在 Rust 没有同构函数；
  Rust 的代偿是 typed reader 在 RPC 前重新校验注入结果，
  由 `advanced_feature_defaults_build_strict_opend_requests` 与
  `advanced_page_size_respects_protocol_limits` 覆盖。

### 回归可证性（临时探针，已回滚）

- 把 `normalize_research_protocol_payload` 里的 `normalize_open_d_value(payload)`
  换回 `payload.clone()`（即恢复「无 wire 规范化」）→
  `wire_security_rows_gain_canonical_fields_without_losing_opend_fields` 失败：
  `assertion left == right failed, left: Null, right: String("US.AAPL")`。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
```

结果：`jftrade-integration-futu` 498 passed / 1 skipped。

## 批次：pkg/futu/stream_connection_quote_boundaries_test.go（12 项）

### 结果：12 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
| :--- | :--- | :--- | :--- |
| `TestStreamCloseCancelsAndJoinsOwnedWorkers` | 22 | `[x]` | `runtime_task.rs::tests::runtime_shutdown_cancels_then_joins_its_worker` |
| `TestStreamConnectionAndSubscriptionBoundaries` | 78 | `[x]` | `subscriptions_tests.rs::basic_quote_requests_extract_only_the_basic_channel_like_go` + `session_coordinator.rs::tests::empty_demand_sessions_connect_repeatedly_without_subscription_traffic` |
| `TestStreamPushHandlersRejectInactiveMalformedAndEmptyQuotes` | 115 | `[x]` | `product_runtime_opend_listener_tests.rs::basic_quote_pushes_drop_rows_without_a_usable_security_or_price` |
| `TestStreamConvertsCumulativeQuoteVolumeToIncrementalTradeQuantity` | 143 | `[x]` | `trade_volume.rs::tests::first_sample_is_a_baseline_and_decreases_or_negatives_reset_the_baseline` |
| `TestStreamPreservesFractionalCumulativeVolumeDelta` | 174 | `[x]` | `trade_volume.rs::tests::fractional_and_out_of_fixedpoint_counters_keep_their_exact_delta` |
| `TestStreamMarketTradeCarriesDeltaAndCumulativeVolume` | 188 | `[x]` | `product_runtime_opend_listener_tests.rs::basic_quote_pushes_publish_delta_and_cumulative_volume` |
| `TestStreamMarketTradePreservesVolumeBeyondLegacyFixedpointRange` | 215 | `[x]` | `product_runtime_opend_listener_tests.rs::basic_quote_pushes_keep_exact_volume_beyond_fixedpoint_range` |
| `TestStreamRejectsNegativeSnapshotVolume` | 239 | `[x]` | `product_runtime_opend_listener_tests.rs::negative_cumulative_volume_publishes_no_trade_event` |
| `TestBasicQuotePushSubscriptionErrorsAndIdempotency` | 255 | `[x]` | `subscription_executor.rs::tests::basic_subscribe_registers_push_delivery_like_go_stream` |
| `TestOrderBookStreamConnectionBoundaries` | 292 | `[x]` | `subscriptions_tests.rs::order_book_connect_boundaries_reject_empty_invalid_and_unavailable_demand` + `subscription_executor.rs::tests::hk_order_book_subscribe_requests_detail_and_registers_push` + `product_runtime_opend_listener_tests.rs::order_book_pushes_publish_depth_for_the_subscribed_instrument` |
| `TestStreamReconnectAndClientWatcherExitPaths` | 317 | `[x]` | `product_runtime_opend_listener_tests.rs::provider_reconnect_publishes_resync_events_and_wakes_reconciliation` + `session_coordinator.rs::tests::coordinator_close_is_terminal_and_prevents_orphaned_reconnect` + `client_recovery_boundaries.rs::closed_ready_session_is_replaced_on_peer_close` |
| `TestStreamConnectReportsPhysicalSubscriptionFailures` | 369 | `[x]` | `subscription_executor.rs::tests::executor_maps_qot_sub_rejection_without_replaying_first_attempt` |

### 本批修复的功能差异（2 项，均为真实缺口）

1. **`TestStreamRejectsNegativeSnapshotVolume`（行 239）**
   修复前：Rust 的 `publish_basic_quote_tick` 对负累计量仍会下发
   `volumeDelta="0"` 的 `market-data.tick`。Go 的 `emitBasicQotSnapshot`
   在负累计量时**直接 return**，既不发任何 trade 事件，也不把该样本写进
   `tradeVolumes` baseline。
   修复：新增 `jftrade_marketdata::TradeVolumeTracker::is_negative_cumulative`，
   在 `LiveHubOpenDEventListener::publish_basic_quote_tick` 里**消费 tracker 之前**
   早退，保证被拒绝的样本不污染 baseline。
2. **`TestStreamReconnectAndClientWatcherExitPaths`（行 317）**
   修复前：reconnect 分支（`OpenDSessionCoordinatorOutcome::Reconnected`）
   是 listener 里唯一**完全没有 Rust 测试**的分支——`market-data.resync`、
   `console.refresh` 与 reconciliation wake 都无人固定，去掉 `notify_one()`
   时全套测试仍然全绿。
   修复：新增 `provider_reconnect_publishes_resync_events_and_wakes_reconciliation`，
   固定「重连 → notify wake + resync/refresh 事件」与「普通断线只发 stale、
   不伪装重连」两条不可互换的语义。

### 边界保留结论（非缺口）

- `TestStreamConnectionAndSubscriptionBoundaries` 中 Go 的
  `basicQotRequestsFromSubscriptions` 是**函数级返回错误**；Rust 的等价 owner 是
  `desired_subscriptions` + `InstrumentRef::normalize`，非法项被丢弃而不是让整次
  调用失败——这是 Rust 的分层差异（校验在 `marketdata` 的规范化层，订阅计划不再
  产生「部分失败」的中间态），已由 `desired_physical_subscriptions_reject_incomplete_refs_and_normalize_symbols`
  固定为「丢弃而非编码」，不迁移 Go 的 error 返回值形态。
- `TestStreamCloseCancelsAndJoinsOwnedWorkers` 里 Go 的
  「disconnect 回调内 `Connect()` 返回 `opend.ErrClosed`」在 Rust 由
  `OpenDSessionCoordinatorError::Closed` 承载，由
  `coordinator_close_is_terminal_and_prevents_orphaned_reconnect` 覆盖；
  worker 必须在**自持回调里被 join** 这一时序契约由
  `runtime_shutdown_cancels_then_joins_its_worker` 用阻塞 listener 直接证明
  （shutdown 在 worker 停在回调内部时 100ms 内不得返回）。

### 回归可证性（临时探针，均已回滚）

- 删除 `Reconnected` 分支里的 `wake.notify_one()` →
  `provider_reconnect_publishes_resync_events_and_wakes_reconciliation` 失败：
  `reconnect must wake reconciliation: Elapsed(())`。
- 把 `OpenDSessionRuntime::shutdown` 的 `worker.join()` 换成 `drop(worker)` →
  `runtime_shutdown_cancels_then_joins_its_worker` 失败：
  `shutdown returned while its worker was still parked in a callback`。
- 把 `desired_subscriptions` 的 `let Ok(reference) = raw.clone().normalize() else { continue }`
  改成 `unwrap_or_else(|_| raw.clone())` →
  `order_book_connect_boundaries_reject_empty_invalid_and_unavailable_demand` 失败：
  `an unparsable order-book symbol must be dropped: ... key: "ORDER_BOOK:.BAD"`。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine -p jftrade-marketdata --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine -p jftrade-marketdata --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

结果：1799 passed / 1 skipped；审计 OK: 495 function_exact。

## 批次：pkg/futu/adapter_combo_transport_test.go（4 项）

### 结果：4 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
| :--- | :--- | :--- | :--- |
| `TestFutuOptionComboPreviewLegalityAndTransportFailures` | 16 | `[x]` | `product_production_ports_execution_preview_tests.rs::option_combo_preview_rejects_invalid_legality_and_hidden_transport_failures` |
| `TestFutuOptionComboNonSpreadOpenDLegalityBranches` | 80 | `[x]` | `product_production_ports_execution_preview_tests.rs::option_combo_preview_validates_non_spread_legality_against_opend_strategies` |
| `TestFutuOptionComboPreviewNormalizesAllAccountImpacts` | 149 | `[x]` | `product_production_ports_execution_preview_tests.rs::option_combo_preview_normalizes_every_account_impact_and_unlimited_bounds` |
| `TestFutuComboPlaceValidatedLegAccountAndTransportFailures` | 199 | `[x]` | `product_production_ports_execution_preview_tests.rs::combo_place_rejects_invalid_legs_accounts_and_hidden_transport_failures` |

### 本批修复的功能差异（3 项，均为真实缺口）

1. **无界盈亏哨兵被当成有限值下发**
   Go `pkg/futu/adapter_combo.go::optionComboBound` 把 OpenD 的 `9999999` 折叠成
   `maxProfitUnlimited`/`maxLossUnlimited` 并返回 nil 边界。Rust 的
   `option_combo_analysis` 直接把 `maxProfit`/`maxLoss` 原样写入，于是预览里出现
   `maxProfit: 9999999`，而冻结契约 `broker.OptionComboAnalysis`
   （`docs/swagger/swagger.json`）与 `OptionComboRiskStrip.vue` 依赖
   `maxProfitUnlimited` 渲染「无限」。修复：新增
   `OPTION_COMBO_UNLIMITED_BOUND = 9_999_999.0`，达到阈值即写
   `*Unlimited=true` 并**不再**下发哨兵数值。
2. **私有 ReasonCode 泄漏到公开 wire**
   Go 的 `ILLEGAL_OPTION_SPREAD` / `ILLEGAL_OPTION_COMBINATION` 只是
   `deniedProductRule` 的内部 `ReasonCode`；`PreviewExecutionCombo` 把它包成
   `requestErrorf(reason)`，`executionCommandError` 再映射为 400 `BAD_REQUEST`
   （见冻结 fixture `tests/fixtures/compatibility/api-transport/execution-write.json`
   中 `combo-preview-mixed-legs` 等用例：`code` 一律 `BAD_REQUEST`）。Rust 之前把
   这两个私有码直接写进错误信封，属于公开契约偏差。修复：两处分支改回
   `400 BAD_REQUEST`，原因文本保留在 message。
3. **未限定的 underlying 被静默替换成请求市场**
   Go `futuSecurityFromSymbol` → `market.ParseInstrument` 对无市场前缀的符号返回
   `market is required when symbol has no market prefix`；Rust 的
   `underlying_security` 却在 `rsplit_once('.')` 失败时用
   `market_label(header.trd_market)` 兜底，使 `"BAD"` 变成 `US.BAD` 并**成功返回
   allowed=true 预览**（本次实测复现）。修复：缺少 `MARKET.CODE` 形态直接 400；
   随之删除仅剩该兜底在用的 `product_production_ports_execution_order_markets.rs::market_label`
   （消除 dead_code 警告）。

### 回归可证性（临时探针，均已回滚）

- 撤销无界翻译（恢复 `maxProfit`/`maxLoss` 直写）→
  `option_combo_preview_normalizes_every_account_impact_and_unlimited_bounds` 失败。
- 把 spread 分支改回 `ILLEGAL_OPTION_SPREAD` →
  `option_combo_preview_rejects_invalid_legality_and_hidden_transport_failures` 失败：
  `illegal spread error = Failed { status: 400, code: "ILLEGAL_OPTION_SPREAD", ... }`。
- 恢复 `"BAD"` → 请求市场兜底 →
  `option_combo_preview_rejects_invalid_legality_and_hidden_transport_failures` 失败：
  `an invalid underlying must fail closed: Object {..., "allowed": Bool(true), ...}`。

### 边界保留结论（非缺口）

- Go 用 `OptionStrategyType` 枚举值（`vertical→4`、`straddle→6` 等）决定走
  spread 还是 strategy 分支，Rust 用 payload 的 `optionStrategy` 字符串做同一分流；
  两条路径的 RPC（3258 vs 3256）与比对语义一致，属实现形态差异。
- Go 在 `Qot_GetOptionStrategy` 解析失败时报错早退，Rust 由 typed reader 在
  `OptionStrategyQueryError` 层拒绝；本批以 `InvalidResponse` 模拟 drop 分支，
  证明错误不会被吞成 `allowed=true`。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu -p jftrade-marketdata --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-engine -p jftrade-integration-futu -p jftrade-marketdata --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

结果：1803 passed / 1 skipped；审计 OK: 499 function_exact。

## 批次：pkg/futu/adapter_marketdata_search_test.go（4 项）

### 结果：4 `[x]`

| Go 测试 | 行号 | 状态 | Rust 证据 |
| :--- | :--- | :--- | :--- |
| `TestFutuSearchMarketCodePreservesEveryStableDisplayMarket` | 12 | `[x]` | `instrument_search_query_tests.rs::search_market_codes_are_mapped_like_go_for_every_stable_display_market` |
| `TestCanonicalSearchQuoteSymbolHandlesOpenDPrefixedCodes` | 38 | `[x]` | `product_production_ports_market_data_catalog_tests.rs::futu_search_canonicalizes_open_d_prefixed_symbols_like_go` + `instrument_search_query_tests.rs::search_symbols_normalize_open_d_prefixed_codes_like_go` |
| `TestBrokerAdapterSecuritySearchMapsCrossMarketOpenDResults` | 58 | `[x]` | `product_production_ports_market_data_catalog_tests.rs::futu_search_maps_cross_market_rows_and_drops_unusable_entries` |
| `TestBrokerAdapterSecuritySearchRejectsInvalidQueriesBeforeConnecting` | 113 | `[x]` | `product_production_ports_market_data_catalog_tests.rs::futu_search_rejects_invalid_queries_before_reaching_opend` |

### 本批修复的功能差异（1 项，真实缺口）

1. **带前缀的 OpenD 搜索码被二次加前缀**
   Go `canonicalSearchQuoteSymbol` 只在「码自带的前缀等于该行市场」时剥离前缀
   （`CNSH`/`CNSZ` 归一到 `SH`/`SZ`，`CC`→`CRYPTO`），否则保留原始码。Rust 的
   `candidate()` 直接做 `format!("{market}.{code}")`，于是 loopback 返回的
   `US.AAPL` / `CNSH.600519` 被投影成 **`US.US.AAPL` / `SH.CNSH.600519`**，
   公开的 `instrumentId` 不再指向券商返回的证券身份。修复：在
   `product_production_ports_market_data_catalog_futu.rs` 新增
   `canonical_search_code` + `canonical_search_market_prefix` 等价 owner，
   由投影层统一产出 `code`/`symbol`/`instrumentId`。

### 回归可证性（临时探针，已回滚）

- 把 `canonical_search_code(&entry.market, &entry.code)` 换回裸 `entry.code.clone()` →
  `futu_search_canonicalizes_open_d_prefixed_symbols_like_go` 与
  `futu_search_maps_cross_market_rows_and_drops_unusable_entries` 双双失败：
  `left: String("US.US.AAPL")`。

### 边界保留结论（非缺口）

- `securityType` 大小写：Go 的 `enumName` 透出 protobuf 原始名（`Eqty`），
  Rust 的 `map_entry` 输出 `EQUITY`。控制台
  `instrumentPresentation.ts::SECURITY_TYPE_LABELS` 同时登记 `EQTY` 与
  `EQUITY`（以及 `STOCK`）并归一化，公开 schema 只声明 `string`，属于双方都能
  消费的拼写差异而非行为缺口，本批按原样固定（`EQTY` 由 fixture 传入）。
- 路由 `market` 参数只接受 US/HK/CN/SH/SZ，而 Go 用例是直接驱动 reader 覆盖
  JP 等非路由市场；因此 `futu_search_canonicalizes_open_d_prefixed_symbols_like_go`
  省略 `market` 参数，由行自身市场决定前缀，与 Go 的调用层级一致。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu -p jftrade-marketdata --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-engine -p jftrade-integration-futu --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

结果：1808 passed / 1 skipped；审计 OK: 503 function_exact。

## 批次：`pkg/futu/adapter_prediction_stream_test.go`（4 项，全部 `missing` → `[x]`）

### 结果：4 `[x]`（4 条 function_exact，含 2 处真实功能修复）

| Go 测试 | 行号 | 状态 | Rust 证据 |
| --- | --- | --- | --- |
| `TestFutuPredictionStreamListenersSequencesAndMalformedPushes` | :13 | `[x]` | `prediction_push_stream.rs::prediction_listeners_sequence_rows_and_drop_malformed_pushes` |
| `TestFutuPredictionPushHandlerInstallationReplayAndFailure` | :87 | `[x]` | `prediction_push_stream.rs::prediction_push_handlers_install_once_and_fail_closed_on_invalid_demand` |
| `TestFutuPredictionNormalizationCatalogPaginationAndIdentity` | :139 | `[x]` | `prediction_push_stream.rs::prediction_catalog_pagination_and_identity_follow_the_go_rules` |
| `TestFutuAdvancedSecurityNormalizationAllPublicMarkets` | :239 | `[x]` | `prediction_push_stream.rs::prediction_security_normalization_covers_all_public_markets` |

### 本批修复的功能差异（2 项，真实缺口）

1. **3450/3451/3452 事件合约推送在线路上被完全丢弃**
   Go 用 `subscribePredictionPush` 给每个 OpenD client 装三个订阅
   （`Qot_UpdateEventContractOrderBook`=3450、`Kline`=3451、`Ticker`=3452），
   只有「能解码 + `retType==0` + `s2c` 非空」三条件同时成立才把 `S2C` 交给
   `emitPredictionPush`；adapter 再按行投影成 `PredictionMarketUpdate`。
   Rust 此前完全没有这条路径：`build.rs` 不编译三个 `Qot_UpdateEventContract*.proto`，
   `trade_proto` 无对应模块，`decode_quote_push` 把这三个协议一律落进最后的
   `_ => Ok(None)`，全仓也搜不到任何 prediction 推送符号——即使用户已经持有
   prediction 订阅租约，市场推送也永远不会到达前端。
   修复：新增 crate 级 owner `crates/jftrade-integration-futu/src/prediction_push.rs`
   （`PredictionDataType`/`PredictionPushRow`/`PredictionPushRegistry`/
   `decode_prediction_push`/`entry_instrument_id`/`entry_sequence`），
   补 `build.rs` 协议表与 `trade_proto` 三个 `PROTOCOL_ID` 模块；
   accept 规则与 Go 的三重检查逐条对应（畸形 protobuf 仍向直接调用方报 `Err`，
   由会话泵按既有 quote-push 约定 drop，不升级为重连）。
   handler 注册语义对齐 Go：普通注册是可移除槽位、重复移除安全、
   移除后不再投递，且 listener 集合在派发前快照，
   listener 内部的注册/注销不会改变本次派发的扇出。
2. **数值市场 101 / 2 无法解析（`qot_market_label` 表不全且语义错位）**
   Go 的 `normalizeOpenDSecurity` 在 `value["market"]` 是 `float64` 时先用
   `qotcommonpb.QotMarket(int32(n)).String()` 还原枚举名，再走同一套
   `contains("future")`/`contains("event")` 子串判断，因此 `float64(101)`
   （`QotMarket_EventContract`）得到 `US` + `productClass=event_contract`，
   `float64(2)`（`QotMarket_HK_Future`）得到 `HK` + `future`。
   Rust 原先只登记 7 个市场标签且直接返回「标签」而非枚举名，
   数值 101/2 双双解析失败；实测数值 101 的断言失败为
   `left: String("QotMarket_Event")`（原始串未被改写）。
   修复：把 `qot_market_label` 改为 `qot_market_enum_name`（0/1/2/11/21/22/
   31/41/51/61/71/81/91/101），保留既有子串判断消费，
   与 Go 的 `String()` → `switch` 顺序等价。

### 探针（改坏实现 → 跑测试 → 确认失败 → 已回滚）

- 去掉 `PredictionPushRegistry` 接受路径中的 `ret_type == 0` 检查
  → `prediction_listeners_sequence_rows_and_drop_malformed_pushes` 失败：
  `a rejected push must not reach listeners`。
- 去掉推送投影里的 `code.is_empty()` 守卫
  → 同一测试的 updates 断言失败，出现伪造身份
  `("US.", "", "ORDER_BOOK")`。
- 删掉 `101 => Some("QotMarket_EventContract")`
  → `prediction_security_normalization_covers_all_public_markets` 失败：
  `numeric 101 must resolve to US`。

### 边界保留结论（非缺口）

- `resolvedPredictionInstrument`（Go 在无类型 protocol map 上算首行身份）
  在 Rust 由 typed reader 的 `code()`/`security()`/`snapshot_value` 与
  推送行的 `entry_instrument_id` 分别承担，故意不复制 Go 的 `listKey` 表：
  prediction 读取入口是 typed reader，再造一层 map 投影只会产生第二个 owner。
- Go 的 `predictionPushResult` 5 秒新鲜度缓存位于 `internal/productfeatures`
  （Go 侧 assembly owner），Rust 的对应读取路径是 engine 的
  `ProductionMarketDataPredictionPort`，不在本 crate；
  该用例的断言面（listener/install/replay/normalization）已全部落地，
  缓存 TTL 语义归属 `internal/productfeatures` 批次继续跟踪。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine -p jftrade-marketdata --all-targets --locked --no-fail-fast
cargo fmt --all --check
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
python3 scripts/compatibility/audit_test_parity.py
```

结果：1812 passed / 1 skipped（新增 4 项）；审计 OK: 507 function_exact。

---

## 批次：pkg/futu/exchange_kline_test.go（4 项）

基线：`go:pkg/futu/exchange_kline_test.go` 的 4 条 `[~]` 用例。

### 修复的真实功能缺口

1. **`:385 TestQueryKLinesIncludesCurrentRealtimeBucketFromGetKL` — `Qot_GetKL`
   当前桶没有按调用方窗口过滤**
   Go 在 `mergeKLinesByStartTime` 之前先跑
   `filterKLinesByWindow(currentKLines, beginAt, endAt)`；`Qot_GetKL` 只能按
   bucket 数量请求，无法表达窗口，所以窗口必须在响应上强制。Rust 之前直接
   合并 `current_res.klines`，会把窗口外的桶交给调用方。
   新增 `crates/jftrade-engine/src/product_production_ports_market_data_quote_reads_futu.rs::filter_current_klines_by_window`
   （配 `parse_window_bound`），并按 Go 的
   `finishAt.Before(beginAt) || startAt.After(endAt)` 语义丢弃越窗桶；
   窗口能比较就用窗口端点，无法解析的边界或标签保留蜡烛。

2. **`:150 TestQueryKLinesForSessionsFiltersCurrentUSBucket` — 合并后的整表
   没有重跑 session 过滤**
   Go 的 `QueryKLinesForSessions` 跑两次
   `filterKLinesBySessions`：路由历史页一次，`Qot_GetKL` 当前桶合并后再一次。
   当前桶这一路不经路由，所以 `sessions=regular` 的请求可能拿到盘前/盘后桶。
   新增 `product_production_ports_market_data_quote_reads_futu.rs::filter_klines_by_sessions`，
   在 merge 之后按 `route_sessions.is_some()` 重跑，`extended` 覆盖 pre+after。

3. **`merge_current_bucket` 抽取（同时守住 800 行生产文件上限）**
   上述两步把主读取路径推到 827 行，超过
   `check:workspace-architecture` 的 800 行上限。抽取到
   `crates/jftrade-engine/src/product_route_current_bucket_helper.rs`
   （与既有 `product_route_session_helper.rs` 同风格，经 `include!` 引入），
   主文件回到 795 行。

### 补齐的行为证据

4. **`:17 TestQueryTickersBatchesBasicQotRequests`**
   `crates/jftrade-integration-futu/src/basic_quote_query.rs::query_ticks_batches_every_instrument_into_one_get_basic_qot_call`
   扩到完整 Go 形状：一条 TCP 会话（acceptCount=1）内先解码两次 `Qot_Sub`
   （每次一个 security，断言 `isSubOrUnSub=true` 与 market/code），再用一次
   `Qot_GetBasicQot` 批量携带两个 security，最后断言 `calls==1`、两条 tick。
   测试模块新增 `WireQotSubRequest/WireQotSubC2s/WireQotSubResponse` 镜像结构
   用于解码线上字节，生产 `QotSub*` 类型保持私有。

5. **`:428 TestStreamConnectEmitsBasicQotPushAsBBGOEvents`**
   新增
   `crates/jftrade-engine/src/product_runtime_opend_listener_tests.rs::one_live_subscription_publishes_both_trade_and_depth_pushes`：
   同一条 `HK.00700` 订阅先收到 BasicQot push（price=700、
   cumulativeVolume=1000、volumeDelta=0，对应 Go 的 `ticker.Quantity=0`，
   首笔累计量只作基线），再收到 OrderBook push（bids/asks 最佳价=700）。
   Rust 的等价 wire 是引擎侧 `market-data.tick` / `market.depth` envelope，
   而非 BBGO `Trade`/`BookTicker` 类型。

### 夹具修正（既有测试）

- `candle_route_keeps_latest_history_after_all_forward_pages_and_current_bar`
  原用固定 `2026-01-05` 的 current 夹具。窗口过滤落地后该桶落在真实 `now`
  窗口之外并被正确丢弃（与 Go 同行为），因此按 Go 的 `time.Now()` 形状把
  history 页与未闭合桶都锚到真实时钟，并把断言扩到 Go 的
  `klines[1].Closed == false` 与 OHLC/volume。默认 `PagedHistory` 分页形状
  改由 `single_page` + 真时钟 times 承载。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- `filter_current_klines_by_window` 不接入 merge
  → `candle_route_trims_current_bucket_to_the_callers_window` 失败：
  `a bucket outside the explicit window must not be merged: ["2026-09-18T01:01:00Z", "2026-09-18T01:15:00Z"]`。
- `if route_sessions.is_some()` 改成 `if false`
  → `us_regular_only_request_drops_an_extended_hours_current_bucket` 失败：
  `a regular-only request returned a overnight candle: [... "session": String("overnight")]`。
  两处探针均已回滚并复跑通过。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --all
python3 scripts/compatibility/audit_test_parity.py
```

结果：1755 passed / 1 skipped；审计 OK: 511 function_exact（本批 4 条 `[~]` → `[x]`）。

---

## 批次：pkg/futu/exchange_trade_price_test.go（4 项）

基线：`go:pkg/futu/exchange_trade_price_test.go` 的 4 条 `[~]`/`missing` 用例。

本批为**补齐证据**批次：Rust 生产实现（`trade_price.rs` 的
`normalize_submit_order_price`/`submit_order_price_step`/`round_price_to_step`/
`step_rounded_unit`/`count_step_decimals`/`is_finite_positive`，以及
`trade_session.rs::place_order_command` 的归一化与字段透传）已与 Go 语义一致，
未发现功能缺口；缺的是逐条断言。

### 补齐的行为证据

1. **`:13 TestFutuRequestLocationUsesMainlandMarketFallback`**
   新增
   `crates/jftrade-engine/src/product_production_ports_trade_tests.rs::mainland_trade_market_falls_back_to_the_shanghai_request_location`。
   Rust 没有 Go 的 `market.ProfileForSymbol` 表，等价链是
   `trade_market_authority(3) == "CN"` → `normalize_history_time` 的
   `"CN" | "SH" | "SZ" => "Asia/Shanghai"`，断言
   `2026-01-01T00:00:00Z` 渲染为 `2026-01-01 08:00:00`（UTC+8），
   与 Go 的 `TrdMarket_CN → SH profile → time.Location` 回退等价。

2. **`:24 TestNormalizeSubmitOrderPriceForUSMarkets`**
   既有 `trade_price.rs::tests::normalize_submit_order_price_rounds_us_prices_to_their_tick`
   已逐条覆盖 Go 的四条断言（US ≥ $1 用 0.01、低于 $1 用 0.0001、
   `0.12345 → 0.1235` 半值上取整、HK `320.123` 原样），本批仅确认并登记。

3. **`:39 TestPriceStepHelpersCoverEdgeCases`**
   新增 `trade_price.rs::tests::price_step_helpers_cover_their_edge_cases`，
   补齐 Go 的边界矩阵：`submitOrderPriceStep(US 150 / US 0.55 / HK 380)`、
   `roundPriceToStep(0.12344,0.0001)`、`roundPriceToStep(123.456,0.01)`、
   非十进制 tick `roundPriceToStep(10.03,0.05) → 10.05`、
   `stepRoundedUnit(4)`、`countStepDecimals`，以及
   `isFinitePositive` 对 `0 / -1 / NaN / +Inf` 的拒绝与对 `0.01` 的接受。
   `countStepDecimals` 的既有断言保留在独立的
   `step_decimals_follow_the_shortest_representation` 中（新增 `0.05 → 2`）。

4. **`:75 TestPlaceOrderRequestFromSubmitOrderNormalizesUSPriceAndFlags`**
   扩展 `trade_session_tests.rs::place_order_rounds_us_prices_to_the_venue_tick_before_encoding`：
   在 `Trd_PlaceOrder` 线上字节上，除既有价格/aux 归一化断言外，
   新增 `session == Some(1)`（RTH）与 `fill_outside_rth == Some(true)` 透传断言，
   对应 Go 的 `GetSession() == 1` 与 `GetFillOutsideRTH()`。
   价格部分覆盖 US `123.456→123.46`、`12.3456→12.35`、`0.12345→0.1235`、
   `0.99994→0.9999`、HK 原样、以及零价不发线字段。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- `submit_order_price_step` 的 US cents 分支改成 `return 0.0`
  → `price_step_helpers_cover_their_edge_cases` 失败：
  `assertion left == right failed: left: 0.0, right: 0.01`。
  探针已回滚并复跑通过。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --all
python3 scripts/compatibility/audit_test_parity.py
```

结果：1757 passed / 1 skipped；审计 OK: 515 function_exact（本批 4 条 → `[x]`）。

---

## 批次：pkg/futu/trade_account_test.go（4 项）

基线：`go:pkg/futu/trade_account_test.go` 的 4 条 `[~]`/`missing` 用例。
本批发现并修复 **4 处真实功能缺口**。

### 修复的真实功能缺口

1. **`:15 TestDiscoverAccountsDeduplicatesAndFallsBackToCardIdentifier` —
   runtime 账户列表不去重、不排序，`accountsDiscovered` 也不是去重后数量**
   Go 的 `runtimeAccountsFromProto` 在返回前按
   `key := AccountID + "|" + TradingEnvironment` 去重，再按
   「environment → AccountID」排序，`AccID == 0` 时回退到 `CardNum` /
   `UniCardNum`（任意非空字符串，不要求纯数字）。
   Rust 之前直接把 provider 顺序原样投影，两条共享 `SIM-CARD` 的模拟账户
   会重复出现（且各自带不同枚举投影），`accountsDiscovered` 报 3。
   新增 `crates/jftrade-engine/src/trade_projection.rs::accounts_value`，
   在 runtime 路由 `product_production_ports_trade.rs` 中先投影再去重，
   计数取去重后长度。
   注意：`execution_reconciliation_discovery.rs::account_identity` 仍要求
   正数字 ID ——那是**对账**路径，与 Go 不同 owner，本批未改动。

2. **`:118 TestQueryBrokerOrdersFiltersAndSortsWorkingOrders` —
   broker 订单既不排序也不做 symbol 过滤**
   Go 的 `brokerOrderSnapshotsFromProto` 先用
   `strings.EqualFold(strings.TrimSpace(order.GetCode()), canonicalSymbol)`
   过滤（provider 的 filter 只是提示），再按 `brokerOrderSortKey`
   （`UpdatedAt`，缺失回退 `SubmittedAt`）倒序、以 `BrokerOrderID` 降序破平。
   Rust 之前依赖 provider 返回顺序，`symbol=hk.00700` 时会把 `US.AAPL` 行
   一起返回（实测 3 条 / 期望 2 条）。
   新增 `trade_projection.rs::orders_value` + `order_sort_key` +
   `parse_broker_order_time`，并在 `ResolvedTradeRequest` 上新增
   `order_symbol_filter` 字段承载 canonical symbol。

3. **`:199 TestPlaceOrderRequestFromSubmitOrderCoversValidationAndRemarkSemantics`
   — 非 US 订单的 `fillOutsideRTH` 未拒绝、market 单未丢弃该 flag**
   Go 在 `placeOrderRequestFromSubmitOrder` 中先判
   `secMarket != TrdSecMarket_US` → 报
   `"fillOutsideRTH is supported for US orders only"`，再判
   `supportsFillOutsideRTH`（只覆盖 Normal / StopLimit）才写入 wire。
   Rust 之前把显式 flag 原样透传：HK 订单被静默接受，market 单会带上
   `fillOutsideRTH`（实测 `left: Some(true) / right: None`）。
   在 `product_production_ports_execution_order_parse.rs` 补两处守卫，
   并把显式 flag 也纳入 `supports_fill_outside_rth` 判定。

### remark 语义澄清（非缺口）

Go 有**两层**remark 规则：
`internal/trading/execution_normalize.go` 是「显式 `remark` 优先、缺失回退
`clientOrderId`」；更下游的 `placeOrderRequestFromSubmitOrder` 才是
「`SubmitOrder.ClientOrderID` 优先、`Tag` 回退」。Rust 的 wire `remark`
对应前者（后者在 Rust 没有独立的 `tag` 字段），现有实现与 Go 一致，
本批只补齐断言，未改语义。

### 补齐的行为证据

4. **`:72 TestResolveTradeMarketCoversRequestedAndFallbackBranches`**
   既有
   `resolve_account_honors_requested_authority_and_falls_back_like_go`
   已逐条覆盖 Go 的五个分支（请求市场需在 authority 内、未授权返回
   no-match 而非错误、无 authority 走直接映射、非法市场名硬报错、
   非法 authority 项跳过取首个有效项、无 authority 默认 HK），本批登记。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- `accounts_value` 去掉排序
  → `runtime_account_discovery_deduplicates_sorts_and_falls_back_to_card_identity`
  失败：`left: String("SIM-CARD") / right: "1002"`。
- `orders_value` 去掉 canonical symbol 过滤
  → `broker_working_orders_are_filtered_sorted_and_symbol_normalized_like_go`
  失败：`left: 3 / right: 2`。
  两处探针均已回滚并复跑通过。

### 既有测试的期望值修正

- `broker_current_orders_hide_terminal_statuses_while_history_keeps_them`：
  `OrderFixtureRead` 的 2001/2002 共享同一 update time，排序生效后按 Go
  的 tie-breaker（broker order id 降序）2002 应排在前，断言随之调整。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --all
python3 scripts/compatibility/audit_test_parity.py
```

结果：1760 passed / 1 skipped；审计 OK: 519 function_exact（本批 4 条 → `[x]`）。

---

## 批次：pkg/futu/trade_helpers_boundary_test.go（4 项）

基线：`go:pkg/futu/trade_helpers_boundary_test.go` 的 4 条 `[~]`/`missing`。
本批发现并修复 **2 处真实功能缺口**，另有 2 条为 Go 专有边界。

### 修复的真实功能缺口

1. **`:68 TestTradeProtoConversionSkipsNilFeeAndInvalidMarginSecurity` —
   非法 margin security 让整个读取失败**
   Go 的 `brokerMarginRatioSnapshotFromProto` 对无法解析的 security 是**降级**：
   `futuSymbolFromSecurity` 失败则把 `symbol` 置空，**该行仍然保留**在其他字段
   正常返回的结果里。Rust 原先在响应校验层
   （`trade_proto_margin_ratio_validation.rs`）对
   `security.market ∉ {1,11,21,22,31,41,51,61,71}` 直接
   `UnsupportedValue` 整包拒绝——Go 能正常返回的数据在 Rust 变成硬错误。
   已改为：只有 market 可解析时才校验 `security.code` 非空，非法 market 交由
   `trade_snapshots::margin_ratios_projection` 置空 symbol。
   新测试 `margin_ratio_response_keeps_a_row_with_an_unusable_security_market`
   断言两行都保留（非法行 symbol 为空、合法行 `HK.00700`，按 symbol 排序）。

2. **`:81 TestAccountAndPushConversionBoundaries`（候选排序部分）—
   账户选择依赖 provider 返回顺序**
   Go 的 `sortResolvedTradeAccounts` 先按 environment priority
   （SIMULATE `0` < REAL `1` < UNKNOWN `2`）→ `AccountID` → `Market` 排序，
   再取 `candidates[0]`。Rust 的 `resolve_account_with_environment` 直接
   `candidates.into_iter().next()`，谁先返回谁被扣款。新增
   `environment_priority` 与 `account_market_order_key`，在选账户前排序。

### Go 专有边界（不迁移）

3. **`FeeList: []*OrderFeeItem{nil, ...}` 的 nil 跳过**：
   Rust 的 `repeated` 由 prost 解码成值，不可为 nil，没有对应行可跳。
   该结论写进了 `margin_ratio_with_an_invalid_security_market_keeps_an_empty_symbol`
   的注释。

4. **`fixedpointFromDifference` / `optionalFloat64Value` / `parseUint64`**：
   Go `pkg/bbgo/fixedpoint` 与指针辅助层的专有符号。Rust 用
   `Option<f64>` / `Option<String>` 直接表达同一语义，不存在同形函数；
   `brokerOrderStatusFilterValues` 的空输入与去重语义改由
   `TradeRequest::status_codes` 承担并已断言。

### 补齐的行为证据

- **`:14 TestTradeReadConversionBoundaries`**：Rust 的等价物是
  `qualify_symbol` + 已解析请求市场。新测试
  `symbol_qualification_falls_back_to_the_resolved_market_like_go` 覆盖
  「US 前缀限定」「已限定代码不被二次加前缀」「空市场不加前缀」，以及未知
  runtime 市场时沿用账户解析市场（对应 Go
  `resolveBrokerOrderMarket(999, "US.AAPL", "HK") == "US"`）。
  balance/currency 两半另由既有
  `portfolio_cash_balances_fall_back_to_*` 覆盖。
- **`:46 TestTradeReadHelperBoundaries`**：新测试
  `empty_status_filters_produce_no_codes_like_go` 覆盖空参数返回空列表与
  `[" ", "submitted", "SUBMITTED"] → [5]` 去重收敛。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- 去掉 `resolve_account_with_environment` 的候选排序
  → `runtime_account_candidates_are_sorted_before_selection_like_go` 失败：
  `left: "9" / right: "1"`（provider 顺序第一个 REAL 账户被选中）。
  探针已回滚并复跑通过。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --all
python3 scripts/compatibility/audit_test_parity.py
```

结果：1765 passed / 1 skipped；审计 OK: 523 function_exact（本批 4 条 → `[x]`）。

## 批次：pkg/futu/marketdata_reader_boundaries_test.go（4 项，全部 `missing`/`[~]` → `[x]`）

Go 侧 `TestMarketDataReaderSurfacesTransportAndPayloadBoundaries`(17)、
`TestBrokerKLineSessionHelpersNormalizeAndRejectSelections`(108)、
`TestMarketRuleFallbacksExplainTheirSourceAndFailures`(153)、
`TestMarketDataRuleHelpersRejectIncompleteBrokerPayloads`(188)。Rust 没有单一的
"reader" 类型，四个断言组分别落到 live 路由校验、会话选择/标签投影、market-rule
fallback 链和 catalog 前缀归一；映射按 owner 拆分而不是造一个平行 reader。

### 行为映射

- **:17 运输与 payload 边界**：非法 symbol 在 `securities/snapshots/candles/depth`
  四条 live 路由上以 400 `BAD_REQUEST` 失败（新增
  `live_read_routes_reject_malformed_instruments_before_any_provider_access`）；
  缺租约由 `live_read_routes_require_a_logical_subscription_lease` 证明 provider
  调用计数为 0；`QuerySecuritySnapshot`/`QuerySecurityInfo` 的非法 symbol 由
  `snapshot_reader_rejects_invalid_symbols_before_any_opend_call` 与新增的
  `market_rules_reject_invalid_symbols_before_touching_opend` 在 wire 之前拒绝；
  `QueryOrderBook` 由新增
  `microstructure_reader_rejects_invalid_instruments_before_any_opend_call` 覆盖
  （`BAD`、`HK.`、`.00700`、`MARS.AAPL`、`CN.600519` 全部拒绝）。
- **:108 会话选择与标签（含 2 处真实修复）**：
  1. `parse_requested_sessions` 原先保留调用方顺序与重复形态，Go 的
     `resolveBrokerKLineSessions` 收集进 set 后按
     `regular→extended→overnight` 固定顺序输出。改为 `BTreeSet` + 固定顺序表。
  2. `historical_snapshot` 原先用"market 是否支持扩展时段"推导
     `extendedHours`/`session`，与 Go 的
     `hasNonRegularBrokerSession`/`brokerKLineSessionLabel` 不符：Go 看的是
     **已解析的会话选择**。修复后 US intraday 的 `sessions=regular` 返回
     `extendedHours=false`/`session="regular"`，单独的 `extended` 或
     `overnight` 折叠成 `session="all"`。
  证据：`session_selection_normalizes_aliases_and_rejects_unsupported_ones`、
  `broker_kline_snapshot_session_fields_follow_go_classification_helpers`、
  `sessions_default_and_validation_follow_go_extended_hours_rules`。
- **:153 fallback 来源与失败说明**：空 symbols 先拒绝；快照 fallback 成功时单条
  warning 同时含 `QuerySecuritySnapshot fallback` 与主错误文本；主错误 + 空
  fallback 走 `FallbackEmpty`（新增测试断言渲染信息含
  `returned no market rules`）；主/备同时失败走 `FallbackFailed`；两侧都空走
  `NoRules`。
- **:188 不完整 broker payload**：`marketRulesFromSecurityInfo`/
  `marketRulesFromSecuritySnapshot` 丢弃空 symbol、缺 lot、非正 lot 行；
  `canonicalSearchQuoteMarketPrefix` 与 `canonicalSearchQuoteCode` 由 futu 模块内
  新增的两条测试逐项断言（函数私有，测试与 production 同文件）。本批同时清理了
  审计脚本要求的"`[x]` 条目必须引用唯一 Rust 测试"约束：
  `trade_account_test.go:72` 与 `read_account_test.go:37` 原先共用
  `resolve_account_honors_requested_authority_and_falls_back_like_go`，现拆出
  `resolve_trade_market_covers_requested_and_fallback_branches_like_go`
  （authority 首项为 Unknown(0)、无 authority 默认 HK、非法市场硬报错三条分支）。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- `historical_snapshot` 的 `extendedHours` 改回"按 market 能力"推导
  → `broker_kline_snapshot_session_fields_follow_go_classification_helpers`
  失败：`left: Bool(true) / right: Bool(false)`。已回滚并复跑通过。
- `parse_market_symbol_path` 去掉 `market.is_empty() || symbol.is_empty()`
  → `live_read_routes_reject_malformed_instruments_before_any_provider_access`
  失败：`/snapshots/BAD` 返回 `Unavailable("no cached snapshot available for BAD.")`
  而不是 400。已回滚并复跑通过。
- `OpenDMarketMicrostructureReader::security` 去掉未知市场分支
  → `microstructure_reader_rejects_invalid_instruments_before_any_opend_call`
  失败：`instrument "BAD" must be rejected`。已回滚并复跑通过。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
python3 scripts/compatibility/audit_test_parity.py
```

结果：1773 passed / 1 skipped（新增 5 条测试，含 1 条审计约束拆分）；审计
OK: 527 function_exact（本批 4 条 `[~]` → `[x]`）。

## 批次：pkg/futu/snapshot_fallback_test.go（4 项）+ snapshot_fallback_parsing_test.go（3 项）

本批把 Go 的延迟 `Qot_StockScreen`（3252）快照回退完整落到 Rust，并把 7 条
`[~]`/missing 映射升级为 `[x]`/`function_exact`（审计 527 → 534）。

| Go 测试 | 映射状态 | Rust 入口 |
| --- | --- | --- |
| `snapshot_fallback_test.go:18` TestStockScreenSnapshotParamsUseStrictDelayedQuoteFields | `[x]` | `snapshot_fallback.rs::tests::snapshot_fallback_params_use_strict_delayed_quote_fields` |
| `snapshot_fallback_test.go:58` TestFutuStockScreenSnapshotFallbackUsesStaticIDsWithoutSubscription | `[x]` | `snapshot_fallback.rs::tests::futu_stock_screen_snapshot_fallback_uses_static_ids_without_subscription` + `tests/snapshot_fallback_protocol.rs::delayed_snapshot_fallback_reads_static_info_and_pages_stock_screen_without_subscribing` |
| `snapshot_fallback_test.go:98` TestStockScreenSnapshotCoordinatorCachesRowsAndNegativeResults | `[x]` | `snapshot_fallback.rs::tests::stock_screen_snapshot_coordinator_caches_rows_and_negative_results` |
| `snapshot_fallback_test.go:153` TestFutuStockScreenSnapshotFallbackReportsScreenErrors | `[x]` | `snapshot_fallback.rs::tests::futu_stock_screen_snapshot_fallback_reports_screen_errors` |
| `snapshot_fallback_parsing_test.go:14` TestStockScreenFallbackWireValueHelpers | `[x]` | `snapshot_fallback.rs::tests::stock_screen_fallback_wire_value_helpers_match_go_coercions` |
| `snapshot_fallback_parsing_test.go:93` TestStockScreenFallbackParsesRowsAndMarketGroups | `[x]` | `snapshot_fallback.rs::tests::stock_screen_fallback_parses_rows_and_market_groups` |
| `snapshot_fallback_parsing_test.go:146` TestStockScreenFallbackCoordinatesCopiesAndErrors | `[x]` | `snapshot_fallback.rs::tests::stock_screen_fallback_cancellation_and_adapter_entry_point` |

### 真实功能缺口与修复

1. **延迟回退 owner 此前完全缺失（P0）**
   - 复现：watchlist / 行情路由在 3203 快照读取失败或未返回某标的时，只能落到
     tick 缓存，无 BasicQot 权益的标的直接报“未返回”；`rg` 确认仓库内既没有
     `Qot_StockScreen` 回退 owner，也没有 `futu:stock-screen-delayed` 生产方。
   - 修复：`crates/jftrade-integration-futu/src/snapshot_fallback.rs` 新增唯一
     owner——`encode_snapshot_page`（严格延迟字段）、`OpenDSnapshotFallbackReader`
     （3202 + 每市场分页）、`project_screen_page_at`（`source`/`session` 注记）、
     `StockScreenSnapshotCoordinator`（15s 正/负 TTL + single-flight）、
     `StockScreenSnapshotFallback`（适配器能力入口）。
   - 回归：上表 6 条 Rust 测试。

2. **引擎没有把延迟回退接入快照投影（P0）**
   - 修复前：`SharedTradeReadRuntime::security_snapshots` 只有 3203 → tick 缓存两段，
     回退能力无处安放。
   - 修复后：新增 `product_trade_runtime_snapshot_fallback.rs`，投影函数保持
     `source=futu:stock-screen-delayed`，并由 `security_snapshots` 在 tick 缓存之前
     消费；`install_security_catalog_readers` 同一 OpenD 会话下安装该 owner，
     `reset` 一并清空。
   - 回归：`product_production_ports_trade_tests.rs::trade_runtime_security_snapshots_uses_the_delayed_fallback_for_unanswered_symbols`。

3. **分页守卫散落且消息硬编码**
   - 修复：抽出 `validate_snapshot_page`，错误信息由
     `STOCK_SCREEN_SNAPSHOT_PAGE_SIZE` 插值，避免出现与常量不一致的文案。

4. **取消语义缺失（P1）**
   - Go 在 `singleflight` 等待上用 `select` 监听 `ctx.Done()`；Rust 原先只能死等。
   - 修复：`StockScreenSnapshotCoordinator::query_with_cancel` 以既有的
     `SecuritySnapshotCancelToken` 做 2ms 轮询等待，取消返回
     `SnapshotFallbackError::Canceled`，且不打扰 leader 的物理读、
     也不让取消者另起一次读取。
   - 回归：`stock_screen_fallback_waiter_can_be_canceled_before_the_read_completes`。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- `security_snapshots` 里把延迟回退结果集换成空 Vec
  → `trade_runtime_security_snapshots_uses_the_delayed_fallback_for_unanswered_symbols`
  失败：`delayed fallback resolves the first instrument: "Futu market-data router is unavailable"`。已回滚并复跑通过。
- `delayed_snapshot_value` 把 `source` 写成 `futu:security-snapshot`
  → 同一测试失败：`left: String("futu:security-snapshot") / right: "futu:stock-screen-delayed"`。已回滚并复跑通过。
- `StockScreenSnapshotCoordinator::store` 只写正结果（去掉负缓存）
  → `stock_screen_snapshot_coordinator_caches_rows_and_negative_results` 失败：
  `second query hit the cache: left 2 / right 1`。已回滚并复跑通过。
- `encode_snapshot_page` 删除 `simpleField=4` 的 watchlist filter
  → `snapshot_fallback_params_use_strict_delayed_quote_fields` 失败：
  `filter_list.len()` 为 1 而非 2。已回滚并复跑通过。

### 边界与有意区分

- **负结果缓存**：Go 的 `cachedStockScreenSnapshot.item` 允许 nil 并缓存 15s，
  Rust 用 `CachedRow.item: Option<..>` 表达同一语义；失败（Err）不缓存。
- **TTL 边界**：Go 的 `expiresAt.After(now)` 使 TTL 边界本身即过期，测试据此断言
  半 TTL 命中、`TTL + 1ns` 过期，未引入包含式边界。
- **市场值空间**：`Qot_StockScreen` 的市场枚举（CN/SH/SZ→3）与 `Qot_Common.QotMarket`
  不同，`screen_market_value` 保持独立映射，未与 `market_code` 合并。
- **无订阅**：Rust 侧不是“断言没有 Qot_Sub 调用”，而是 `SnapshotFallbackFetchPort`
  在类型层不暴露任何订阅入口；`tests/snapshot_fallback_protocol.rs` 的 framed
  server 另外断言请求序列里绝不出现 3001。
- `pkg/futu/live_opend_test.go` 中依赖真实 OpenD 的 `QuerySnapshotFallback` 用例仍需
  `JFTRADE_FUTU_LIVE_TEST=1`，不在本批。

### 验证

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo fmt --all --check
python3 scripts/compatibility/audit_test_parity.py
```

结果：1786 passed / 1 skipped（本批新增 11 条 Rust 测试）；审计
OK: 534 function_exact（本批 7 条 `[~]` → `[x]`）。

## 批次：pkg/futu/opend/advanced_combo_protocol_test.go（4 项）

本批把 Go 高级协议分发器与组合交易客户端响应形状的 4 条 `[~]` 升级为
`[x]`/`function_exact`（审计 534 → 538）。

| Go 测试 | 映射状态 | Rust 入口 |
| --- | --- | --- |
| `:17` TestAdvancedDispatcherKeysValidationSuccessAndFailures | `[x]` | `trade_session_tests.rs::tests::combo_protocol_ids_are_the_go_advanced_dispatcher_ids` |
| `:52` TestCallAdvancedSuccessEnvelopeAndAllErrorBoundaries | `[x]` | `trade_session_tests.rs::tests::combo_trading_client_response_shapes` |
| `:105` TestAdvancedResponseValidationAndPayloadHelpers | `[x]` | `trade_session_tests.rs::tests::advanced_response_validation_helpers_match_go` |
| `:132` TestComboTradingClientResponseShapes | `[x]` | `trade_session_tests.rs::tests::combo_clients_reject_absent_requests_and_unauthenticated_clients` |

### 真实功能缺口与修复（1 处，P1）

1. **组合交易把「空成功」误判为失败，同时可能把拒绝当成空成功**
   - 复现：`Trd_GetComboMaxTrdQtys` 返回 `retType=0` 且没有 `s2c`（或 `s2c`
     里没有 `maxTrdQtys`）时，Rust 的 `decode_response` 走 `MissingS2c` /
     `MissingMaxTradeQuantity` 报错；Go 的 `GetComboMaxTrdQtys` 明确返回零值
     `ComboMaxTrdQtys{}` 作为成功。`Trd_PlaceComboOrder` 同理：Go 在
     `s2c == nil` 时返回空 order id 且不报错。
   - 影响：组合预览在部分行情/券商没有该字段时整单失败，而 Go 只是把资金占用
     显示为空；这是用户可见的行为差异。
   - 修复：`crates/jftrade-integration-futu/src/trade_proto.rs` 为两个组合协议
     各自提供 `decode_response`——零 `retType` 时 `unwrap_or_default()` 得到空
     payload，非零 `retType` 一律映射为带 `retType/errCode/retMsg` 的
     `ResponseError::ReturnCode`；`trade_session.rs` 的
     `read_combo_max_trade_quantity` 同步改为 `unwrap_or_default()`（不再
     `ok_or(MissingMaxTradeQuantity)`）。
   - 边界：**权益协议仍然严格**——`trd_get_max_trd_qtys::decode_response` 对
     缺失 `maxTrdQtys` 继续报 `MissingMaxTradeQuantity`
     （`trade_proto_tests.rs` 既有断言保持不变）。
   - 回归：`combo_trading_client_response_shapes`（空信封成功、填充字段投影、
     拒绝必须报错）、`advanced_response_validation_helpers_match_go`。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- 把 combo max 改回严格 `MissingS2c`
  → `combo_trading_client_response_shapes` 失败：
  `an empty combo max envelope is a success: Response(MissingS2c)`。已回滚并复跑通过。
- 删掉 combo place 的 `ret_type != 0` 检查
  → 同一测试失败于 `combo place rejection`：返回
  `TradePlaceComboOrderResult { order_id_ex: None, .. }` 而不是错误。已回滚并复跑通过。

### 有意区分（未强行统一）

- **分发模型**：Go 用 `AdvancedProtocols` 运行时表 + `map[string]any` +
  protojson；Rust 每个协议一个强类型模块，未知协议/未知字段在类型层不可表达。
  因此 `:17` 的「键有序、未知字段 false、chan 请求报错」映射为
  protocol id + 编码 round-trip 断言，而不是复刻运行时表。
- **请求可空性**：Go 的 `*C2S` 可以是 nil 并在入口报错；Rust 请求是值类型，
  等价 fail-closed 条件是已关闭 session 下调用必须失败且不触网（测试用服务端
  记录的协议列表为空来证明）。
- **错误文本**：Go 的 `CallAdvanced` 错误串形如
  `retType=1 errCode=429 retMsg=limited`；Rust 用结构化
  `ResponseError::ReturnCode { ret_type, err_code, message }`，测试断言结构化
  字段与文本均含 retMsg/errCode。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
python3 scripts/compatibility/audit_test_parity.py
```

结果：1789 passed / 1 skipped（本批新增 4 条 Rust 测试）；审计
OK: 538 function_exact（本批 4 条 `[~]` → `[x]`）。

---

## 批次：pkg/futu/exchange_business_boundary_test.go（4 项）

基线 `go:452dea11`，本批只处理上一批剩余的 4 条（其余 8 条已在前批完成）。

| Go 测试 | 状态 | Rust 证据 |
| :--- | :--- | :--- |
| `:326` TestExchangeInvalidateClientClearsReadyStateAndSubscriptions | `[x]` | `subscriptions_tests.rs::tests::exchange_invalidate_client_clears_ready_state_and_every_subscription_kind` |
| `:433` TestKLineSessionRegistryResolvesExactRecordAndQuoteSamples | `[~]` boundary | `history_session_plan.rs::HistoricalKlineRequestPlan::resolve_market_session` + `session_resolver.rs::QuoteSessionResolver` + `basic_quote_tick.rs::quote_session_label` |
| `:461` TestKLineSessionSamplePruningAndWindowFallback | `[~]` boundary | `basic_quote_tick.rs::quote_session_label` + `session_resolver.rs::QuoteSessionResolver::resolve_quote_session` |
| `:487` TestMergeStaticInfoIntoSecurityDetailsFillsMissingFieldsWithoutClobberingSnapshot | `[~]` boundary | `product_production_ports_market_data_quote.rs::enrich_security_from_snapshot` |

### 本批新增 Rust 测试（1 条）

`crates/jftrade-integration-futu/src/subscriptions_tests.rs`
`exchange_invalidate_client_clears_ready_state_and_every_subscription_kind`：
先用 `reconcile_demand` + `record_subscription_success` 建立 SNAPSHOT/KLINE/ORDER_BOOK
三类活跃物理订阅（并断言 `own_active_count > 0` 作为前置条件），再断言
`OpenDSubscriptionLifecycle::close` 之后：

- `physical_snapshot().entries` 为空，`own_active_count`/`fallback_count`/
  `pending_release_count` 全为 0；
- `active_basic_instruments()` 为空；
- 重复 `close()` 返回 false（幂等，不产生第二个 owner 或二次释放）；
- `reconcile_demand` 返回空（closed lifecycle 不再接受新需求）。

### 无同名 owner 的边界结论（3 项）

这三项都不是“测试没写”，而是 Rust 侧**不存在同名 owner**，因此保留
`evidence_type = boundary`，并在 `rust_entry` 写明真实的等价机制位置：

1. **`:433` kline session registry**：Go 的 `klineSessions` 按 record key 精确命中，
   `marketSessionSamples` 再按 ±interval 窗口取最新，两者都是 `Exchange`
   进程内的 Wails 侧缓存。Rust 没有该缓存：会话在**请求期**由
   `HistoricalKlineRequestPlan::resolve_market_session`（`SESSION_RTH`/
   `SESSION_ETH`/`SESSION_ALL`/`SESSION_OVERNIGHT`）与注入的
   `QuoteSessionResolver`（calendar）判定，K 线/快照投影直接经
   `basic_quote_tick::quote_session_label` 携带会话标签。
2. **`:461` 采样修剪/窗口回落**：Go 的 `pruneMarketSessionSamples`
   （12h TTL / 256 上限 / 丢弃 Unknown）与 `resolveSessionFromSamples`
   （±interval 窗口取最新已知会话）维护的是同一份进程内采样缓存。
   Rust 没有采样缓存即无修剪与窗口回落语义；会话边界由 calendar resolver
   在请求期解析，并由 `basic_quote_tick` 的 HK 午休与 US 常规盘边界测试覆盖。
3. **`:487` static info 合并**：Go 的 `mergeStaticInfoIntoSecurityDetails`
   按“只填空、不覆盖非空”把 `GetStaticInfo` 合并进既有 `SecurityDetails`
   （保留 snapshot 的 `Name`/`SecurityType`/`LotSize`/`Option.OptionType`）。
   Rust 的 `SecurityDetails` 由 `security_snapshot_query` 的
   `SecurityDetailsMap` 直接构建，唯一相近的合并逻辑是 engine 的
   `enrich_security_from_snapshot`，它把 snapshot 合并进 securities 视图且按
   字段覆盖，**不保留 snapshot 冻结字段**，语义不同，不引入 Go 的静态信息
   合并模型。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

- 让 `OpenDSubscriptionLifecycle::physical_snapshot` 绕过 `self.closed` 检查
  → `exchange_invalidate_client_clears_ready_state_and_every_subscription_kind`
  失败并打印 3 条残留 active entries。已回滚，测试恢复通过。
- 其余 3 项无生产改动，无需探针。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:zero-go
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
```

结果：1791 passed / 1 skipped（本批新增 1 条 Rust 测试）；审计
OK: 539 function_exact（`:326` 由 `partial` 升级为 `[x]`，另 3 条保留
boundary 并补齐了可解析的真实 Rust 位置）。

---

## 批次：pkg/futu/opend/prediction_push_test.go（2 项）

基线 `go:452dea11`。

| Go 测试 | 状态 | Rust 证据 |
| :--- | :--- | :--- |
| `:14` TestPredictionPushSubscribersDispatchOnlySuccessfulTypedUpdates | `[x]` | `tests/prediction_push_stream.rs::prediction_subscribers_dispatch_only_successful_typed_updates` |
| `:69` TestPredictionPushSubscribersIgnoreNilHandlers | `[x]` | `tests/prediction_push_stream.rs::prediction_registry_registration_after_noop_slots_still_delivers` |

### 本批新增 Rust 测试（2 条，均 `missing` → `[x]`）

1. `prediction_subscribers_dispatch_only_successful_typed_updates`：逐条覆盖 Go 的
   「只有成功的 typed 推送才分发」规则——
   - 畸形 body：`decode_prediction_push` 返回 `Err`（直接调用方必须看到损害），
     且零投递；
   - `retType = -1` 但带 `s2c`：返回 `None`，零投递（证明拒绝判定看的是
     `retType` 而不是 payload 是否存在）；
   - 三个协议各自 `retType = 0` 且列表为空：仍返回已接受的数据类型，
     `rows` 为空；
   - 三个协议各推一条真实数据：3 个 listener 每个都恰好看到 3 行，
     类型与 `instrumentId` 完全正确（合计 9 次回调）。
   `PredictionPushRegistry::dispatch` 先快照 listener 集合再扇出，因此每个
   listener 的观测顺序是稳定的协议顺序，断言按 listener 分组比较。

2. `prediction_registry_registration_after_noop_slots_still_delivers`：覆盖
   Go 的 nil-handler 语义。Rust 的 `PredictionPushListener` 是
   `Arc<dyn Fn + Send + Sync>`，nil handler 在类型层不可表达，因此断言保留
   类型差异下仍成立的不变量：空 registry 是无害 no-op（`dispatch` 不 panic、
   不占 slot）；之后注册的 handler 能收到下一次推送且只看到自己的流与身份；
   移除后再推不投递；重新注册从同一干净状态开始并正常收到推送。

### 有意差异（已在结论中记录）

- **空 `s2c` 的回调次数**：Go 的 `subscribePredictionPush` 把 message 本身交给
  回调，因此 `retType=0` + 空列表也会触发一次回调。Rust listener 收到的是
  **行**，空列表没有行可播，所以「接受」由 decode 层断言（返回数据类型 +
  空 rows），「扇出次数」由非空推送断言。这不是缺口：Rust 契约里 listener
  语义单位是行。
- **nil handler**：Go 用 nil 闭包表达「什么都不装」；Rust 用一个空 registry
  表达同一状态（没有可用 slot）。两者都保证后续真实注册不受影响。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

1. 把 `qot_get_event_contract_order_book::Response::accept` 的
   `(self.ret_type == 0).then_some(self.s2c).flatten()` 改成直接 `self.s2c`
   → `prediction_subscribers_dispatch_only_successful_typed_updates` 失败：
   `a rejected push must not be an accepted update`，
   `left: Some((OrderBook, [PredictionPushRow { instrument_id: "US.EC.REJECTED", ... }]))`
   而 `right: None`。已回滚。
2. 把 `PredictionPushUnsubscribe::drop` 里 `.remove(&self.id)` 注释掉
   → `prediction_registry_registration_after_noop_slots_still_delivers` 失败：
   `listener_count` left 1 / right 0（已移除的 handler 仍留在 registry）。
   已回滚。

回滚后 `git diff --stat` 对生产文件为零改动。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked --no-fail-fast
cargo clippy -p jftrade-integration-futu -p jftrade-engine --all-targets --locked
pnpm run check:zero-go
pnpm run check:rust:architecture
python3 scripts/compatibility/audit_test_parity.py
pnpm run check:quick
```

结果见下方「批次验证结果」小节；本批无生产代码改动。

---

## 批次：internal/exchangecalendar/manager_runtime_test.go（4 项）

基线 `go:452dea11`。本批是 calendar 领域的第一个子批，按文件切分。

| Go 测试 | 状态 | Rust 证据 |
| :--- | :--- | :--- |
| `:17` TestManagerBackgroundRefreshFollowsSettingsReload | `[x]` | `tests/manager_lifecycle.rs::background_refresh_follows_an_auto_refresh_settings_reload` |
| `:86` TestManagerRefreshKeepsValidSnapshotWhenPersistenceFails | `[x]` | `tests/manager_lifecycle.rs::persistence_failure_keeps_the_fetched_snapshot_served_from_memory` |
| `:125` TestManagerRestoreReportsMalformedCachedSnapshot | `[x]` | `tests/manager_lifecycle.rs::restore_reports_malformed_cached_snapshot_with_its_path` |
| `:142` TestManagerStatusReportsManualAndRemoteOverrideModes | `[x]` | `tests/manager_lifecycle.rs::status_reports_manual_and_remote_override_modes_distinctly` |

### 真实功能缺口与修复（2 处，P0/P1）

1. **落盘失败会丢弃刚抓到的合法快照（P0，恢复/回滚 + 唯一写入所有权）**
   `crates/jftrade-calendar/src/manager.rs`

   Go 的 `refresh`（`internal/exchangecalendar/manager_refresh.go`）顺序是
   `m.cacheSnapshot(snapshot)` → `m.store.SaveSnapshot(snapshot)`：持久化失败只
   计 `failures++` 并把错误记到 source 上，**内存快照继续服务当天**。
   Rust 的 `refresh_market` 是 `persistence.save()` 失败就 `continue`，于是
   durable store 短暂不可用时，一份完全合法的远端日历被整份丢弃、当天直接
   回退 `builtin_rules`。

   修复：先求 `persistence_error`，无条件 `cache_snapshot`，再按结果记
   `record_failure` 或 `record_success + updated++`。

   复现条件：store 拒绝写入 + provider 返回合法快照。
   预期：`updated=0 / failures=1`，但 `schedule()` 仍返回该远端快照。
   回归测试：`persistence_failure_keeps_the_fetched_snapshot_served_from_memory`。

2. **恢复损坏缓存时丢失出错路径（P1，可观测性）**
   `crates/jftrade-calendar/src/manager.rs::restore_snapshots`

   Go 的 store 用 `fmt.Errorf("decode %s: %w", path, err)` 包装读/解码失败，
   操作员能直接定位坏文件；Rust 的 `CalendarSnapshotStore` 已经产出带
   `path` 的 `CalendarSnapshotLoadError`，但 manager 只把 `message` 记进
   `last_error`，路径被丢掉。

   修复：`record_failure(BUILTIN_SOURCE_ID, format!("{}: {}", error.path.display(), error.message))`。

   回归测试：`restore_reports_malformed_cached_snapshot_with_its_path`。

### 其余两项（无生产改动）

- `:17`：Go 的「禁用期不抓取 + reload 立即 warmup + close 只取消一次」在 Rust
  由 `reload_settings` → `ManagerCommand::Reload` → 后台循环承载，测试用
  `fetch_count`、`wait_for_fetch` 与 `close:<id>` 事件计数逐条断言。
- `:142`：`manual_override` 与 `remote_override` 的分流在 Rust 已由
  `manager_projection::market_status` 实现，测试补上两种模式在同一交易日上
  必须可区分的断言（含各自的 `effectiveReason` 文案）。
  注意 fixture 细节：`Status()` 描述的是**当前**交易日，因此手工 override 的
  date、远端日程的时间戳都要落在 clock 当天；US 的本地日边界使
  `2026-07-02T16:00:00Z` 才是当天，UTC 午夜会落到前一天。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

1. 让 cache 只在落盘成功时发生 → `persistence_failure_keeps_the_fetched_snapshot_served_from_memory`
   失败：`left: "restored" / right: "remote emergency closure"`。已回滚。
2. 只记 `error.message` 不记 path → `restore_reports_malformed_cached_snapshot_with_its_path`
   失败：`last_error = "EOF while parsing a value at line 1 column 12"`
   （不含 broken.json）。已回滚。
3. 把 `remote_override` 分支改成 `remote_covered_day` →
   `status_reports_manual_and_remote_override_modes_distinctly` 失败。已回滚。
4. 让 `reload_settings` 不再向后台循环发命令 →
   `background_refresh_follows_an_auto_refresh_settings_reload` 失败：
   `fixture source was not fetched`。已回滚。

回滚后 `crates/jftrade-calendar` 41/41 通过。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar --all-targets
python3 scripts/compatibility/audit_test_parity.py
```

审计：539 → 545 function_exact。

---

## 批次：internal/exchangecalendar/manager_test.go 第一批（8 项）

基线 `go:452dea11`。`manager_test.go` 有 20 条测试，按主题拆成三批；本批是
「生命周期/策略/状态与覆盖」这一组中不与既有 Rust 测试重复的部分。

| Go 测试 | 状态 | Rust 证据 |
| :--- | :--- | :--- |
| `:33` TestManagerFailureBackoffUsesHoursAndCapsAtTwentyFour | `[x]` | `failure_backoff_contracts.rs::source_failure_retry_delay_starts_at_one_hour_and_caps_at_one_day` |
| `:50` TestDefaultWarmupRefreshTimeoutCoversSequentialRemoteSources | `[x]` | `manager_lifecycle.rs::probe_budget_bounds_one_provider_call_without_hanging_the_manager` |
| `:64` TestManagerFallsBackToBuiltinWhenOfficialRefreshFails | `[x]` | `manager_lifecycle.rs::failing_provider_falls_back_to_builtin_and_keeps_the_error_visible` |
| `:119` TestManagerStatusIncludesSnapshotSummariesAndSampleSchedules | `[x]` | `manager_lifecycle.rs::status_summaries_expose_snapshot_metadata_and_non_open_samples` |
| `:207` TestManagerManualOverridesBeatRemoteAndBuiltin | `[x]` | `manager_lifecycle.rs::manual_override_reopens_a_day_instead_of_the_builtin_closure` |
| `:235` TestManagerSharedMainlandSourceAppliesToSHAndSZ | `[x]` | `manager_lifecycle.rs::shared_mainland_snapshot_applies_to_shanghai_and_shenzhen` |
| `:291` TestManagerIgnoresStaleRemoteSnapshots | `[x]` | `manager_lifecycle.rs::stale_remote_snapshot_is_ignored_in_favour_of_builtin_rules` |
| `:346` TestManagerDiscardInvalidCachedSnapshotOnRestore | `[x]` | `manager_lifecycle.rs::invalid_cached_snapshot_is_discarded_deleted_and_replaced_by_builtin_rules` |

### 新增 Rust 测试（6 条；另 2 条复用既有测试）

`crates/jftrade-calendar/tests/manager_lifecycle.rs` 新增 6 条，覆盖失败回退、
状态摘要与样例日程、手工 override 优先、沪/深共享内地快照、过期快照忽略、
以及损坏缓存丢弃；`crates/jftrade-calendar/tests/failure_backoff_contracts.rs`
与 `manager_test.go:33`、`:50` 分别复用/改写既有证据。

fixture 侧新增 `FixtureSource::descriptor_markets`，使 fixture 可以声明
`CN/SH/SZ` 覆盖（此前只能声明 `US`）；同时把内部构造函数 `manager(..)`
改名为 `build_manager(..)`，避免与测试内局部变量 `manager` 同名遮蔽。

### 无生产改动的两个关键结论（已写入 conclusion）

1. **`:33` — Go 的 `NextRefreshAt` 只是状态展示，不是抓取门。**
   `git grep NextRefreshAt` 显示 Go 只在 `manager_alert.go` 写入、在
   `manager_status.go` 输出，`manager_refresh.go` 的 `refresh` 从不读它；
   而 Rust 的 `refresh_market` 会先 `in_backoff()` 命中就
   `skipped_backoff++` 并跳过抓取。这是 **Rust 侧附加的节流**，不改变公开
   状态字段语义（`nextRefreshAt` 的计算与 Go 一致：1h 起步、每次 +1h、上限
   24h），因此本项按「状态语义等价 + 记录差异」标记 `[x]`，差异已写入映射
   结论与 automation。

2. **`:50` — Rust 没有任何远端 calendar provider。**
   `impl CalendarSourcePort` 在 `crates/` 里只出现在 3 个测试文件与
   `product_tests.rs` 的 fixture 中；生产组合
   （`product_production_ports.rs:574`）用 `CalendarSourceRegistry::default()`
   注册零个 source，`jftrade-calendar` 也没有 reqwest/ureq 依赖。因此
   Go 的 `defaultWarmupRefreshTimeout >= 3 * defaultHTTPTimeout` 在 Rust
   没有可对照的常量；测试改为断言等价的可保证性质（单次 provider 调用受
   probe 预算约束、不会挂死 warmup）。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

1. `take(8)` → `take(99)`（sampleSchedules 上限）：
   `status_summaries_expose_snapshot_metadata_and_non_open_samples` 失败
   （`left: 10 / right: 8`）。已回滚。
2. 注释 `snapshot_fresh` 的 `validUntil` 判断：
   `stale_remote_snapshot_is_ignored_in_favour_of_builtin_rules` 失败
   （`left: "nyse_official" / right: "builtin_rules"`）。已回滚。
   ——这条探针第一次没有失败，因为原 fixture 同时触及 `staleAfterHours`；
   补了 `staleAfterHours = 0` 的第二段把 `validUntil` 规则单独隔离后，
   探针才真正被守卫。
3. 跳过恢复期的持久层删除：`invalid_cached_snapshot_is_discarded_deleted_...`
   失败（文件仍在磁盘上）。已回滚。

回滚后 `crates/jftrade-calendar` 48/48 通过，`git diff` 对 `src/` 无改动。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar --all-targets
python3 scripts/compatibility/audit_test_parity.py
```

审计：545 → 553 function_exact。

---

## 批次：internal/exchangecalendar/manager_test.go 第二批（12 项）

基线 `go:452dea11`。本批覆盖探针/告警、注册表顺序、状态文案与时钟归一。

| Go 测试 | 状态 | Rust 证据 |
| :--- | :--- | :--- |
| `:393` TestManagerProbeMarksHealthySources | `[x]` | `manager_lifecycle.rs::probe_marks_a_productive_provider_healthy_with_its_market_and_count` |
| `:444` TestManagerProbeMarksEmptyParsesUnhealthy | `[x]` | `manager_lifecycle.rs::probe_treats_an_empty_parse_as_structure_changed_unhealthy` |
| `:488` TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts | `[x]` | `manager_lifecycle.rs::refresh_treats_an_empty_parse_as_structure_changed_failure` |
| `:543` TestManagerSourceAlertsDeduplicateAndRecover | `[x]` | `manager_lifecycle.rs::source_alerts_deduplicate_repeats_and_record_recovery` |
| `:613` TestManagerSourceAlertsDeduplicateNetworkTimeoutVariants | `[x]` | `manager_lifecycle.rs::network_timeout_variants_share_one_alert_fingerprint` |
| `:670` TestManagerProbeRecoveryClearsCurrentFetchError | `[x]` | `manager_lifecycle.rs::successful_probe_recovery_clears_the_recorded_fetch_failure` |
| `:736` TestSourceRegistryHonorsPreferredSourceOrder | `[x]` | `manager_lifecycle.rs::source_registry_honours_preferred_order_over_registration_order` |
| `:751` TestManagerSourcesExposeAvailabilityNotes | `[x]` | `manager_lifecycle.rs::every_source_row_exposes_an_availability_note` |
| `:779` TestManagerStatusExplainsBuiltinEffectiveReason | `[x]` | `manager_lifecycle.rs::status_explains_why_builtin_rules_serve_a_market` |
| `:811` TestManagerStatusUsesRemoteCoverageSourceForRegularDay | `[x]` | `manager_lifecycle.rs::status_credits_the_covering_provider_on_a_regular_day` |
| `:917` TestManagerCurrentTimeNormalizesInjectedClockToUTC | `[x]` | `manager_lifecycle.rs::injected_clock_is_normalized_to_utc_for_projected_timestamps` |

（`:890` TestSnapshotCacheIndexesEveryCoveredMarketYear 与已映射的
`:876` 同属 fetch-window 批次，见 `fetch_window_timezone.rs`。）

### 真实功能缺口与修复（2 处，P1）

1. **refresh 路径完全不写健康/告警状态（P1，可观测性 + 恢复语义）**
   `crates/jftrade-calendar/src/manager.rs`

   Go 有两条不同的失败记录路径：
   `recordOperationFailure`（store/恢复类）只写 `LastError` + 退避梯度，
   **不碰** `HealthState`；`recordSourceFailure`（provider 抓取/解析）额外
   置 `HealthState=unhealthy` 并按 `sourceAlertFingerprint` 去重告警。
   Rust 只有单一的 `record_failure`，而且只写 `last_error` + 退避——
   `refresh` 路径**从不**写 `health_state`/`health_fingerprint`/`last_alert_*`。

   修复：按 Go 拆成 `record_operation_failure` 与 `record_source_failure`，
   共用内部的 `record_failure_state`；新增 `source_alert_fingerprint`
   helper（`structure_changed` → 固定详情；`context canceled` /
   `context deadline exceeded` / `client.timeout exceeded` 归并为
   `network_timeout_or_cancelled`；空消息 → `unknown_error`）。

   复现条件：provider 返回合法但零日程的快照。
   预期：failures=1、provider `unhealthy`、
   `health_fingerprint = <id>|<market>|structure_changed|structure_changed`、
   `last_alert_status = triggered`、快照不入缓存、当天回退 builtin。
   回归测试：`refresh_treats_an_empty_parse_as_structure_changed_failure`、
   `source_alerts_deduplicate_repeats_and_record_recovery`、
   `network_timeout_variants_share_one_alert_fingerprint`。

2. **注入时钟未按 UTC 归一（P1，时区）**
   `crates/jftrade-calendar/src/manager.rs::ManagerInner::now`

   Go 的 `currentTime()` 一律 `.UTC()`，所以 UTC+8 的 09:30 被投影成 01:30Z。
   Rust 原先直接返回时钟值，注入带偏移的时钟会让 `checkedAt`/`lastFailureAt`
   等投影带上 `+08:00`，与 Go 的 wire 不一致（也让持久化时间戳依赖宿主偏移）。
   修复：`now()` 返回 `(self.clock)().to_offset(UtcOffset::UTC)`。

   回归测试：`injected_clock_is_normalized_to_utc_for_projected_timestamps`。

### 有意边界（1 处，已写入 conclusion）

- `:751` availability notes：Go 的 `NewManager` 会
  `DefaultRegistry(nil)`，因此 `Sources()` 把四个官方 HTTP provider 与
  builtin/manual 并列展示。Rust 没有 calendar HTTP adapter，生产注册空
  registry，且**注入 registry 会替换 curated 集合**——这正是冻结 fixture
  `calendar-status.json` 锁定的行为（只有 builtin/fixture/manual 三行）。
  因此测试分两半：manager 实际列出的每行都必须有非空 note；
  `default_source_descriptors` + `source_availability_note` 必须仍保留四个
  官方 provider 的 curated 描述与说明。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

1. refresh 的 `kind` 固定成 `fetch_failed` → 结构变更指纹断言失败
   （`...|fetch_failed|no schedules parsed` vs
   `...|structure_changed|structure_changed`）。已回滚。
2. 超时归并条件改成永不匹配 → 指纹退化成原始错误文本，失败。已回滚。
3. `now()` 去掉 UTC 归一 → `checkedAt` 变成 `09:30+08:00`，失败。已回滚。

`RecordingOrderSource` fixture 新增（记录 fetch 顺序），用于黑盒断言注册表
偏好顺序——`ordered_source_ids` 是私有方法，早期直接调用的写法编译失败。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar --all-targets
python3 scripts/compatibility/audit_test_parity.py
```

审计：553 → 564 function_exact；`jftrade-calendar` 59/59 通过。

## 批次：internal/exchangecalendar 收尾（5 项）

基线 `go:452dea11`。本批收掉 `internal/exchangecalendar` 剩余的 5 个
`[~]` 行，全部判定为 `function_exact`（无生产改动，测试侧只做两处加固）。

| Go 测试 | 状态 | Rust 测试 |
| --- | --- | --- |
| `manager_test.go:890` TestSnapshotCacheIndexesEveryCoveredMarketYear | `[x]` | `manager_lifecycle.rs::cross_year_snapshot_is_cached_for_every_covered_year_and_summarised_once` |
| `manager_probe_test.go:14` TestManagerProbeMarketWarmupAndSnapshotOrdering | `[x]` | `manager_lifecycle.rs::probe_targets_one_market_while_warmup_follows_settings` |
| `source_json_test.go:10` TestSourceStatusJSONOmitsZeroTimes | `[x]` | `sources.rs::tests::nonzero_time_fields_use_wire_rfc3339_and_zero_status_omits_them` |
| `source_health_status_test.go:17` TestRefreshAndProbeKeepPerSourceHealthTruthful | `[x]` | `manager_lifecycle.rs::refresh_and_probe_keep_per_source_health_truthful` |
| `source_health_status_test.go:98` TestStatusDistinguishesRemoteCoverageFromRemoteOverride | `[x]` | `manager_lifecycle.rs::fresh_snapshot_without_a_special_day_is_coverage_not_override` |

### 本批新增/加固的测试

前三条 Go 行为在上一批已由新写的 Rust 测试承载（`manager_lifecycle.rs`
本轮 +484 行、3 个新 fixture），本批补齐映射并修正细节：

1. `cross_year_snapshot_*`：Go 只断言“第二年可命中 + 汇总为 1 条”，Rust
   额外断言第一年（2026-06-19）也命中、`checksum` 一致、
   `schedules_parsed=2`，把跨年索引做成了双向证据。
2. `probe_targets_one_market_*`：新增 `MarketRecordingSource`，用真实的
   `probe_market` / `refresh_all` / `refresh_market` 调用黑盒断言
   `fetch_log == ["US", "HK", "US"]`，并断言快照行按 market-local 排序键
   （HK 先于 US）而非插入顺序。两条 fixture 规则写入 conclusion：
   定向 probe 永不写缓存快照；US 本地日界是 `2026-07-02T16:00:00Z`
   而不是 UTC 午夜。
3. `nonzero_time_fields_*`（测试加固）：Go fixture 是
   `SourceStatus{SourceID: "nyse_official", Enabled: true}` 后断言 8 个零值
   时间字段全部不出现；Rust 原先构造的是 `enabled: false`，现在同样置
   `enabled: true`（并断言 `zero["enabled"] == true`），使“启用状态下零值时间
   仍然省略”这一点被真正锁住。

### 有意差异（已写入 conclusion）

- `:17`：Rust 在重试窗口内跳过 fetch（`skipped_backoff`），Go 的
  `NextRefreshAt` 只用于展示、`refresh` 从不读取它。这是 Rust 侧刻意的额外
  节流，不是回归；测试按 Rust 语义断言 provider 级健康真相（1 healthy /
  2 unhealthy、unknown market 为 no-op）。

### 探针（改坏实现 → 跑测试 → 确认守卫 → 回滚）

本批为验证 `:98` 的 `remote_covered_day` 分支，曾把
`manager_projection.rs` 的覆盖分支临时改成 `_ if false && covered.is_some()`，
确认断言从 `effectiveMode="remote_covered_day"` 变成
`effectiveSource="builtin_rules"` 后失败，随后 `git checkout --` 完整还原
（`git status` 确认无残留）。生产代码本批零改动。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar --all-targets --locked --no-fail-fast
python3 scripts/compatibility/audit_test_parity.py
```

审计：564 → 569 function_exact（4451 条映射、569 个 `[x]` 且
`rust_entry` 全局唯一）；`jftrade-calendar` 63/63 通过。

`internal/exchangecalendar` 至此全部收口（72 条映射中已无该目录 `[~]` 行）。

## 批次：internal/exchangecalendar/manager_boundaries_test.go（9 项）

基线 `go:452dea11`。本批 9 条：7 条 `[x]`（其中 1 条含真实修复）+ 2 条
boundary。新增 `crates/jftrade-calendar/tests/manager_boundaries.rs`（8 个测试）
与 `manager_calendar.rs` 的 `mod tests`（3 个 owner 级单元测试）。

| Go 测试 | 状态 | Rust 测试 |
| --- | --- | --- |
| `:14` TestManagerLifecycleAndTemplateBoundaries | `[x]` | `manager_boundaries.rs::manager_lifecycle_and_market_lookup_boundaries_match_go` |
| `:50` TestManualOverrideStatusAndSessionBoundaries | `[x]` | `manager_boundaries.rs::manual_override_status_and_session_window_boundaries_match_go` |
| `:119` TestHTTPCalendarSourceValidateSnapshotBoundary | `boundary` | `manager_calendar.rs::tests::validate_snapshot_rejects_the_go_boundary_table` |
| `:146` TestCalendarSourceAvailabilityNotesAndRefreshTargets | `[x]` | `manager_boundaries.rs::source_availability_notes_and_refresh_target_folding_match_go` |
| `:172` TestManagerValidateCachedSnapshotRejectsCorruptSnapshots | `[x]` | `manager_calendar.rs::tests::validate_snapshot_rejects_the_go_boundary_table` + `manager_boundaries.rs::corrupt_cached_snapshots_are_rejected_with_the_go_conditions` |
| `:216` TestManagerValidateCachedSnapshotUsesRegisteredSourceValidator | `[x]` | `manager_boundaries.rs::registered_provider_snapshots_pass_domain_validation_before_caching` |
| `:248` TestManagerCachedSnapshotMainlandFallbackAndFreshnessBoundaries | `[x]` | `manager_boundaries.rs::cached_mainland_snapshot_fallback_and_freshness_boundaries_match_go` |
| `:287` TestSourceRegistryNilDuplicateAndMarketNormalizationBoundaries | `[x]` | `manager_boundaries.rs::source_registry_normalization_and_duplicate_boundaries_match_go` |
| `:333` TestExtractNYSEHeaderYearsSkipsMalformedRowsBeforeValidHeader | `boundary` | `manager_boundaries.rs::nyse_header_year_extraction_is_a_retired_go_parser_boundary` |

### 真实功能缺口（1 处）

`:146` 的 `refreshMarketsForTarget` 折叠暴露了 `refresh_market` 与
`probe_market` 的不对称：

- Go 的 `refresh(ctx, targetMarket)` 先用 `refreshMarketsForTarget` 折叠目标
  （`""`/`CN`/`SH`/`SZ`→`CN`，其余归一后原样），返回体里的 `market` 仍是
  `normalizeMarket(targetMarket)`。
- Rust 的 `probe_market` 早就这样折叠（`scope = ["CN"]`），但
  `ManagerInner::refresh_market` 直接用请求市场取 policy 并 fetch。mainland
  provider 声明的是 `CN`，所以 `refresh_market("SH")` 永远匹配不到任何
  source，`updated=0`、`failures=0`，前端“刷新上交所”会静默空转。

修复：`crates/jftrade-calendar/src/manager.rs::ManagerInner::refresh_market`
先归一得 `target`，把 `SH`/`SZ` 折到 `CN` 作为 fetch/policy 的市场，结果体
的 `market` 仍写 `target`。`calendar-control.json` 冻结 fixture 的
`refreshAll`/`refreshUnknown` 未受影响（下一次 nextest 全绿即证）。

探针：把折叠行改回 `let market = target.clone()` 后，
`source_availability_notes_and_refresh_target_folding_match_go` 的 fetch 日志
从 `["CN","CN","CN"]` 变成 `["CN","SH","SZ"]` 并断言失败；已回滚并复跑通过。

### 有意差异（3 处，已写入 conclusion）

1. `:14` 零时间：Go 的 `Schedule("US", time.Time{})` 返回 not-ok；Rust 的
   `WireTimestamp` 没有零值概念，`0001-01-01T00:00:00Z` 会按日历年历返回
   `status=closed/reason=new_years_day`。属类型边界差异，不复制 Go 的哨兵值。
2. `:287` 空白 source id：Go `Register(nil)` 与 `Register(id=" ")` 静默忽略；
   Rust `register` 返回 `InvalidSettings`，调用方必须显式处理，属更严格行为。
3. `:119`/`:333` 两条 boundary：无 adapter 级 `ValidateSnapshot`、无 NYSE HTML
   parser，理由见清单 conclusion。

### 验证

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-calendar --all-targets --locked --no-fail-fast
python3 scripts/compatibility/audit_test_parity.py
```

审计：569 → 576 function_exact；`jftrade-calendar` 75/75 通过。
