# Trading 执行域第五批：组合订单生命周期 12 条 partial 的逐条收口

本文件记录 `internal/trading/execution_combo_lifecycle_test.go` 全部 12 条 `partial` 的逐条收口。
该文件是 Go 交易服务的组合订单（option_combo / event_parlay）生命周期测试集合：购买力预览、
组合预览与凭证消费、提交前风控重检、组合风险形状矩阵、旧购买力兼容、不安全边界、provider/store/
风控/网关失败、辅助分支、详情解析与订单更新缓存失败。Rust 对应实现分布在
`crates/jftrade-engine` 的生产执行端口（预览/下单/撤单）、`crates/jftrade-store-sqlite` 的
预览凭证与订单身份栅栏、`crates/jftrade-engine` 的 `ExecutionRiskCoordinator`，
以及 `crates/jftrade-trading` 的风控域模型。

分片计划（每片一次提交）：

- 分片一 `:15` 组合完整预览/下单/撤单/购买力、`:83` 下单前风控重检；
- 分片二 `:110` 事件 parlay 完整预览与金额风控、`:150` 事件 parlay 拒绝 caller 控制价、
  `:172` option combo 风险形状校验；
- 分片三 `:205` 旧购买力兼容、`:237` 拒绝全部不安全边界、`:319` provider/store/风控/网关失败；
- 分片四 `:412` 组合辅助分支、`:441` 预览与提交失败契约、`:571` 剩余生命周期与更新辅助、
  `:635` 详情解析与订单更新缓存失败分支。

## 第一百一十九批（分片一）：完整生命周期与提交前风控重检（2 条）

### 范围

- P0 `internal/trading/execution_combo_lifecycle_test.go:15`：
  `TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower`。
- P0 `internal/trading/execution_combo_lifecycle_test.go:83`：
  `TestExecutionOrderRechecksRiskImmediatelyBeforeBrokerSubmission`。

结论：2 条均升 `[x]`/`function_exact`。全仓 `[x]` 1345 → **1347**、`partial` 2513 → **2511**、
`boundary` 589、`module_only` 4、`missing` 0（合计 4451 不变）；Rust 测试 3086 → **3088**。
reconcile anchors 1349 → **1350**（已记账 1293 → 1294、unrecorded 0、unknown 55、stale 1，
stale 为既有 `internal/api/trading/execution_test.go:47` 行，与本批无关）。

### 关键事实（本批 recon 与实测）

- **Go 的 `Operation`（PLACE_COMBO/CANCEL_COMBO）是服务层信封**：Rust 的公开契约由
  `ExecutionWriteOperation` 标签（`combo-place`/`combo-cancel`）与订单投影承载，
  没有 `Operation` 字段；`order_risk_compatibility.rs` 里同名占位测试只比较
  `OrderStatus` 终态，不构成行为证据，本批另行补齐真实链路断言。
- **featureId 由路由承载**：Go 的 `PreviewExecutionBuyingPower` 把
  `broker.FeatureExecutionBuyingPower` 交给 broker 规则提供方；Rust 无 `ProductRuleProvider`
  参数，buying-power 路由本身即 `execution.buying_power` 特性
  （`product_broker_capabilities_projection.rs`、`product_mcp_schema_catalog_product.rs`），
  等价证据是它把期权合约转发为 Futu `Trd_GetMaxTrdQtys` 探测（本次实测记录到
  `acc_id=42`、`trd_market=2`、`code=AAPL260717C00200000`、`price=1.25`、`orderType=1`）。
- **规范化 intent 落在订单记录**：Go 预览行保存 canonical `normalizedComboIntent`，Rust 预览行保存
  原始 payload（无读取方），规范 intent 由 `canonical_combo_intent` 写入订单 `normalized_request`
  并参与 `preview_request_hash`——这也是幂等重放与 `PREVIEW_INVALID` 的判据来源。
- **提交前重检在提交门内**：`ExecutionRiskCoordinator::execute_with_risk_guard` 持有
  `submission_gate` 并重新从磁盘读取 real-trade 控制面，再决定是否调用 broker 提交闭包；
  Go 用「网关调用两次」表达同一时点语义，Rust 单次读取最新状态。
- **凭证单次绑定的真实判据是 request hash**：`execution_order_preview.rs:167` 先比对
  `request_hash`（含 clientOrderId）再判断 `consumed_at`，因此换身份复用同一 previewId
  会以 `PREVIEW_INVALID` 被拒；`consume_preview` 的 `consumed_at` 分支只用于同身份重放。

