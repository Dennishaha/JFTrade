# Trading Broker 域：`internal/trading/broker_test.go` 剩余 5 条 partial 收口（第一百二十二批）

本文件记录 `internal/trading/broker_test.go` 中第 93 批遗留的 5 条 `partial` 的逐条收口。
该文件是 Go 交易服务（`internal/trading`）的 broker 门面测试集合：13 个读操作的路由与投影、
组合视图与降级响应、写操作与超时行为、隐式 REAL 环境的预交易风控、符号与运行时默认值。
Rust 侧对应实现分布在 `crates/jftrade-engine`（broker/组合读端口投影
`product_production_ports_trade_tests.rs`、执行下单端口与风控协调器
`product_production_ports_execution_preview_tests.rs` / `product_execution_risk_coordinator*`、
券商写端口 `product_brokers_write_port.rs`）与 `crates/jftrade-trading`（pre-trade risk 域）。

分片计划（每片一次提交）：

- 分片一 `:186`、`:453`：读端口投影矩阵与组合/降级响应；
- 分片二 `:533`：写操作转发与超时行为；
- 分片三 `:680`、`:735`：隐式 REAL 风控与符号/运行时默认值。

## 第一百二十二批（分片一）：读端口投影矩阵与组合响应（2 条）

### 范围与结论

- `internal/trading/broker_test.go:186`：`TestServiceBrokerReadOperationsMapSnapshotsAndQueries`
  → 保留 `[~]`/`partial`（投影矩阵已覆盖，fills/历史 K 线正向投影与历史订单作用域回填待补）。
- `internal/trading/broker_test.go:453`：`TestServicePortfolioAndFallbackResponses`
  → 保留 `[~]`/`partial`（现金回退链与持仓映射已覆盖，`createdAt` 与超时映射待补，降级形状证据被占用）。

### 关键事实（本批 recon 与实测）

- **读投影矩阵基本齐备**：`product_production_ports_trade_tests.rs` 已逐操作断言
  资金+币种余额、组合持仓、当前/历史订单分流、现金流、订单费用、保证金比率、最大可买、
  报价与证券快照，另有参数校验与失败分级用例；引用清单见 `manual-test-mappings.json`。
- **fills 成功投影是真实缺口**：`fills` 目前只出现在降级形状
  （`test_service_broker_read_operations_return_fallback_when_market_data_unavailable`，已被
  `internal/trading/broker_boundaries_test.go:11` 的 `[x]` 行占用）与执行对账侧的 fill 用例中，
  缺“broker 返回 fills → 响应数组与字段映射”的正向断言。
- **历史 K 线正向投影缺用例**：现有覆盖是无源 fail-closed
  （`broker_klines_valid_request_fails_closed_without_historical_source`）、参数校验与
  `SharedTradeReadRuntime::current_kline` 委派，`/api/v1/brokers/futu/klines?limit=` 的正向路径没有断言。
- **组合现金回退链已断言**：币种行优先（未知币种枚举丢弃）、无币种明细回退市场默认币种；
  唯一断言 `updatedAt` 的用例 `portfolio_cash_balances_fall_back_to_summary_currency_when_breakdown_is_empty`
  同时是 `internal/app/apiserver/servercoretest/portfolio_routes_test.go:12` 的 `[x]` 证据，
  本行按引用唯一性约束不重复占用。
- **超时映射无 Rust owner**：Go 的 `FundsWithTimeout`/`PositionsWithTimeout` 把读端口超时
  渲染成 `LastError` 文本与 `connectivity=disconnected`；Rust 读端口没有 timeout 参数，
  deadline 属 transport 层，需新增用例才能固定该契约。

### 分片一验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b122_p1.json` | 2 行写入，`[x]` 保持 1357，键集 4451 不变 |
| 定向测试 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(broker_read_projects_futu_funds_from_neutral_client) or test(position_projection_prefers_diluted_cost_and_account_pnl_with_legacy_fallback) or test(broker_current_orders_hide_terminal_statuses_while_history_keeps_them) or test(portfolio_cash_balances_prefer_currency_rows_over_summary_fallback) or test(portfolio_cash_balances_fall_back_to_market_currency_when_summary_currency_is_absent)'` | 5 条通过 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过：1357 function_exact 引用均可解析、无重复引用 |
| 锚点对账 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1361、已记账 1305、unrecorded 0、unknown 55、stale 1（既有） |

