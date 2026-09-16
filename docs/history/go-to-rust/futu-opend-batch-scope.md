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
