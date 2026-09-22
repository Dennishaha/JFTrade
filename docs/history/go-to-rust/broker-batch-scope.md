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
