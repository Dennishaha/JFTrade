# Broker 能力、快照错误与 Research Screen 领域对齐批次

本文件记录 Go 基线 `pkg/broker`（券商中立类型与快照错误分类、能力目录与特性路由、
行情规则、research screen 契约）迁移到 Rust `jftrade-broker`（中立 taxonomy 与快照错误）、
`jftrade-engine`（能力目录投影、生产适配绑定、research screen 读写端口）与
`jftrade-research`（因子定义归一化）的逐测试核对结论。

## 第九十三批：`pkg/broker` 全域收口（29 条）

### 范围与分片

`pkg/broker/**` 剩余 **29 条 `missing`**（5 个文件），分三片：
P0 快照错误分类与行情规则 5 条（`market_rules_snapshot_errors_test.go`）→
P1 broker 核心与能力目录 17 条（`broker_test.go` 10、`catalog_test.go` 7）→
P2 能力契约与 research screen 7 条（`product_capability_contracts_test.go` 4、
`research_screen_test.go` 3）。

### 结果

- 29 条全部给出结论：**10 条 `[x]`/`function_exact` + 19 条 `partial`**。
- `pkg/broker/**` 归零：30 条 = **11 `[x]` + 19 `partial`，0 `missing`**
  （既有 `[x]` 为 `product_capability_contracts_test.go:11` 的 legs hash 行）。
- 全局：4451 = function_exact **1234** + partial **2491** + boundary **541** + module_only 4 +
  missing **181**（前批为 1224 / 2472 / 541 / 4 / 210），Rust 测试 2940 → **2944**。

### 本批修复与新增回归

`crates/jftrade-engine/src/product_broker_capabilities_projection.rs::ui_surface_id`
按冻结 Go `capabilityUISurfaceID` 的 path → query → fallback 顺序重写。修复前：
`"/research"` 得到 `research`（应为 `research.market`）、`"/"` 得到空串（应为 `app.root`）、
`"/workspace?unknown=1"` 得到 `workspace.unknown.1`（应为 `workspace.root`）。
这些分支当前目录表未使用，属对外 `uiSurfaceId` 契约的低风险对齐。

新增回归测试（4 条）：

- `crates/jftrade-engine/src/product_broker_capabilities_projection_tests.rs::ui_surface_ids_and_operation_overrides_follow_the_catalog_route_table`
  （11 行路由表 + `prediction.history` subscription 覆盖 + 无 UI 路由）。
- `...::capability_catalog_publishes_required_product_surfaces`（目录 feature 数、必备 id、warrants/futures UI surface）。
- `...::declared_features_publish_the_reader_interface_that_gates_them`（`adapterInterface` 发布 + 未装 reader 时 fail-closed）。
- `crates/jftrade-broker/tests/market_rules_snapshot_errors.rs::apply_market_rule_ignores_missing_zero_and_negative_lot_size`
  （`lotSize` 为 `None`/`0`/`-100` 时 min/step 不变）。

探针（改坏 → 转红 → 按字节回滚）：

- 移除 `ui_surface_id` 的 `/research` 分支与 `app.root` 兜底：新路由表测试转红
  （`left: "research"`）；`shasum -a 256` 校验回滚一致（`d3d69e3a…`）。
- 去掉 `apply_market_rule` 的 `lot_size > 0` 过滤：新 lot-size 测试与既有非法约束测试同时转红；
  `shasum -a 256` 校验回滚一致（`cba5cebe…`）。

### 新增 `[x]` 锚点

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `market_rules_snapshot_errors_test.go:14 TestApplyMarketRulesMatchesAndOverridesConstraints` | `market_rules_snapshot_errors.rs::market_rules_match_trimmed_symbols_and_apply_overrides_in_order` | `[x]`：trim + 大小写无关首命中、显式约束覆盖 lotSize、无匹配原样返回。 |
| `market_rules_snapshot_errors_test.go:33 TestApplyMarketRuleIgnoresInvalidExplicitConstraints` | `market_rules_snapshot_errors.rs::market_rules_ignore_missing_non_positive_and_non_finite_constraints` | `[x]`：0/负数显式约束不写入，非有限值与非正 lotSize 同样忽略。 |
| `market_rules_snapshot_errors_test.go:43 TestSymbolScopedSnapshotError` | `market_rules_snapshot_errors.rs::symbol_scoped_snapshot_errors_are_detectable_through_context` | `[x]`：保留原因文本、外层包裹后可识别、普通错误不标记（`nil` 分支为 Go-only）。 |
| `market_rules_snapshot_errors_test.go:63 TestSnapshotRateLimitErrorCarriesRetryDelay` | `market_rules_snapshot_errors.rs::snapshot_rate_limits_preserve_retry_delay_and_context` | `[x]`：保留上游消息、包裹后取回 2.5s、0 延时归一 1s、普通错误无延时。 |
| `market_rules_snapshot_errors_test.go:85 TestSnapshotAvailabilityErrorsExposeFallbackEligibility` | `market_rules_snapshot_errors.rs::snapshot_availability_kinds_control_fallback_eligibility` | `[x]`：entitlement/unsupported/subscription_quota 可回落、未知 kind 不可回落、普通错误不可读。 |
| `broker_test.go:175 TestApplyMarketRuleIgnoresMissingAndInvalidLotSize` | `market_rules_snapshot_errors.rs::apply_market_rule_ignores_missing_zero_and_negative_lot_size`（本批新增） | `[x]`：`None`/`0`/`-100` 三种 lotSize 均不改变 min/step=5。 |
| `broker_test.go:194 TestBrokerError` | `foundation_corpus.rs::preserves_broker_error_display_and_snapshot_fallback_rules` | `[x]`：冻结语料断言 `broker futu: [NOT_CONNECTED] …` 渲染，覆盖 id/code/message 三字段。 |
| `catalog_test.go:11 TestCapabilityCatalog` | `product_broker_capabilities_projection_tests.rs::capability_catalog_publishes_required_product_surfaces`（本批新增） | `[x]`：feature 数 ≥45、7 个必备 feature、warrants/futures UI surface；Rust 无运行时 `Validate()`。 |
| `product_capability_contracts_test.go:26 TestCapabilityOperationSurfaceFallbacksAndOverrides` | `product_broker_capabilities_projection_tests.rs::ui_surface_ids_and_operation_overrides_follow_the_catalog_route_table`（本批新增 + 修复） | `[x]`：10 行路由表、subscription 覆盖（POST/无 tool/workspace.chart）、无 UI 路由空 surfaceId。 |
| `product_capability_contracts_test.go:159 TestBrokerFeatureRouterRejectsDeclaredFeatureWithoutAdapterInterface` | `product_broker_capabilities_projection_tests.rs::declared_features_publish_the_reader_interface_that_gates_them`（本批新增） | `[x]`：目录发布 `adapterInterface`（含 `OptionAnalyticsReader`），未装 reader 时 fail-closed。 |

### 保留差异候选（保持 `partial` 的理由）

- **P1 broker 注册表（5 条）**：`TestRegistryBasic`、`RegisterAndLookup`、`ReplaceUpdatesActiveBroker`、
  `RemoveDeletesOnlySelectedBroker`、`DuplicatePanics` 描述 Go 的运行时多券商注册表（Register/Replace/Remove、
  `ActiveBroker`、重复注册 panic）。Rust 组合根只安装单一 Provider/OpenD 套件
  （`SharedTradeReadRuntime`、`ActiveProviderState`、显式生产适配绑定表），没有按 id 增删/替换/重复检测入口；
  Replace 的最近 owner 是行情 provider 激活切换（fail-closed、保留旧代际、健康恢复后生效）。
- **P1 broker 适配辅助（3 条）**：`ConvertFutuReadQuery`（Rust 读端口直接按 accountId/environment/market 构造请求，
  无多券商抽象层）、`PointerHelpers`（Go 指针辅助，Rust 以 `Option<T>` 表达，无 nil 指针别名问题）、
  `TestApplyMarketRuleUsesLotSizeAsQuantityConstraints`（行为已由 `pkg/futu/exchange_test.go:62` 的 `[x]` 行占用同一
  Rust 测试，按全局唯一引用约束保持 `partial` 并写明引用来源）。
- **P1 能力目录校验与路由（5 条）**：`ImplementsAdapterInterface` 反射探测（Rust 无反射，改由显式绑定表 +
  逐 operation readiness）、`RejectsUnsafeWriteMCP` 与 `ValidationAndOrderingBranches` 的运行时 `Validate()`
  （Rust 为 const 目录 + 正向不变量测试，非法目录不可构造）、`HonorsExplicitSelectionAndFallback`、
  `FailureAndProductBranches`、`CandidateOrderingDeduplicationAndReasonFallback`
  （Rust 无 feature router；候选排序/去重与可用性裁决位于行情 provider router）。
- **P2 能力契约（2 条）**：`CapabilityCatalogOperationValidationFailures` 的 5 种非法 operation 拒绝矩阵
  （无运行时 `Validate()`）、`RuntimeEvaluatorFailuresAndReasons` 的 reason 优先级与
  `defaultCapabilityEvaluation` 形状（Rust 由运行时快照派生，测试断言 code/state，未逐条断言 reason 文本）。