### 新增测试（带 `// Parity:` 锚点，测试内锚点指向 go:452dea11）

- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs::option_combo_lifecycle_preview_place_cancel_and_buying_power_keep_go_contract`
  （锚 `:15`）：期权 buying-power 返回 allowed 且探测走券商最大可买读；组合预览回传
  `allowed/option/option_combo` 并把空白+小写腿规范化为 `US.AAPL260717C00200000`/`BUY`；
  `build_pre_trade_risk_combo_order` 产出 `quantityMode=contracts`、组合数量 2、两条腿各 2 手；
  REAL 下单在 `maxOrderQuantity=2` 控制面下通过（broker 恰好一次 `Trd_PlaceComboOrder`，
  quantity=2、腿 code/side 正确）；订单记录 `normalized_request` 含 `option_combo`、
  `quantityMode=contracts`、`requestedQuantity=2`；同身份重放不追加提交；
  撤单走 `/api/v1/execution/combos/%20{internalOrderId}%20/cancel` 由 `parse_order_id` percent-decode+trim
  命中同一订单并恰好一次 `Trd_ModifyOrder(operation=2)`。
- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs::real_option_combo_rechecks_risk_after_consuming_preview_before_submission`
  （锚 `:83`）：预览签发后把控制面文件改为激活 kill switch；`place_combo` 先消费凭证并落库，
  再在提交门内重读控制面并以 `403 REAL_TRADE_KILL_SWITCH_ACTIVE` 拒绝，broker 零调用；
  被拒订单落库为 `REJECTED`（`lastErrorCode=REAL_TRADE_KILL_SWITCH_ACTIVE`、
  `lastErrorSource=risk`），同身份重放返回该 REJECTED 投影且仍不下单；
  换 clientOrderId 复用同一 previewId 被拒为 `400 PREVIEW_INVALID` 且 broker 零调用。
- fixture 扩展：`ComboPreviewTradeReader` 增加 `read_max_trade_quantity` 记录（原为
  `unsupported()`），供 buying-power 探测断言复用；既有组合测试改用 `..Default::default()` 构造。

### 探针记录（破 → 红 → 按字节回滚）

1. `crates/jftrade-engine/src/product_execution_write_port.rs::parse_order_id` 去掉 `trim()`
   （`decoded.trim().to_owned()` → `decoded.into_owned()`，原 sha `d712b28728338f502bb1f64e7b862326d608fb2d6252592aedb5d06e46736701`）：
   测试红于 `assert_eq!(cancel_response.status, 200)`，实际 404 `EXECUTION_ORDER_NOT_FOUND`；
   回滚后 sha 一致。
2. `crates/jftrade-store-sqlite/src/execution_order_preview.rs::consume_preview_in_transaction`
   把 `consumed_at` 早退提前到 `request_hash` 比对之前（原 sha
   `6a660cfdc563a526e080de38976658eef40874b648c5798423f9ddc33eea419e`）：测试红于复用凭证场景，
   实际 403 `REAL_TRADE_KILL_SWITCH_ACTIVE` 而非 400 `PREVIEW_INVALID`；回滚后 sha 一致、
   `git diff` 归零。
3. `crates/jftrade-engine/src/product_production_ports_execution_order_helpers.rs::build_pre_trade_risk_combo_order`
   把 `quantity` 改为 `parsed.order.quantity`（原 sha
   `16347b8468c7614a028759c96ea23cb57d390f03c652a0ab064c77c6cc7deb04`）：测试红于
   `risk_order.quantity`（"1" vs "2"）；回滚后 sha 一致。

### 保留差异与升级路径

- Go 的 `ExecutionComboPreview.Operation` 与 `ExecutionCommandResponse.Operation` 是服务层信封字段；
  Rust 用路由标签与订单状态投影表达，若未来需要逐字节兼容该字段，应在
  `crates/jftrade-engine/src/product_execution_write_port.rs` 的成功响应装配处补齐，并同步
  `contracts/openapi/openapi.json` 与前端类型（当前无消费者，故不引入）。
- Go 的 `KILL_SWITCH_CHANGED` 细分原因码在 Rust 归一为 `REAL_TRADE_KILL_SWITCH_ACTIVE`；
  若前端需要区分“预览后变化”，需在 `ExecutionRiskCoordinator` 的拒绝映射中扩展，并补回归测试。
