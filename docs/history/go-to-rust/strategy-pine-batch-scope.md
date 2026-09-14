# Strategy、Pine Runtime 领域对齐批次

本批范围覆盖策略错误分类、broker account 规范化、indicator interval
fallback、catalog activity 空页，以及 catalog runtime 恢复的 2 条生命周期
测试。逐项核对 Go 源码断言与 Rust 函数后，3 条基础 helper/错误测试保留
`function_exact`；2 条 runtime reconciliation 新增真实 Rust 入口但保留
`[~]`/`partial`，因为 Rust 未覆盖 Go 的审计详情、save/changed 计数和二次
调用幂等等断言。

| Go 测试 | Rust 证据 | 状态 |
| --- | --- | --- |
| `internal/strategy/errors_test.go:8` | `jftrade-strategy::model::test_classified_strategy_errors_match_sentinel_kinds` | `[x]`，错误类别与消息均有断言 |
| `internal/strategy/instancebinding/binding_test.go:137` | `jftrade-strategy::model::test_normalize_broker_account_drops_empty_input` | `[x]`，空值与 None 边界有断言 |
| `pkg/strategy/indicatorwarmup/warmup_internal_test.go:142` | `jftrade-strategy::pine::planner::test_resolve_interval_minutes_supports_broker_intervals_and_safe_fallbacks` | `[x]`，broker interval 与 fallback 矩阵一致 |
| `internal/strategy/catalog/activity_degraded_test.go:65` | `jftrade-engine::strategy_runtime_activity::test_catalog_activity_returns_empty_pages_when_activity_store_is_unavailable` | `[x]`，空日志/审计页与 page metadata 一致 |
| `internal/strategy/catalog/runtime_reconciliation_business_test.go:52` | `strategy_runtime_port::restore_invalid_running_binding_marks_instance_failed` | `[~]`/`partial`，缺 stopped 保持、审计详情和错误日志内容 |
| `internal/strategy/catalog/runtime_reconciliation_business_test.go:80` | `strategy_runtime_port::restore_running_instances_ignores_paused_and_stopped_instances` | `[~]`/`partial`，缺 stale running/paused 重置、save 次数和二次幂等 |

验证命令已逐条写入 `manual-test-mappings.json`，使用 `jftrade-strategy` 和
`jftrade-engine` 的 nextest wrapper。后续应在所属 runtime/store 领域先补失败
回归测试，再考虑将两条 partial 升级为 function_exact。