- **P2 research screen（3 条）**：`TestFactorRefIdentityAndStableConstruction`（Rust `FactorRef` 为内部类型，
  稳定身份由 normalization 的 `stable_instance_id`/`factor_configuration_key` + 冻结语料 case 承担，哈希串形状不同）、
  `TestNewFactorRefRejectsUnrepresentableOrInvalidParameters`（func/string/nil 的宽松降级在 Rust 强类型 wire 下不可表达）、
  `TestResearchScreenRateLimitErrorContract`（Rust 在 provider 边界映射为 429 + Retry-After，
  毫秒文本与“0 → 1s”默认归一由 Futu integration 产生，无类型级对应断言）。

### 后续待办

- 若恢复多 broker 支持，注册表/特性路由批次需重新评估（当前单一 Provider 模型下不可达）。
- 能力目录 reason 文本优先级（account → feature → 兜底）尚无逐条断言，可在 productfeatures 域后续批次补。
- research screen 参数宽松降级（非法形状 → 空参数）与 Go 的业务兼容性需在 wire 兼容批次单独确认。

验证：`cargo fmt --all -- --check`；`cargo clippy -p jftrade-broker -p jftrade-engine --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-broker -p jftrade-engine -p jftrade-integration-futu -p jftrade-research --all-targets --locked --no-fail-fast`（**2277 passed / 1 skipped**）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2944 Rust** / **1234 `[x]`**；missing 181、partial 2491、boundary 541、module_only 4；`OK: 1234 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 196、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors 1232；already recorded 1137、unrecorded 6 = `internal/settings` 基线、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0，含 API/Assistant/Backtest/Desktop/Provider/SQLite/Trading 六组 replay）；
`node scripts/check-zero-go.mjs`（2894 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`。

`pnpm run check:quick` **EXIT=0**（workspace 测试 **2288 passed / 1 skipped**，web/python/desktop/contract 各阶段通过）。
首次运行时 `check:rust:target-health` 因 `target/debug/deps` 存在 50652 个残留 `.rcgu.o` 中间产物失败；
确认无 cargo/rustc 进程后按批处理约定删除该目录下的 `*.rcgu.o`（未执行 `cargo clean`，未触碰源码），重跑通过。

`pnpm run check:rust` **未通过（EXIT=1）**，唯一失败项不在本批 diff 范围内，保留原始证据不记为通过：
`check:rust:policy` 的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44 TLS 1.3 握手加密层级校验缺陷，修复版本 >=0.23.45；
根 `Cargo.toml` 精确锁定 `=0.23.44`，解除需一次显式的依赖升级批次并复核 `deny.toml` 许可例外），
另有 8 条 `warning[advisory-not-detected]`（陈旧 ignore 条目）。同一次运行中 target-health、architecture、
production-policy、`cargo fmt --check` 与 clippy 均通过；本批未改 `Cargo.lock` / `deny.toml`。

## 第一百二十五批：trading/broker 域剩余 72 条收口（分片一 11 条）

本批按 `pkg/broker` + `internal/trading` 前缀下 `evidence_type != function_exact` 的 72 行推进，
分片划分与执行标准见 goal 文本。分片一覆盖三个文件：`internal/trading/execution_products_test.go`(4)、
`internal/trading/responses_test.go`(4)、`internal/trading/service_test.go`(3)。

### 分片一：产品枚举、读响应形状与服务默认值（11 条）

#### 生产修复（契约 required 字段）

`contracts/openapi/openapi.json` 中 `trading.BrokerFundsResponse` 与 `trading.BrokerPositionsResponse`
都把 `lastError` 列入 `required`；Go 的 typed DTO 在无错误时 marshal 为显式 null，而 Rust 的两条成功投影
此前**缺这个键**（失败路径因 fail-closed 不会走到该形状，cutover fixture 又只回放注入端口，所以既有门禁未捕获）：

- `crates/jftrade-engine/src/trade_projection.rs::funds_value` 增加 `"lastError": Value::Null`；
- `crates/jftrade-engine/src/product_production_ports_trade.rs` broker positions 成功分支增加 `"lastError": Value::Null`。

#### 新增测试（5 条，均带 `// Parity:` 锚点）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `derivative_single_leg_preview_requires_client_order_id_and_locks_the_place` | `execution_products_test.go:12` | 衍生品预览缺 clientOrderId → 400 含 clientOrderId 且券商零调用；补 clientOrderId → 预览成功（productClass=option、previewId 前缀）；下单缺 previewId → 400 含 previewId |
| `expired_parlay_rfq_is_rejected_before_the_combo_gateway` | `execution_products_test.go:149` | quoteExpiresAt=2020-01-01 → 400 含 "request a new RFQ" 且组合网关零调用 |
| `real_futures_preview_requires_futures_authority` | `execution_products_test.go:128` | 从 `single_and_prediction_submission_failure_contracts_keep_go_boundaries` 抽出期货权限矩阵（缺权限/权限在别的账户 → 400 含 "FUTURES account authority"；有权限 → 预览成功），使 [x] rust_entry 全局唯一 |
| `broker_funds_response_serializes_the_contract_keys_with_null_last_error` | `responses_test.go:55` | 排序键集合 == {checkedAt,connectivity,currencyBalances,lastError,marketAssets,summary}、lastError 显式 null、summary 对象、两数组存在 |
| `broker_positions_response_serializes_the_contract_keys_with_null_last_error` | `responses_test.go:107` | 排序键集合 == {checkedAt,connectivity,lastError,positions}、lastError 显式 null、两条仓位条目 symbol/accountId=42 |
| `broker_read_query_without_market_defaults_to_hk` | `service_test.go:8` | 无 market → summary.market=HK；显式 market=US 原样透传（既有实现，仅补断言面） |

另把 `product_production_ports_execution_reconciliation_provider_tests.rs` 内
`responses_test.go` 的锚点行号由 `:140` 修正为 `:141`（与 Go `func` 行一致），`unknown go line` 由 55 降到 54。

#### 探针记录（均按字节回滚并核对 shasum）

| 探针 | 预期转红 | 回滚后 shasum |
| --- | --- | --- |
| 删除 funds 投影的 `lastError` 键 | `broker_funds_response_...`（left 缺 lastError） | `a485e2131e73aadb2b437366e300b540db034061bde9f39ba6fc64855a163813` |
| 删除 positions 投影的 `lastError` 键 | `broker_positions_response_...`（left=[checkedAt,connectivity,positions]） | `9d866afc9e7c9d10df6158a912e0f2552237eadd6b09e316cd80102d99a93d9b` |
| 删除 `order_preview` 的 clientOrderId 守卫 | `derivative_single_leg_preview_...`（预览直接 previewValid=true） | `397da3e5926666799ea7dd328464330806712e55a88968187839b50f942f8a9e` |
| 把 parlay 过期判定短路为 `if false` | `expired_parlay_rfq_...`（过期报价被放行） | `35b4b5f139cce1142391bae51ad9032d9db6988378945f65625bdc6c9b8502b4` |

#### 分片一映射结论（11 条）

- `[x]` 6 条：`execution_products_test.go:12`、`:128`、`:149`、`responses_test.go:12`、`:141`、
  `service_test.go:8`。
- partial 3 条：`:48`（预测资格判定未接线到执行预览/下单路径——判定函数 `prediction_account_eligibility`
  只服务行情订阅与预测推送，缺口与升级路径已写明）、`responses_test.go:55`/`:107`
  （成功形状已收口；Go 的 200 降级信封在 Rust 为 fail-closed 错误状态，属既有产品差异）。
- boundary 2 条：`service_test.go:24`/`:45`（Rust 无“可选 worker + 服务门面”对象，worker 由 engine 组合期直接持有）。

