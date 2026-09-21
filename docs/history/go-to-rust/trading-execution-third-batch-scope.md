# Trading 执行域第三批（第 117 批·分片一）

本文件记录 `internal/trading/execution_test.go` 剩余 `partial` 行的逐条收口。该文件共 17 条
`partial`，按“错误分类 / 校验时序 / 委派参数 / 风控时序 / 详情边界”分片推进；已完成**前三片**
（分片一 `:285`、`:327`；分片二 `:173`、`:342`、`:470`；分片三 `:367`、`:397`、`:426`）
共 8 条 → `function_exact`，新增 8 条 Rust 测试，其余 9 行的分片计划见文末。三批均无生产语义变更，
被探测的生产文件一律按字节回滚。

## 第一百一十七批（分片一）：facade 错误分类与校验时序（2 条）

### 范围与分片

Go 侧 2 条（同文件 17 条中的前两行）：

- P0 `internal/trading/execution_test.go:285`：`TestExecutionOrderServiceFacadeReturnsBusinessErrors`
  （四个入口原样返回注入的 upstream 错误；预览入口的非法 side 是 `RequestError`；普通 upstream
  错误不得被判为请求错误）。
- P0 `internal/trading/execution_test.go:327`：`TestCreateExecutionOrderRejectsInvalidPayloadBeforeBrokerCall`
  （quantity=0 在下单前被拒为请求错误，假 broker 被调用即 `t.Fatal`）。

分类：2 条 `[x]`/`function_exact`（原均为 `partial`）；
全仓 `[x]` 1329 → **1331**、`partial` 2544 → **2542**、`boundary` 574、`module_only` 4、
`missing` 0；Rust 测试 3067 → **3069**。

### 关键事实（本批 recon 与实测）

- **Go 机制**：`newExecutionTestService` 注入 `WithListOrders/WithPlaceOrder/WithCancelOrder/
  WithGetOrderEvents`；`:285` 让四个入口都返回同一个 `upstream` 错误并逐入口 `errors.Is`，
  再用 `PreviewExecutionOrder(Side: "HOLD")` 断言 `IsRequestError`，最后断言
  `IsRequestError(upstream) == false`。`:327` 的假 `placeOrder` 直接 `t.Fatal`，
  因此“broker 未被调用”由测试框架的致命断言表达。
- **Rust owner（写侧）**：`crates/jftrade-engine/src/product_execution_write_port.rs::dispatch_execution_write`
  把端口结果映射成 wire envelope——`Ok` → 成功体、`Unavailable(msg)` →
  503 `EXECUTION_WRITE_UNAVAILABLE`、`Failed{status,code,message}` → **原样**透传该 status/code/
  message（不做请求错误/上游错误的重分类）。
- **Rust owner（校验时序）**：`crates/jftrade-engine/src/product_production_ports_execution_orders_impl.rs::place_order`
  先 `parse_order_with_defaults`（失败 → `Failed{400,"BAD_REQUEST",message}`），**之后**才
  `self.writer()` 解析券商写端口；`product_production_ports_execution_order_parse.rs:254` 的
  `quantity <= 0.0` 即 `"quantity must be positive"`。因此“非法 payload 不触达 broker”在 Rust 的
  等价证据是「broker 侧 `TradeWritePort` 记录为空 + 400 BAD_REQUEST」。
- **Rust owner（读侧）**：`product_api_execution.rs::execution_read_snapshot_failure` 同样保留
  端口 `Failed{code,message}`，`Unavailable` → 503 `EXECUTION_UNAVAILABLE`、`NotFound` → 404。

### 新增测试（均带 `// Parity:` 锚点）

