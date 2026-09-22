# Trading Broker 域：`internal/trading/risk_status_broker_boundaries_test.go` 5 条 partial 收口（第一百二十三批）

本文件记录 `internal/trading/risk_status_broker_boundaries_test.go` 中 5 条 `partial` 的逐条收口。
该文件是 Go 交易服务的风控与状态边界测试集合：预交易风控决策与硬停匹配（`RequiresApproval`、
`RiskRejectedError`、`symbolMatches`、`hardStopMatches`/`matchHardStop`）、订单状态生命周期映射与
对账推进、broker 身份不匹配在读写入口的传播、低层回退 helper（`firstNonEmpty`、`floatValue`、
`withTimeout`、函数适配器）、以及 broker 解析与预测行情存储注入边界。
Rust 侧对应实现分布在 `crates/jftrade-trading`（`risk.rs` 的 `HardStop::matches_pre_trade` 与
`evaluate_pre_trade_risk`、`model.rs` 的 `canonical_broker_status`/`canonical_stored_status`/
`reconcile_status`）与 `crates/jftrade-engine`（broker/portfolio 读端口
`product_production_ports_trade.rs`、执行写端口与风控协调器、错误 wire 映射）。

分片计划（每片一次提交）：

- 分片一 `:124`：broker 身份不匹配在 14 条读路由与 3 条写路由上的 404 传播；
- 分片二 `:13`、`:82`：硬停匹配矩阵与订单状态生命周期映射；
- 分片三 `:187`、`:228`：低层回退 helper、broker 解析与预测存储边界结论。

## 第一百二十三批（分片一）：broker 身份不匹配的读路由 404 传播（1 条）

### 范围与结论

- `internal/trading/risk_status_broker_boundaries_test.go:124`：`TestBrokerIdentityMismatchPropagatesAcrossReadAndWriteOperations`
  → 保留 `[~]`/`partial`（读写矩阵已收口并落地生产守卫；回退运行时投影与函数适配器仍为缺口）。

### 关键事实（本批 recon 与实测）

- **读端口此前没有 broker 身份守卫**：`ProductionBrokerPort::read` 与 `ProductionPortfolioPort::read`
  在 `TradeRequest::parse` 之后直接使用活动 Futu 会话，请求其它 broker 只会沿路径落到 503
  `BROKER_READ_UNAVAILABLE` / `PORTFOLIO_UNAVAILABLE`，与 Go 的 `ErrBrokerNotFound`（404）不符，
  也与 `docs/history/go-to-rust/verification-matrix/03-routes-and-writerlease.md` 记录的 404 契约不符。
- **错误枚举缺少 NotFound 分支**：`BrokerReadSnapshotError`、`PortfolioSnapshotError` 只有
  `Unavailable`/`Invalid`，wire 层因此无法表达 404 `BROKER_NOT_FOUND`。
- **写路由已有等价守卫**：`ProductionExecutionPort::mutate` 对非 futu 的 broker 直接返回 404
  `BROKER_NOT_FOUND`（第 122 批已加回归 `broker_write_routes_reject_an_inactive_broker_before_the_trade_writer`，
  该引用已被 `internal/trading/broker_test.go:533` 占用，本行不重复占用）。
- **组合根只激活一个交易 broker**：Futu 交易会话与行情 helper provider 相互独立
  （`helper_market_data_provider_keeps_futu_trade_reads_on_the_trade_session`），因此守卫条件是
  “请求 broker 必须是 futu”，而不是“活动 provider 必须是 Futu”。

### 生产修复

- `crates/jftrade-engine/src/product_production_ports_trade.rs`：新增 `ACTIVE_TRADE_BROKER_ID = "futu"`
  与读端口/组合端口守卫，不匹配时在触达 `TradeReadPort` 之前返回 `NotFound`。
- `crates/jftrade-engine/src/product_snapshot_errors.rs`：`BrokerReadSnapshotError`、
  `PortfolioSnapshotError` 增加 `NotFound(String)`。
- `crates/jftrade-engine/src/product_wire_brokers.rs`、`product_api_portfolio.rs`：
  `NotFound` → HTTP 404 `BROKER_NOT_FOUND`。
- `crates/jftrade-engine/src/product_mcp_production_executor_errors.rs`：MCP 工具失败映射补 `NotFound` 分支。

### 新增测试与探针

- 新增 `crates/jftrade-engine/src/product_production_ports_trade_tests.rs::broker_read_routes_reject_a_broker_that_is_not_active`：
  覆盖 12 条 broker 读路由（runtime/funds/positions/orders/fills/cash-flows/order-fees/margin-ratios/
  max-trade-qtys/quote/klines/securities）与 2 条 portfolio 路由（cash-balances/positions），
  断言返回 `NotFound` 且读客户端调用数为 0（用拒绝型客户端 `RefusingTradeRead` 计数）。
- 探针：把读端口守卫短路为 false 后新测试转红
  （`/api/v1/brokers/other/runtime` 返回 `Unavailable("Futu trade runtime projection is unavailable")`），
  按字节回滚并核对 `product_production_ports_trade.rs` shasum
  `2abd28fcf898b854c6d6c4cd96d639ae42e73369cf3f44dc0827d9a5c5e4310d`。