#### 分片一验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(...)'`（按测试名逐一/组合） | 5 条全部通过 |
| 抽出后回归 | `... -E 'test(real_futures_preview_requires_futures_authority) or test(single_and_prediction_submission_failure_contracts_keep_go_boundaries)'` | 2 条通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b125s1_apply.json`、`/tmp/b125s1_fix.json` | 13 行写入，`[x]` 1362 → 1368 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 无重复 rust_entry、0 条 `[x]` 缺少 function_exact |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1375（已记账 1320、unrecorded 0、unknown 54、stale 1 既有） |

### 分片二：broker 一致性 harness 与控制面幂等/审计（10 条）

范围：`internal/trading/broker_conformance_test.go`(3)、`internal/trading/control_plane_idempotency_test.go`(3)、
`internal/trading/control_plane_state_audit_test.go`(3)，另含同域一行 `internal/trading/execution_test.go:738`
（其唯一缺口正是本片修复的拒绝审计事件形状）。

#### 生产修复：硬停拒绝审计事件的 eventType/action 对调

`crates/jftrade-engine/src/product_execution_risk_coordinator.rs` 的拒绝审计事件原先写成
`{eventType: "HARD_STOP_REJECT", action: "REJECT"}`；而
`crates/jftrade-trading/src/real_trade.rs::events_with_prefix` 按 **action** 前缀 `HARD_STOP_` 过滤，
导致该事件只存在于持久化审计文件、**不出现在 snapshot.hardStopEvents**。Go 的
`recordRejectedHardStop`（`go:internal/trading/control_plane.go:509`）是
`{EventType: "rejected", Action: "HARD_STOP_REJECT"}`。现已按 Go 对齐：

- `event_type: "rejected"`、`action: "HARD_STOP_REJECT"`；
- 既有断言随之从 `event_type == "HARD_STOP_REJECT"` 改看 `action == "HARD_STOP_REJECT"`
  （`real_order_rejects_when_hard_stop_matches`、`kill_switch_and_hard_stop_survive_a_restart_with_rejection_audit`）。

该修复同时解除了 `internal/trading/execution_test.go:738` 行进（原 partial 结论写明“修复位置：把拒绝事件的
action 归入 HARD_STOP_ 前缀”），本批据此升为 `[x]`。

#### 新增测试（6 条）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `conformance_partial_full_fill_average_and_out_of_order_updates_hold` | `broker_conformance_test.go:15` | 4@100 → PARTIALLY_FILLED；乱序旧快照不回退；6@101 → FILLED 且加权均价 100.6；终态快照不回退 |
| `control_plane_kill_switch_release_is_idempotent_and_audited` | `control_plane_idempotency_test.go:12` | 未激活 release 成功且 1 条事件（activatedAt=None）；激活后两次 release → 4 条事件、最新无引用、前一条引用激活时刻 |
| `control_plane_hard_stop_release_is_single_shot` | `:65` | 首释成功；重复 release 报错含 "not found"；事件恰好 2 条且不追加 |
| `control_plane_hard_stops_block_until_every_entry_released` | `:99` | 两条累积 hard stop、REAL 单被拒、逐条释放、每次拒绝入审计（HARD_STOP_REJECT==2）、全释放后放行 |
| `control_plane_retains_activation_and_bounds_repeated_audit_events` | `control_plane_state_audit_test.go:13` | 重复激活保留首次 activatedAt/刷新 operator；风控上限更新保留 activatedAt；201 次重激活后事件封顶 200 且表头为新事件 |
| `control_plane_executes_simulated_orders_through_the_risk_guard_without_audit` | `:75` | SIMULATE 单放行、提交回调执行、三组审计列表为空 |

#### 探针记录

| 探针 | 预期转红 | 回滚后 shasum |
| --- | --- | --- |
| 把拒绝审计事件改回 `action="REJECT"` | `control_plane_hard_stops_block_until_every_entry_released`（拒绝计数 0≠2） | `5c9dab4bf6797837d7139e177f0e75bda5f448c8ee058a0e322a8a3d9adebfed` |

#### 分片二映射结论

- `[x]` 7 条：`broker_conformance_test.go:15`、`control_plane_idempotency_test.go:12/:65/:99`、
  `control_plane_state_audit_test.go:13/:75/:190`，加 `execution_test.go:738`（同片修复解除）。
- partial 2 条：`broker_conformance_test.go:58`（撤单被拒的券商回执语义缺失：Rust cancel RPC 失败转 UNKNOWN
  + `cancel_failed`，Go 保持 CANCEL_REQUESTED + `broker.cancel` + BROKER_CANCEL_REJECTED）、
  `broker_conformance_test.go:107`（place rejected 在 Rust 为 fail-closed UNKNOWN；`TradeSessionError::Unsupported`
  映射为 503 而非 Go 的请求级 capability 错误；push-before-query 由快照发现承接）。

#### 分片二验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(...)'` | 6 条全部通过 |
| 事件形状回归 | 同上 `-E 'test(real_order_rejects_when_hard_stop_matches) or test(kill_switch_and_hard_stop_survive_a_restart_with_rejection_audit)'` | 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b125s2_apply.json` | 10 行写入，`[x]` 1368 → 1376 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 无重复 rust_entry、0 条 `[x]` 缺少 function_exact |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1379（已记账 1324、unrecorded 0、unknown 54、stale 1 既有） |

### 分片三：`pkg/broker` 剩余 16 行复核（1 升 [x]、3 收敛为边界、12 复核保留）

范围：`pkg/broker/research_screen_test.go`(3)、`pkg/broker/product_capability_contracts_test.go`(2 pending)、
`pkg/broker/broker_test.go`(7 pending)、`pkg/broker/catalog_test.go`(5 pending)。

#### 结论变更

| Go 行 | 变化 | 依据 |
| --- | --- | --- |
| `pkg/broker/broker_test.go:157` `TestApplyMarketRuleUsesLotSizeAsQuantityConstraints` | partial → `[x]` | 行为已由 `jftrade-broker::broker_lot_size_initializes_minimum_and_step_quantity` 逐字覆盖（lotSize=100 → min/step 双 100）；此前 partial 的唯一原因是该 Rust 用例的 entry 已被 `pkg/futu/exchange_test.go:62` 占用，本批确认无功能缺口后按共享证据登记（entry 文本显式说明共享） |
| `pkg/broker/product_capability_contracts_test.go:89` `TestCapabilityCatalogOperationValidationFailures` | partial → boundary | Rust 能力目录是 const `FEATURE_SPECS`，无运行时 `Validate()`；非法 operation 不可构造，正向面由 `reviewed_tool_operation_schemas_are_catalog_backed` 覆盖 |
| `pkg/broker/catalog_test.go:44` `TestAdapterInterfaceSupportRejectsNilMissingAndUnknownImplementations` | partial → boundary | Go 用反射按名字探测接口实现；Rust 无名称→接口查询，可用性由 `FEATURE_SPECS.adapterInterface` + 组合根装入的 reader 决定（`prediction_readiness_is_independent_per_operation_adapter`） |
| `pkg/broker/catalog_test.go:141` `TestCapabilityCatalogValidationAndOrderingBranches` | partial → boundary | 同上：9 条非法目录形状在 const 表下不可表达；可达的稳定 id/唯一映射面由 `every_catalog_protocol_maps_to_a_feature_with_one_stable_id` 覆盖 |

#### 复核保留（12 行，逐条确认 Go 断言、Rust 证据与差异描述仍然准确）

- `research_screen_test.go:11`（FactorRef 稳定身份：Rust 由 normalization 的 stable_instance_id/配置键 + 冻结语料承担）、
  `:31`（宽松降级不可表达：Rust 强类型 wire）、`:46`（rate limit：Rust 在 provider 边界 429 + Retry-After）。
- `product_capability_contracts_test.go:174`（运行时评估器 reason 优先级：Rust 输出 state/code 维度）。
- `broker_test.go` 注册表 5 条 + 指针 helpers 1 条（Go-only 运行时注册表与指针别名语义；Rust 为组合根静态注入 + 所有权模型）。
- `catalog_test.go:98`/`:195`/`:233`（per-feature router 选择/失败/候选排序：Rust 无 per-feature 候选列表，provider 解析只有 explicit_broker/active_provider 两种 reason）。

#### 分片三验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| `pkg/broker` 证据 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-broker -p jftrade-research --all-targets --locked -E 'test(broker_lot_size_initializes_minimum_and_step_quantity) or test(normalization_and_field_errors_match_the_go_owner_corpus)'` | 2 条通过 |
| engine 证据 | `... -p jftrade-engine ... -E 'test(research_screen_definition_rejects_unsupported_market_and_stable_keys) or test(futu_screen_write_errors_keep_their_transport_contract) or test(reviewed_tool_operation_schemas_are_catalog_backed) or test(capabilities_mark_declared_but_missing_readers_unavailable) or test(capabilities_filters_by_broker_market_and_feature_id) or test(every_catalog_protocol_maps_to_a_feature_with_one_stable_id) or test(prediction_readiness_is_independent_per_operation_adapter)'` | 8 条通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b125s3_apply.json` | 4 行变更，`[x]` 1376 → 1377 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 无重复 rust_entry、0 条 `[x]` 缺少 function_exact |

### 分片四：服务/风控/端口边界 15 行复核（1 升 [x]、2 收敛为边界、12 复核保留）

范围：`internal/trading/broker_test.go`(4)、`risk_status_broker_boundaries_test.go`(4)、
`broker_account_read_failures_test.go`(2)、`risk_shape_boundaries_test.go`(2)、
`broker_boundaries_test.go`(1)、`ports_test.go`(2)。触发点覆盖 30 条 Rust 证据用例（全部隔离复跑通过）。

#### 结论变更

| Go 行 | 变化 | 依据 |
| --- | --- | --- |
| `broker_account_read_failures_test.go:87` `TestFundsMapsMarketAssetsAlongsideCashBalances` | partial → `[x]` | `funds_projection_preserves_currency_and_market_asset_arrays` 逐条断言 2 条 currencyBalances（HKD/USD + cash/availableWithdrawalCash/netCashPower）与 2 条 marketAssets（HK/US + assets），与 Go 的同一 snapshot 双向映射一致；此前 partial 仅因该 Rust 用例 entry 已被 `pkg/futu/adapter_new_methods_test.go:112` 占用 |
| `ports_test.go:50` `TestServiceUsesExplicitTradingPorts` | partial → boundary | Rust 无“默认端口可被显式覆盖”的服务对象：未提供 test port 时执行写路由不注册，生产组合根显式注入端口 |
| `ports_test.go:82` `TestServiceDefaultTradingPortsFailExplicitly` | partial → boundary | 同上；缺端口的 fail-closed 不变量已由读路由 503 与控制面不可用拒绝覆盖 |

#### 复核保留（12 行）

- `broker_account_read_failures_test.go:11`（降级信封 vs fail-closed：**同一产品差异**，与 `responses_test.go:55/:107`、
  `servercoretest/broker_routes_test.go:14` 同源；本批把结论改为显式交叉引用并保留 fail-closed 证据）。
- `broker_test.go:186`（13 个读操作映射与查询透传：证据 9 条逐路由断言，缺口是无会话降级信封）、
  `:453`（组合回退与 degraded 信封：证据 3 条，缺口同源）、
  `:533`（写操作/超时矩阵：证据 3 条，缺口是读端口无 deadline/LastError 契约）、
  `:735`（行情/运行期默认值与显式查询：证据 2 条，缺口是 runtime 缺 session 的降级投影）。
- `risk_status_broker_boundaries_test.go:13`（审批决策/风险错误类型为 Go 对象；symbolMatches/hardStopMatches 矩阵已覆盖）、
  `:124`（17 入口不匹配：12 读 + 2 组合 + 3 写已覆盖，缺口是无活动 broker 时 Runtime 回退投影）、
  `:187`（低层 helper 与 withTimeout：函数适配器/nil 接收者为 Wails 期模式，deadline 契约缺口同 `:533`）、
  `:228`（resolveBroker required/optional 语义：Rust 单 broker 模型下“无活动 broker”不可达）。
- `risk_shape_boundaries_test.go:11`（伪造字段矩阵：Rust 在解析/风控层拒绝，未逐组合对照）、
  `:78`（产品×价格必填矩阵：Rust 拆两条断言）。
- `broker_boundaries_test.go:62`（上游失败分类矩阵：Rust 断言错误码/消息与快照回退规则，未逐读操作分类）。

#### 分片四验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| `broker_test.go` 证据 | `... -p jftrade-engine -p jftrade-trading -p jftrade-broker ... -E 'test(broker_read_projects_futu_funds_from_neutral_client) or ... or test(broker_runtime_requires_real_projection_sources)'` | 14 条通过 |
| 其余证据 | 同上 `-E 'test(portfolio_cash_balances_prefer_currency_rows_over_summary_fallback) or ... or test(unavailable_control_plane_fails_closed)'` | 16 条通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b125s4_apply.json` | 4 行变更，`[x]` 1377 → 1378 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 无重复 rust_entry、0 条 `[x]` 缺少 function_exact |