- `crates/jftrade-engine/src/product_execution_write_product_tests.rs::execution_error_envelopes_keep_request_errors_distinct_from_upstream_failures`
  （锚 `:285`）：同一产品实例里注入写端口三类错误与读端口 `Failed{EXECUTION_STORAGE_UNAVAILABLE}`，
  断言 400 `BAD_REQUEST`（请求错误保持 400）、500 `BROKER_REJECTED`（上游码/消息逐字保留）、
  503 `EXECUTION_WRITE_UNAVAILABLE`（fail-closed 不冒充请求错误）、读侧 500
  `EXECUTION_STORAGE_UNAVAILABLE`，并断言写调用序列 `order-place`/`order-preview`/`order-cancel`。
- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs::invalid_order_payload_is_rejected_before_the_broker_is_called`
  （锚 `:327`）：用 `RecordingTradeWriter` 构造生产端口，断言 quantity=0 →
  `Failed{400,BAD_REQUEST}` 且消息含 `quantity must be positive`、side=HOLD → 400 `BAD_REQUEST`，
  最后断言 `writer.placed` 为空（broker 未收到非法单）。

### 探针记录（破 → 红 → 按字节回滚）

- 探针 ①（`:327`）：把 `product_production_ports_execution_order_parse.rs` 的
  `quantity <= 0.0` 改为 `< 0.0`（放行 0）→
  `invalid_order_payload_is_rejected_before_the_broker_is_called` 转红；按字节回滚，回滚后
  shasum `3e1856df3c7e5528222accca3ea8fc6a79a9d87bdd6aea0c0f72a9c958e74dce` 与探测前一致。
- 探针 ②（`:285`）：把 `product_execution_write_port.rs` 的 `Failed` 分支 status 硬编码为 400 →
  `execution_error_envelopes_keep_request_errors_distinct_from_upstream_failures` 转红
  （`left: 400 / right: 500`）；按字节回滚，回滚后 shasum
  `d712b28728338f502bb1f64e7b862326d608fb2d6252592aedb5d06e46736701` 与探测前一致。

### 保留差异

- **分类载体不同**：Go 用 `errors.Is` + `IsRequestError` 在同一 facade 内区分；Rust 用端口错误枚举
  （`Failed{status,code,message}` / `Unavailable`）与 wire envelope 表达，映射点集中在
  `dispatch_execution_write` 与 `execution_read_snapshot_failure`。
- **校验位置不同**：Go 在 facade 归一化阶段校验后委派；Rust 在写端口入口 `place_order` 校验，
  transport 只做 JSON 解析。对外语义相同（400 `BAD_REQUEST` 且不下单），但 Rust 的“未触达 broker”
  是对生产端口的断言，而不是对 handler 的断言。
- **错误对象同一性**：Go 断言的是同一个 error 值（`errors.Is`）；Rust 断言的等价物是 status/code/
  message 三元组逐字一致（错误类型在跨端口时已被序列化为 envelope）。

## 第一百一十七批（分片二）：风控前置与读端口过滤归一（3 条）

### 范围与分片

Go 侧 3 条（同文件 17 条中的 `:173`、`:342`、`:470`）：

- P0 `:173`：`TestExecutionOrderServiceFacadeUsesInjectedStoresAndBrokerCommands`
  （过滤条件归一 REAL/acc-1/US 后委派 store；cancel/events 传订单 ID）。
- P0 `:342`：`TestCreateExecutionOrderRunsPreTradeRiskBeforeBrokerCall`
  （REAL 关闭时风控先拒绝，错误含 `real trading is disabled`，broker 零调用）。
- P0 `:470`：`TestPlaceExecutionOrderFailsClosedWithoutRealRiskGateway`
  （默认 REAL + 无风控网关 → `PRE_TRADE_RISK_UNAVAILABLE` 失败关闭，broker 零调用）。

分类：3 条全部 `[x]`/`function_exact`（原均为 `partial`）。

### 关键事实

- **过滤归一的 owner**：`ProductionExecutionPort::read`（`product_production_ports_execution_orders.rs:355`）
  用 `QueryMap::parse` 解析 `scope/brokerId/tradingEnvironment/accountId/market`，逐字段 `str::trim()`
  后按大小写不敏感（`account_id` 用 `trim() ==`）匹配存储订单，再按 `updated_at/created_at/internal_order_id`
  倒序输出；`/orders/{id}/events` 与 `/orders/{id}` 用 `decode_order_id` 解析 ID。
- **风控前置的 owner**：`ExecutionRiskCoordinator::execute_with_risk_guard`
  （`product_execution_risk_coordinator.rs:166`）持有 `submission_gate`，REAL 单逐次重读控制面文件
  （读取失败 → 500 `CONTROL_PLANE_UNAVAILABLE`），再用 `evaluate_pre_trade_risk` 决策；
  REAL 交易关闭时返回 403 `REAL_TRADING_DISABLED`（消息 `real trading is disabled; enable runtime
  real-trade risk config before placing REAL orders`）。`ProductionExecutionPort::execute_order_under_guard`
  在 `risk_coordinator = None` 且环境为 REAL 时返回 403 `PRE_TRADE_RISK_UNAVAILABLE`
  （`pre-trade risk gateway is unavailable; REAL orders are blocked`）——两者都在券商闭包之前发生。

### 新增测试（均带 `// Parity:` 锚点，均在 `product_production_ports_execution_preview_tests.rs`）

