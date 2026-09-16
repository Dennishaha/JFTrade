# Trading、Broker、Execution 第二批次对齐

本批覆盖 `internal/trading/execution_test.go` 的 P0 子集：执行单归一化
5 项与 pre-trade risk 3 项转为 `[x]`/`function_exact`，并保留仍待处理的
控制面与详情项。

## 已闭环条目

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `:16 TestNormalizeExecutionOrderDefaultsUSLimitOrder` | `product_production_ports_execution_order_validation_tests::test_normalize_execution_order_defaults_us_limit_order` | `[x]`：US 小写归一化、SIMULATE、TIF=DAY、Session=RTH(1)、fillOutsideRTH=false、brokerId 保留。 |
| `:47 TestNormalizeExecutionOrderSupportsExtendedUSLimitSessions` | `...::test_normalize_execution_order_supports_extended_us_limit_sessions` | `[x]`：SELL+LIMIT+ETH → session=2、fillOutsideRTH=true。 |
| `:71 TestNormalizeExecutionOrderSupportsStopAndMarketOrders` | `...::test_normalize_execution_order_supports_stop_and_market_orders` | `[x]`：STOP 保留 stopPrice；HK MARKET 无 session。 |
| `:96 TestNormalizeExecutionOrderRejectsBusinessRuleViolations` | `...::test_normalize_execution_order_rejects_business_rule_violations` | `[x]`：缺 price/缺 stopPrice/非 US session/FOK 全部拒绝，STOP_LIMIT 双价通过。 |
| `:159 TestNormalizeExecutionOrderPreservesBrokerAbstraction` | `...::test_normalize_execution_order_preserves_broker_abstraction` | `[x]`：brokerId=ib 保留。 |
| `:573 TestPreTradeRiskRejectsKillSwitchAndLimits` | `pre_trade_risk_domain_tests::pre_trade_risk_fails_when_kill_switch_active`（配合 quantity/notional 两条） | `[x]`：三类拒绝码与 Go 完全一致。 |
| `:687 TestPreTradeRiskSnapshotUsesNonNilEmptySlices` | `pre_trade_risk_domain_tests::pre_trade_risk_snapshot_uses_empty_vectors_not_null` | `[x]`：默认快照四个集合序列化为 `[]` 而非 `null`。 |
| `:703 TestPreTradeRiskDoesNotLetUnknownNotionalBypassRealTradeControls` | `pre_trade_risk_domain_tests::pre_trade_risk_requires_price_for_notional_limit` | `[x]`：缺 price 且配置 notional 上限时 `RISK_PRICE_UNAVAILABLE`。 |
| `:965 TestNormalizeExecutionOrderUsesEnvFallbackAndSupportsNonLimitUSSessions` | `...::test_normalize_execution_order_uses_env_fallback_and_supports_non_limit_us_sessions` | `[x]`：（既有）env fallback + OVERNIGHT。 |
| `:997 TestNormalizeExecutionOrderRejectsInvalidInstrumentAndUnsupportedOrderType` | `...::test_normalize_execution_order_rejects_invalid_instrument` | `[x]`：（既有）缺 symbol 与 iceberg 拒绝。 |

## 本批发现

- `ParsedOrder.header.trd_market` 使用 OpenD 的 **trade market 枚举**（US=2），
  与读取侧的 quote market 枚举（US=11）不同，测试已按此断言并在注释中固化，
  避免后续把两个枚举混用。
- Go 的 `TimeInForce FOK` 拒绝消息为
  `does not support timeInForce FOK`，Rust 为
  `unsupported timeInForce "FOK"`；语义一致但文案不同，测试断言以
  “同时包含 timeInForce 与 FOK” 表达，保留在 P2 文案差异边界内。

## 剩余执行域缺口（保持 `[~]`）

`:173/:285` facade 注入与业务错误传播、`:327/:342/:367/:397/:426/:470`
下单前置校验与 real-trade fail-closed、`:496` kill switch 等待在途下单、
`:613` amount-mode 限额、`:718` env 不配置风控、`:738/:813/:874/:939`
控制面持久化/回滚/加载失败、`:1014/:1047` 订单详情事件窗口。这些项仍需在
后续批次逐条对照 `product_production_ports_execution_*` 与
`real_trade_control.rs` 的真实断言。
