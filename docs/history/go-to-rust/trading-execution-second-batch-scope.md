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

## 批次 88：`internal/trading` 全量收口

### 范围与分片

`internal/trading/**` 共 16 个测试文件、108 条清单行。本批处理其中 **80 条
`missing`**，其余 28 条是前批已闭环证据。分片方式：P0 39 条（下单/风控/
控制面/组合生命周期）与 P1/P2 41 条（broker 读写边界、订单更新、响应形状），
分别落盘后合并写入 `manual-test-mappings.json`。

| Go 文件 | 本批行数 |
| --- | ---: |
| `execution_test.go` | 17 |
| `order_updates_test.go` | 15 |
| `execution_combo_lifecycle_test.go` | 10 |
| `execution_products_test.go` | 6 |
| `broker_test.go` | 5 |
| `risk_status_broker_boundaries_test.go` | 5 |
| `responses_test.go` | 4 |
| `control_plane_state_audit_test.go` | 3 |
| `service_test.go` | 3 |
| `broker_account_read_failures_test.go` | 2 |
| `order_update_recovery_test.go` | 2 |
| `order_updates_reconnect_test.go` | 2 |
| `ports_test.go` | 2 |
| `risk_shape_boundaries_test.go` | 2 |
| `broker_boundaries_test.go` | 1 |
| `order_updates_concurrency_test.go` | 1 |

### 结果

- 80 条 `missing` 全部给出结论：**2 条 `[x]`/`function_exact` + 78 条 `partial`**。
- `internal/trading` 的 `missing` 归零：108 = 22 `[x]` + 86 `partial`。
- 全局：4451 = function_exact **1211** + partial **2311** + boundary 529 +
  module_only 4 + missing **396**（前批为 1209 / 2233 / 529 / 4 / 476）。
- 唯一引用约束：`execution_test.go:342`、`execution_test.go:470`、
  `broker_test.go:453` 对应的 Rust 测试已被其他 Go 行作为 `function_exact`
  占用，本批不重复占用，保持 `partial` 并在结论中写明引用来源。

### 新增 `[x]` 锚点

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `execution_products_test.go:85 TestSingleNonEventOrderRejectsAmountAndPredictionFields` | `product_production_ports_execution_order_validation_tests::single_equity_rejects_event_only_fields` | `[x]`：单腿股票单在 broker 调用前对 `amount`/`predictionSide`/`quantityMode` 返回 BAD_REQUEST。 |
| `execution_products_test.go:193 TestPreviewHashesBindStableClientOrderID` | `...::single_preview_hash_binds_client_order_id`、`...::combo_preview_hash_binds_client_order_id` | `[x]`：preview hash 绑定 clientOrderId，单腿与组合两条路径一致。 |

两条锚点以 `// Parity: go:452dea11:<file>:<line> <TestName>` 注释固定在
`crates/jftrade-engine/src/product_production_ports_execution_order_validation_tests.rs`，
审计脚本可解析（本批后 `OK: 1211 function_exact mappings cite existing workspace tests`）。

### 保留差异候选（保持 `partial` 的理由）

- **P0 控制面与风控路径**：`execution_test.go:496` kill switch 等待在途 REAL 下单、
  `broker_test.go:680` 隐式 REAL 环境解析后再走风控、`control_plane_state_audit_test.go`
  的审计事件上界与模拟单完整审计路径，Rust 仅覆盖“fail-closed/原子生效”等
  单点断言，缺少组合路径与上界断言。
- **P1 订单更新 worker**：`order_updates_test.go`（15）、
  `order_updates_reconnect_test.go`（2）、`order_update_recovery_test.go`（2）、
  `order_updates_concurrency_test.go`（1）共 20 条描述 Go 侧订单更新 worker 的
  throttle/force、TTL 缓存与防御性拷贝、终态移除、元数据、失效上限、
  fee-id 去重、并发 brokerID 初始化、刷新失败保留推送、订阅等待取消等语义；
  Rust 没有等价的 order-updates worker，相邻行为由 ports/projector/reconciliation
  承担，因此逐条记 `partial` 并保留差异。