### 分片五/六：execution 家族与 order-updates 家族剩余边界复核（15 行）

范围：`execution_combo_lifecycle_test.go`(2)、`execution_test.go`(1)、`order_updates_test.go`(12)。
这 15 行在本批之前已全部收敛为 `boundary` 或带明确缺口的 `partial`，本片只做**逐行复核 + 证据复跑**，
不改实现、不改结论分级（除下述 1 处交叉引用补记）。

#### 复核要点

- `execution_combo_lifecycle_test.go:571`（partial）：缺口仅剩 `BuildOrderUpdateQueries` 的账户级去重
  （Rust 由 reconciliation discovery 的账户枚举承担，已有重复分页去重证据但无账户级去重回归）与
  `(*OrderUpdatesWorker)(nil)` 的 nil-receiver/besteffort 语义（Go/Wails 旧 worker）。其余断言
  （预览落库失败可见、组合意图走单腿端点被拒、期权分数数量/ETH 会话 400、CanonicalStoredOrderStatus 同态）
  均有 Rust 用例；若后续给订单更新订阅补账户级去重，应在 discovery 账户枚举处加回归。
- `execution_combo_lifecycle_test.go:635`（partial）：多 broker 解析器、`NewService()` 无默认 broker 的
  「服务门面」语义为 Go-only；Rust 侧详情读的失败传播（空白 id、正常投影、事件账本损坏 500）已由
  `execution_details_read_boundaries_keep_go_failure_propagation` 覆盖。
- `execution_test.go:1047`（boundary）：读时刷新在 Rust 归后台 `ExecutionReconciliationWorker`（唯一写入所有者），
  详情读为纯存储投影——与 Go 的 GetHistoryOrders 读时刷新属有意架构差异，保留边界。
- `order_updates_test.go` 12 条（boundary）：节流/内存缓存 TTL/防御性拷贝/终态移除/订阅生命周期/订阅重连/
  批量费用/低层 helper 均属于 Go 内存 worker 与推送订阅对象，Rust 由 SQLite 唯一真相 + 15s 对账 worker
  （含第 124 批新增的有界 invalidation 与 connectivity 投影）承接；各行的保留差异、复现条件与升级路径已在
  第 118/124 批及本文件前述章节登记。
- 交叉引用补记：`order_updates_test.go:11`（节流边界）与 `:31`（缓存 TTL 边界）的结论已指向
  `reconciliation_worker_bounds_recent_invalidations_to_twenty_entries` 等第 124 批证据，本批复跑确认有效。

#### 分片五/六验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 证据复跑（12 条） | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu --all-targets --locked -E 'test(execution_details_read_boundaries_keep_go_failure_propagation) or ... or test(trade_push_subscription_failure_surfaces_a_transport_error)'` | 12 条通过 |
| 映射 | 本片无新增/改级 | `[x]` 保持 1378 |

### 第一百二十五批收口与门禁

- 分片一 `2971ea04`：broker 读响应补齐 OpenAPI `required` 的 `lastError`（funds/positions 成功面显式 null），
  新增 5 条产品枚举/读形状/默认市场测试。
- 分片二 `30f01a45`：硬停拒绝审计事件按 Go 对齐（`eventType=rejected` + `action=HARD_STOP_REJECT`），
  解除 `execution_test.go:738` 行级缺口；新增 6 条一致性/控制面测试。
- 分片三 `37f58a24`：`pkg/broker` 剩余 16 行复核（1 升 `[x]`、3 收敛 boundary、12 复核保留）。
- 分片四 `38fe1b18`：服务/风控/端口边界 15 行复核（1 升 `[x]`、2 收敛 boundary、12 复核保留）。
- 分片五/六 `95865610`：execution 与 order-updates 家族剩余 15 行边界复核（证据 12 条复跑通过）。

批次计数：4451 = function_exact **1378** + partial **2466** + boundary **603** + module_only 4 + missing 0；
Rust 测试 **3215**；anchors 1379（已记账 1324、unrecorded 0、unknown 54、stale 1 既有）。

