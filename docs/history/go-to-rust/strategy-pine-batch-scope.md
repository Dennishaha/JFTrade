# Strategy、Pine Runtime 领域对齐批次

本批范围覆盖策略错误分类、broker account 规范化、indicator interval
fallback、catalog activity 空页，以及 catalog runtime 恢复的 2 条生命周期
测试。逐项核对 Go 源码断言与 Rust 函数后，3 条基础 helper/错误测试保留
`function_exact`；2 条 runtime reconciliation 新增真实 Rust 入口但保留
`[~]`/`partial`，因为 Rust 未覆盖 Go 的审计详情、save/changed 计数和二次
调用幂等等断言。

| Go 测试 | Rust 证据 | 状态 |
| --- | --- | --- |
| `internal/strategy/errors_test.go:8` | `jftrade-strategy::model::test_classified_strategy_errors_match_sentinel_kinds` | `[x]`，错误类别与消息均有断言 |
| `internal/strategy/instancebinding/binding_test.go:137` | `jftrade-strategy::model::test_normalize_broker_account_drops_empty_input` | `[x]`，空值与 None 边界有断言 |
| `pkg/strategy/indicatorwarmup/warmup_internal_test.go:142` | `jftrade-strategy::pine::planner::test_resolve_interval_minutes_supports_broker_intervals_and_safe_fallbacks` | `[x]`，broker interval 与 fallback 矩阵一致 |
| `internal/strategy/catalog/activity_degraded_test.go:65` | `jftrade-engine::strategy_runtime_activity::test_catalog_activity_returns_empty_pages_when_activity_store_is_unavailable` | `[x]`，空日志/审计页与 page metadata 一致 |
| `internal/strategy/catalog/runtime_reconciliation_business_test.go:52` | `strategy_runtime_port::restore_invalid_running_binding_marks_instance_failed` | `[~]`/`partial`，缺 stopped 保持、审计详情和错误日志内容 |
| `internal/strategy/catalog/runtime_reconciliation_business_test.go:80` | `strategy_runtime_port::restore_running_instances_ignores_paused_and_stopped_instances` | `[~]`/`partial`，缺 stale running/paused 重置、save 次数和二次幂等 |

验证命令已逐条写入 `manual-test-mappings.json`，使用 `jftrade-strategy` 和
`jftrade-engine` 的 nextest wrapper。后续应在所属 runtime/store 领域先补失败
回归测试，再考虑将两条 partial 升级为 function_exact。

## 批次：Pine public helper guard 功能补齐

本批处理 `pkg/strategy/pine/parse_test.go` 中最关键的一组公共入口保护：
编译期拒绝 JFTrade 内部 helper，并给出 Pine v6 替换建议。

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `parse_test.go:54 TestCompileRejectsPublicInternalHelperCalls` | `jftrade-strategy::pine::public_helper_guard_tests::compile_rejects_public_internal_helper_calls` | `[x]`：`ma/bollinger/cross_over/cross_under/notify` 等调用使编译失败并输出替换建议。 |
| `parse_test.go:84 TestAnalyzeScriptReportsPublicInternalHelperDiagnostics` | `...::analyze_script_reports_public_internal_helper_diagnostics` | `[x]`：`PINE_INTERNAL_HELPER_PUBLIC` 与 `PINE_PUBLIC_TA_SHORTCUT` 两个稳定诊断码、第 3 行定位、替换文案与 Go 一致。 |

### 发现并修复的真实功能缺失

- 复现：`//@version=6` + `strategy(...)` + `fast = ma(EMA, 14)`。
- 修复前：Rust `semantic.rs::visit_call` 只报
  `function "ma" is not supported by the Pine v6 runtime`，没有 Go 的
  `PINE_INTERNAL_HELPER_PUBLIC` 诊断码，也没有可执行的迁移建议。
- 预期（Go `public_helper_guard.go`）：内部 helper 报
  `PINE_INTERNAL_HELPER_PUBLIC`，`ta.adx` 报 `PINE_PUBLIC_TA_SHORTCUT`，
  消息中包含 `use Pine v6 ...`。
- 修复位置：`crates/jftrade-strategy/src/pine/semantic.rs` 新增
  `public_helper_guard`（60 条 Go helper 名单 + `ta.adx` 快捷方式），并在
  `visit_call` 里先于通用 unsupported 诊断触发。
- 回归：上述两条测试，以及该 crate 全量 22 项测试。

### 后续

`pkg/strategy/pine/parse_test.go` 剩余 33 项（编译 IR 降级、订单元数据、
多 bar 历史引用、v12/v13 指标、UDF/静态 for、诊断边界等）继续保持
`[~]`，将在后续批次逐条核对。

## 批次：Pine IR 降级与策略默认数量

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `parse_test.go:10 TestParseScriptLowersPineStrategyToIR` | `jftrade-strategy::pine::parse_ir_tests::parse_script_lowers_pine_strategy_to_ir` | `[x]`：单一 `on_kline_close` hook、3 条语句、第 3 条 if 的条件与 then/else 结构与 Go 一致。 |
| `parse_test.go:126 TestCompileUsesStrategyDefaultQuantityForEntryWithoutQty` | `...::compile_uses_strategy_default_quantity_for_entry_without_qty` | `[x]`：`default_qty_mode=percent_of_equity`、`default_qty_value=10`、`pyramiding=2`。 |