- `execution_read_port_normalizes_filters_and_routes_order_identifiers`（锚 `:173`）：生产端口落两笔
  订单，把其中一笔改写成 REAL/acc-1/US，用带空格与大小写混排的查询串过滤 → 只返回该笔；
  再用该 `internalOrderId` 读 `/orders/{id}/events` → 事件归属同一 ID 且非空。
- `pre_trade_risk_rejection_stops_the_real_order_before_the_broker_is_called`（锚 `:342`）：
  tempdir 控制面（REAL 默认关闭）挂到生产端口，REAL 单 → `Failed{403, REAL_TRADING_DISABLED}`
  且消息含 `real trading is disabled`，`RecordingTradeWriter.placed` 为空。
- `real_order_without_a_risk_gateway_fails_closed_before_the_broker_is_called`（锚 `:470`）：
  未挂风控的端口提交 REAL 单 → `Failed{403, PRE_TRADE_RISK_UNAVAILABLE}`，券商记录为空。

### 探针记录（破 → 红 → 按字节回滚）

- 探针 ①（`:173`）：去掉 `market` 字段的 `str::trim()` → 过滤结果为空、测试转红
  （`filtered orders = {"orders":[]}`）；回滚后 `product_production_ports_execution_orders.rs`
  shasum `eae79748cbde15eeba184fc09026010a2b65e3971cb35b5f0d5b5d5c75d0f01c` 与探测前一致。
- 探针 ②（`:342`）：把 `execute_order_under_guard` 的 `Some(coordinator)` 分支改成直接
  `submit_fn()` → 测试转红（订单越过风控到达 broker）；回滚后
  `product_production_ports_execution_orders_impl.rs` shasum
  `416220828cc78ed0ef133b4babed34b3c8ac7dbd4c95f53dd6107a2716b35bcc` 与探测前一致。
- 探针 ③（`:470`）：删除 `None + Real` 的守卫分支（改为 `None => submit_fn()`）→ 测试转红；
  同一文件按字节回滚，shasum 与探测前一致（同上）。

### 保留差异

- **过滤断言的落点**：Go 断言注入 store 收到的 `listedFilter` 参数；Rust 的 store 查询由读端口
  内部构造，无法从外部观察，故断言改为「归一后的查询命中正确集合 + ID 往返」。
- **风控错误载体**：Go 用 `RiskRejectedError.Decision.ReasonCode`；Rust 用
  `Failed{status,code,message}` 三元组，REAL 关闭与网关缺失分别由 `REAL_TRADING_DISABLED`
  与 `PRE_TRADE_RISK_UNAVAILABLE` 表达。