#### 批次门禁记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 格式 | `cargo fmt --all -- --check` | 通过 |
| Clippy | `pnpm run check:clippy` | 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-trading -p jftrade-broker -p jftrade-research -p jftrade-integration-futu --all-targets --locked` | 2455 条通过（1 skipped） |
| 工作区测试 | `pnpm run test:rust` | 3215 条通过（2 skipped） |
| 兼容回放 | `pnpm run check:compatibility` | 通过（API 278 操作 / assistant 9 状态 / trading-strategy 10 状态 / desktop 3 profile / provider / sqlite）。注：`cargo clean` 后首跑因并行子检查争用构建锁出现 3 个 exit 1，产物预热后单独复跑与整轮复跑均通过；不作为失败记录 |
| 生成物 | `pnpm run check:generated` | 通过（未改工作树） |
| AI 上下文 | `pnpm run check:ai-context` | 通过（6 模块 8 文件） |
| Zero-Go | `pnpm run check:zero-go` | 通过（2941 文件、0 发布产物） |
| 快速门禁 | `pnpm run check:quick` | 无产品改动（工作树干净） |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`advisories FAILED, bans ok, licenses ok, sources ok`，含 `RUSTSEC-2026-0285` 与 `deny.toml` 8 条 `advisory-not-detected`；与本批改动无关，如实记录 |
| 空白检查 | `git diff --check` | 通过 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact、无重复 rust_entry |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1379、unrecorded 0 |

#### 后续待办（第 126 批起）

- trading_broker 域剩余未收口项：`internal/api/trading/*`（execution_test 8、routes 4、execution_validation_contracts 4、
  routes_broker_contracts 2、routes_failure_boundaries 2、routes_helper_boundaries 3、routes_read_handlers 2、
  execution_products 2）、`internal/store/trading/*`（startup_compatibility 7、ledger 5、ledger_lifecycle 5、
  out_of_order_reconciliation 5、execution_composition 4、broker_fill_reconciliation 3、broker_ledger 3 等）、
  `internal/app/apiserver/tradingapp/*`、`internal/assistant/engine/*` 中已路由到本域的行。
- 之后按体量推进：api_transport 478、strategy_pine 465、assistant_workflow 447、other 311、
  backtest_calendar 262、storage_sqlite 178、marketdata_quotes 155、futu_opend 104、settings_watchlist 39。
- 每批收口后立即设定下一批 codex 目标，直到 4451 行全部给出 `[x]`/partial/边界结论。

## 第一百二十六批：`internal/api/trading/` 剩余 28 行收口（分片一）

范围：`internal/api/trading/` 全部 28 条 pending（execution_test 8、execution_validation_contracts 4、
routes_test 4、routes_helper_boundaries 3、execution_products 2、routes_broker_contracts 2、
routes_failure_boundaries 2、routes_read_handlers 2、openapi_route_alignment 1）。
owner：`crates/jftrade-engine` 的 product/trade 投影与写路由错误映射；wire 层在 `crates/jftrade-api`。

### 摸底事实（生产 HTTP 面探针，`/tmp` 临时用例，未入库）

用 `ProductConfig::desktop_production` + `SharedTradeReadRuntime(HttpTradeRead)` 启动真实产品路由后逐路径探测，
确认以下 Go 断言在 Rust 生产面成立（探针随后被正式测试取代）：

| 探测路径 | 结果 |
| --- | --- |
| `orders?scope=bad`、`max-trade-qtys&price=bad`、`klines&limit=bad` | 400 `BAD_REQUEST`（消息 `query parameter <name> is invalid`） |
| `fills?scope=unknown`、`max-trade-qtys` 缺 price、`adjustSideAndLimit=bad`、`positionId=bad`、`klines` 缺 symbol | 400 `BAD_REQUEST` |
| 11 条读路由 + `POST brokers/futu/orders` 携带 `%zz` | 400（读 `invalid query encoding`／写 `invalid broker write query`） |
| `/api/v1/brokers//funds`、`/api/v1/portfolio//cash-balances`、`/api/v1/portfolio/futu/unknown` | 404 `NOT_FOUND` |
| cash-flows 缺 `clearingDate`、order-fees 缺 `orderIdEx`、margin-ratios/securities 缺 `symbol` | 400 `BAD_REQUEST` |
| positions/fills/fees/marginRatios/cashFlows/orders 空集合 | 200 且字段为 `[]`（不是 `null`） |
| `funds`（读取端口返回 `Coordinator(Closed)`） | 503 `BROKER_READ_UNAVAILABLE`（Go 为 200 降级信封，既有产品差异） |
| `portfolio/futu/cash-balances?%zz` | **修复前** 503 `PORTFOLIO_UNAVAILABLE`，**修复后** 400 `BAD_REQUEST` |
| `/api/v1/brokers/ib/runtime` | 404 `BROKER_NOT_FOUND`（单 broker 模型，非 Go 的 200 空态） |
| `POST brokers/futu/orders`（缺 accountId）／（带 accountId&market） | 400 `accountId is required`／503 `BROKERS_WRITE_UNAVAILABLE` |
| `POST brokers/futu/unlock` | 503 `BROKERS_WRITE_UNAVAILABLE` |

### 生产修复：portfolio 畸形 query 的 400 语义

- 复现条件：`GET /api/v1/portfolio/futu/cash-balances?%zz`。
- 预期（Go `routes_helper_boundaries_test.go:131` 的 `bindQuery` 语义）：400 `BAD_REQUEST`，dispatch 前拒绝。
- 修复前：`ProductionPortfolioPort::read` 把 `TradeRequest::parse_with_prefix` 的所有错误折叠为
  `PortfolioSnapshotError::Unavailable` → 503 `PORTFOLIO_UNAVAILABLE`（错误消息虽为 `invalid query encoding`，
  但状态码与错误码不对齐）。
- 修复：新增 `PortfolioSnapshotError::Invalid(String)`；`product_api_portfolio.rs` 映射为 400 `BAD_REQUEST`；
  portfolio 端口先用 `QueryMap` 判定编码错误并抛 `Invalid`，其余路径形状失败继续走 `Unavailable`
  （保持 `production_internal_adapters_dynamic_capability_and_recovery` 的 fail-closed 断言）；
  `product_mcp_production_executor_errors.rs` 的 `portfolio_error` 增补 `Invalid` 分支
  （MCP 工具侧返回 invalid，而非 unavailable）。
- 回归：`crates/jftrade-engine/src/product_production_assembly_tests.rs::production_http_broker_reads_reject_malformed_query_encoding_before_dispatch`
  先红后绿（探针时 `left: 503, right: 400` 断言失败），修复文件 shasum `5bfd38daa3d4270c9e75ce49abd596fe28fc1052d098d8ecff148d8597d75a87`。

### 新增 Rust 测试（7 条，全部带 `// Parity:` 锚点）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `product_production_ports_trade_request_tests.rs::trade_query_normalizes_scope_merges_aliases_and_treats_blank_optionals_as_absent` | `execution_test.go:175` | scope 归一/拒绝、orderId 与 symbol 别名合并去重、optional float/uint/string 的空值与非法值（内联测试外置后父文件 786 行，满足 800 行生产文件上限） |
| `product_production_assembly_tests.rs::production_http_broker_reads_reject_invalid_scope_and_numeric_queries` | `routes_test.go:145` | 3 条非法 scope/数值路径 400 |
| `...::production_http_broker_reads_reject_missing_and_invalid_optional_inputs` | `routes_failure_boundaries_test.go:58` | 5 条缺失/非法可选参数 400 |
| `...::production_http_broker_reads_reject_malformed_query_encoding_before_dispatch` | `routes_helper_boundaries_test.go:131` | 11 读路由 + portfolio + 写路由 `%zz` 全部 400 |
| `...::production_http_broker_reads_serialize_empty_collections_as_arrays` | `routes_read_handlers_test.go:18` | 6 个集合字段等于 `[]` |
| `...::production_http_broker_reads_serve_orders_fills_quotes_klines_and_securities` | `routes_test.go:121` | Go 的 7 条路径全部 200 |
| `...::production_http_broker_reads_reject_missing_uri_broker_and_unknown_resource` | `routes_helper_boundaries_test.go:119` | 未知 portfolio 资源与缺 URI broker 均 404 |

夹具调整：`HttpTradeRead::read_max_trade_quantity` 由「协调器 Closed」改为返回成功快照，
使 `routes_test.go:121` 的 `max-trade-qtys` 200 断言可验证（改前夹具把「夹具失败」误当作路由缺失）。

### 结论变更

- 升 `[x]`（7 行）：`execution_test.go:175`、`routes_test.go:121`、`routes_test.go:145`、
  `routes_failure_boundaries_test.go:58`、`routes_read_handlers_test.go:18`、
  `routes_helper_boundaries_test.go:119`、`routes_helper_boundaries_test.go:131`。
- 修正历史锚点：`execution_test.go:47` 的 `rust_entry` 指向 `product_execution_risk_coordinator_tests.rs::real_order_rejects_when_kill_switch_active`
  （原指向不存在的 `product_execution_risk_coordinator.rs::...`），并把源码锚点补成 `go:452dea11:` 前缀形式，
  锚点对账 stale 由 1 → 0。
- 复核保留（21 行）：`execution_test.go:19/:47/:79/:110/:148/:205/:249`、
  `execution_validation_contracts_test.go:52/:161/:205/:238`、`execution_products_test.go:18/:73`、
  `openapi_route_alignment_test.go:14`、`routes_broker_contracts_test.go:143/:214`、
  `routes_failure_boundaries_test.go:17`、`routes_helper_boundaries_test.go:17`、
  `routes_read_handlers_test.go:71`、`routes_test.go:32/:95`，各自结论已写明缺口、owner 与回归要求。

### 保留差异（本批新增/确认）

- **降级信封 vs fail-closed**：Go 在无活动 broker/上游失败时返回 200 + `connectivity:"disconnected"` + `lastError`
  （funds/quote/positions/portfolio），Rust 返回 503 `BROKER_READ_UNAVAILABLE`/`PORTFOLIO_UNAVAILABLE`。
  既有产品差异，与 `broker_account_read_failures_test.go:11`、`responses_test.go:55/:107`、
  `servercoretest/broker_routes_test.go:14` 同源（见本文件分片四）。
- **broker 读失败映射缺 500/429**：Go `writeReadResult` 有 500 `BROKER_READ_FAILED` 与 429 + `Retry-After`；
  Rust `broker_read_snapshot_failure` 只有 400/404/503，且无 `Retry-After`。若要补齐需先加失败用例再改
  `crates/jftrade-engine/src/product_wire_brokers.rs` 与 `BrokerReadSnapshotError` 变体。
- **单 broker 模型**：`/api/v1/brokers/ib/runtime` 在 Rust 为 404 `BROKER_NOT_FOUND`，Go 为 200 typed 空态。
- **写路由账户解析**：Go 允许请求体不带 `accountId`（落到活动 broker），Rust 要求显式 `accountId`，否则 400。

### 分片一验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增 7 条证据 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked --lib -E 'test(trade_query_) or test(production_http_broker_reads_)'` | 8 条通过（含既有 `trade_query_parses_symbol_and_optional_helpers`） |
| 探针（先红） | 回滚 `product_production_ports_trade.rs` 的 `Invalid` 映射后单跑 | 断言 `503 != 400` 失败，随后按字节还原（shasum 校验 OK） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s1_apply.json` | 28 行更新，`[x]` 1378 → 1385 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact、无重复 rust_entry |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1385、unrecorded 0、stale 0 |

#### 分片一批次门禁

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 格式 | `cargo fmt --all -- --check` | 通过 |
| Clippy | `pnpm run check:clippy` | 通过 |
| 架构 | `pnpm run check:rust:architecture` | 通过（内联测试外置后 `product_production_ports_trade_requests.rs` 786 行，满足 800 行上限） |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1815 条通过（首轮出现 `adk_session_detail_omits_resolved_approval_groups` 与 `api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 两处已知抖动，隔离复跑后整轮 1815/1815） |
| 工作区测试 | `pnpm run test:rust` | 3222 条通过（2 skipped），较上批 3215 增加本批 7 条 |
| 兼容回放 | `pnpm run check:compatibility` | 通过（API 278 操作、assistant 运行时、desktop 3 profile、provider、sqlite） |
| 生成物 | `pnpm run check:generated` | 通过（未改工作树） |
| AI 上下文 | `pnpm run check:ai-context` | 通过（6 模块 8 文件） |
| Zero-Go | `pnpm run check:zero-go` | 通过（2941 文件、0 发布产物） |
| 快速门禁 | `pnpm run check:quick` | 通过。首跑因 `target/debug/deps` 的 `.rcgu.o` 超过 50000 触发 `check:rust:target-health` 失败；确认无 Cargo 进程后执行 `pnpm run clean:rust:artifacts`（清理 136057 文件 / 36.4GiB）并复跑通过 |
| 空白检查 | `git diff --check` | 通过 |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`RUSTSEC-2026-0285` 与 `deny.toml` 8 条 `advisory-not-detected`，与本批无关，如实记录 |

### 第一百二十六批 分片二 a：`internal/store/trading/` 乱序/信用家族 13 行（6 升 `[x]`）

范围：`internal/store/trading/` 共 **44** 条 pending（分片一提交时的口径修正：`submission_safety_test.go:117`
早已是 `[x]`，不在 pending 内）。本片处理乱序与成交信用家族 13 行：`broker_ledger_test.go` 3、
`broker_fill_reconciliation_test.go` 3、`out_of_order_reconciliation_test.go` 5、`submission_safety_test.go` 2；
其余 31 行（`startup_compatibility_test.go` 7、`ledger_test.go` 5、`ledger_lifecycle_test.go` 5、
`execution_composition_test.go` 4、`maintenance_concurrency_test.go` 2、`persistence_failures_test.go` 2、
`snapshot_normalization_test.go` 2、`fill_retention_test.go` 1、`order_leg_merge_test.go` 1、
`persistence_query_plan_test.go` 1、`resource_test.go` 1）留待分片二 b。

#### 关键事实：成交信用算法（Go 两遍扫描 vs Rust 原一遍扫描）

Go `internal/store/trading/broker_ledger.go::brokerSnapshotCoveredFillQuantityLocked` 分两遍：
先取“自身时间戳能覆盖来件成交”的最大快照累计量 `best/bestAt`，再累加 `BROKER_FILL_RECEIVED` 中
`brokerEventCanCoverFill(bestAt, filledAt)` 为真的已记账成交量，最后 `credit = clamp(best-known, 0, fill.qty)`。
Rust `execution_reconciliation_discovery.rs::covered_by_snapshot` 原实现只扫一遍，并把“已记账成交”
与**来件成交时间**比较（而非快照时间），于是同一场景下信用被算成 10（Go 为 6）——即迟到快照会吞掉真实成交。
另外原实现对 `bestAt` 为空直接返回 0，而 Go 对空/畸形时间戳保守覆盖（返回 credit）。

#### 生产修复

- 复现条件：快照事件 `BROKER_PUSH_UPDATED {filledQuantity:10, updatedAt:03:30}` + 已记账成交
  `BROKER_FILL_RECEIVED {filledQuantity:4, filledAt:02:30}` + 来件成交 `create_time=02:00, qty=10`
  → 修复前 credit=10（应 6）；空时间戳快照 → 修复前 credit=0（应 4）。
- 修复：`covered_by_snapshot` 改为与 Go 同形的两遍扫描，`found_snapshot` 标记替代 `best_at.is_empty()` 早退，
  已记账成交按快照时间判定，`filledQuantity<=0` 跳过、缺失 `filledAt` 视为保守覆盖。
- 探针：仅把第二遍判定改回 `time_after(at, fill_at)` 即复现 `left: 10.0, right: 6.0` 失败；按字节还原后
  文件 shasum `2e0b585e3771c39af90a84ee15b7e92b5abc7aa85ff21777f8b8f289b61f8542` 校验通过并转绿。

#### 新增 Rust 测试（3 条）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `product_production_ports_execution_reconciliation_tests.rs::reconciliation_snapshot_coverage_caps_partial_and_exhausted_credit` | `broker_ledger_test.go:10` | 信用封顶 2 / 部分 6 / 耗尽 0 |
| `...::reconciliation_snapshot_coverage_timestamp_boundaries_are_conservative` | `broker_ledger_test.go:50` | 空、畸形、更旧、同刻四种时间戳边界 |
| `product_production_ports_execution_reconciliation_order_state_tests.rs::reconciliation_older_snapshot_fill_progress_is_accepted_with_monotonic_updated_at` | `out_of_order_reconciliation_test.go:96` | 更旧时间戳但成交推进被接受、`updatedAt` 不回退、事件 +1 |

另为既有证据补 `// Parity: go:452dea11:...` 锚点：`reconciliation_rejects_terminal_and_partial_status_regressions`
（`out_of_order_reconciliation_test.go:43`）、`reconciliation_cancel_submitted_resolves_fill_and_cancel_confirmation`
（`:178`）、`reconciliation_duplicate_terminal_snapshot_is_idempotent`（`:227`）。

#### 结论变更（6 升 `[x]`、7 行收紧为可执行缺口）

- 升 `[x]`：`broker_ledger_test.go:10/:50`、`out_of_order_reconciliation_test.go:43/:96/:178/:227`。
- 收紧 `partial`：`broker_fill_reconciliation_test.go:11/:97/:144`、`broker_ledger_test.go:65`、
  `out_of_order_reconciliation_test.go:140`、`submission_safety_test.go:13/:63` —— 每条都写明缺口、
  owner（发现/应用路径、费用守卫、`applied==0` 分支、预留与预览哈希比对）与所需回归断言。
- 本片未触碰的 31 行留在分片二 b，保持原有 `partial`/`boundary` 结论。

#### 分片二 a 验证记录与门禁

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 先红回归 | `... -E 'test(reconciliation_snapshot_coverage)'` | 修复前 `caps_partial_and_exhausted_credit` 报 `left: 10.0, right: 6.0`、`timestamp_boundaries` 报空时间戳信用 0，均失败 |
| 探针 | 仅改回“已记账成交按来件成交时间比对” | 复现 `10.0 != 6.0`；按字节还原 shasum `2e0b585e3771c39af90a84ee15b7e92b5abc7aa85ff21777f8b8f289b61f8542` 校验通过并转绿 |
| 修复后证据 | `... -E 'test(reconciliation_snapshot_coverage) or test(reconciliation_older_snapshot_fill_progress) or test(reconciliation_deduplicates_same_fill) or test(conformance_partial_full_fill)'` | 6 条通过（含既有覆盖/去重/一致性用例，无回归） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s2a_apply.json` | 13 行更新，`[x]` 1385 → 1391 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact、无重复 rust_entry |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1391、unrecorded 0、stale 0 |
| 格式 / 架构 / Clippy | `cargo fmt --all -- --check`、`pnpm run check:rust:architecture`、`pnpm run check:clippy` | 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1818 条通过（本片 +3） |
| 工作区测试 | `pnpm run test:rust` | 3225 条通过（2 skipped） |
| 兼容回放 / 生成物 / AI 上下文 / Zero-Go | `check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go` | 通过（zero-go 2942 文件） |
| 快速门禁 / 空白 | `pnpm run check:quick`、`git diff --check` | 通过 |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`RUSTSEC-2026-0285` + 8 条 `advisory-not-detected`，如实记录 |

### 第一百二十六批 分片二 b-1：`internal/store/trading/` 查询计划/序号/兼容性 5 行（升 `[x]`）

范围：分片二 b 剩余 31 行中的 5 行——`persistence_query_plan_test.go:12`、
`persistence_failures_test.go:58`、`maintenance_concurrency_test.go:43`、
`startup_compatibility_test.go:31`、`startup_compatibility_test.go:73`。本片全部是**补覆盖证据**：
沿用既有生产实现（无功能差异），把此前只标 `partial`/`boundary` 的 Go 行为补成可执行 Rust 断言。

#### 新增 Rust 测试（5 条，全部落在 `execution_order_store_contracts.rs`）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `execution_order_events_load_in_per_order_chronology_without_a_temp_sort` | `persistence_query_plan_test.go:12` | 乱序写入 a/b 两单事件后按 `created_at,id` 读回；`EXPLAIN QUERY PLAN` 命中 `idx_execution_order_events_order` 且无 `USE TEMP B-TREE` |
| `execution_order_sequence_high_water_marks_survive_reopen` | `persistence_failures_test.go:58` | 生产 store 写入 orders=41/events=42，重开后 `get_sequence` 读回原值且 `next_sequence("orders")` 依次 42、43 |
| `execution_order_concurrent_writes_and_reads_survive_reopen` | `maintenance_concurrency_test.go:43` | 24 写线程 + 并发读，`order_count()==24`；drop 重开仍 24 |
| `execution_order_legacy_metadata_is_rejected_without_mutating_the_file` | `startup_compatibility_test.go:31` | 种子库 metadata 降级为 v1 → `open_existing` 返回 `Schema(_)`，拒绝前后文件逐字节相等 |
| `execution_orders_store_rejects_directory_and_missing_parent_paths` | `startup_compatibility_test.go:73` | 目录路径 → `NotRegularFile(_)`；父目录缺失 → 打开失败 |

#### 语义说明：序号 API 形状差异

Go 的 `persistence.persistSequence(name, 41)` 写的是“下一次要分配的序号”，`loadFromDB` 后 `nextOrderSeq == 41`。
Rust `ExecutionOrderStore` 的 `execution_sequences.value` 是**已分配高水位**，`next_sequence` 在同一事务里
`value = value + 1` 后返回（存储值+1）。因此 Rust 断言写为 `get_sequence == 41` 且
`next_sequence` 依次 `42/43`，与 Go “持久高水位驱动下一次分配”的行为等价，而非逐字对齐数值。

#### 本片编译修正

初稿误用测试专用 `ExecutionOrderTestCutoverStore`，该类型只暴露 `next_sequence`。改为生产
`ExecutionOrderStore::open`（暴露 `get_sequence`/`set_sequence` 且共用同一 `execution_sequences` 表）后编译并转绿，
未改动任何生产代码。

#### 分片二 b-1 验证记录与门禁

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 聚焦用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --locked --test execution_order_store_contracts` | 9 条通过（本片 +5） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s2b1_apply.json` | 5 行更新，`[x]` 1391 → 1396，missing 0 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1396、unrecorded 0、stale 0、unknown 54 |
| 格式 / 架构 / Clippy | `cargo fmt --all -- --check`、`pnpm run check:rust:architecture`、`pnpm run check:clippy` | 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --all-targets --locked --no-fail-fast` | 见提交记录 |
| 工作区测试 | `pnpm run test:rust` | 3237 条通过（本片 +7，2 skipped） |
| 兼容回放 / 生成物 / AI 上下文 / Zero-Go | `check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go` | 通过 |
| 快速门禁 / 空白 | `pnpm run check:quick`、`git diff --check` | 通过 |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`RUSTSEC-2026-0285` + 8 条 `advisory-not-detected`，如实记录 |
### 第一百二十六批 分片二 b-2：`internal/store/trading/` 剩余 26 行收口（6 升 `[x]`，10 partial，10 boundary）

范围：分片二 b 剩余全部 26 行 —— `startup_compatibility_test.go` 5、`ledger_test.go` 5、
`ledger_lifecycle_test.go` 5、`execution_composition_test.go` 4、`maintenance_concurrency_test.go` 1、
`persistence_failures_test.go` 1、`snapshot_normalization_test.go` 2、`fill_retention_test.go` 1、
`order_leg_merge_test.go` 1、`resource_test.go` 1。本片新增 7 条测试（无生产改动：差异均为结构差异或
已由既有实现覆盖），并把不可迁移的行为逐条写清缺口、owner 与回归要求。至此
`internal/store/trading/` 全量 45 行中 18 行 `[x]`，其余 27 行为结论明确的 partial/boundary（17 partial + 10 boundary）。

#### 新增 Rust 测试（7 条）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `product_data_management_batch_atomic_startup_tests.rs::execution_order_database_path_resolution_trims_override_and_derives_settings_sibling` | `startup_compatibility_test.go:14` | 覆盖值 trim 生效、空覆盖回落 settings 同级默认名、裸 settings.json 保留裸默认名 |
| `execution_order_store_contracts.rs::execution_orders_store_rejects_partial_legacy_schema_with_rebuild_guidance` | `:87` | 残表被拒 → `Schema(_)` 且消息含 rebuild |
| `...::execution_orders_store_rejects_wrong_column_layout` | `:105` | 四张运行时表错列布局 → `Schema(_)` |
| `...::execution_orders_store_rejects_missing_runtime_tables` | `:156` | 逐张 DROP 四张运行时表 → 打开即拒（`Schema(_)`） |
| `...::execution_orders_store_constructor_rejects_empty_missing_and_malformed_inputs` | `persistence_failures_test.go:14` | 空路径/缺失文件/畸形 metadata/缺表 四类构造失败分类 |
| `...order_state_tests.rs::reconciliation_snapshot_keeps_preview_locked_product_and_quantity_mode` | `ledger_lifecycle_test.go:110` | 快照仍应用（FILLED、成交量 5）但 orderKind/productClass/quantityMode/previewId 不被降级 |
| `...order_state_tests.rs::reconciliation_snapshot_repairs_sparse_order_identity_diagnostics_and_economics` | `ledger_test.go:150`（partial 证据） | 稀疏订单被快照补全（brokerOrderIDEx/symbol/side/type/数量/价格/均价/remark），清除 lastErrorCode；同时断言 Rust 保留下单来源（未做 broker 来源提升） |

#### 结构差异结论（本片新增/收紧）

- **持久化 worker/队列**（`ledger_test.go:236`、`ledger_lifecycle_test.go:131`）：Rust 无异步写队列与显式 Close，
  写入在 store 互斥内同步事务提交 → boundary。
- **placed-merge 家族**（`ledger_test.go:65`、`execution_composition_test.go:32/:313`）：Rust 以 clientOrderId 唯一索引 +
  预留重放身份防重，没有合并/来源提升语义；`source` 恒由写入方标注（api/strategy）→ boundary/partial。
- **seen-fill 持久化与保留期**（`ledger_test.go:330`、`fill_retention_test.go:8`）：Rust 去重只在单次对账扫描内，
  `execution_seen_fills` 仅存在于 schema；保留期仅实现 settings 归一化（默认 90 / clamp 3650）→ partial，
  owner `jftrade-store-sqlite` + `jftrade-engine`。
- **快照归一化边界**（`snapshot_normalization_test.go:11/:44`、`ledger_lifecycle_test.go:12/:41`）：Rust 对账不写
  orderKind/productClass/quantityMode 与 source/sourceDetail；`time_after` 对空/畸形时间戳返回 false（保守拒绝），
  与 Go 的 merge 默认值/保守推进不同形 → partial/boundary。
- **维护 busy 语义**（`maintenance_concurrency_test.go:13`）：Rust 维护为管理库级 `ManagedDatabaseMaintenanceStore` +
  WriterLease 栅栏，未按执行订单终态计算 busy → partial，缺口（非终态订单不阻塞维护）已登记 owner。
- **费用父单落账**（`ledger_lifecycle_test.go:73`）：Rust 订单级 fees 由 `apply_fee_snapshot` 写、无 leg 费用写入路径，
  结构上不会虚构分摊；缺"1.25+0.75=2.00、重复快照 no-op、leg fees 为 NULL"三条等价断言 → partial。
- **nil/序号后缀**（`startup_compatibility_test.go:131`）、**leg 合并**（`order_leg_merge_test.go:10`）、
  **资源 Close**（`resource_test.go:10`）：Go 专属模型，Rust 分别以 Option/单事务/RAII 表达 → boundary。

#### 分片二 b-2 验证记录与门禁

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增 store 用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --locked --test execution_order_store_contracts` | 13 条通过（本片 +4） |
| 新增 engine 用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked -E 'test(execution_order_database_path_resolution) or test(reconciliation_snapshot_keeps_preview_locked_product) or test(reconciliation_snapshot_repairs_sparse_order_identity)'` | 3 条通过（本片 +3） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s2b2_apply.json` | 26 行更新，`[x]` 1396 → 1402 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact、无重复 rust_entry |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1403、unrecorded 0、stale 0、unknown 54 |
| 格式 / 架构 / Clippy | `cargo fmt --all -- --check`、`pnpm run check:rust:architecture`、`pnpm run check:clippy` | 通过 |
| 受影响 crate | `cargo-nextest -p jftrade-store-sqlite` / `-p jftrade-engine --all-targets` | 186 / 1821 条通过（引擎首轮命中已知抖动 `adk_session_detail_omits_resolved_approval_groups`，隔离复跑通过后整轮 1821 全绿） |
| 工作区测试 | `pnpm run test:rust` | 3237 条通过（本片 +7，2 skipped） |
| 兼容回放 / 生成物 / AI 上下文 / Zero-Go | `check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go` | 通过 |
| 快速门禁 / 空白 | `pnpm run check:quick`、`git diff --check` | 通过 |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`RUSTSEC-2026-0285` + 8 条 `advisory-not-detected`，如实记录 |
### 第一百二十六批 分片三 a-1：`internal/app/apiserver/tradingapp/` 13 行 + `servercoretest/contract_test.go` 6 行（7 升 `[x]`）

范围：第 126 批分片三的第一片——`tradingapp` 全部 13 条 pending 与 `servercoretest/contract_test.go` 6 条
（system/settings/markets/broker-runtime/strategy-definitions/backtests 契约）。本片新增 2 条 Rust 测试、
为 5 条既有测试补 `// Parity:` 锚点，7 行升 `[x]`，其余 12 行收紧为 11 partial + 1 boundary（每条含缺口 owner 与回归要求）。

#### 新增 Rust 测试（2 条）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `product_production_ports_execution_order_validation_tests.rs::combo_order_quantity_mode_maps_event_parlays_to_amount_and_option_combos_to_contracts` | `tradingapp/execution_gateway_boundaries_test.go:10` | 期权组合解析后 quantity_mode=contracts、事件组合=amount |
| `product_production_ports_trade_tests.rs::broker_runtime_route_keeps_descriptor_session_and_accounts_keys` | `servercoretest/contract_test.go:165` | `/api/v1/brokers/futu/runtime` 返回体含 descriptor/session/accounts 三键 |

#### 锚点补记（5 条既有测试升 `[x]` 的证据）

- `product_appearance_read_tests.rs::appearance_read_route_matches_go_fixture_for_all_seed_documents` → `contract_test.go:90`
  （逐 seed 重放 `/settings/ui`，断言 ok 信封 + data.appearance 且不写文件）。
- `product_market_data_catalog_read_tests.rs::market_data_catalog_read_routes_match_group_fixture_in_cutover_only` → `contract_test.go:123`
  （`markets-ready` 断言 data 全等，含 defaultMarket 与非空 markets）。
- `product_strategy_definitions_tests.rs::strategy_definition_routes_match_group_fixture_in_cutover_only` → `contract_test.go:205`
  （`list-current-only` 的 data 为直接数组并全等）。
- `product_backtests_tests.rs::backtests_read_routes_match_group_fixture_in_cutover_only` → `contract_test.go:240`
  （list / list-empty 断言 data.runs 数组）。
- `product_production_ports_execution_reconciliation_tests.rs::reconciliation_scope_accepts_only_stock_trade_markets` → `tradingapp/order_update_source_test.go:172`
  （基金等非股票市场不产生 scope，等价于跳过 fund-only 账户）。

#### 本片登记的关键缺口

- **订单通知消息内容**（`notifications_test.go:32`）：Rust 消息是事件固定短语（“订单 {id} 部分成交”），不含 Go 的
  tradingEnvironment/symbol/side/qty/filled/brokerOrderId 标识拼接 → partial，owner `jftrade-engine` 通知投影 + `jftrade-trading`。
- **ExecutionGateway 分支表**（`execution_gateway_lifecycle_test.go:144/:202/:298`）：Rust 无 app 层 gateway 聚合对象，
  broker 不匹配、prepare 错误透传、stale/fresh 落库差异、组合撤单缺标识/非组合 broker 等分支没有 1:1 断言 → partial，
  owner `jftrade-engine` 下单/撤单写路径 + `jftrade-integration-futu` 组合能力探测。
- **订单更新源降级 shape**（`order_updates_test.go:92`）：Rust 读路由 fail-closed，但没有 `ErrOrderUpdateSourceInactive`
  等价错误码与“可 Stop 空订阅”对象 → partial，owner `jftrade-engine` 读端口就绪状态与推送 worker。
- **`/system/status` 组合契约**（`contract_test.go:13`）：稳定字段与 runtimeResources 分别有测试，但没有一条测试同时断言
  信封必填字段 + `execution-orders-db(trading)` 资源存在（冻结 fixture 不含该路径）→ partial，owner `jftrade-engine` 系统状态投影。
- **worker 构造与查询归一**（`order_updates_test.go:13/:70`）：Go 的 NewOrderUpdatesWorker 与 brokerOrderQuery 逐字段 trim
  在 Rust 没有同形对象（对账端口 + 身份解析 scope）→ boundary，附升级路径说明。

#### 分片三 a-1 验证记录与门禁

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked -E 'test(combo_order_quantity_mode_maps_event_parlays) or test(broker_runtime_route_keeps_descriptor_session_and_accounts_keys)'` | 2 条通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s3a1_apply.json` | 19 行更新，`[x]` 1402 → 1409 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact、无重复 rust_entry |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1410、unrecorded 0、stale 0、unknown 54 |
| 格式 / 架构 / Clippy | `cargo fmt --all -- --check`、`pnpm run check:rust:architecture`、`pnpm run check:clippy` | 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked` | 1823 条通过（本片 +2） |
| 工作区测试 | `pnpm run test:rust` | 3239 条通过（本片 +2，2 skipped） |
| 兼容回放 / 生成物 / AI 上下文 / Zero-Go | `check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go` | 通过 |
| 快速门禁 / 空白 | `pnpm run check:quick`、`git diff --check` | 通过 |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`RUSTSEC-2026-0285` + 8 条 `advisory-not-detected`，如实记录 |

#### 后续（分片三 a-2）

`servercoretest` 余下 broker/exec 家族约 30 条：`broker_new_test.go` 18（funds/quote/klines/securities/unlock/place/cancel/
JSON 形状/路径校验）、`exec_validate_test.go` 4、`execution_routes_test.go` 3、`broker_routes_test.go` 2、`broker_read_test.go` 1、
`exec_routes_test.go` 1、`portfolio_routes_test.go` 1；owner 为 `crates/jftrade-engine` 的 broker/execution 读端口与 wire 投影。
### 第一百二十六批 分片三 a-2：`servercoretest` broker/exec 家族 30 行（5 升 `[x]`，5 行收紧，20 行复核维持）

范围：`broker_new_test.go` 18、`broker_read_test.go` 1、`broker_routes_test.go` 2、`exec_routes_test.go` 1、
`exec_validate_test.go` 4、`execution_routes_test.go` 3、`portfolio_routes_test.go` 1。本片新增 3 条生产 HTTP 断言、
为 3 条既有测试补锚点，5 行升 `[x]`；5 行按新证据收紧（broker 断线 shape 与 Go 的差异）；其余 20 行逐条复核后维持
原有 partial/boundary 结论（owner 与回归要求不变）。

#### 新增 Rust 测试（3 条，均为生产 HTTP 装配层）

| Rust 测试 | Go 基线 | 断言要点 |
| --- | --- | --- |
| `production_http_broker_klines_requires_symbol_with_go_message` | `broker_new_test.go:167` | `/klines?period=1d` 缺 symbol → 400 + BAD_REQUEST + `query parameter symbol is required` |
| `production_http_broker_securities_requires_symbol_with_go_message` | `:193` | `/securities` 缺 symbol → 400 + BAD_REQUEST + 同消息 |
| `production_http_broker_reads_reject_invalid_query_shapes_with_rust_messages` | `:205`（partial 证据） | 9 条非法 query 矩阵：400 + BAD_REQUEST + Rust 消息（clearingDate/orderIdEx/scope/price/adjustSideAndLimit/positionId/limit） |

#### 锚点补记

- `production_http_broker_projection_fails_closed_and_validates_before_runtime` → `:141`（quote 缺 symbol：400 + 同消息；同用例断言无行情路由时 503 fail-closed）。
- `broker_funds_response_serializes_the_contract_keys_with_null_last_error` → `:388`（funds 六键集合完全相等 + lastError 显式 null）。
- `portfolio_read_routes_match_group_fixture_in_cutover_only` → `portfolio_routes_test.go:71`（degraded 空态：200 + connectivity=degraded + balances=[] 全等）。

#### 关键事实：broker 断线 shape 的 Go/Rust 差异（P1，本片新登记）

Go 的 `servercoretest` 断言“未配置/断开 broker 时读路由返回 200 + connectivity=degraded|disconnected”：
funds（`:100`/`:388`）、quote 无租约（`:126`）、klines（`:155`）、securities（`:181`）、valid 断线 shape（`:237`）。
Rust 侧：

- 冻结 fixture（`broker-read.json`、`portfolio-read.json`）重放了这些 degraded 形状，但由**测试端口**驱动；
- 生产 HTTP 在缺少行情路由/行情运行时时对 quote 返回 503 `BROKER_READ_UNAVAILABLE`
  （`production_http_broker_projection_fails_closed_and_validates_before_runtime`），即**失败关闭**而非 200 降级信封；
- 没有“无 broker 时 funds/klines/securities 返回 200 degraded/disconnected”的生产断言。

因此 `:100`、`:126`、`:155`、`:181` 收紧为 partial（缺口 owner = `crates/jftrade-engine` broker read 绑定与各读路由；
回归要求 = 确认产品语义后补生产 HTTP 断言或显式记录为有意差异）。`:205` 收紧为 partial：9 条路径的 status/code 已对齐，
但 Go 消息内嵌 `strconv` 解析细节（如 `strconv.ParseFloat: parsing "abc": invalid syntax`），Rust 简化为
`query parameter price is invalid`；owner = 请求解析层（`product_production_ports_trade_requests.rs`）。

#### 复核维持的 20 行（结论不变）

`broker_new_test.go:237/:264/:286/:302/:319/:336/:359/:422/:462`、`broker_read_test.go:72`、
`broker_routes_test.go:14/:61`、`exec_routes_test.go:18`、`exec_validate_test.go:18/:88/:153/:211`、
`execution_routes_test.go:53/:110/:155`。逐条复核要点：断线/非法 payload 的 wire 分支多由端口级或 fixture 级用例覆盖，
缺少 Go 的 app 层逐分支表；执行下单校验的 4 条引用的 `test_normalize_execution_order_*` 用例已绑定到
`internal/api/trading/*` 的 [x] 行（entry 唯一性限制），故维持在 partial 并在结论中记录证据来源。

#### 分片三 a-2 验证记录与门禁

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked -E 'test(production_http_broker_klines_requires_symbol_with_go_message) or test(production_http_broker_securities_requires_symbol_with_go_message) or test(production_http_broker_reads_reject_invalid_query_shapes_with_rust_messages)'` | 3 条通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s3a2_apply.json` | 10 行更新，`[x]` 1409 → 1414 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺少 function_exact、无重复 rust_entry |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1416、unrecorded 0、stale 0、unknown 54 |
| 格式 / 架构 / Clippy | `cargo fmt --all -- --check`、`pnpm run check:rust:architecture`、`pnpm run check:clippy` | 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked` | 1826 条通过（本片 +3） |
| 工作区测试 | `pnpm run test:rust` | 3242 条通过（本片 +3，2 skipped） |
| 兼容回放 / 生成物 / AI 上下文 / Zero-Go | `check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go` | 通过 |
| 快速门禁 / 空白 | `pnpm run check:quick`、`git diff --check` | 通过 |
| 依赖策略 | `pnpm run check:rust:policy` | **未通过（既有阻断）**：`RUSTSEC-2026-0285` + 8 条 `advisory-not-detected`，如实记录 |

#### 后续（分片三 b）

`servercoretest` 其余约 59 条：settings/broker 与 onboarding 11、backtest_runs/provider 8、market_depth 4、
openapi_snapshot 4、server_definitions 6、system/runtime 8、strategy_logs/preview/sync 5、watchlist/research 4、
frontend/installers/plugin/contract 余量等；owner 为 `crates/jftrade-engine` 对应投影 + `crates/jftrade-api` wire 层。