- Go 预览行保存 canonical intent，Rust 预览行保存原始 payload；若将来出现预览记录读取方
  （例如预览详情接口），必须改为写入 `canonical_combo_intent` 并补 anchor 测试。

### 后续待办

- 分片二 `:110`/`:150`/`:172`（事件 parlay 与 option combo 风险形状矩阵）；
- 分片三 `:205`/`:237`/`:319`（旧购买力兼容、不安全边界、失败路径）；
- 分片四 `:412`/`:441`/`:571`/`:635`（辅助分支、失败契约、详情解析与缓存失败）。

## 第一百一十九批（分片二）：事件 parlay 与组合风险形状（3 条）

### 范围

- P0 `internal/trading/execution_combo_lifecycle_test.go:110`：
  `TestExecutionEventParlayCompletePreviewAndAmountRisk`。
- P0 `internal/trading/execution_combo_lifecycle_test.go:150`：
  `TestEventParlayRejectsCallerControlledPrice`。
- P1 `internal/trading/execution_combo_lifecycle_test.go:172`：
  `TestOptionComboValidationRejectsIncompleteRiskShape`。

结论：3 条均升 `[x]`/`function_exact`。全仓 `[x]` 1347 → **1350**、`partial` 2511 → **2508**、
`boundary` 589、`module_only` 4、`missing` 0（合计 4451 不变）；Rust 测试 3088 → **3091**。
reconcile anchors 1350 → **1353**（已记账 1294 → 1297、unrecorded 0、unknown 55、stale 1）。

### 关键事实与两处生产修复（本轮 recon 与实测）

1. **option combo 未拒绝 `amount`（已修复）**：Go 的 `validateOptionComboRequest` 把
   `Amount != nil` 作为第一条检查（文案 `amount is supported for event parlay orders only`）。
   Rust 的 `parse_combo_with_defaults` 的 option_combo 分支此前没有该检查，而
   `parse_order_with_defaults` 的 amount 检查被 `!has_legs` 旁路：带 `amount` 的 option combo
   只在安装风控协调器时被 `evaluate_pre_trade_risk` 以 `INVALID_ORDER_RISK_SHAPE` 拒绝，
   无协调器的 SIMULATE 路径会直接把该请求送往 broker（amount 在 Futu 请求中被静默忽略）。
   修复位置：`crates/jftrade-engine/src/product_production_ports_execution_order_parse.rs`
   的 option_combo 分支首位，返回 Go 同文案的错误 → 线上映射为 400 `BAD_REQUEST`。
2. **parlay 下单不消费/不校验存储 RFQ（已修复）**：Go 的 `CreateExecutionCombo` 顺序为
   绑定 RFQ（`ValidatePredictionQuote`）→ 风控 → 消费预览 → `ConsumePredictionQuote` →
   提交门风控 → 网关预留/提交；网关对服务层错误调用 `MarkSubmissionUnknown`。Rust 此前
   完全没有消费 `execution_prediction_quotes`：全仓仅 store 层实现
   `consume_prediction_quote`（含 Go 的幂等重放语义），引擎无调用方，因此同一 30 秒 RFQ
   可给多个 clientOrderId 重复定价，服务端从未签发的 rfqId 也能成交。修复位置：
   `crates/jftrade-engine/src/product_production_ports_execution_orders_impl.rs::place_combo`
   在订单预留（预览凭证消费）之后、提交门风控之前调用 `consume_prediction_rfq`；失败时按
   Go 网关语义把已预留订单落为 `UNKNOWN`（event `submission_failed`）并返回
   400 `BAD_REQUEST`（消息前缀 `prediction RFQ is invalid:`）并附带存储层细节。绑定哈希复用 RFQ 路由的
   Go 兼容规范化：新增
   `product_prediction_combo_quote::prediction_quote_binding(payload)`（返回 `(mvc, legs_hash)`）
   （`sha256(JSON{mvc,legs})`，`GoNumber` 数字格式），并把两个中间模块放宽为
   `pub(in crate::product::product_production_ports)` 以便执行端口复用同一实现。
3. **caller 控制价**：Rust 早已按 Go 文案拒绝（`event parlay price is bound to the
   server-side RFQ and must not be provided`），本轮补测试固化。
