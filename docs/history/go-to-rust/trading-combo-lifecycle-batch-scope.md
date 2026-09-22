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
