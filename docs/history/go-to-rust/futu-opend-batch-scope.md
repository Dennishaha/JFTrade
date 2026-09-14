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
| `internal/integration/futu/subscription_reconciler_test.go:287:TestSubscriptionReconcilerRetriesFailuresAndCancelsRetryOnReacquire` | retry/取消 | `subscriptions_tests::plain_basic_failure_keeps_retry_semantics_without_fallback_count` | `[~]` partial：普通失败 5s 退避且不计入 fallback 已覆盖；完整退避阶梯、unsubscribe 失败与 reacquire 取消重试断言仍待补 |

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