## 第一百二十二批（分片二）：写操作转发、解锁与超时缺口（1 条）

### 范围与结论

- `internal/trading/broker_test.go:533`：`TestServiceBrokerWriteAndTimeoutBehaviors`
  → 保留 `[~]`/`partial`，并新增一条 404 分支回归把 Go 的 `ErrBrokerNotFound` 对到 Rust 写路由。

### 关键事实（本批 recon 与实测）

- **写操作透传与身份已断言**：`broker_adapter_place_and_cancel_keep_server_order_identity_and_submitted_status`
  断言下单保留内部/券商/扩展订单号与 `SUBMITTED`、`clientOrderId` 作为 OpenD remark 透传、
  撤单恰好一次 `Trd_ModifyOrder`；`accepted_cancel_persists_and_broker_failure_never_advertises_a_cancel`
  断言 broker 失败时不宣告撤单成功。
- **解锁透传已断言**：`broker_unlock_route_forwards_password_md5_and_unlock_flag_to_opend`
  断言 `unlock=true` 与 `passwordMd5` 透传、恰好一次 `Trd_UnlockTrade`，且未配置的 security firm 不被默认；
  解锁成功信封与重试/重启语义由 `brokers_write_product_replays_browser_boundary_failure_recovery_and_restart` 断言。
- **404 分支此前无测试**：`ProductionExecutionPort::mutate` 对非 futu broker 返回
  `404 BROKER_NOT_FOUND`（`product_production_ports_execution_orders.rs:518`），但全仓没有断言该分支的用例，
  属真实覆盖空洞；本批补齐。
- **超时与不支持能力仍是缺口**：Go 用 `FundsWithTimeout`/`PositionsWithTimeout` 把读端口超时渲染成
  `LastError` 文本与 `connectivity=disconnected`，Rust 读端口没有 timeout 参数（deadline 属 transport 层）；
  `ErrTradingUnsupported`/`ErrUnlockUnsupported` 对应的 `NO_TRADING`/`NOT_SUPPORTED` 信封也没有专门用例。
- **`placedAt` 无 Rust 生产字段**：全仓仅冻结契约 `trading.BrokerPlaceOrderResponse`、compatibility fixtures
  与生成类型出现 `placedAt`；route ledger 记录该时间戳为 replay 注入值，生产响应未写该字段。

### 新增测试与探针

- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs::broker_write_routes_reject_an_inactive_broker_before_the_trade_writer`
  （锚 `internal/trading/broker_test.go:533`）：对 `PlaceOrder`/`CancelOrders`/`Unlock` 三个操作断言
  非 futu broker 返回 `404 BROKER_NOT_FOUND`，且 `TradeWritePort` 未被触碰。
- 探针：把 404 守卫短路（`if false && !broker_id.eq_ignore_ascii_case("futu")`）后，place 分支继续解析空 payload，
  新测试转为 `400 BAD_REQUEST` 失败；随后按字节回滚，`shasum -a 256` 复核
  `product_production_ports_execution_orders.rs` = `e01eb7c23b4933860aba277c73f82ae5dcca9058744a0dc707ce1850fc3e6128`（与探针前一致）。

### 分片二验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b122_p2.json` | 1 行写入，`[x]` 保持 1357，键集 4451 不变 |
| 新增测试（绿） | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(broker_write_routes_reject_an_inactive_broker_before_the_trade_writer)'` | 通过 |
| 探针（红 → 回滚 → 绿） | 同上命令（守卫短路后） | 探针红（`400 BAD_REQUEST`），回滚后复跑通过 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过；Trading & Broker Rust 测试 405 → **406**，全仓 Rust 测试 3098 → **3099** |
| 锚点对账 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1362、已记账 1306、unrecorded 0、unknown 55、stale 1（既有） |