- **引用唯一约束**：`:342` 原先复用的 `jftrade-trading` 域测试已被 `internal/trading/broker_test.go`
  的 `[x]` 占用，本批为它新增了引擎层端到端测试，避免同一 Rust 测试被两个 `[x]` 引用。

## 第一百一十七批（分片三）：运行期 REAL 放行与隐式环境（3 条）

### 范围与分片

Go 侧 3 条（同文件 `:367`、`:397`、`:426`）：

- P0 `:367`：运行期启用 REAL + `RuntimeMaxOrderNotional=500` → 1×100 的 REAL 单被放行且到达 broker。
- P0 `:397`：REAL 关闭时 SIMULATE 单仍被创建且到达 broker。
- P0 `:426`：默认环境 REAL + 请求省略 `tradingEnvironment` → 隐式解析 REAL → 风控拒绝且订单网关零调用。

分类：3 条全部 `[x]`/`function_exact`（原均为 `partial`）。

### 关键事实

- `ProductionExecutionPort::place_order` 先读取 `default_trading_environment` 再调用
  `parse_order_with_defaults(payload, default_env)`，因此省略 `tradingEnvironment` 的请求会落到
  运行期默认环境（REAL）并进入风险协调器。
- `ExecutionRiskCoordinator::execute_with_risk_guard` 只在 `trading_environment == Real` 时求值
  （SIMULATE 根本不进入 `evaluate_pre_trade_risk`）；REAL 启用与限额来自控制面文件
  `riskConfig.realTradingEnabled` / `maxOrderNotional`，每次提交前重读。
- 风控批准后由 `execute_order_under_guard` 调用券商闭包，因此“到达 broker”可用
  `RecordingTradeWriter.placed` 直接证明。

### 新增测试（均带 `// Parity:` 锚点，均在 `product_production_ports_execution_preview_tests.rs`）

- `runtime_enabled_real_trade_reaches_the_broker_after_risk_approval`（锚 `:367`）：tempdir 控制面写入
  `realTradingEnabled=true` + `maxOrderNotional=500`，REAL 单（100×1）经风控批准后返回
  `internalOrderId` 且 `placed` 恰好 1 条。
- `simulate_order_reaches_the_broker_while_real_trading_is_disabled`（锚 `:397`）：默认（REAL 关闭）
  控制面 + SIMULATE 单 → 返回 `internalOrderId` 且 `placed` 恰好 1 条。
- `implicit_real_environment_is_risk_rejected_before_the_broker_is_called`（锚 `:426`）：默认环境 REAL +
  payload 删除 `tradingEnvironment` → `Failed{403, REAL_TRADING_DISABLED}` 且 `placed` 为空。

### 探针记录（破 → 红 → 按字节回滚）

- 探针 ①（`:367`）：把 coordinator 的决策快照替换为 `Default::default()`（忽略运行期 REAL 启用）→
  测试转红；回滚后 `product_execution_risk_coordinator.rs` shasum
  `8576a2f2ab7457081fd2af92c9b6a6f3992f852811a679936e36a8d3132de0ab` 与探测前一致。
- 探针 ②（`:397`）：把 `execute_order_under_guard` 的 coordinator 分支改为对所有订单返回
  403 `REAL_TRADING_DISABLED`（“REAL 关闭时误伤 SIMULATE”）→ 测试转红；回滚后
  `product_production_ports_execution_orders_impl.rs` shasum
  `416220828cc78ed0ef133b4babed34b3c8ac7dbd4c95f53dd6107a2716b35bcc` 与探测前一致。
  过程记录：先试过关闭 `jftrade-trading/src/risk.rs` 的 SIMULATE allow 分支，探针**未**转红——
  因为引擎路径上 SIMULATE 不进入 `evaluate_pre_trade_risk`；该次探测同样按字节回滚
  （`risk.rs` shasum `2f03d6b4b51181c4740f531208835ac9057ba7829cef286033cf48e7f34fd401` 一致），
  并作为“SIMULATE 不经 REAL 风控求值”的实现证据记录在此。