### 契约差异（已固化在测试注释中）

- Rust 的表达式渲染不插入空格（`ta.crossover(fast,slow)`），Go 为
  `cross_over(fast, slow)`；测试做归一化后比较。
- Rust 的 `LoweredStatement::Action.arguments` 保留命名参数的
  `qty=1` 等值表达式，Go 的 `OrderStmt` 直接存 `QuantityExpression`；
  测试断言“存在 qty 值”，并保留结构差异说明。

## 批次：Pine 元数据、历史引用与显式数量

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `parse_test.go:145 TestCompileParsesBacktestStrategyMetadata` | `parse_metadata_tests::compile_parses_backtest_strategy_metadata` | `[x]`：初始资金、佣金、滑点、收盘处理全部一致。 |
| `parse_test.go:186 TestCompileExplicitEntryQtyOverridesStrategyDefaultQuantity` | `parse_metadata_tests::compile_explicit_entry_qty_overrides_strategy_default_quantity` | `[x]`：显式 qty 覆盖策略默认。 |
| `parse_test.go:232 TestCompileSupportsMultiBarHistoryReferences` | `parse_history_reference_tests::compile_supports_multi_bar_history_references` | `[x]`：四个多 bar 引用在 lowering 后保留。 |
| `parse_test.go:167 TestCompilePreservesOrderNotificationMetadataAndImmediateClose` | `lower.rs::lower_statement` | `[~]`/boundary：Rust `Action` 无 typed Comment/AlertMessage/DisableAlert/Immediate 投影。 |

### 发现并修复的真实功能差异

- 复现：`commission_type=strategy.commission.percent`。
- 修复前：Rust 原样保留 `strategy.commission.percent`。
- 预期（Go `normalizeStrategyCommissionType`）：剥离 `strategy.commission.`
  前缀并规范为 `percent`，未知值不加警告即忽略。
- 修复位置：`crates/jftrade-strategy/src/pine/lower.rs::lower_metadata`。
- 回归：`compile_parses_backtest_strategy_metadata`。