4. **组合风险形状矩阵**：Rust 的 5 条文案与 Go 完全一致（underlyingInstrumentId /
   nearExpiry / positive spread / farExpiry / unsupported optionStrategy），本轮补矩阵测试。

### 新增测试（带 `Parity: go:452dea11` 锚点）

- `crates/jftrade-engine/src/product_production_ports_execution_preview_tests.rs::event_parlay_preview_place_and_amount_risk_keep_go_contract`
  （锚 `:110`）：parlay 预览 allowed/event_contract/event_parlay；风控命令
  `quantityMode=amount`、`quantity=10`、`amount=10`、`price=None`；下单写入的
  `TradePlaceComboOrderRequest.quote_id=\"rfq-1\"`（对应 Go `placedIntent.RFQID`）；
  同 RFQ 换 clientOrderId 二次下单 400（含 `already consumed`）且 broker 零调用；
  未签发 rfqId 下单 400（含 `prediction RFQ`）且 broker 零调用。
- `crates/jftrade-engine/src/product_production_ports_execution_order_validation_tests.rs::event_parlay_rejects_caller_price_and_option_combo_rejects_amount`
  （锚 `:150`）：caller 价错误含 `server-side RFQ`；option combo + amount 错误文案与 Go 一致。
- `crates/jftrade-engine/src/product_production_ports_execution_order_validation_tests.rs::option_combo_validation_rejects_incomplete_risk_shape_matrix`
  （锚 `:172`）：5 条不完整形状逐条文案 + 完整 straddle 可解析。

### 探针记录（破 → 红 → 按字节回滚）

1. 删除 option_combo 的 amount 拒绝（原 sha
   `b02e885f9b517f0d85bbb8f61881b324359383a5f945eb6a44ed4ffb36f01588`）：`:150` 测试红于
   “option combo amount must be rejected”；回滚后 sha 一致。
2. 把 `place_combo` 的 parlay 分支条件改为不匹配（原 sha
   `a01121195db19f2e64a427abff2b96210d928f0c22ba5cb3ea7810342f40ea9d`）：`:110` 测试红于
   第二个 parlay 被接受并提交（`rust-combo-2` SUBMITTED）；回滚后 sha 一致。
3. 放宽 store 的已消费幂等判断为“任意 pair 均返回 Ok”（原 sha
   `a207d513a436163965f310752b0e7912864af0235a30f7139887804b2913d66c`）：`:110` 测试同样红于
   第二个 parlay 被接受；回滚后 sha 一致且该文件 `git diff` 归零。

### 保留差异与升级路径

- Go 在**预览**阶段就用 `ValidatePredictionQuote` 校验存储 RFQ；Rust 预览仍以“RFQ 适配器可用 +
  活跃事件合约”为证据（`persist_event_combo_preview` 内既有设计注释），伪造 rfqId 可过预览但
  必在下单期被拒。若要与 Go 完全对齐，应在预览期补 `validate_prediction_quote`，并同步
  `docs/history/go-to-rust` 的差异记录与锚点测试。

### 结构拆分（为满足生产文件 800 行上限）

- 新建 `crates/jftrade-engine/src/product_production_ports_execution_order_parse_combo.rs`（206 行）：
  承载 `parse_combo_with_defaults`（含本轮新增的 option_combo `amount` 拒绝）与
  `validate_event_parlay_quote`（caller 控制价拒绝），对外 `pub(in super::super)`。
  `crates/jftrade-engine/src/product_production_ports_execution_order_parse.rs` 由 804 → **605** 行，
  原位置改为 `#[path = "...combo.rs"] mod combo_parse;` 再 `pub(super) use ...` 转发，
  公开路径与行为不变。
- 新建 `crates/jftrade-engine/src/product_production_ports_execution_orders_rfq.rs`（55 行）：
  承载 `impl ProductionExecutionPort { fn consume_prediction_rfq }`（绑定哈希校验 + 单次消费 +
  失败落 `UNKNOWN`）。`crates/jftrade-engine/src/product_production_ports_execution_orders_impl.rs`
  由 830 → **778** 行，`crates/jftrade-engine/src/product_production_ports_execution_orders.rs`
  增加第二个 `include!` 引入该分片。
- 上述拆分均为纯搬运（除新增的两处校验/消费逻辑本身），未改任何公开 DTO 或 wire 契约；
  `cargo check`、`cargo fmt --all --check`、`node scripts/quality/check-workspace-architecture.mjs`
  与 `pnpm run check:quick` 在拆分后复跑通过。