- 探针 ③（`:426`）：把 `parse_order_with_defaults` 的默认环境参数改为 `None`（丢失隐式 REAL 解析）→
  测试转红（订单以 SIMULATE 到达 broker）；按字节回滚，shasum 与探测前一致（同上）。

### 保留差异

- **放行证据**：Go 断言 facade 的 `Accepted/InternalOrderID`；Rust 断言端口返回体 + 券商记录条数，
  直接覆盖“批准后确实提交”这一环节。
- **控制面来源**：Go 用 `StaticPreTradeRiskGateway` 闭包；Rust 用磁盘控制面文件（每次提交重读），
  因此测试以 JSON 状态文件作为夹具，更接近生产路径。

### 后续待办（本文件剩余 9 条）

- 下批（第 117 批·分片四）：`:496`（kill switch 激活等待在途 REAL 下单的时序）、
  `:738`（kill switch + hard stop 重启往返）、
  `:874`（持久化失败整体回滚：内存与磁盘一致）、`:939`（持久化状态读取失败 fail-closed）。
- 分片五：`:613`（amount/quantity/notional 组合矩阵聚合断言）、`:718`（REAL 相关环境变量不得配置
  预交易风控的负例）、`:813`（校验失败/禁用事件的审计事件流文本）、`:1014`（详情最近事件数量上界）、
  `:1047`（返回前刷新目标订单历史的调用顺序）。
- 该文件清零后，继续 trading_broker 域其余 21 个文件（`order_updates_test.go` 15、
  `execution_combo_lifecycle_test.go` 12、`pkg/broker/broker_test.go` 8 …），随后按
  api_transport / strategy_pine / assistant_workflow / backtest_calendar / storage_sqlite /
  marketdata_quotes / futu_opend / settings_watchlist 顺序逐域收口。
- 既有独立项：docs/history 尖括号占位符导致 `build:docs:generated` 失败；桌面周期性更新检查
  （`startDesktopUpdateChecks` 24h 节奏）。

验证：
- 分片一定向 nextest（2 条新测试）EXIT=0：`invalid_order_payload_is_rejected_before_the_broker_is_called`
  与 `execution_error_envelopes_keep_request_errors_distinct_from_upstream_failures`。
- 分片二定向 nextest（3 条新测试）EXIT=0：`execution_read_port_normalizes_filters_and_routes_order_identifiers`、
  `pre_trade_risk_rejection_stops_the_real_order_before_the_broker_is_called`、
  `real_order_without_a_risk_gateway_fails_closed_before_the_broker_is_called`。
- 两次探针（分片一）与三次探针（分片二）全部转红并按字节回滚，回滚后 shasum 与探测前一致（见上）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3072 Rust、`[x]` 1329 → 1334、
  `partial` 2544 → 2539、`missing` 0、0 破坏引用；未锚定 function_exact 202、partial 无解析引用 7、
  无断言 2（均既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1335 唯一引用（已记账 1280、
  unrecorded 0、unknown 55、stale 0）。
- `cargo fmt --all --check` EXIT=0；`cargo clippy -p jftrade-engine --all-targets --locked` EXIT=0。
- 全量 `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
  --no-fail-fast`：1766 passed / 0 failed / 0 skipped，EXIT=0（含分片一、分片二共 5 条新测试）。
- `pnpm run check:compatibility` EXIT=0；`node scripts/check-zero-go.mjs` EXIT=0（2931 tracked
  files）；`pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick`：首轮在 `check:rust:test` 阶段因**与本批无关的既有抖动**失败——
  `product::product_production_ports_adk::tests::adk_session_detail_omits_resolved_approval_groups`
  （ADK 会话详情，本批未触碰）；隔离重跑 2/2 通过、同轮全量 nextest 亦通过，随后完整重跑
  `check:quick` EXIT=0（含 pineworker 10 files / 98 tests）。
