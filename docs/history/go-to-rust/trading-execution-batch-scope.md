# Trading、Broker、Execution 领域对齐批次

本批限定交易安全与执行生命周期中的 8 条 Go 复合键：broker conformance
3 条、control-plane hard-stop/kill-switch 3 条、execution combo 2 条。逐项
读取 Go 测试后，对照 Rust 函数的实际断言；虽然每条都有同名 Rust 入口，
但 Rust 多数只验证状态枚举或内存结构，未达到 Go 的服务调用、审计事件、
错误传播和完整 envelope 语义，因此全部保持 `[~]`/`partial`。

| Go 测试 | Rust 入口 | 主要缺口 |
| --- | --- | --- |
| `broker_conformance_test.go:15` | `order_risk_compatibility::test_fake_broker_conformance_accepted_partial_full_and_out_of_order_updates` | 缺真实创建、成交数量/均价、旧更新与 terminal 回归断言 |
| `broker_conformance_test.go:58` | `...::test_fake_broker_conformance_cancel_accepted_and_cancel_rejected` | 缺 cancel requested、拒绝错误、lastError 和拒绝事件 |
| `broker_conformance_test.go:107` | `...::test_fake_broker_conformance_place_rejected_push_before_query_and_unsupported_capability` | 缺 push-before-query 发现、拒单详情和 capability 错误 |
| `control_plane_idempotency_test.go:12` | `hard_stop_environment_scope::test_real_trade_control_plane_kill_switch_release_is_idempotent_and_audited` | 缺控制面调用与审计事件数量/顺序/ActivatedAt |
| `control_plane_idempotency_test.go:65` | `...::test_real_trade_control_plane_hard_stop_release_is_single_shot` | 缺 not-found 返回、快照和审计不重复 |
| `control_plane_idempotency_test.go:99` | `...::test_real_trade_control_plane_hard_stops_block_until_every_entry_released` | 缺真实下单阻断、拒绝审计及全部释放后放行 |
| `execution_combo_lifecycle_test.go:15` | `order_risk_compatibility::test_execution_combo_complete_preview_place_cancel_and_buying_power` | 仅 terminal 属性，缺 preview/place/cancel/envelope/buying-power |
| `execution_combo_lifecycle_test.go:635` | `...::test_execution_details_resolver_and_order_update_cache_failure_branches` | 仅空 ID 判定，缺 resolver 与 cache failure 错误传播 |

后续实现应先补失败回归测试，再在 `jftrade-trading` 领域修复，不在 API
handler 中复制业务逻辑。每条验证命令已写入 `manual-test-mappings.json`。