## 批次：request.security 语义校验对齐

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `pkg/strategy/pine/parse_test.go:113 TestCompileAcceptsNativePineIndicatorPublicEntry` | `jftrade-strategy::pine::request_security_tests::compile_accepts_native_pine_indicator_public_entry` | `[x]`：`ta.ema`/`ta.bb` 与 `request.security(syminfo.tickerid, "D", ta.sma(close, 20))` 组合脚本编译通过，无 request.security 诊断。 |
| `pkg/strategy/pine/parse_test.go:202 TestValidateScriptRejectsUnsupportedPineRuntimeFeature` | `jftrade-strategy::pine::request_security_tests::validate_script_rejects_unsupported_pine_runtime_feature` | `[x]`：外部 symbol、`alert(...)` 副作用、`lookahead=barmerge.lookahead_on`、`gaps=barmerge.gaps_on` 四个子用例的诊断码、消息关键词与行号全部对齐。 |

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(request_security_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked
```

### 2026-09-16 变更说明

- 复现：Rust 语义层原先完全不检查 `request.security(...)` 的 symbol、
  timeframe、嵌套调用、merge 标志和表达式副作用；Go 的
  `pkg/strategy/pine/validate.go::requestSecurityUnsupportedDiagnostic`
  会按固定顺序返回六类稳定诊断码。
- 修复位置：`crates/jftrade-strategy/src/pine/semantic.rs`
  新增 `request_security_diagnostic` / `is_supported_request_security_ticker` /
  `request_security_expression_has_side_effect` /
  `request_security_named_argument`，并在 `visit_call` 中先于
  `is_supported_call` 生效，避免 `request.security` 落入通用“不支持函数”分支。
- 支持边界（沿 Go 语义）：symbol 仅接受 `syminfo.tickerid` 及
  `ticker.heikinashi/standard/inherit(...)` 且参数中含 `syminfo.tickerid`；
  timeframe 仅接受字符串字面量或标识符别名；表达式必须是纯表达式。
- 具名参数解析：Rust 解析器把 `lookahead=barmerge.lookahead_on` 解析成
  `ExprKind::Binary { op: Equal }`，`request_security_named_argument`
  将其还原为 `name/value` 文本对，与 Go 的 `name=value` 文本检查等价。
- 未迁移：Go 另有 tuple 宽度/别名匹配（`PINE_REQUEST_SECURITY_TUPLE_*`）、
  可执行 `ta.*` 白名单（`PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED`）与
  timeframe 单位白名单校验；Rust 当前没有等价 tuple 校验入口，保留在后续批次。

## 批次：history 引用、broker boundary 与订单子集编译

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `parse_test.go:253 TestValidateScriptReportsUnsupportedHistoryReferences` | `jftrade-strategy::pine::history_reference_boundary_tests::validate_script_reports_unsupported_history_references` | `[x]`：函数结果 history 与 `close[501]` 两条诊断消息对齐，码为 `PINE_HISTORY_REF_UNSUPPORTED`。 |
| `parse_test.go:464 TestValidateScriptReportsUnsupportedAdvancedOrders` | `jftrade-strategy::pine::advanced_order_diagnostic_tests::validate_script_reports_unsupported_advanced_orders` | `[x]`：trail + stop 混用返回 `PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED`。 |
| `parse_test.go:488 TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics` | `jftrade-strategy::pine::advanced_order_diagnostic_tests::analyze_script_reports_v40_broker_boundary_diagnostics` | `[x]`：OCA×2、qty 冲突、close_all 非法参数、trail bracket、无触发器 exit 六个子用例码与行号对齐。 |
| `parse_test.go:367 TestCompileCapturesStrategyExitSpecificMetadata` | `jftrade-strategy::pine::order_subset_compile_tests::compile_keeps_strategy_exit_named_metadata` | `[x]`：8 个 exit 元数据字段在 lowering 后逐项保留。 |
| `parse_test.go:1044 TestHistoryReferencesIgnoreStringLiterals` | `jftrade-strategy::pine::history_reference_boundary_tests::history_references_ignore_string_literals` | `[x]`：字符串字面量中的 `close[1]` 不触发历史引用诊断。 |
| `parse_test.go:286 / :317 / :348 / :390 / :432 / :448` | `jftrade-strategy::pine::order_subset_compile_tests::compile_accepts_strategy_order_subset_scripts` | `[~]`/partial：脚本可编译且 `strategy.*` 调用与参数保留；Go 的 typed `OrderStmt`/`ExitStmt`/`CancelStmt` 字段投影由 PineTS worker 与 backtest matcher 承担，不在 `jftrade-strategy` IR 中。 |

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(history_reference)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(advanced_order_diagnostic_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(order_subset_compile_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked
```

### 发现并修复的真实功能差异

- 复现 1：`if ta.sma(close, 20)[2] > close`。
  修复前：Rust 静默接受调用结果上的历史引用。
  预期（Go `historyDiagnosticMessage`）：`history references are supported only on
  identifiers or object fields; assign the function result first`。
- 复现 2：`if close[501] > close`。
  修复前：Rust 静默接受超长 lookback。
  预期（Go `maxHistoryLookback = 500`）：`history reference lookback 501 exceeds
  JFTrade maximum 500`。
- 修复位置：`crates/jftrade-strategy/src/pine/semantic.rs`
  新增 `MAX_HISTORY_LOOKBACK` 与 `report_history_reference_diagnostics`，
  在 `ExprKind::Index` 访问时按结构（而非 Go 的正则）判定。
- 复现 3：`strategy.entry(..., oca_name="group")`、`strategy.close(..., qty=1,
  qty_percent=50)`、`strategy.exit(..., stop=..., trail_points=...)`、
  `strategy.exit("Exit", "Long")`、`strategy.close_all(foo=1)`。
  修复前：Rust 只把诊断码写在 pinespec 规格文本里，实际不会抛出。
  预期（Go `diagnosticCodeForCompileMessage` + `validateStrategyExitTriggers`）：
  返回 `PINE_ORDER_OCA_UNSUPPORTED`、`PINE_ORDER_QTY_CONFLICT`、
  `PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED`、
  `PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED` 与 `PINE_COMPILE_ERROR`。
- 修复位置：`crates/jftrade-strategy/src/pine/semantic.rs::strategy_order_diagnostic`，
  在 `visit_call` 中于 `request.security` 分支之前生效；具名参数沿用
  `request_security_named_argument` 的 `name=value` 还原逻辑。
- 边界保留：`strategy.cancel_all` 只在带参数时拒绝，与 Go 一致；OCA/partial
  fill 本身仍属 out-of-scope，不进入可执行分数。

### 未迁移项

- Go 的 typed `ExitStmt`/`OrderStmt`/`CancelStmt` 字段投影（Direction、
  QuantityMode、QuantityExpression、WhenExpression、Immediate、TrailPoints 等）
  在 Rust 归属 PineTS worker（`workers/pineworker/src/pinetsOrderIntents.ts`）
  与 `jftrade-integration-pine`/`jftrade-backtest`，不在 `jftrade-strategy` 的
  lowered IR 中；本轮以 `partial` 记录，不在错误 crate 复制实现。

## 批次：v12/v13 高级指标、风险声明与静态 for 边界

| Go 测试 | Rust 证据入口 | 状态 |
| --- | --- | --- |
| `parse_test.go:560 TestCompileSupportsFrameworkLanguageFeatures` | `jftrade-strategy::pine::framework_language_feature_tests::compile_supports_framework_language_features` | `[x]`：var/reassign mode、三元 `nz(close[1], close)`、if 条件归一化对齐。 |
| `parse_test.go:587 / :621 / :651 / :690` | `jftrade-strategy::pine::advanced_indicator_requirement_tests`（4 个测试） | `[x]`：v12/v13 高级指标 key 与 MTF（`15m` 后缀）key 逐项与 Go 一致。 |
| `parse_test.go:718 TestCompileSupportsAllowEntryInRiskDeclaration` | `jftrade-strategy::pine::risk_declaration_metadata_tests::compile_supports_allow_entry_in_risk_declaration` | `[x]`：`allowed_entry_direction=long`。 |
| `parse_test.go:732 TestCompileSupportsRuntimeRiskDeclarations` | `jftrade-strategy::pine::risk_declaration_metadata_tests::compile_supports_runtime_risk_declarations` | `[x]`：五类风险声明全部投影到 metadata。 |
| `parse_test.go:790 TestCompileSupportsExpressionUDFAndStaticForUnroll` | `jftrade-strategy::pine::udf_and_loop_boundary_tests::compile_accepts_expression_udf_and_static_for_unroll` | `[~]`/partial：UDF 与 typed For 保留；展开/内联由 PineTS worker 承担。 |
| `parse_test.go:838 TestValidateScriptReportsUnsupportedUDFAndStaticForCases` | `jftrade-strategy::pine::udf_and_loop_boundary_tests::validate_script_reports_supported_udf_and_static_for_boundaries` | `[~]`/partial：zero step 与 100 次上限已实现，另有 4 条 Go 校验待补。 |

验证命令：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(advanced_indicator_requirement_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(risk_declaration_metadata_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(udf_and_loop_boundary_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -E 'test(framework_language_feature_tests)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked
```

### 发现并修复的真实功能差异

- 复现 1：`lr = ta.linreg(close, 5, 0)`、`ta.obv`、`ta.pivothigh/pivotlow`、
  `ta.kc/kcw`、`ta.alma`、`ta.cmo`、`ta.tsi`、`ta.correlation`、`ta.dev`、
  `ta.median`、`ta.percentile_*`、`ta.percentrank`、`ta.swma`。
  修复前：`is_supported_call` 直接报 “function ... is not supported”，
  planner 也不产出 requirement。
  预期（Go `planner_indicator_adv.go`）：产出与其完全一致的
  `kind:source:...` key；MTF 形式追加 `:15m` 之类的时间单位后缀。
- 修复位置：
  - `crates/jftrade-strategy/src/pine/semantic.rs`：`is_supported_call` 白名单
    增加 16 个高级指标。
  - `crates/jftrade-strategy/src/pine/planner.rs`：新增 `source_period_parts`、
    `security_inner_binding`、`indicator_time_unit`；`ta.obv` 作为裸成员访问
    也会注册 `obv:close`（以及 MTF 的 `obv:close:15m`）。
- 复现 2：`strategy.risk.allow_entry_in(...)`、`max_drawdown`、`max_intraday_loss`、
  `max_intraday_filled_orders`、`max_position_size`、`max_cons_loss_days`。
  修复前：`strategy.risk.*`（除 allow_entry_in 外）报不支持，metadata 无风险字段。
  预期（Go `strategy_call_helpers.go`）：归一化写入 strategy metadata。
- 修复位置：`crates/jftrade-strategy/src/pine/lower.rs`
  新增 11 个 metadata 字段与 `apply_risk_declarations`；
  `semantic.rs::is_supported_call` 放开五个风险声明函数。
- 复现 3：`for i = 0 to 3 by 0`、`for i = 0 to 100`。
  修复前：无任何诊断。
  预期（Go `static_for_helpers.go`，上限 100）：`for loop step cannot be 0`、
  `for loop expands to more than 100 iterations`。
- 修复位置：`crates/jftrade-strategy/src/pine/semantic.rs`
  新增 `MAX_STATIC_FOR_ITERATIONS`、`constant_int` 与
  `report_static_for_diagnostics`，仅在循环上下界为常量字面量时生效。

### 未迁移项

- Go 的 `for` 静态展开与 UDF 内联（`expandStaticForLoop`、
  `parseExpressionUDF` 等）不在 Rust planner；相应结果为 `partial`，
  不得按 `function_exact` 计入。
- Go UDF 参数数量校验、递归 UDF、循环变量只读、循环内调用结果 history
  四条诊断仍缺 Rust 实现，保留在清单中。

## 第七十九批：`pkg/strategy` 全域收尾（333 条，策略/Pine 域归零）

### 范围与结果

- 范围：`pkg/strategy` 剩余 333 条 `missing`（76 个文件），按 P0（pineworker 进程/客户端生命周期）→ P1（
  Pine 解析/语义/校验、`ir/planner`、`indicatorbinding`、`indicatorwarmup`、`pineengine`/`pinespec`）→ P2（
  Pine 助手与解析器恢复边界、`request.security` 降级、策略调用边界）顺序分 10 片逐条核对。
- 结果：`missing` 2085 → **1752**（本批结清 333 条）、`partial` 846 → **1147**、`boundary` 320 → **352**、
  `[x]` 保持 **1196**。本批 333 条 = **0 `[x]` + 301 partial + 32 boundary**：本批没有新的批准项，
  因此没有新增 `// Parity:` 锚点，未锚定告警保持 193 的前批基线。
- **`pkg/strategy/**` 全域归零**：域内 364 条 = **22 `[x]` + 309 partial + 33 boundary，0 `missing`**。

### 分片执行

- **P0-1（37 条）**：pineworker 客户端/管理器/类型/gRPC 与硬切、就绪恢复。证据面为
  `crates/jftrade-integration-pine/src/{asset,process,pool,readiness,execution/tests,mock_worker}.rs`、
  `crates/jftrade-integration-pine/tests/real_worker_smoke.rs` 与
  `crates/jftrade-settings/src/pine_worker.rs::worker_limits_and_nested_quotes_match_go_settings_owner`。
- **P0-2（30 条）**：`process_launcher*`、`proto_mapping`、`payload_size`、`process_smoke`、
  `runtime_boundaries`。同上进程/池证据面；Go 的性能门（延迟/吞吐）、繁忙队列开关与 NODE_OPTIONS 规则
  没有 Rust 对应实现，按边界保留。
- **P1-1（41 条）**：`pine/parse_collection*`、`parse_object*`、`parse_semantic*`。证据面为
  `crates/jftrade-strategy/src/pine/mod.rs`（compile/validate/analyze 用例）与
  `crates/jftrade-strategy/tests/pine_mcp_contract.rs`（原生管线、语义拒绝、spec/validation 负载）。
- **P1-2（30 条）**：`pine/language_execution_boundaries`、`validation_semantics_boundaries`、
  `parse_request`、`parser_and_lowering_recovery`。
- **P1-3（31 条）**：`indicatorbinding/parse_test`、`parse_semantics` 与 `definition/source_format`（后者引用
  `crates/jftrade-store-sqlite/tests/strategy_definition_store_contracts.rs::strategy_definition_lifecycle_versioning_and_restart_durability`）。
- **P1-4（29 条）**：`ir/planner*` 全部。证据面为 `crates/jftrade-strategy/src/pine/planner.rs::test_resolve_interval_minutes_supports_broker_intervals_and_safe_fallbacks`
  与 compile/validate 用例；Go 的分支/内部边界用例在 Rust 由规划需求断言覆盖。
- **P1-5（28 条）**：`indicatorwarmup/**`。Rust 没有指标配置排序与周期标签格式化层（周期以分钟/枚举表达），
  全部按 partial 或边界记录。
- **P1-6（26 条）**：`pineengine` 16 + `pinespec` 10。证据面为 `pine_mcp_contract.rs` 的 spec 章节冻结、
  validation 负载与保存提示契约用例。
- **P2-1（41 条）**：`pine` 集合/对象/编译器/控制流/表达式等助手文件。
- **P2-2（40 条）**：`pine` 解析器恢复、语义助手、`request.security` 降级与策略调用边界收尾。证据面为
  `crates/jftrade-strategy/src/pine/mod.rs` 的 `udf_and_loop_boundary_tests`、`history_reference_boundary_tests`、
  `framework_language_feature_tests`、`advanced_order_diagnostic_tests`、`request_security_tests`、
  `advanced_indicator_requirement_tests`、`risk_declaration_metadata_tests`、`order_subset_compile_tests`
  与 `crates/jftrade-backtest/{src/indicators.rs,tests/pine_indicator_compatibility.rs}`。

### 新增缺口登记（功能缺失，保留，均为 P2）

- **兼容性评分注册表**：Go `CompatibilityScore()` + `SupportedFeatureIDs()` 返回 v4.0 分模型、5 个维度与
  feature id 注册表；Rust 只在 MCP 负载冻结 `compatibilityScore=98.30`、`scoreModelVersion`、
  `compatibilityDimensions` 与 `supportMatrix`（证据
  `crates/jftrade-engine/tests/strategy_pine_mcp_contract.rs::spec_leaf_preserves_frozen_sections_and_rejects_unknown_section`），
  没有 feature id 注册表。驱动行 `pkg/strategy/pine/parse_test.go:763`。
- **v33 高级语言边界诊断**：递归 UDF、嵌套 UDF、UDF 签名不匹配、循环变量只读四条在 Rust 无实现
  （Rust 已实现循环迭代上限与 step=0 两条）。驱动行 `pkg/strategy/pine/parse_test.go:903`。
- **switch 表达式重写与多语句 UDF 内联**：Rust 编译入口不产出 ifelse/内联 IR，该形态由 PineTS 运行时承担。
  驱动行 `pkg/strategy/pine/parse_test.go:977`。
- **Pine MA 别名表**：Go 映射 `ema/SMA/rma/wma/HMA/vwma → EMA/SMA/SMMA/LWMA/HMA/VWMA`；Rust 侧没有
  SMMA/LWMA/VWMA 标识（别名解析在 worker/需求键层），只有 `ma:EMA:*`/`ma:SMA:*` 需求键断言。
  驱动行 `pkg/strategy/pine/public_lowering_test.go:64`。
- **对象/UDT 解析层**：Go 的构造器/方法参数越界与默认值恢复、重复类型/方法/字段拒绝矩阵在 Rust 编译入口
  没有对应解析器。驱动行 `parser_recovery_boundaries_test.go:104`、`request_security_object_contracts_test.go:86`。
- **request.security 逐表达式纯度助手**：Go 的 `requestSecurityLoweredASTIsPure`、TA mask 与逐指标缺参断言；
  Rust 只有静态日内白名单降级与稳定诊断码。驱动行 `request_security_ast_contracts_test.go:5/26`、
  `security_lowering_test.go:9/92/168`、`request_security_diagnostics_test.go:8/52`。
- **解析器恢复状态与结构化 AST 回退**：Go 的 `parsedLinesFromStructuredAST` 回退、集合/while 循环体无效时
  的 parseState 状态恢复与嵌套深度上限；Rust 没有可变解析状态层。驱动行
  `runtime_and_parser_boundaries_test.go:9/134`。
- **Go 语义助手**：声明签名、导入路径/版本解析、集合类型注解与参数计数、对象/可视化回退分支；Rust 语义
  检查只产出“不支持声明/非法条件”诊断。驱动行 `semantic_helper_boundaries_test.go:8/58/99/128`。
- **pineworker 运维/许可策略**：性能门、繁忙队列、NODE_OPTIONS、AGPL 许可门与 `pinespec` 的 v 版本语言门、
  生成支持快照属于 Go-only 策略，Rust 不迁移，逐条已写入清单结论。

### 跨批 follow-up 汇总

- P0 无新增；P1 = 前批清单不变（本批未发现 P0/P1 级行为差异）。P2 = 前批清单 + 本批登记项：
  兼容评分 feature id 注册表、v33 UDF/循环只读诊断、switch 与 UDF 内联、MA 别名表、UDT 解析层、
  request.security 纯度助手、解析器恢复状态、Go 语义助手。

### 仍未结清（下一批）

- 下一批（第八十批）范围：按域余量排序的下一块 **`pkg/backtest` 237 条**，先按文件分组 recon 再按
  P0 → P1 → P2 分片；其后：`internal/store` 206、`internal/api` 180、`internal/strategy` 169、
  `pkg/bbgo` 145、`internal/integration` 141、`internal/marketdata` 112、`pkg/futu` 86、
  `internal/trading` 80，直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-strategy -p jftrade-integration-pine -p jftrade-backtest -p jftrade-settings --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy -p jftrade-integration-pine -p jftrade-backtest -p jftrade-settings --all-targets --locked --no-fail-fast`（**174 passed / 1 skipped，17 binaries**）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1196 `[x]`**；missing 1752、partial 1147、boundary 352、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）、`pnpm run check:compatibility`、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`、`pnpm run check:ai-context`。

## 第八十三批：`internal/strategy` 全域收尾（169 条，策略域归零）

### 范围与结果

- 范围：`internal/strategy` 剩余 169 条 `missing`（30 个文件），按 P0（策略实例唯一写入所有权、
  `runtimecontrol` 运行控制策略、`instancebinding` 绑定归一化、下单/风险链路与幂等）→ P1（PineTS 运行时
  生命周期、Pine 实时执行器断线重连与取消、`instanceview` 投影）→ P2（`catalog` 归一化/边界、`service`
  只读与 wire DTO）顺序分 5 片逐条核对。
- 结果：`missing` 1129 → **960**（本批结清 169 条）、`partial` 1682 → **1837**、`boundary` 436 → **448**、
  `[x]` 1200 → **1202**。本批 169 条 = **2 `[x]` + 155 partial + 12 boundary**。
- **`internal/strategy/**` 全域归零**：域内 174 条 = **6 `[x]` + 156 partial + 12 boundary，0 `missing`**。

### 分片执行

- **P0-1（38 条）**：`runtimecontrol/*`、`instancebinding/binding_test.go`、`liveruntime/*`。本片把两条运行风险
  用例升级为 `[x]`（`crates/jftrade-trading/tests/risk_engine_tests.rs::runtime_risk_normalizes_modes_and_clears_off_limits`、
  `runtime_risk_off_ignores_configured_limits`，新增 `// Parity:` 锚点），其余按 partial 记录
  Rust 以规范化/白名单表达 Go 的多态可选值。
- **P1-1（35 条）**：`pine_live_executor_test.go`（24）+ `pine_live_command_test.go`（11）。证据面为
  `crates/jftrade-integration-pine/src/{pool,process,execution/tests}.rs` 与
  `crates/jftrade-engine/src/strategy_runtime_owner_tests.rs`（断线重连、取消、有界停止、检查点重放）。
- **P1-2（36 条）**：`pineruntime/*`（24）+ `live_command_business_boundaries_test.go`（12）。Go 的默认 Pine
  模板生成与 worker 启动脚本形态在 Rust 由 embedded bundle/worker 端口承担。
- **P1-3（29 条）**：`liveruntime/manager_boundaries_test.go`、`pineworker_live_business_test.go`、
  `runtime_boundaries_test.go`、`instanceview/*`。Rust 无 manager 维护态字符串与
  `closedKLineSyncInterval` 配置，按部分覆盖记录。
- **P2-1（31 条）**：`catalog/*`（19）+ `service_test.go`（10）+ `types_test.go`（2）。证据面为
  `strategy_runtime_port.rs`、`strategy_runtime_activity.rs`、
  `crates/jftrade-engine/tests/{strategies_write_compatibility,strategy_definitions_write_compatibility}.rs`、
  `crates/jftrade-store-sqlite/tests/{strategy_runtime_store_contracts,strategy_definition_store_contracts}.rs`
  与 `product_*` wire/plugin 用例。本片同时修复 P0 启动对账缺口（见下）并重写两条受影响的映射。

### 发现并修复的真实功能缺失（P0：启动对账漏掉 stale PAUSED）

- 复现条件：策略实例以 `PAUSED` 状态持久化后重启引擎（`restore_running_instances` 只筛选
  `runtime_active || RUNNING`），实例继续以 PAUSED 对外提供服务，而参考实现的
  `ReconcileOnStartup`（`internal/strategy/catalog/lifecycle.go:92`）把 RUNNING 与 PAUSED 一律重置为
  STOPPED 并写审计 `server startup reset stale <status> state to STOPPED`。
- 先红：先把
  `crates/jftrade-engine/src/strategy_runtime_port.rs::startup_reconcile_resets_stale_paused_state_and_keeps_stopped_instances`
  改为断言 PAUSED → STOPPED、`RECONCILED` 审计与 warning 日志、STOPPED 不变且不追加事件，运行
  `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(startup_reconcile_resets_stale_paused_state_and_keeps_stopped_instances)'`
  得到 2 个 target 各 1 例失败（`left: "PAUSED" right: "STOPPED"`）。
- 修复：
  `crates/jftrade-engine/src/strategy_runtime_port.rs` 把 `restore_running_instances` 拆为
  `resume_persisted_running_instance`（RUNNING 恢复路径保持不变：重新获取 demand、启动 worker，失败则收敛
  STOPPED）与新增 `reset_stale_paused_instance`（`update_status(STOPPED)` + warning 日志
  `reconciled strategy state from PAUSED to STOPPED after server startup` + `RECONCILED` 审计），
  调用点 `crates/jftrade-engine/src/product_production_ports.rs:640` 不变。
- 探针：把 PAUSED 分支改成永不匹配后同一命令复现红灯（2 失败），按字节回滚（`cmp` 通过）后复绿
  （2 passed）。
- 回归要求：任何改动 `restore_running_instances`/启动对账的提交必须保持该用例通过，并保持运行中实例的
  恢复语义（重新获取 demand 或收敛 STOPPED），锚点为
  `runtime_reconciliation_business_test.go:80 TestCatalogStartupReconcileResetsStaleRunningAndPausedState`。

### 保留的差异（均为 P2，逐条写在清单结论）

- **`definitionSync` 缺 `blockedReason`**：Rust 投影 `definitionId/appliedVersion/latestVersion/isLatest/canApplyLatest`，
  运行中实例的阻断原因未投影。驱动行 `catalog_boundary_behavior_test.go:84`。
- **活动流读失败 fail-closed**：Go 的 activity store 查询失败降级为空页（found=true），Rust 返回
  `StrategyReadSnapshotError::Unavailable` 由 API 层 fail-closed。驱动行 `catalog_boundary_behavior_test.go:34`。
- **活动流写失败整笔回滚**：Go 允许活动流写入失败而控制状态前进；Rust 与实例状态同库同事务。驱动行
  `catalog_boundary_behavior_test.go:58`。
- **RUNNING 启动语义**：Rust 走恢复路径且不返回 `changed`/`saveCount`，Go 的 servercore 用例断言重启后
  `activeStrategies=0`；差异已在 `runtime_reconciliation_business_test.go:80` 与
  `system_reconcile_strategy_states_test.go:11` 两行登记。
- **无对应模型**：插件 `saveCount`/operation 列表、manager maintenance-busy 原因字符串、
  `closedKLineSyncInterval`、`AppendRuntimeEvent` 通用入口、`ActiveInstrumentIDs` 聚合。
- 本批 12 条 boundary 为 Go-only 默认 Pine 模板与 worker 启动脚本形态，不迁移实现。

### 跨批 follow-up 汇总

- P0：本批 1 项（启动对账漏 stale PAUSED）已修复并留回归测试，关闭前批挂账。
- P1：无新增；P1 分片均以现有 runtime/store 证据 partial 结清。
- P2：前批清单 + 本批登记项（`definitionSync.blockedReason`、活动流降级语义、插件 operation 计数、
  manager 维护态与轮询配置）。

### 仍未结清（下一批）

- 下一批（第八十四批）范围：按域余量排序的下一块 **`pkg/bbgo` 145 条**，先按文件分组 recon 再按
  P0 → P1 → P2 分片；其后：`internal/integration` 141、`internal/marketdata` 112、`pkg/futu` 86、
  `internal/trading` 80、`internal/backtest` 63、`pkg/market` 56、`internal/marketdataassets` 36、
  `internal/watchlist` 31、`pkg/broker` 29，直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine -p jftrade-trading -p jftrade-strategy --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-trading -p jftrade-strategy --all-targets --locked --no-fail-fast`（**1838 passed / 0 skipped**）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1202 `[x]`**；missing 960、partial 1837、boundary 448、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）、`pnpm run check:compatibility`、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（先按 target 健康门执行 `cargo clean`）、`pnpm run check:ai-context`。

## 第 130 批 06B 切片一：internal/strategy/catalog 22 条 recon（2026-09-24）

范围：strategy_pine 域按文件行号升序首片，`internal/strategy/catalog/` 7 文件 22 条（partial 21 + function_exact 1）。
全域基线（audit classifier 口径）：strategy_pine 538 条（[x] 81 + partial 383 + boundary 74），
backtest_calendar 376 条（[x] 69 + partial 262 + boundary 45），合计 914 条。

方法：逐条 `git show 452dea11:<path>` 核对 Go 断言与账本结论，引用 Rust 测试逐一确认存在且断言相符；
引用存在不等于断言等价，过宽 [x] 必须纠正。

结论：21 条 partial  verdict 成立（缺口与修复位置维持原结论，P1 4 项：活动查询降级空页 ×2、删除状态门、启动对账幂等计数；
P2 17 项： Observability 富化、定义同步状态对象等）；1 条 [x]（runtime_reconciliation:52）复核发现引用缺口——
运行失败只对 RUNNING 生效的“停止态零落盘”半侧无显式断言。已补强：
`recovery_failure_converges_the_running_instance_to_stopped` 追加 already-stopped 实例，
断言 reconcile 后其 audit/log 行数不变且状态保持 STOPPED（lib 与集成两 binaries 均过），[x] 维持，账本结论追加补强记录。
无生产代码变更（restore 循环天然跳过 STOPPED，属测试补强）。

验证：`cargo fmt -p jftrade-engine -- --check` 过；定向 nextest（recovery_failure…两 binaries）过；
`audit_test_parity.py --write-report` 须 exit 0；`parity_anchor_reconcile.py` 过；`check:ai-context`、
`check:migration-manifest`、`check:zero-go`、`check:quick`（单实例）、`git diff --check`。

下一片：strategy_pine 余量按文件行号继续（internal/strategy/catalog 之后）。

## 第 130 批 06B 切片二：errors/instancebinding/instanceview 16 条 recon（2026-09-24）

范围：strategy_pine 域按文件行号升序第二片，`internal/strategy/errors_test.go`（1）、
`internal/strategy/instancebinding/binding_test.go`（9）、`internal/strategy/instanceview/`（6），
共 16 条（[x] 3 + partial 13）。

方法：逐条核对 Go 断言与账本结论，[x] 行逐一确认 Rust 测试存在、锚点被识别、断言等价或超集。

结论：3 条 [x] 全部成立——errors:8 四类哨兵等价（Rust 以枚举匹配 + 消息保留翻译 errors.Is，
Display 全串断言为超集）；binding:137 三分支归一等价（Go 单断言，Rust 覆盖空/None/有效三分支）；
view:46 已有先红探针证据。锚点均为无 `go:` 前缀形态，reconciler 三形态均识别，已逐条验证。
13 条 partial 结论与 Go 体相符（instruments 优先、旧 params 回填字段、ApplyParams 规范字段、
chartType 保留与未知值静默清空、审计明细文案、params 回退与 trim），
其中 binding:96 记录在案的行为差异（Rust 未知 chartType 报 400，Go 静默清空）维持 partial。
本片无判定变更、无代码变更，账本仅做只读校验。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、`check:quick`（单实例）、`git diff --check`。

下一片：strategy_pine 余量按文件行号继续（instanceview 之后）。

## 第 130 批 06B 切片三：live_command/manager_boundaries 22 条 recon（2026-09-24）

范围：strategy_pine 域按文件行号升序第三片，
`internal/strategy/live_command_business_boundaries_test.go`（11：
[x] 1 + partial 6 + boundary 4）与
`internal/strategy/liveruntime/manager_boundaries_test.go`（11：partial 11）。

方法：Go 体逐条核对结论；[x] 确认 Rust 测试与锚点；boundary 核对结构 claim（Rust 侧检索）。

结论：22 条 verdict 全部成立，无判定变更、无代码变更——
[x] live_command:33 身份保持等价（未知 id None、definitionId 归属、版本快照、软删除历史保留）；
4 boundary 结构属实（atomicGroupId 仅存在于 PineTS worker 意图标签，Rust 引擎无原子组校验/提交语义；
默认 Pine 模板无后端生成器）；partial 缺口描述与 Go 体一致（含 :127 整 bar 预检五分支、
:545 陈旧取消容忍对 Rust 硬错误的决策差、:116 健康覆盖零调用、:94 双模式流式能力门）。
本片账本仅做只读校验。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、`check:quick`（单实例）、`git diff --check`。

下一片：strategy_pine 余量按文件行号继续（manager_boundaries 之后：manager_close/nil/order_risk/live_business/lifecycle 等）。

## 第 130 批 06B 切片四：liveruntime 余量 32 条 recon（2026-09-24）

范围：`internal/strategy/liveruntime/` 除 manager_boundaries 外 9 文件 32 条
（[x] 3 + partial 27 + boundary 2）。

方法：[x]/boundary 全验（测试存在、锚点识别、断言/结构相符）；partial 逐条核对结论，
高风险项（时区、日界计数、关停聚合、预热、租约回滚）抽查 Go 体与交叉引用。

结论：32 条 verdict 全部成立——3 [x]（止损单字段、原因码表、日界计数）测试与锚点俱在；
2 boundary 属实（nil 接收者归类型系统、反射断言归架构门禁）；partial 缺口与 Go 体一致。
唯一打磨：order_risk:216 的交叉引用补全测试名
（internal/strategy/runtimecontrol/policy_test.go:83:TestMarketDayStartUTCUsesOrderSymbolTimezone），
并核实本包 marketDayStartUTC（risk.go:110）确为转调；其引用测试的锚点指向 runtimecontrol 行，
属 partial 证据复用（允许），非缺口。
本片除该行结论补全外无判定与代码变更。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、`check:quick`（单实例）、`git diff --check`。

下一片：strategy_pine 余量按文件行号继续（pine_live_command/pine_live_executor/pineruntime 等）。