- `pnpm run check:rust` EXIT=1：`:target-health` / `:architecture` / `:production-policy` 通过，
  唯一失败阶段 `check:rust:policy`（`cargo deny`）报 RUSTSEC-2026-0285 与陈旧 advisory 告警，
  与本仓既有基线同源。**不记为通过**。
- 分片三定向 nextest（3 条新测试）EXIT=0：`runtime_enabled_real_trade_reaches_the_broker_after_risk_approval`、
  `simulate_order_reaches_the_broker_while_real_trading_is_disabled`、
  `implicit_real_environment_is_risk_rejected_before_the_broker_is_called`；三次探针全部转红并按字节回滚
  （含一次“探针不转红”的过程记录，见分片三探针小节）。
- 分片三门禁：`cargo fmt --all --check` EXIT=0；`cargo clippy -p jftrade-engine -p jftrade-trading
  --all-targets --locked` EXIT=0；`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine
  -p jftrade-trading --all-targets --locked --no-fail-fast` **1849 passed / 0 failed**；
  `pnpm run check:compatibility` EXIT=0；`node scripts/check-zero-go.mjs` EXIT=0（2931 tracked files）；
  `pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- 分片三门禁续：`pnpm run check:quick` 首轮在 `check:rust:target-health` 报 `.rcgu.o ≥ 50000`
  （多轮 nextest 编译累积），确认无 Cargo 进程后 `pnpm run clean:rust:artifacts`
  （Removed 139409 files / 34.6GiB）并复跑 EXIT=0（含 pineworker 10 files / 98 tests）；
  `pnpm run check:rust` 复跑后 `:target-health`/`:architecture`/`:production-policy` 通过、
  仅 `:policy` 失败（RUSTSEC-2026-0285），**不记为通过**。
- 分片三审计：`python3 scripts/compatibility/audit_test_parity.py` 4451 Go / **3075** Rust、
  `[x]` 1334 → **1337**、`partial` 2539 → **2536**、`missing` 0、0 破坏引用；
  `python3.12 scripts/compatibility/parity_anchor_reconcile.py` **1338** 唯一引用
  （已记账 1283、unrecorded 0、unknown 55、stale 0）。
- `cargo fmt --all --check` EXIT=0（首轮 fmt 报测试文件一处换行，`cargo fmt --all` 后复检通过）；
  `cargo clippy -p jftrade-engine --all-targets --locked` EXIT=0。
- 全量 `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
  --no-fail-fast`：1763 passed / 0 failed / 0 skipped，EXIT=0（含本批 2 条新测试）。
- `pnpm run check:compatibility` EXIT=0（provider-runtime / trading-strategy / assistant-runtime /
  api-transport 278 operations / desktop-runtime 3 profiles 6 link cases 10 commands 4 events）；
  `node scripts/check-zero-go.mjs` EXIT=0（2930 tracked files）；`pnpm run check:ai-context` EXIT=0；
  `git diff --check` 干净；`pnpm run check:quick` EXIT=0（含 pineworker 10 files / 98 tests）。
- `pnpm run check:rust`：首轮 `check:rust:target-health` 因 `.rcgu.o ≥ 50000` 失败（本批多次
  nextest 编译累积），确认无 Cargo 进程后 `pnpm run clean:rust:artifacts`（Removed 110158 files /
  31.9GiB）并复跑——`:target-health` / `:architecture` / `:production-policy` 通过，唯一失败阶段
  `check:rust:policy`（`cargo deny`）报 RUSTSEC-2026-0285 与陈旧 advisory 告警，与本仓既有基线同源。
  **不记为通过**；等价测试面由上面的全量 nextest 与 `check:quick` 覆盖。