### 分片一验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(broker_read_routes_reject_a_broker_that_is_not_active)'` | 1 条通过 |
| 邻域回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(broker) or test(portfolio)'` | 147 条通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b123_p1.json` | 1 行写入，`[x]` 保持 1358，键集 4451 不变 |

## 第一百二十三批（分片二）：硬停作用域矩阵与订单状态生命周期映射（2 条）

### 范围与结论

- `internal/trading/risk_status_broker_boundaries_test.go:82`：`TestOrderStatusMapsEveryBrokerLifecycleFamily`
  → 升 `[x]`/`function_exact`，新增逐条复刻 Go 断言的 Rust 用例。
- `internal/trading/risk_status_broker_boundaries_test.go:13`：`TestRiskDecisionAndHardStopBoundarySemantics`
  → 保留 `[~]`/`partial`，硬停匹配矩阵与符号匹配语义落地为 Rust 用例，决策结构差异登记为缺口。

### 关键事实（本批 recon）

- **状态映射只有表示差异**：Go `CanonicalBrokerOrderStatus` 对 raw 值做 trim/大写/空格转下划线后匹配
  生命周期族，Rust `model.rs::canonical_broker_status` 的实现逐分支等价（含 `EXPIRED`、
  `SUBMITTED|NEW|ACCEPTED`、`FILLED_PART|PARTIAL_FILLED|PARTIALLY_FILLED` 等），只是返回 `OrderStatus`
  枚举而非大写字符串常量。
- **既有覆盖没有钉住小写/带空格输入**：`canonical_broker_status_covers_futu_lifecycle_families` 使用
  PascalCase/混合大小写样本，`stored_status_and_terminal_classification_match_go_contract` 覆盖存储态与
  终态分类，都没有逐条复刻 Go 的 12 条矩阵与 4 条对账断言。
- **硬停匹配语义分散在私有 helper**：`symbol_matches`/`exact_matches`/`owner_matches` 是模块私有函数，
  集成测试无法直接调用；`HardStop::matches_pre_trade` 是公开组合入口，可在不新增测试专用 API 的前提下
  断言 Go 的每条作用域分支与首条命中语义。
- **决策结构存在真实差异**：Rust `PreTradeRiskDecision` 以 `allowed`/`reason_code`/`reason_message`/
  `matched_hard_stop_id` 表达，没有 Go 的 `decision` 字符串、`RequiresApproval()`、`RiskRejectedError`
  与默认文案；审批语义在 assistant 域（`tool_requires_approval`），交易域不重复实现。

### 新增测试与探针

- 新增 `crates/jftrade-trading/tests/order_risk_compatibility.rs::broker_lifecycle_families_map_from_lowercase_and_spaced_values`：
  逐条复刻 `:82` 的 12 条状态映射 + 2 条存储态 + 3 条对账 + 终态判定。
- 新增 `crates/jftrade-trading/tests/pre_trade_risk_domain_tests.rs::pre_trade_risk_hard_stop_scope_matrix_matches_market_symbol_account_and_broker`：
  覆盖 `:13` 的作用域矩阵（broker/environment/account/market/symbol 命中与不命中、大小写、
  `MARKET.SYMBOL` 双向等价、空 order symbol 不命中、空/通配作用域命中）与 `matchHardStop` 首条命中语义
  （`matched_hard_stop_id = hs-2`）及空策略放行。
- 探针一：删除 `model.rs` 的 `"EXPIRED"` 映射后状态用例转红（`raw status "expired"`：Unknown ≠ Expired），
  按字节回滚并核对 shasum `316a2077e0c8ddaca416da3c1abfc616ea6241699f4e2f15f9404f29c3d6b16d`。
- 探针二：把 `HardStop::matches_pre_trade` 的 `symbol_matches` 短路后硬停用例转红
  （`symbol: Some("MSFT")` 被判为命中），按字节回滚并核对 shasum
  `d2d50143725158ebf0bfba25f3fee0fbb2b99cd3c73b6e2c6e3c2bcd2f4d4129`。

### 分片二验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增回归 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading --all-targets --locked -E 'test(pre_trade_risk_hard_stop_scope_matrix_matches_market_symbol_account_and_broker) or test(broker_lifecycle_families_map_from_lowercase_and_spaced_values)'` | 2 条通过 |
| crate 全量 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading --all-targets --locked` | 84 条通过 |
| clippy | `pnpm run check:clippy` | 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b123_p2.json` | 2 行写入，`[x]` 1358 → 1359 |

## 后续待办

- 分片三：`:187` 低层回退 helper（Rust 无 `firstNonEmpty`/`withTimeout` 同形函数）与
  `:228` broker 解析（`resolveBroker` 必需/可选语义）与预测行情存储注入边界。
- 本批登记的缺口：`/runtime` 无会话回退投影；`RiskRejectedError` 默认文案与 `RequiresApproval` 决策语义；
  硬停 `tradingEnvironment = "*"` 的 Go 字面量比较与 Rust 通配语义差异（Rust 更严格，已有
  `hard_stop_environment_scope_trims_case_and_supports_wildcards` 固定）。