### 验证记录（分片二）

| 检查 | 结果 |
| --- | --- |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(event_parlay_preview_place_and_amount_risk_keep_go_contract) + test(event_parlay_rejects_caller_price_and_option_combo_rejects_amount) + test(option_combo_validation_rejects_incomplete_risk_shape_matrix)' --all-targets --locked --no-fail-fast` | 3 passed |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(combo) + test(parlay)' --all-targets --locked` | 20 passed（组合/parlay 相关全量） |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-trading --all-targets --locked --no-fail-fast` | 1865 passed（首轮出现既有 launcher 抖动 `api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal`，隔离复跑通过后重跑全绿） |
| `cargo fmt --all --check` / `cargo clippy -p jftrade-engine -p jftrade-trading -p jftrade-store-sqlite --all-targets --locked` | 通过 |
| `pnpm run test:rust` | 3188 passed, 2 skipped（首轮因既有 launcher 抖动 `api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 失败，隔离复跑通过后重跑全绿） |
| `pnpm run check:compatibility` | 全部 replay 通过（含 API transport / storage / trading-strategy / assistant / desktop） |
| `node scripts/check-zero-go.mjs` / `pnpm run check:ai-context` / `git diff --check` | 通过 |
| `pnpm run check:quick` | 通过（首轮因 `check:rust:architecture` 报两个生产文件 804/830 行超 800 上限失败 → 按职责拆出 `product_production_ports_execution_order_parse_combo.rs` 与 `product_production_ports_execution_orders_rfq.rs`；随后因 `check:rust:target-health` 的 .rcgu.o 计数超限，确认无 Cargo 进程后 `pnpm run clean:rust:artifacts`（36.8GiB）复跑通过） |
| `pnpm run check:rust` | **未通过**：既有 `check:rust:policy` advisories 阶段失败（RUSTSEC-2026-0285 + 陈旧 advisory）；前置 `check:rust:target-health`、`check:rust:architecture`、`check:rust:production-policy` 通过 |
| `python3 scripts/compatibility/audit_test_parity.py` | 4451 = function_exact 1350 + partial 2508 + boundary 589 + module_only 4 + missing 0；Rust 测试 3091 |
| `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1353 / 已记账 1297 / unrecorded 0 / unknown 55 / stale 1（既有行） |

### 验证记录（分片一）

| 检查 | 结果 |
| --- | --- |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(option_combo_lifecycle_preview_place_cancel_and_buying_power_keep_go_contract) + test(real_option_combo_rechecks_risk_after_consuming_preview_before_submission)' --all-targets --locked --no-fail-fast` | 2 passed |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(combo)' --all-targets --locked` | 14 passed（组合相关全量） |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-trading --all-targets --locked --no-fail-fast` | 1862 passed |
| `cargo fmt --all --check` | 通过 |
| `cargo clippy -p jftrade-engine -p jftrade-trading --all-targets --locked` | 通过 |
| `pnpm run test:rust` | 3185 passed, 2 skipped |
| `pnpm run check:compatibility` | 全部 replay 通过（assistant-runtime / api-transport / desktop-runtime / storage / trading-strategy 等） |
| `node scripts/check-zero-go.mjs` | 通过 |
| `pnpm run check:ai-context` | 通过 |
| `git diff --check` | 通过 |
| `pnpm run check:quick` | 通过（首次因 `check:rust:target-health` 的 75480 个 `.rcgu.o` 失败；确认无 Cargo 进程后 `pnpm run clean:rust:artifacts`（清理 34.6GiB）复跑通过） |
| `pnpm run check:rust` | **未通过**：既有 `check:rust:policy` 在 advisories 阶段失败（RUSTSEC-2026-0285 与陈旧 advisory 条目）；其前置 `check:rust:target-health`、`check:rust:architecture`、`check:rust:production-policy` 通过，Rust 静态与测试由 `check:quick`/`test:rust`/nextest 覆盖 |
| `python3 scripts/compatibility/audit_test_parity.py` | 4451 = function_exact 1347 + partial 2511 + boundary 589 + module_only 4 + missing 0；Rust 测试 3088 |
| `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1350 / 已记账 1294 / unrecorded 0 / unknown 55 / stale 1（既有行） |