- **P1 组合与响应形状**：`execution_combo_lifecycle_test.go`（10）覆盖组合腿
  下单/撤销/终态；`responses_test.go`（4）覆盖响应 JSON 形状与 `lastError`
  为 null 的断言，Rust 侧未逐字段断言。
- **P2 读写边界**：`broker_account_read_failures_test.go`、
  `risk_status_broker_boundaries_test.go`、`risk_shape_boundaries_test.go`、
  `ports_test.go`、`service_test.go`、`broker_test.go` 的分类矩阵与超时分类，
  由多条 Rust 投影测试分散覆盖，未形成与 Go 一一对应的矩阵断言。

验证：`cargo fmt --all -- --check`；`node scripts/quality/cargo-nextest.mjs run -p jftrade-trading -p jftrade-engine --all-targets --locked`（**1793 passed / 0 skipped**）；`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2927 Rust** / **1211 `[x]`**；missing 396、partial 2311、boundary 529、module_only 4；`OK: 1211 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 193、7 条 partial 无解析引用、2 条无断言为前批基线告警；32 组同名 Go 测试）；`pnpm run check:compatibility`（EXIT=0）；`pnpm run check:rust:architecture`；`pnpm run check:rust:workspace`（**3044 passed / 2 skipped**，见下）；`node scripts/check-zero-go.mjs`（2891 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`。

`pnpm run check:quick` **未通过（EXIT=1）**，两项失败均不在本批 diff 范围内，
保留原始证据不记为通过：

1. `pnpm run check:rust:static` 的 `cargo-deny` advisories 失败：
   `RUSTSEC-2026-0285`（rustls 0.23.44 TLS 1.3 握手加密层级校验缺陷，
   修复版本 >=0.23.45）。本批未改 `Cargo.lock`；运行时该依赖由根
   `Cargo.toml` 的 `rustls = "=0.23.44"` 精确锁定，解除需要一次显式的
   依赖升级批次（含 `deny.toml` 许可例外复核），故未在本批擅动。
2. `pnpm run check:rust:workspace` 首次随 `check:quick` 并发负载运行时，
   `product_adk_input_response_parity::an_answered_input_request_can_transition_into_an_approval_wait`
   因 5 秒轮询期限内未到 COMPLETED 失败（`product_adk_input_response_parity_tests.rs:708`）；
   该用例单独重跑与独立重跑 `check:rust:workspace` 均通过（3044 passed / 2 skipped），
   判定为并发饱和下的既有 flaky 时限问题，列入后续清理候选。

## 2026-09-27 P1：Trading/Broker strict evidence C 批

范围：`internal/trading/execution_products_test.go:12`、`internal/trading/execution_test.go:939`、`internal/trading/execution_test.go:965`。

逐条复核 Go 与 Rust 断言并保持三条既有 `function_exact`：衍生品单腿预览锁定 `clientOrderId`/`previewId` 且券商写端口零调用；不可读 real-trade control plane 通过严格 `open` 错误、REAL fail-closed、snapshot 错误可见与 mutation 拒绝；`env=real` 回退 REAL，US MARKET+OVERNIGHT 映射 `session=4` 且非 LIMIT 不设置 `fillOutsideRTH`。Rust control-plane 使用 500 `CONTROL_PLANE_UNAVAILABLE`，与 Go 的 `REAL_TRADE_KILL_SWITCH_ACTIVE` 错误码不同，但失败关闭语义一致。本批无生产代码变更，三条均已有单引用 reuse 与 `Parity:` anchor。

验证：联合定向 nextest 3/3 passed，receipt `sha256:77f918d03712f238ef42d3340f4cc5c73f951a0406e534d74b4c3ad251cfe258`（[receipt](verification-receipts/strict-trading-c-p1-2026-09-27.json)）；`audit_test_parity.py --write-report`、`parity_anchor_reconcile.py`、JSON 校验通过。最新扫描 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；anchor `1792/1746/0/0/46`；strict 仍真实失败 3915 个历史 evidence/receipt gaps。
