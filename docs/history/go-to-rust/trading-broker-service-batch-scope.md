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

## 第一百二十二批（分片三）：隐式 REAL 风控升锚与符号/运行时默认值（2 条）

### 范围与结论

- `internal/trading/broker_test.go:680`：`TestPlaceBrokerOrderCannotBypassRiskWithImplicitRealEnvironment`
  → 升 `[x]`/`function_exact`（新增隐式 REAL + kill switch 的端到端回归）。
- `internal/trading/broker_test.go:735`：`TestNormalizeSymbolsAndRuntimeDefaults`
  → 保留 `[~]`/`partial`（符号前缀与运行时投影已覆盖，默认市场读配置的功能差异已登记）。

### 关键事实（本批 recon 与实测）

- **隐式 REAL 解析在订单解析层**：`product_production_ports_execution_order_parse.rs` 在
  payload 缺少 `tradingEnvironment`/`env` 时取组合根注入的 `default_trading_environment`，
  风控由 `ExecutionRiskCoordinator::execute_with_risk_guard` 执行；既有用例只在
  `REAL_TRADING_DISABLED` 场景断言过隐式 REAL，本批补上 kill switch 场景。
- **默认市场是真实功能差异**：`product_production_ports_trade_requests.rs::market_label()`
  在 query 缺 `market` 时硬编码回退 `HK`，而设置里存在对应字段 `FutuIntegrationConfig.trade_market`
  （默认 `HK`）却未被交易读路径使用；`resolve_trade_request` 改从账户权限列表推导市场。
  Go 的 `WithDefaultMarket` 语义（省略 market → 取配置默认市场，显式 market 优先）在 Rust 未实现。
- **符号前缀与拒绝已覆盖**：`trade_security_parsing_accepts_every_go_prefix_and_rejects_invalid_symbols`
  断言各市场前缀与非法前缀拒绝；裸代码在 `TradeRequest::securities()` 用请求 market 标签前缀化，
  全空输入返回 `query parameter symbol is required`——但“裸代码 + market → MARKET.CODE”缺专门断言。
- **运行时投影 fail-closed 而非空 session**：`broker_runtime_projects_configured_connection_and_live_hub`
  断言配置齐全时的会话字段，`broker_runtime_requires_real_projection_sources` 断言缺投影源时报错；
  Go 在无运行时投影时返回空 session/accounts，属保留差异。

### 新增测试与探针

- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs::implicit_real_environment_hits_the_kill_switch_before_the_broker_is_called`
  （锚 `internal/trading/broker_test.go:680`）：写入 `riskConfig.realTradingEnabled=true` + `killSwitch.id`
  控制面，省略 `tradingEnvironment` 下单后断言 403 `REAL_TRADE_KILL_SWITCH_ACTIVE` 且
  `RecordingTradeWriter` 无下单记录。
- 探针：把默认环境解析短路为 `.or(None)` 后，新用例与既有
  `implicit_real_environment_is_risk_rejected_before_the_broker_is_called` 同时转红
  （订单以 `SIMULATE` 落到 broker）；按字节回滚后 `shasum -a 256` 复核
  `product_production_ports_execution_order_parse.rs` = `26dcca142cf7c2e952257ca6e9e334d19b04feaceed00d58ae43cf13b4f3851d`（与探针前一致）。

### 批次结果

- 5 条全部给出结论：**1 条升 `[x]` + 4 条保留 `partial`（缺口逐条写明，其中 1 条功能差异已给出复现/预期/修复位置/回归要求）**。
- `internal/trading/broker_test.go` 归零待办：7 条 = **3 `[x]` + 4 `partial`**，0 `missing`。
- 全局：4451 = function_exact **1358** + partial **2494** + boundary **595** + module_only 4 + missing 0
  （本批前为 1357 / 2495 / 595 / 4 / 0）。Rust 测试 3098 → **3100**（新增 2 条回归）。
- reconcile：anchors **1363**、已记账 **1307**、unrecorded 0、unknown 55、stale 1
  （stale 为既有 `internal/api/trading/execution_test.go:47`，与本批无关）。

### 后续待办（trading_broker 域剩余 62 条 partial）

按文件收敛顺序：`internal/trading/risk_status_broker_boundaries_test.go` 5、
`internal/trading/execution_products_test.go` 4、`internal/trading/responses_test.go` 4、
`internal/trading/order_updates_test.go` 14、`internal/trading/broker_conformance_test.go` 3、
`internal/trading/control_plane_idempotency_test.go` 3、`internal/trading/control_plane_state_audit_test.go` 3、
`internal/trading/service_test.go` 3、`pkg/broker/research_screen_test.go` 3、
`pkg/broker/product_capability_contracts_test.go` 2 及其余单条文件；
另有本批登记的两个契约缺口（交易读默认市场来自设置、`placedAt` 响应字段）需要独立批次修复。

## 批次门禁记录（第一百二十二批）

| 门禁 | 命令 | 结果 |
| --- | --- | --- |
| 格式 | `cargo fmt --all --check` | 通过 |
| 静态检查 | `cargo clippy -p jftrade-engine -p jftrade-trading -p jftrade-store-sqlite --all-targets --locked` | 通过 |
| 分片定向测试 | 见三片验证记录 | 5 + 1 + 5 条通过 |
| 三 crate 全量 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-trading -p jftrade-store-sqlite --all-targets --locked --no-fail-fast` | 通过：2051 条全绿 |
| workspace 测试 | `pnpm run test:rust` | 通过：3197 条通过、2 条 skipped |
| 兼容 replay | `pnpm run check:compatibility` | 通过（含 desktop runtime 3 profiles） |
| 生成物 | `pnpm run check:generated` | 通过：contracts 校验未改动工作树 |
| Zero-Go | `node scripts/check-zero-go.mjs` | 通过：2940 个跟踪文件、0 个发布产物 |
| AI 上下文 | `pnpm run check:ai-context` | 通过：6 个模块、8 个指令文件 |
| 架构/策略 | `pnpm run check:rust:architecture`、`check:rust:production-policy` | 通过 |
| 目标健康 | `pnpm run check:rust:target-health` | 首次失败（`.rcgu.o` 69642 ≥ 50000）→ 确认无 Cargo 进程后 `pnpm run clean:rust:artifacts`（清理 35.5GiB / 140228 文件）→ 复跑通过 |
| 工作树空白 | `git diff --check` | 通过 |
| 快速门禁 | `pnpm run check:quick` | 通过（退出码 0；本次受影响计划未包含 `check:rust:static`） |
| Rust 全量门禁 | `pnpm run check:rust` | **未通过（既有阻断）**：`check:rust:static` 在 advisories 阶段中止，退出码 1 |

### 既有阻断说明（与本批无关）

`cargo deny check advisories` 仍报 1 条 `error[vulnerability]`（RUSTSEC-2026-0285）
与 8 条 `warning[advisory-not-detected]`（`deny.toml` 陈旧忽略项）；本批未改动依赖或 `deny.toml`。
