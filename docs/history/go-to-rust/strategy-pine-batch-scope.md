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

## 第 130 批 06B 切片五：pine_live_command/pineruntime 23 条 recon（2026-09-24）

范围：`internal/strategy/pine_live_command_test.go`（11：partial 9 + boundary 2）、
`internal/strategy/pineruntime/recovery_contracts_test.go`（1：boundary 1）、
`internal/strategy/pineruntime/runner_lifecycle_test.go`（4：partial 4）、
`internal/strategy/pineruntime/runtime_failure_contracts_test.go`（7：partial 7）。

方法：Go 体逐条核对结论；全部 Rust 引用测试逐一确认存在（rg 全命中 15 个去重条目）；
boundary 核对结构 claim（引擎意图层确无 OCO 展开逻辑、nil 接收者归类型系统）；
partial 高风险项（作用域退出数量保留、条件单类型、容量等待取消、关停排空、worker 上限映射）
抽查 Go 体与 Rust 用例体，确认“引用存在但断言不等价”口径诚实。

结论：23 条 verdict 全部成立，无判定变更、无代码变更——
pine_live_command 9 partial 均诚实（方向归一/数量百分比/失败关闭落在执行结果而非命令 DTO、
时间戳数值由 wire DTO 承担、缺省数量与条件触发无同形断言）；
2 boundary 属实（OCO 展开与括号拒绝归 backtest 撮合 owner，引擎侧不再展开）；
recovery boundary 属实（nil 接收者编译器排除）；
runner_lifecycle 4 partial 相符（无 done channel/容量归还语义，池 pin + readiness monitor 组合承担）；
runtime_failure 7 partial 相符（含 :30 worker 上限映射缺失 P1、:99 排队等待缺失 P1、
:171 关停排空缺失 P1、:52/:76 回落与发布层缺失 P2）。
抽查 Rust 用例体（quantity_pct/close_short/unknown_risk/wire 枚举/closed 布尔/pool CapacityExceeded
即时拒绝）与结论描述一致，无夸大引用。
附带核实审计提醒项：当前工作区 [x] Rust 条目重复组为 0（已解），审计通过后再跑生成器。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、`check:quick`（单实例）、`git diff --check`。

下一片：strategy_pine 余量按文件行号继续（pine_live_executor 24 条 + pineruntime/runtime_test 13 条）。

## 第 130 批 06B 切片六：pine_live_executor/runtime_test 37 条 recon（2026-09-24）

范围：`internal/strategy/pine_live_executor_test.go`（24：[x] 4 + partial 17 + boundary 3）、
`internal/strategy/pineruntime/runtime_test.go`（13：partial 13）。

方法：Go 体逐条核对结论；4 [x] 确认 Rust 测试存在、Parity 锚点被识别、断言等价；
全部 37 条 Rust 引用逐一 rg 命中（0 缺失）；[x] Rust 条目唯一性复核重复组为 0；
高风险缺口（未知 kind 回落、timeInForce 缺失、空头标签缺失、0 值语义、半发布回滚）
抽查 Rust 生产代码（strategy_runtime_execution.rs 无 timeInForce/tag 字段、
数量回落分支、INTENT_SKIPPED 与 cancel_all partially failed 聚合）确认结论诚实。

结论：37 条 verdict 全部成立，无判定变更、无代码变更——
4 [x]（平仓百分比按现仓取整、缺省全平、REAL 下缺数量 fail-closed 且零券商调用、
权益百分比按 notional/price 取整）与 Go 算式同值，:80 的 SIMULATE 缺省 1 语义
在结论中已显式区分为离线模拟语义，live 行为等价；
3 boundary 属实（实时无原子括号路径，括号语义归 backtest 撮合 owner）；
partial 缺口与 Go 体一致（含 :556 未知 kind 落入 entry 分支、:36 缺 GTC 组合断言、
:79 0 值 clamp 与参考默认语义差 P1、:228 半发布回滚缺失 P1、
:18 20 变量合并无单一入口、:278 容量与 revision 分层承担）。
:105 的 runtime_path 新增锚点在位（runtime_dependencies.rs:548）。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（integration-pine 全量 + engine strategy_runtime_execution 过滤）、
`check:quick`（单实例）、`git diff --check`。

下一片：strategy_pine 收尾 22 条（runtimecontrol 10 + service 10 + types 2），随后进 backtest_calendar。

## 第 130 批 06B 切片七：strategy_pine 收尾 22 条 recon（2026-09-24）

范围：`internal/strategy/runtimecontrol/`（10：[x] 5 + partial 5，含 boundary 2）、
`internal/strategy/service_test.go`（10：[x] 1 + partial 9）、
`internal/strategy/types_test.go`（2：partial 2）。
本片关闭 strategy_pine 域（538 条全量 recon 完毕）。

方法：Go 体逐条核对结论；6 [x] 确认 Rust 测试存在、Parity 锚点被识别、断言等价或超集；
全部 22 条 Rust 引用逐一 rg 命中（:83 初查 NOTEST 系显示截断误报，实为双用例组合引用，
两用例俱在）；[x] Rust 条目全文唯一性成立（含组合区分形式）；
高风险项抽查 Rust 生产代码与用例体（market_day_start 日历委托、cancel_all 部分失败聚合、
429 忙碌映射、payload 无 timeInForce/tag 字段）。

结论：22 条 verdict 全部成立，无判定变更、无代码变更——
6 [x]（off 模式零决策、原因码表、monitor 记录不拒绝、模式归一清零、持仓符号匹配、
Pine 非法格式 400 且分析器零调用）断言等价，其中原因码表与 Pine 校验用例各带双锚点，
分别被 order_risk:67 与路由行复用，账本以组合形式区分，无重复违反；
:83 维持 partial（前期分片二十 [x] 过宽纠正成立：Rust 经 jftrade-calendar 取市场本地午夜，
夜盘时刻与参考扩展时段边界差一交易日，DST 用例内注释已显式记录差值，P1 缺口与修复位置登记在案，
manager_session 已建模 20:00 延续）；
2 boundary 属实（Decimal 无负零、Rust 无限制列表对象）；
service/types 10 partial 缺口与 Go 体一致（含 :236 启动后回滚 P1、:223 忙碌文案指引、
:255/:278 刷新计数与顺序、:36 timeInForce 缺失）。
本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、`check:migration-manifest`、
`check:zero-go`、定向 nextest（trading risk/portfolio + engine strategy 位）、
`check:quick`（单实例）、`git diff --check`。

下一域：backtest_calendar（376 条），按文件行号分片推进。
## 第 130 批 09 切片二十九：internal/strategy 服务面 18 条 [x] 复核，零纠正（2026-09-24）

范围：`internal/strategy` 服务面 18 条 `[x]`（errors、instancebinding、
instanceview、liveruntime 3、pine_live_executor 4、runtimecontrol 5、
catalog、live_command、service 各 1）。本片为独立复核抽查，非新映射。

方法：Go 体全读；20 个具名 Rust 引用逐一全仓核实存在（0 缺失）；7 个
高风险行逐字比对断言（`:33` 委派身份、`:235` 市场日窗口、`:80` 缺数量
拒绝、`:52` 失败收敛、`:198` 非法格式前置校验、`:67`/`:11` 原因码表）；
其余 11 行结论与锚点抽查。

结论：18 条全部维持 `[x]`，零纠正（`[x]` 全量保持 1501）。核实要点：

- `:33`：Rust 经 ProductionStrategyDefinitionPort 按身份键读写（未知
  id 答 None、definitionId 回显、软删除保留历史），错配身份即失败，
  委派契约成立。
- `:235`：同一时刻两事件 US 计 2、HK 计 1、他实例计 0，与 Go 完全一致；
  计数按实例加日界毫秒过滤，边界语义差已在测试内注明。
- `:80`：REAL 绑定缺数量拒绝且零 broker 调用；SIMULATE 缺省 1 为离线语义，
  测试内已区分。
- `:52`：RUNNING 转 STOPPED 加错误日志，已停止实例审计与日志行数不变，
  补强断言在位。
- `:198`：非法格式 400 且端口零调用，合法请求才落端口；层位差已在结论
  写明。
- `:67`/`:11`：原因码表 6 断言逐条复刻，证据集不同（单用例与组合），
  无重复 `[x]` 冲突。

队列探查：strategy_pine、backtest_calendar、store 三域均已收官；
api_transport 面 `[x]` 余量 30 个目录，以 marketdataapp 60、
servercoretest 59、servercore 50、datamigration 38 为大头。

验证：`audit_test_parity.py --write-report` exit 0（1501 exact 全引用
存在；inventory 重生但计数不变）；`parity_anchor_reconcile.py` 过
（1742 total、已记录 1696、unrecorded 0、stale 0、unknown 46）；本片无
Rust 文件改动故免 fmt 与 nextest；`check:ai-context`、
`check:migration-manifest`、`check:zero-go`、`check:quick` 与
`git diff --check` 见收尾。

下一片：130-09 切片三十，datamigration 38 条 `[x]` 纠正式复核（s19 已深 recon，需逐字比对）。

## 第 130 批 09 切片 s35：strategy [x] 前 20 条二次复核（2026-09-24）

范围（台账顺序）：catalog:52、errors:8、binding:137、view:46、live_command:33、
order_risk:18/:67/:235、pine_executor:80/:93/:119/:150、risk_off:33、
policy:11/:53/:68/:120、service:198、warmup_internal:142、warmup_plan:12，
共 20 条。Go 体全读，Rust 引用全部可解析且带 Parity 锚点。

结论：20 条全部维持，零改判。抽核要点已逐项比对：

- catalog:52 用双用例组合覆盖（恢复路径的 stopped 无写 + RUNTIME_EXITED 审计、
  实时退出路径的原因透传审计/错误日志/通知），与 Go 的 saveCount/audit/log
  三簇对应。
- policy:11 与 order_risk:67 共用原因表用例（同表逐值一致，含 SELL 6 超量行），
  前者另以 enforce 用例覆盖 PauseOnReject/日计数标志；monitor(:53) 含日计数
  detail 前缀场景；normalize(:68)/risk_off(:33)/positions(:120) 逐项一致。
- pine_executor 四条算式一致（50% 权益 5 股、50% 平仓 5、缺省全平 3）；
  :80 的 REAL/ SIMULATE 区分与文案差已在结论披露。
- order_risk:18 的 Rust payload orderType 取值为 STOP（Go 为 STOP_MARKET），
  结论原文如实记录，止损价透传/无线价/reduce-only 三簇一致。
- warmup:142 的周/月系数经 Go 源码核实为 390×5/×20，与 Rust 断言一致；
  warmup_plan:12 以最大需求 20×390 为准一致。

`[x]` 保持 1476。本片仅文档记录，无台账与代码变更。

验证：`audit_test_parity.py`（只读）无告警增量；定向 nextest（jftrade-strategy
104/104、risk_engine 8/8、strategy_pine_compatibility 7/7、engine strategy
25/25、trading portfolio 2/2）；`git diff --check` 干净。

## 第 130 批 09 切片 s36：strategy [x] 第 21–40 条二次复核（2026-09-24）

范围（台账顺序）：warmup_plan:40/:74/:98、warmup_script:14/:51/:67、
ir/planner:100/:204、expression:5/:11/:21、extended_ticker:49、
language_failure:239、order_command:11、parse_object:136、parse_request:82、
parse_semantic:377、parse:10/:54/:84，共 20 条。Go 体全读，Rust 引用全部可解析。

结论：19 条维持，1 条收回过宽 `[x]`→partial：

- `parse_test:54` 公共 helper 拒绝：Go 用 11 个命名用例锁定，Rust 引用的 guard
  用例只锁 5 个（ma/bollinger/cross_over/cross_under/notify）；history 与 ta.adx
  由另行用例锁定；security_source、barssince、valuewhen、ifelse 四个 Go 命名
  用例在 Rust 零测试锁定（生产 guard 表虽含替换建议但零断言即零证据）。回归
  要求已记入 uncovered（guard 用例补四行，先红后绿；若某行通过则为行为分叉）。
- 其余维持要点：warmup 跨市场/扩展日/无 floor 三值一致；script 与 plan 双入口
  一致；position/legacy 键集与排除项一致；表达式三条与扩展 ticker 脚本一致；
  订单元数据 20+ 诊断逐条一致；v29 七码一致；stdev 键一致；visual 4 警告一致；
  parse:10 的 cross_over/ta.crossover 方向差（Rust 以 ta.crossover 为规范形，
  运行时绑定一致）与 qty 断言形态差已在结论如实记录；parse:84 双诊断含行号一致。

`[x]` 1476→1475。本片仅台账 + 报告/库存再生，无 Rust 代码变更。

验证：`audit_test_parity.py --write-report` exit 0（1475 exact，dup 0）；
`parity_anchor_reconcile.py` 1742/1696/0/0/46；jftrade-strategy 104/104；
`git diff --check` 干净。

## 第 130 批 09 切片 s37：strategy [x] 第 41–60 条二次复核（2026-09-24）

范围（台账顺序）：parse:126/:145/:186/:202/:232/:253/:367/:464/:488/:560/
:587/:621/:651/:690/:718/:732/:763/:1014/:1044、parser_loop:38，共 20 条。
Go 体全读，Rust 引用全部可解析。

结论：19 条维持，1 条收回过宽 `[x]`→partial：

- `parse_test:126` 策略默认数量继承：Go 的标题行为是无数量 entry 在解析期继承
  策略默认（OrderStmt QuantityMode==account_position_percent、
  QuantityExpression==10）；Rust 只锁了元数据一半，lowered IR 无 QuantityMode
  概念，全仓无 account_position_percent——策略默认数量从未被解析进订单。
  执行侧以缺省 1（SIMULATE）/拒绝（REAL）代替继承。回归要求已记入 uncovered
 （实现继承或登记设计决策后升级）。
- 其余维持要点：回测元数据全字段、显式 qty 覆盖、request.security 四拒绝
  （码+行号）、历史引用四处保留（typed Index 形态已披露）、历史引用两拒绝、
  exit 元数据、trail 拒绝、v40 六诊断（码+行号）、framework 语言特性、V12/V13
  键集（含 MTF）、风险声明全字段、兼容性注册表（68 在列+2 排除+唯一）、结构化
  诊断行号、字符串字面量豁免、静态循环边界黑盒等价。

`[x]` 1475→1474。本片仅台账 + 报告/库存再生，无 Rust 代码变更。

验证：`audit_test_parity.py --write-report` exit 0（1474 exact，dup 0）；
`parity_anchor_reconcile.py` 1742/1696/0/0/46；strategy parse 相关 4/4；
`git diff --check` 干净。

## 第 130 批 09 切片 s38：strategy [x] 第 61–79 条二次复核，strategy 域收官（2026-09-24）

范围（台账顺序）：strategy_business:11/:130、call_bounds:98/:120、tuple:11、
validation:43/:70/:86/:96/:108、pineengine:72/:97、pineworker manager:46/:74/
:119、types:41/:92/:105/:147，共 19 条。Go 体全读，Rust 引用全部可解析。

结论：19 条全部维持，零改判。抽核要点：

- 风险参数矩阵双用例覆盖（方向/类型/正数/count/单参有效 + 16 项拒绝），
  助手级 raw 字符串形态不可达已披露；risk 边界四例为其子集。
- call_bounds 双组合覆盖 11 边界 + 27 表达式拒绝（码名差 P2 已登记）；
  cancel_all 带参由同族 guard 覆盖。
- tuple 三拒绝 + reassign 模式一致（注错注入 seam 无 Rust 同形对象）。
- validation 五条一致（含真实修复记录与 P2 文案差）。
- pineengine 两条一致（PayloadMap 助手缺席已披露）；pineworker 池级等价
  与请求校验 9+1（非有限值单独成例，结论已说明）一致。
- types:41 的 rust_entry 保持单引用：非有限值用例另有 :164 行引用，
  为保 [x] 唯一性不做双组合，结论文字已准确。

strategy 域 `[x]` 二次复核全部完成（s35–s38 共 79 条，2 条降 partial）。
`[x]` 保持 1474。本片仅文档记录，无台账与代码变更。

验证：`audit_test_parity.py`（只读）OK（1474 exact，dup 0）；
定向 nextest（integration-pine 50/50、risk_and_block 10/10、mcp_contract 4/4）；
`git diff --check` 干净。

## 第 130 批 12 切片十七：strategy_pine 域 partial 第 1–20 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 1–20 行（internal/strategy/catalog 活动降级 1、
目录边界 5、实例生命周期 4、插件归一 5、仓库失败 3、运行时对账 2，
按文件加行号升序）。本域共 459 partial。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

20 行维持（活动存储不可用降级空页 P1、查询失败空页、写失败不阻塞控制、
定义同步六分类 P1、归一化调用方隔离、私有助手空值宽容、停止边界与删除门 P1、
错误分类矩阵、定义刷新位置保留与关联分类、刷新使用注入存储、插件排序持久化、
缺失资源分类、遗留快照迁移 P1、非法运行时两段式、插件兼容与卸载指引、
构造失败传播、保存失败回滚 P1、独立副本隔离、转换计数与审计、启动对账幂等 P1，
均与账本缺口一致）。

抽核要点：20 条 Rust 引用逐项存在；第 1 行 fail-closed 链逐行核实
（port 层 Unavailable → product_api_strategies 500 STRATEGY_FAILED）；
第 4 行 definitionSync 五字段无 BlockedReason 已核实；
第 3 行多处 let _ 忽略审计失败已核实；
第 7/13 行缺口（删除门、遗留迁移）在引用文件内无对应断言。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-store-sqlite 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片十八，strategy_pine 域 partial 第 21–40 行。

## 第 130 批 12 切片十八：strategy_pine 域 partial 第 21–40 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 21–40 行（目录活动分页富化 1、绑定归一 8、
实例视图 5、实时命令边界 6，按文件加行号升序）。本域已核对 40/459。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

20 行维持（活动分页过滤与观测富化、显式 instruments 优先、旧 params 回填、
ApplyParams 规范写回、chartType 保留与未知清空的行为差异、审计明细文案、
旧数组载荷、类型边界、nil 与陈旧字段清理、非类型化视图、runtime/source
回退、definitionId trim、视图绑定归一与副本隔离、实例 ID 前缀、默认模板边界、
OCO 腿校验边界、方向别名、整 bar 预检、原子组形状、原子提交，缺口 owner 与
回归要求均与账本一致）。

抽核要点：20 条 Rust 引用逐项存在；第 25 行行为差异逐行核实
（Rust renko 直接 is_err，Go 静默清空为 standard）；
第 24 行 Rust 无 ApplyParams 写回层已核实；
第 37/38 行 Rust 仅有 invalid direction 错误、无 kind 白名单已核实；
第 39/40 行原子组概念缺席成立。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-backtest 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片十九，strategy_pine 域 partial 第 41–60 行。

## 第 130 批 12 切片十九：strategy_pine 域 partial 第 41–60 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 41–60 行（实时命令方向/告警/数量/取消 4、
运行时管理边界 11、关闭聚合 3、nil 边界 2，按文件加行号升序）。
本域已核对 60/459。

方法：Go 体全读；缺口验到代码行与用例断言；引用存在不等于断言等价。

20 行维持（方向感知平仓六分支、告警回退身份、数量最小与精度、取消别名去重与
陈旧容忍、维护忙碌与轮询配置、兼容解析失败关闭、流式行情启动门、不健康拒绝与
覆盖跳过、精确券商解析、live 绑定逐字段校验、仅通知免账户、依赖缺失点名、
激活预留与未知成交、构建前置条件、回调事件记录、关闭错误聚合、等待中启动、
后台同步顺序、nil 空状态边界、命令回调委托，缺口 owner 与回归要求均与账本一致）。

抽核要点：20 条 Rust 引用逐项存在；第 41/44 行 Rust 行为字符串逐行核实
（INTENT_SKIPPED/no open position to close、is not owned by instance 硬错误）；
第 56 行 close session 日志形状已核实；第 43 行回测侧流动性告警引用存在；
第 45/53/56 行共用 partial 引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-broker、jftrade-strategy 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十，strategy_pine 域 partial 第 61–80 行。

## 第 130 批 12 切片二十：strategy_pine 域 partial 第 61–80 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 61–80 行（pineworker 实盘 3、产品生命周期 4、
运行时边界 5、风控证据 1、订阅生命周期 3、符号失败 2、pine 实盘命令 2，
按文件加行号升序）。本域已核对 80/459。

方法：Go 体全读；17 个去重 Rust 引用逐项 rg 存在性核查；引用存在不等于断言等价。

20 行维持（权益价格参数边界、live 会话失败边界、可执行告警过滤、快照身份与
账户helper、风控先行与观测容忍差异、网关失败透传与摘要排序、空白币种宽容、
启动校验与预留、成交桶合并滚动与通知文案、显示格式化边界、符号市场归一与
启动错误映射、刷新失败上报、风控审计与暂停迁移、租约失败回滚、panic 释租、
订阅符号归一、零时成交建桶与乱序隔离、关停降级、K线转Candle逐字段、意图批量
映射，缺口 owner 与回归要求均与账本一致）。

抽核要点：第 76 行 Rust 引用实现逐行核实（只覆盖空白过滤，未覆盖小写转大写
归一与 15m 周期映射，partial 成立）；第 65/73 行失败关闭与参考实现的容错差异
已在账本结论中显式登记；partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-trading、jftrade-broker、
jftrade-integration-pine 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十一，strategy_pine 域 partial 第 81–100 行。

## 第 130 批 12 切片二十一：strategy_pine 域 partial 第 81–100 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 81–100 行（pine 实盘命令 7、pine 实盘执行器 13，
按文件加行号升序）。本域已核对 100/385（审计口径总量 385，历史 459 为旧口径）。

方法：Go 体全读；11 个去重 Rust 引用逐项 rg 存在性核查；引用存在不等于断言等价。

20 行维持（做空方向保留、空头退出转买入、作用域退出数量保留、条件单四格矩阵、
卖出开仓规范化、缺省数量、非法意图拒绝、执行器端口保留、提交字段与 GTC、
无定量时 quantityPct 报错、空头标签、无持仓平仓忽略、低于步长忽略、港股碎股忽略、
缺市场规则忽略、告警聚合、自动平仓回补空头、显式空头平仓三分支、跟踪撤单、
cancel-all 两笔清空，缺口 owner 与回归要求均与账本一致）。

抽核要点：共享锚点 test_execute_strategy_intents_close_short_maps_to_buy 实现逐行核实
（空头平仓 side=BUY、quantity=20、reduceOnly=true，断言落在执行结果而非命令 DTO，
第 81/82/85/91/97/98 行 partial 成立）；第 84 行 limit 加 stop 意图层映射分歧
（Rust 取 LIMIT，参考实现取 StopLimit）仍在账本结论中登记为候选分歧；
partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-broker 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十二，strategy_pine 域 partial 第 101–120 行。

## 第 130 批 12 切片二十二：strategy_pine 域 partial 第 101–120 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 101–120 行（pine 实盘执行器 4、runner 生命周期 4、
运行时失败契约 5、运行时配置解析 7，按文件加行号升序）。本域已核对 120/385。

方法：Go 体全读；14 个去重 Rust 引用逐项 rg 存在性核查；引用存在不等于断言等价。

20 行维持（提交取消错误透传、业务边界错误集、订单 ID 生成与跟踪回退、
取消边界四分支、会话关闭退出监视、注册期关闭清理、取消边界双分支、
关闭初始化完成信号、嵌入 bundle 不可用上报、运行时回退与 worker 上限、
getwd 失败工作目录定位、失败不发布、容量等待与启动失败传播、
nil 与已关闭生命周期边界、关闭排空活跃会话、环境与设置合并二十字段、
嵌入与外部选择、禁用开关与非法值、worker 默认与运行时优先级、
仓库定位与 proto 覆盖，缺口 owner 与回归要求均与账本一致）。

抽核要点：第 119 行 Rust 引用实现逐行核实（runtime_test.go:105 Parity 锚点
仍在 runtime_dependencies.rs:548，优先级逐层锁定，但 settings 0 值语义分歧
未收敛，partial 成立）；第 102/104 行未知 kind 与缺失归属硬错误分歧仍在
账本结论中登记；partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-integration-pine、jftrade-settings
视触及范围）全过；cargo fmt --check 与 git diff --check 干净；
无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十三，strategy_pine 域 partial 第 121–140 行。

## 第 130 批 12 切片二十三：strategy_pine 域 partial 第 121–140 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 121–140 行（运行时依赖与配对管理 8、
风控时间与持仓辅助 1、交易日边界 1、观测投影 1、Service 门面 9，
按文件加行号升序）。本域已核对 140/385。

方法：Go 体全读；13 个去重 Rust 引用逐项 rg 存在性核查；引用存在不等于断言等价。

20 行维持（依赖注入工厂、路径与工作目录回退、配对构建发布退休、
半成品配对回滚、nil 与不可关闭边界、并发容量与会话 revision、
失败边界五分支、打开会话四拒绝、可选时间与持仓边角、交易日夜盘边界、
观测时间精度与错误裁剪、门面 store 与 runtime 委托、存储错误透传、
分析器注入与默认格式、启动前拒绝零触碰、容量转忙碌文案、
三条启动失败路径、启动后刷新计数、暂停停止顺序与刷新、
目录运行时生命周期入口委托，缺口 owner 与回归要求均与账本一致）。

抽核要点：第 130 行双引用逐项存在（含 DST 变体），账本结论把历史过宽 [x]
纠正为 partial 并登记夜盘边界分歧（P1，calendar 暴露交易日边界起点 API），
本轮复核 Go 体三断言与该结论一致；partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-integration-pine、jftrade-trading、
jftrade-settings 视触及范围）全过；cargo fmt --check 与 git diff --check 干净；
无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十四，strategy_pine 域 partial 第 141–160 行。

## 第 130 批 12 切片二十四：strategy_pine 域 partial 第 141–160 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 141–160 行（定义视图与绑定契约 2、
源码格式与脚本校验 3、指标绑定时间单位与均线键 6、数量模式 2、
保护模式与方向 5、正整数解析 1，按文件加行号升序）。本域已核对 160/385。

方法：Go 体全读；15 个去重 Rust 引用逐项 rg 存在性核查；引用存在不等于断言等价。

20 行维持（视图扁平、绑定 JSON 契约、格式缺省 PineV6、脚本校验与可实例化、
v6 源码接受、引号时间单位表、均线可选参数表、价格源白名单与带源键、
均线类型十表、类型归一回退、DSL 词表、归一缺省、均线键三值、数量模式八表、
数量归一回退、保护模式表、保护模式归一、保护方向表、保护方向归一、
正整数解析，缺口 owner 与回归要求均与账本一致）。

抽核要点：第 151 行 Rust 引用实现逐行核实（词形十表加 bar 空后缀与 year 拒绝
与账本一致，hr/hrs/mins/mon 等别名确未逐项断言，partial 成立）；
第 149 行均线键用例逐字断言 ma:EMA:14:minute 与 ma:SMA:5:day，
Go 侧 MA 类型在 Rust 无对应调用名缺口仍在；
partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-strategy 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十五，strategy_pine 域 partial 第 161–180 行。

## 第 130 批 12 切片二十五：strategy_pine 域 partial 第 161–180 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 partial 第 161–180 行（指标数值解析 2、indicatorwarmup
解析与校验 15、IR planner 1、按文件加行号升序）。本域已核对 180/385。

方法：Go 体全读；15 个去重 Rust 引用逐项 rg 存在性核查；引用存在不等于断言等价。

20 行维持（正浮点与百分比解析、advanced key 形状拒绝、固定周期族校验、
风险时间与策略校验、source-aware/legacy 指标键解析、plan 空白裁剪与严格拒绝、
全指标族非法边界、均线与风险键边界、时间单位与 source 归一、warmup 综合窗口、
交易周期回退、advanced lookback、固定周期合法与低频错误、脚本/plan 非法输入、
loop/exit/divergence/legacy protect 需求收集，缺口 owner 与回归要求均与账本一致）。

抽核要点：第 161/162 行 Rust percentile 用例逐项确认正值、百分比边界与规范化；
第 173 行 Rust planner 只覆盖 broker interval 与安全回退，不等同 Go 的完整
normalizeWindowFunction/normalizeSourceOrClose 表；第 174–177 行 warmup 族群
引用分别覆盖指标值、period 拒绝、MACD lookback 与风险元数据投影，综合窗口和
固定周期组合仍为 partial；第 180 行 planner requirement key 用例逐项核对
loop/exit/divergence/protect 键，引用存在不等于所有 legacy 键断言等价。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-strategy、jftrade-backtest 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十六，strategy_pine 域 partial 第 181–200 行。

## 第 130 批 12 切片二十六：strategy_pine 域 partial 第 181–200 行，20 行维持、零改判（2026-09-25）

范围：strategy_pine 域 partial 第 181–200 行（IR planner 分支/表达式/指标绑定/参数
边界与 PlanRequirements 契约，按文件加行号升序）。本域已核对 200/385。

方法：Go 体全读；5 个去重 Rust 引用逐项 rg 存在性核查，并抽读 planner requirement
keys、native pipeline 与 account value 用例；引用存在不等于断言等价。

20 行维持（循环、对象和退出表达式收集、业务非法参数拒绝、nil/unsupported statement、
指标支持矩阵、参数校验矩阵、复合表达式收集与拒绝、source-aware 归一、均线/风险键
边界、legacy close 键、运行时需求键完整性、PlanRequirements position/account value
需求、无效绑定与均线类型、数量模式、保护时间单位、runtime binding parity、
unsupported window source、advanced indicator bindings，缺口 owner 与回归要求均与账本一致）。

抽核要点：Go 侧 planner_business_boundary、indicator_matrix、internal boundaries、
planner_internal 与 planner 五组测试体均已读完；Rust 的
`window_and_oscillator_keys_keep_the_requested_source`、
`legacy_and_explicit_sources_keep_the_planned_keys`、
`account_value_usage_in_statements_requires_total_account_value` 与
`native_pipeline_parses_lowers_and_plans_strategy_requirements` 逐项存在。Rust 引用
覆盖规划键和失败关闭主路径，但没有一一复刻 Go 的全部错误文案、unsupported statement
集合和每个指标族的参数矩阵，因此继续标记 partial。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-strategy、jftrade-backtest 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十七，strategy_pine 域 partial 第 201–220 行。

## 第 130 批 12 切片二十七：strategy_pine 域 partial 第 201–220 行，20 行维持、零改判（2026-09-25）

范围：strategy_pine 域 partial 第 201–220 行（IR planner 高级/跨语句收集 2、
Pine 编译与 security 诊断 4、tuple/control-flow/UDF 4、ticker 与语言执行边界 6、
order/security metadata 3、benchmark corpus 1，按文件加行号升序）。本域已核对 220/385。

方法：Go 体全读；15 个去重 Rust 引用逐项 rg 存在性核查；对无 rust_entry 的条目按
账本结论复核 Rust 探针和功能缺口，不把“引用缺失”误记为覆盖。

20 行维持（高级指标键、跨语句/元组表达式收集、planner 诊断行号、security 纯度与
可选链、编辑器恢复/能力证据、unsupported security 文案、tuple helper arity、
畸形 TA 原文保留、control-flow/UDF 拒绝、降序/条件循环、ticker whitelist、
MTF TA 执行键、tuple 派生别名、PineV6 lexical helpers、UDF/runtime loop、
TA 默认参数、security tuple diagnostics、tuple parser boundary、order metadata、
benchmark scripts，缺口 owner 与回归要求均与账本一致）。

抽核要点：第 201 行确认 Rust 只覆盖 10/12 高级指标族，`ta.cog` 与 `ta.bbw` 仍静默
缺失；第 204/205/209/210/214/215 行没有 Rust 同形对象，继续保留 P1/P2 缺口；
第 217 行 Rust tuple diagnostics 用例逐项确认 Go 码值映射，但 security 内 TA 白名单
和高级参数校验仍有探针差异；第 216 行窗口族默认值新用例已逐字核实，TA 原文保留
面仍为 partial；第 220 行 benchmark corpus 未迁移，且 udf_static_for 在 Rust 解析层
仍拒绝。引用存在不等于断言等价，partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-strategy、jftrade-backtest 视触及范围）全过；
cargo fmt --check 与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十八，strategy_pine 域 partial 第 221–240 行。

## 第 130 批 12 切片二十八：strategy_pine 域 partial 第 221–240 行，20 行维持、零改判（2026-09-25）

范围：strategy_pine 域 partial 第 221–240 行（Pine collection/declaration 语义与执行边界、
V21–V30 语言能力、对象/方法/循环/MTF、request.security 诊断，按文件加行号升序）。
本域已核对 240/385。

方法：Go 体全读；共享 Rust framework 能力引用、ticker/tuple/order/security 边界引用逐项
rg 核查；对同一 Rust framework 测试覆盖的多条 Go 测试仍按断言粒度保持 partial。

20 行维持（V20 collection 与 declaration AST/semantic、collection 签名诊断、method-style
操作、typed collection、V21 可执行 collection core/alias/BBW/COG、V22 tuple/structured
AST/dynamic loop、V23 UDT/method/request.security pure object、V24 collection expansion
与 MTF stoch、V24 named method/runtime loop、V25 array/string/timeframe helpers、V26
collection iteration/history/object fields、V27 collection/timeframe/MTF helpers、V28
object history/method/export、V29 receiver/MTF history、V29/V32 request.security 诊断，
缺口 owner 与回归要求均与账本一致）。

抽核要点：Rust `framework_language_feature_tests::compile_supports_framework_language_features`
存在但只提供聚合能力证据；`request_security_tickers_follow_the_go_whitelist`、
`request_security_tuple_diagnostics_match_go_codes`、`compile_accepts_supported_order_positional_metadata`
分别存在。Go 侧 collection/UDF/runtime loop 大量断言的执行性、声明字段和诊断矩阵不能由
聚合测试替代；其中 UDF、多层循环、collection runtime 与部分 MTF TA 仍是已登记 P1/P2
缺口。引用存在不等于断言等价，partial 共用引用不受 [x] 唯一性约束。

验证：audit --write-report 过（1472 exact；dup 0）；anchor 过（1742/1696/0/0/46）；
定向 nextest（jftrade-engine、jftrade-strategy 视触及范围）全过；cargo fmt --check
与 git diff --check 干净；无台账变更、无 Rust 生产代码改动。

下一片：130-12 切片二十九，strategy_pine 域 partial 第 241–260 行。

## 第 130 批 12 切片二十九：strategy_pine 域 partial 第 241–260 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 `evidence_type=partial` 的第 241–260 行（parse_object、parse_request、
parse_semantic 的 V14–V17/V30 声明、语义、MTF、tuple、TA 与控制流行为），按账本写入顺序逐条核对。
strategy_pine 审计口径共 385 条 partial；另有 74 条 P2 boundary 不计入本序列。本片没有 Rust
生产代码修改；共享 framework 测试只作证据引用，不因存在引用而升为 exact。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 241 | `parse_object_test.go:206:TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy` | `src/pine/mod.rs::framework_language_feature_tests::compile_supports_framework_language_features` | P2 | `partial`；Rust 只覆盖 assignment/ternary/if，未覆盖 Go 的 PriceBox type/method/export/import、varip warning 与 semantic declaration signatures。 |
| 242 | `parse_object_test.go:249:TestAnalyzeScriptReportsCollectionTypeDiagnostics` | 同上 | P2 | `partial`；Go 的 7 条 collection type/namespace/element mismatch 诊断没有 Rust 同形矩阵。 |
| 243 | `parse_object_test.go:289:TestAnalyzeScriptReportsCollectionMethodStyleSignatureDiagnostics` | 同上 | P1 | `partial`；Go 覆盖 array/matrix method-style 操作与两类 signature error，Rust framework 聚合测试不含 collection diagnostics。 |
| 244 | `parse_object_test.go:313:TestAnalyzeScriptReportsDeclarationSemanticDiagnostics` | 同上 | P2 | `partial`；重复字段、重复参数、semantic declaration 数量和字段投影尚无 Rust 断言。 |
| 245 | `parse_object_test.go:341:TestAnalyzeScriptReportsTypeAndMethodRegistryDiagnostics` | 同上 | P2 | `partial`；type registry、receiver 类型、重载 method、map receiver 和 object operation projection 未逐项迁移。 |
| 246 | `parse_object_test.go:410:TestAnalyzeScriptReportsImportAliasDeclarationDiagnostics` | 同上 | P2 | `partial`；import path/version/alias 及重复 alias diagnostic 无 Rust 对应测试。 |
| 247 | `parse_object_test.go:438:TestAnalyzeScriptReportsObjectOperationSignatureDiagnostics` | 同上 | P2 | `partial`；对象构造和 method 调用的缺参/超参四格 signature diagnostics 未覆盖。 |
| 248 | `parse_request_test.go:10:TestCompileSupportsMovingAverageRequestSecuritySubset` | `tests/pine_request_and_visual_contracts.rs::request_security_moving_average_keys_keep_type_period_source_and_time_unit` | P2 | `partial`；Rust 只核对 4 个 moving-average requirement keys，Go 另有 plain source、dynamic timeframe、history 与 merge flags 的完整 lowering/requirements。 |
| 249 | `parse_request_test.go:97:TestCompileSupportsCommonTradingViewTAFunctions` | `tests/pine_request_and_visual_contracts.rs::common_ta_window_keys_keep_the_requested_source` | P2 | `partial`；Rust 核对 window keys 子集，未断言 Go 的 Bollinger tuple、wpr 条件和完整 hook statement 顺序。 |
| 250 | `parse_request_test.go:141:TestCompileSupportsV14WindowMomentumAndStatefulIndicators` | `src/pine/mod.rs::framework_language_feature_tests::compile_supports_framework_language_features` | P1 | `partial`；stdev/variance/highestbars/lowestbars/change/mom/roc/rising/falling 与 barssince/valuewhen/tr 的 V14 矩阵未覆盖。 |
| 251 | `parse_request_test.go:183:TestCompileSupportsV14RequestSecurityPureExpression` | 同上 | P2 | `partial`；Rust 未逐项验证 security expression、nested history lowering 及四类 requirement keys。 |
| 252 | `parse_request_test.go:217:TestCompileSupportsV15RequestSecurityCommonTAExpression` | 同上 | P2 | `partial`；Rust 未覆盖 RSI/MACD/ATR/Bollinger/Supertrend 的 security expression fragments 与 range/mode keys。 |
| 253 | `parse_request_test.go:263:TestCompileSupportsV16RequestSecurityTupleWhitelist` | 同上 | P2 | `partial`；Rust 未覆盖 tuple security 的 EMA/SMA/MACD/Bollinger expression、alias projection 和 requirement matrix。 |
| 254 | `parse_semantic_test.go:10:TestAnalyzeScriptIncludesV17SemanticSummary` | 同上 | P2 | `partial`；semantic symbols、tuple bindings、function calls 的 supported/value-kind 投影没有 Rust 对应断言。 |
| 255 | `parse_semantic_test.go:55:TestAnalyzeScriptReportsSupportedTASemanticSignatures` | 同上 | P2 | `partial`；9 个 TA semantic signatures 的 supported/signature 字段未逐项映射。 |
| 256 | `parse_semantic_test.go:98:TestAnalyzeScriptReportsSupportedUtilitySemanticSignatures` | 同上 | P2 | `partial`；input/math/string 25 项 utility semantic signature 矩阵无 Rust 同形证据。 |
| 257 | `parse_semantic_test.go:180:TestAnalyzeScriptReportsSemanticSignatureDiagnostics` | 同上 | P2 | `partial`；Rust 没有 semantic signature 错误码、行号和缺参诊断断言。 |
| 258 | `parse_semantic_test.go:195:TestCompileSupportsV15StaticForLoopControl` | 同上 | P2 | `partial`；Rust 只保留静态 loop 的聚合编译/边界证据，未断言 Go 的 continue/break 展开结果和顺序。 |
| 259 | `parse_semantic_test.go:230:TestCompileSupportsInputMathCrossAndSourceAwareMovingAverages` | 同上 | P2 | `partial`；input defaults、source-aware MA、math/cross lowering 与 if condition 结构未逐项覆盖。 |
| 260 | `parse_semantic_test.go:268:TestCompileSupportsPineStrategyPositionVariables` | 同上 | P2 | `partial`；position_avg_price/position_size 的表达式和 close hook 结构未由 Rust framework 测试单独证明。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast
cargo fmt --check
git diff --check
```

实测：jftrade-strategy nextest `104/104` 通过；audit/anchor、`cargo fmt --check` 与
`git diff --check` 均通过。20/20 均保持 `partial`；不存在可安全升为 `[x]` 的条目，也没有发现
应立即修复的 Rust 生产功能差异。下一片为 strategy_pine partial 第 261–280 行。

## 第 130 批 12 切片三十：strategy_pine 域 partial 第 261–280 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 `evidence_type=partial` 的第 261–280 行（parse_semantic、parse、
parser/lowering recovery 的 TA/visual/order/UDF/loop/诊断边界），按账本写入顺序逐条核对。
本片没有 Rust 生产代码修改；Rust 已有测试只在其实际断言范围内计证据，未把聚合/子集测试提升为 exact。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 261 | `parse_semantic_test.go:287:TestAnalyzeScriptSupportsTrendAndStatefulTAFunctions` | `src/pine/mod.rs::framework_language_feature_tests::compile_supports_framework_language_features` | P1 | `partial`；Rust framework 用例未覆盖 supertrend/dmi/vwap/mfi、barssince/valuewhen 的完整 semantic/requirements 矩阵。 |
| 262 | `parse_semantic_test.go:301:TestAnalyzeScriptSupportsSarBarstateSessionAndPineConstants` | 同上 | P1 | `partial`；SAR requirement、颜色/时间戳 lowering、barstate/session/dayofweek/month 条件结构无 Rust 同形断言。 |
| 263 | `parse_semantic_test.go:340:TestCompileSupportsOrderQtyPercentStrategyOrderAndCloseAll` | 同上 | P2 | `partial`；Rust framework 未覆盖 qty_percent、net short/default quantity、close_all 五条 typed order projection。 |
| 264 | `parse_semantic_test.go:401:TestAnalyzeScriptReturnsVisualMetadata` | `tests/pine_request_and_visual_contracts.rs::visual_metadata_lists_every_drawing_call` | P2 | `partial`；Rust 只断言 7 个 visual 的部分 kind/call/target，未覆盖 Go 的 title、named args、variable、semantic mirror 全量字段，且 qualified target 表示不同。 |
| 265 | `parse_test.go:54:TestCompileRejectsPublicInternalHelperCalls` | `src/pine/mod.rs::public_helper_guard_tests::compile_rejects_public_internal_helper_calls` | P2 | `partial`；Rust 只覆盖 5 类 helper rejection，Go 还覆盖 barssince/valuewhen/history/ifelse 与 ta.adx 等完整 11 项矩阵。 |
| 266 | `parse_test.go:126:TestCompileUsesStrategyDefaultQuantityForEntryWithoutQty` | `src/pine/mod.rs::parse_ir_tests::compile_uses_strategy_default_quantity_for_entry_without_qty` | P2 | `partial`；Rust 验证 metadata/default quantity，但未断言 Go 的 lowered `OrderStmt` quantity mode/expression。 |
| 267 | `parse_test.go:167:TestCompilePreservesOrderNotificationMetadataAndImmediateClose` | `src/pine/mod.rs::order_subset_compile_tests::compile_preserves_order_notification_metadata_and_immediate_close` | P2 | `partial`；Rust 以 Action 参数字符串保留 metadata，未证明 Go typed order 字段及完整表达式类型。 |
| 268 | `parse_test.go:286:TestCompileSupportsStrategyExitSubset` | `src/pine/mod.rs::order_subset_compile_tests::compile_accepts_strategy_order_subset_scripts` | P2 | `partial`；Rust 仅验证调用可编译且保留 strategy.* 名称，未逐项断言四类 ExitStmt 的方向、stop/limit/trailing 与数量字段。 |
| 269 | `parse_test.go:317:TestCompileCapturesWhenExpressionsForWorkflowOrders` | 同上 | P2 | `partial`；Rust order subset 含 when 输入但未核对每个 entry/order/close/exit 的条件表达式 lowering。 |
| 270 | `parse_test.go:348:TestCompileSupportsStrategyExitProfitLossTicks` | 同上 | P2 | `partial`；Rust 未断言 profit/loss/ticks 到 typed exit 字段的映射，只检查 strategy.* action 存在。 |
| 271 | `parse_test.go:390:TestCompileSupportsPendingStopAndCancelOrders` | 同上 | P1 | `partial`；Rust 未逐项验证 stop/stop-limit、close stop/limit、cancel/cancel_all 的订单语义和参数保留。 |
| 272 | `parse_test.go:432:TestCompileSupportsCloseAllPositionalMetadata` | 同上 | P2 | `partial`；Rust 只验证 close_all 调用保留，未断言四个 positional metadata 到 comment/alert/disable 字段。 |
| 273 | `parse_test.go:448:TestCompileSupportsClosePositionalQty` | 同上 | P2 | `partial`；Rust 只保留 `strategy.close` call，未证明 positional quantity 的 shares projection。 |
| 274 | `parse_test.go:790:TestCompileSupportsExpressionUDFAndStaticForUnroll` | `src/pine/mod.rs::udf_and_loop_boundary_tests::compile_accepts_expression_udf_and_static_for_unroll` | P1 | `partial`；Rust 明确保留 typed For/UDF 而不做 Go 的展开/inlining，未复现 4 条 history 语句、condition 与 requirement key。 |
| 275 | `parse_test.go:838:TestValidateScriptReportsUnsupportedUDFAndStaticForCases` | `src/pine/mod.rs::udf_and_loop_boundary_tests::validate_script_reports_supported_udf_and_static_for_boundaries` | P1 | `partial`；Rust 仅覆盖 step=0、迭代上限，Go 还覆盖参数 mismatch、递归、只读 loop var、loop 内 call history。 |
| 276 | `parse_test.go:903:TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics` | 同上 | P2 | `partial`；Rust 没有 V33 recursive/nested/signature/readonly diagnostic code+line 全量矩阵。 |
| 277 | `parse_test.go:977:TestCompileSupportsSwitchAndMultiStatementUDF` | `src/pine/mod.rs::framework_language_feature_tests::compile_supports_framework_language_features` | P2 | `partial`；Rust framework 测试无 switch、多语句 UDF、ifelse body 和 switch order/alert branch 结构。 |
| 278 | `parse_test.go:1029:TestAnalyzeScriptPreservesOriginalLineNumbers` | `tests/pine_parse_diagnostics.rs::blank_lines_before_the_script_keep_later_diagnostic_lines` | P2 | `partial`；Rust 已验证 blank-line 行号，但仅覆盖 zero-step loop 三种偏移，未覆盖 Go 的所有 parser/semantic diagnostics。 |
| 279 | `parser_and_lowering_recovery_test.go:8:TestIncompleteColorAndRequestCallsRemainRecoverable` | `tests/pine_mcp_contract.rs::parser_handles_strings_history_and_nested_calls_without_regex` | P1 | `partial`；Rust parser fixture 未断言 incomplete color call 原文保留和 request.security 未闭合参数返回值。 |
| 280 | `parser_and_lowering_recovery_test.go:19:TestUDFExpansionRetainsWhitespaceAndUnwindsRejectedArguments` | `src/pine/mod.rs::udf_and_loop_boundary_tests::validate_script_reports_supported_udf_and_static_for_boundaries` | P1 | `partial`；Rust UDF compile/validation 不覆盖 whitespace-only zero-arg expansion、collection mutation 参数拒绝和 recursion stack 清理。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast
cargo fmt --check
git diff --check
```

本片结论：20/20 均保持 `partial`；P1 缺口已明确 owner/回归方向，但本片没有足够证据升格或触发生产修复。下一片为 strategy_pine partial 第 281–300 行。

## 第 130 批 12 切片三十一：strategy_pine 域 partial 第 281–300 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 `evidence_type=partial` 的第 281–300 行（parser recovery/helper、动态
loop、order/tuple boundary、request.security purity/diagnostics、动态 for fallback），按账本写入顺序逐条核对。
本片没有 Rust 生产代码修改；共享 MCP、order 和 request.security 测试仅按已断言字段计证据。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 281 | `parser_and_lowering_recovery_test.go:43:TestMethodAndControlFlowFailuresKeepSourceContracts` | `tests/pine_mcp_contract.rs::semantic_checker_rejects_non_boolean_conditions_and_unsupported_declarations` | P1 | `partial`；Rust 未覆盖 multiline method body/default、incomplete assignment/if、invalid else、switch mutation 等私有 parser 错误。 |
| 282 | `parser_and_lowering_recovery_test.go:119:TestNestedLoopStateAndImportRecoveryRemainStable` | `src/pine/mod.rs::udf_and_loop_boundary_tests::compile_accepts_expression_udf_and_static_for_unroll` | P1 | `partial`；Rust 只证明 public UDF/static loop 可编译，未覆盖 collection/static loop state restore 与 empty import version。 |
| 283 | `parser_and_lowering_recovery_test.go:162:TestOrderAndTupleErrorsPreserveExecutableBoundaries` | `src/pine/mod.rs::public_helper_guard_tests::compile_accepts_strategy_order_subset_scripts` | P1 | `partial`；Rust order subset不是同形错误测试，缺 collection mutation、advanced exit、qty conflict 和 malformed tuple error。 |
| 284 | `parser_and_lowering_recovery_test.go:206:TestOrderCallsRejectUnknownNamedArgumentsBeforePlanning` | `tests/pine_mcp_contract.rs::validation_payload_matches_save_hint_and_rejection_contract` | P1 | `partial`；Rust validation payload 只验证公开 rejection contract，未逐项覆盖 entry/exit unknown named argument 的 parser-before-planning 断言。 |
| 285 | `parser_and_lowering_recovery_test.go:235:TestSourceAnnotationsAndCommentsKeepTheirSeparateRoles` | `src/pine/mod.rs::history_reference_boundary_tests::history_references_ignore_string_literals` | P1 | `partial`；Rust 覆盖字符串中的 history，不覆盖 entry policy annotation、comment tokenization 和 unannotated default。 |
| 286 | `parser_helper_boundaries_test.go:11:TestParserHelperContractsCoverEmptyHeadersAndLexicalEdges` | `tests/pine_mcp_contract.rs::parser_handles_strings_history_and_nested_calls_without_regex` | P1 | `partial`；Rust 未覆盖 empty/indented header、fallback compilation result、declaration block skip、callArgs/callName/unquote 边界。 |
| 287 | `parser_helper_boundaries_test.go:66:TestValidationAndDynamicWhileHelpersKeepDiagnosticsActionable` | `tests/pine_request_and_visual_contracts.rs::unsupported_request_security_forms_keep_the_go_diagnostic_codes` | P2 | `partial`；Rust request.security 诊断子集未覆盖 dynamic while AST/depth、unsupported TA scanner 和 tuple diagnostic helper 细节。 |
| 288 | `parser_helper_boundaries_test.go:110:TestRuntimeLoopAndTupleParserErrorContracts` | `tests/pine_request_and_visual_contracts.rs::request_security_moving_average_keys_keep_type_period_source_and_time_unit` | P2 | `partial`；Rust MTF key 测试不覆盖 runtime loop normalization/body/while error、tuple MTF helper 和 request tuple width。 |
| 289 | `parser_loop_boundaries_test.go:63:TestTupleIndicatorsExposeUnsupportedCallHistory` | `tests/pine_tuple_contracts.rs::malformed_tuple_and_switch_scripts_are_rejected` | P2 | `partial`；Rust malformed tuple 子集未覆盖 indicator call-result history 的 assign-first 诊断和 tuple helper arity。 |
| 290 | `parser_recovery_boundaries_test.go:12:TestParserRejectsMalformedUDFAndStatementBoundaries` | `src/pine/mod.rs::udf_and_loop_boundary_tests::validate_script_reports_supported_udf_and_static_for_boundaries` | P1 | `partial`；Rust 只测部分 UDF/loop validation，未覆盖 malformed header、nested/empty UDF、unsupported executable/statement boundary 矩阵。 |
| 291 | `public_lowering_test.go:38:TestTALoweringBoundariesPreserveInvalidNativeCalls` | `src/pine/mod.rs::advanced_indicator_requirement_tests::compile_supports_v12_advanced_indicators` | P2 | `partial`；Rust advanced indicator compile 不断言 invalid native TA call 原文保留、错误 arity 与 unsupported lowering。 |
| 292 | `public_lowering_test.go:84:TestRequestSecurityArgsFromLineCoversAssignmentForms` | `src/pine/mod.rs::request_security_tests::compile_accepts_native_pine_indicator_public_entry` | P2 | `partial`；Rust public entry 只验证脚本可编译，未覆盖 line-level request.security 参数拆分、assignment forms 和 tuple extraction。 |
| 293 | `public_lowering_test.go:130:TestStrategyQuantityAndMetadataBoundaries` | `src/pine/mod.rs::parse_metadata_tests::compile_parses_backtest_strategy_metadata` | P2 | `partial`；Rust metadata 测试覆盖 backtest fields，未覆盖 Go 的 quantity mode/pyramiding/explicit quantity normalization helper 矩阵。 |
| 294 | `request_security_ast_contracts_test.go:5:TestRequestSecurityPurityRejectsUnsafeLoweredExpressions` | `tests/pine_order_metadata_and_security_rejections.rs::request_security_rejects_impure_member_and_visual_side_effects` | P2 | `partial`；Rust 公共 compile 覆盖若干 side effects，未覆盖 Go lowered-AST purity、source-ambiguous MA 与 unterminated masking helper。 |
| 295 | `request_security_ast_contracts_test.go:26:TestRequestSecurityRejectsMalformedAdvancedIndicatorArguments` | `src/pine/mod.rs::advanced_indicator_requirement_tests::compile_supports_v13_indicators_in_static_intraday_security` | P2 | `partial`；Rust advanced indicator success case 不覆盖 bb/stoch/correlation malformed argument rejection。 |
| 296 | `request_security_diagnostics_test.go:8:TestRequestSecurityDiagnosticsRejectUnsafeOrAmbiguousInputs` | `tests/pine_order_metadata_and_security_rejections.rs::request_security_merge_flags_are_rejected_in_named_and_positional_form` | P2 | `partial`；Rust 只断言 merge flags 子集，Go 还逐项覆盖 unclosed/missing expression/dynamic symbol/timeframe/nested/side-effect/tuple/unsupported TA code+line。 |
| 297 | `request_security_diagnostics_test.go:52:TestRequestSecurityLoweringRetainsOnlyPureStaticExpressions` | `tests/pine_order_metadata_and_security_rejections.rs::request_security_rejects_impure_member_and_visual_side_effects` | P2 | `partial`；Rust 未覆盖 source history、source-aware MA、OBV property、general tuple lowering 与 pure allowlist 全量结果。 |
| 298 | `request_security_object_contracts_test.go:11:TestRequestSecurityLoweringRejectsUnrepresentableExecutionBoundaries` | `tests/pine_order_metadata_and_security_rejections.rs::request_security_merge_flags_are_rejected_in_named_and_positional_form` | P2 | `partial`；Rust merge rejection 不覆盖 gaps tuple、history mask、advanced source/arity 与 object/collection execution boundary。 |
| 299 | `runtime_and_parser_boundaries_test.go:9:TestOrderAndParserBoundaryContracts` | `tests/pine_order_metadata_and_security_rejections.rs::compile_accepts_supported_order_positional_metadata` | P2 | `partial`；Rust order metadata success 子集未覆盖 stale AST fallback、empty lowered source、object/collection normalization 和 no-argument UDF history rejection。 |
| 300 | `runtime_and_parser_boundaries_test.go:46:TestDynamicForBoundsUseRuntimeFallback` | 无可解析 Rust 测试 | P1 | `partial`；动态 start/end/step 应退回 runtime loop 的 Go 私有 parser 语义暂无 Rust 同形测试，需后续补回归。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast
cargo fmt --check
git diff --check
```

本片结论：20/20 均保持 `partial`；第 300 条 P1 动态 loop fallback 明确登记为待补 Rust 回归测试。下一片为 strategy_pine partial 第 301–320 行。

## 第 130 批 12 切片三十二：strategy_pine 域 partial 第 301–320 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 `evidence_type=partial` 的第 301–320 行（runtime/parser、security lowering、
semantic helper、strategy/order、tuple/UDF、PineTS worker client/payload），按账本写入顺序逐条核对。
本片没有 Rust 生产代码修改；跨 crate 引用分别执行了 strategy、integration-pine 和 engine 的精准测试。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 301 | `runtime_and_parser_boundaries_test.go:62:TestNormalizationPreservesInvalidUserSyntaxAndErrors` | 无可解析 Rust 测试 | P2 | `partial`；object/collection normalization、no-arg/nested UDF history、stale AST fallback 与 empty lowered source 尚无 Rust 同形回归。 |
| 302 | `runtime_and_parser_boundaries_test.go:94:TestMalformedTAExpressionsRemainVisibleForValidation` | 无可解析 Rust 测试 | P2 | `partial`；VWAP/anchored VWAP/extrema/stoch malformed call 原文保留没有 Rust 测试证据。 |
| 303 | `runtime_and_parser_boundaries_test.go:134:TestControlFlowErrorsRemainActionableBeforeRuntime` | 无可解析 Rust 测试 | P2 | `partial`；dynamic for/collection/while state rollback、switch invalid source 和 empty if branch 的 parser 错误矩阵未映射。 |
| 304 | `security_lowering_test.go:9:TestRequestSecurityLoweringHelperBusinessBoundaries` | `src/pine/mod.rs::advanced_indicator_requirement_tests::compile_supports_v13_migration_indicators` | P2 | `partial`；Rust advanced indicator success 不覆盖 malformed replacement、symbol/timeframe/merge rejection、tuple width 与 inner lowering 全量 helper 矩阵。 |
| 305 | `security_lowering_test.go:92:TestRequestSecurityPurityAndMergeArgumentBoundaries` | `src/pine/mod.rs::advanced_indicator_requirement_tests::compile_supports_v13_indicators_in_static_intraday_security` | P2 | `partial`；Rust static security success 未覆盖 pure helper allowlist、collection/alert rejection、history/source whitelist 和 merge argument matrix。 |
| 306 | `security_lowering_test.go:168:TestRequestSecurityAdvancedTALoweringBoundaries` | `src/pine/mod.rs::advanced_indicator_requirement_tests::compile_supports_v12_advanced_indicators_in_static_intraday_security` | P2 | `partial`；Rust 高级指标编译证据未覆盖 pivot/kc/tsi/correlation/percentile/swma 的 lowering 与错误 arity/timeframe 拒绝。 |
| 307 | `semantic_helper_boundaries_test.go:99:TestSemanticHelpersReportMalformedScriptBoundaries` | `tests/pine_tuple_contracts.rs::tuple_assignments_keep_the_go_alias_and_width_contract` | P2 | `partial`；Rust tuple alias/width 子集未覆盖 semantic helper 的 malformed script、line/code projection 和 declaration/utility diagnostics。 |
| 308 | `shared_structure_corpus_test.go:35:TestSharedPineStructureCorpusMatchesBackendIR` | `tests/pine_risk_and_block_parity.rs::shared_structure_corpus_projects_statement_kinds_and_branches` | P2 | `partial`；Rust corpus 只比较 statement kinds/branches 子集，未证明 Go corpus 全部 expression/order/risk/IR 字段。 |
| 309 | `strategy_business_test.go:95:TestCompileCoversTrailPriceExitAndShortCloseBusinessSemantics` | `tests/pine_risk_and_block_parity.rs::compile_requires_trailing_offset_for_trailing_exits` | P2 | `partial`；Rust 只验证 trailing offset guard，未覆盖 trail_price、short close、quantity/方向和完整 ExitStmt 业务投影。 |
| 310 | `strategy_call_bounds_test.go:11:TestParseStrategyCallCoversOrderLifecycleBusinessBoundaries` | `tests/pine_order_metadata_and_security_rejections.rs::compile_accepts_supported_order_positional_metadata` + `tests/pine_risk_and_block_parity.rs::compile_keeps_valid_risk_declarations_and_projects_their_limits` | P2 | `partial`；Rust 分散覆盖 success subset，未覆盖 Go 的 entry/order/close/exit/cancel 参数边界与 20+ invalid expression 矩阵。 |
| 311 | `tuple_switch_reject_test.go:8:TestCompileRejectsMalformedSwitchAndTupleContracts` | `tests/pine_tuple_contracts.rs::malformed_tuple_and_switch_scripts_are_rejected` | P2 | `partial`；Rust 覆盖大部分 reject bodies，但 switch/tuple 诊断对象、行号和完整错误消息等价性仍未逐项证明。 |
| 312 | `udf_expansion_contracts_test.go:8:TestUDFExpansionRejectsMalformedRecursiveAndDeepCalls` | 无可解析 Rust 测试 | P2 | `partial`；UDF unclosed/missing/extra args、recursive body、depth cap 和 member-call exclusion 无 Rust 同形测试。 |
| 313 | `validation_semantics_boundaries_test.go:8:TestRequestSecurityValidationExplainsMalformedAndUnsafeExpressions` | `tests/pine_order_metadata_and_security_rejections.rs::request_security_rejects_impure_member_and_visual_side_effects` | P2 | `partial`；Rust compile pipeline 覆盖若干 side effects，未覆盖 Go helper 的 unclosed/missing expression、assignment mutation 和 code/line assertions。 |
| 314 | `validation_semantics_boundaries_test.go:29:TestRequestSecurityExpressionTASubsetValidation` | 无可解析 Rust 测试 | P2 | `partial`；supported/unsupported TA-call scanner 的纯表达式子集与 malformed parenthesis 无 Rust 直接证据。 |
| 315 | `validation_semantics_boundaries_test.go:57:TestRejectUnsupportedReturnsRuntimeAndCollectionBusinessErrors` | 无可解析 Rust 测试 | P2 | `partial`；runtime.error、unsupported collection 和 ordinary assignment 的 rejectUnsupported business errors 未映射。 |
| 316 | `pineengine/pine_ts_client_test.go:20:TestPinetsWorkerClientEngineInfoAndRunIndicator` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_binary_request_and_order_intent_response` | P2 | `partial`；Rust 映射 binary request/order response，但 Go 还验证真实 Node worker EngineInfo、license、plots/signals 和 30s lifecycle。 |
| 317 | `pineengine/pine_ts_client_test.go:56:TestPinetsWorkerClientMapsRuntimeErrors` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_remote_unavailable_timeout_and_cancellation` | P2 | `partial`；Rust 覆盖 remote unavailable/timeout/cancel mapping，未复现 Go 空 candles Node worker error 与 client Close 行为。 |
| 318 | `pineengine/pine_ts_payload_test.go:18:TestExternalModeFromEnvAndDisabledShadowPayload` | `jftrade-engine/src/product_mcp_production_executor_tests.rs::pine_external_mode_parser_accepts_only_supported_values` + `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_freezes_go_owner_sections_and_key_payload_fields` | P2 | `partial`；Rust 验证 mode parser/fixture owner，未覆盖 Go 的 disabled payload compliance map、trimmed env 与 difference summary。 |
| 319 | `pineengine/pine_ts_payload_test.go:39:TestShadowPayloadReportsWorkerStartupFailure` | `jftrade-engine/src/product_mcp_production_executor_tests.rs::pine_shadow_error_payload_keeps_the_worker_failure_message` | P2 | `partial`；Rust 保留 worker failure message，但未逐项验证 Go 的 enabled/OK/status/repository/单诊断字段。 |
| 320 | `pineengine/pine_ts_payload_test.go:55:TestShadowPayloadRunsConfiguredWorkerAndReturnsExternalResult` | `jftrade-engine/src/product_mcp_production_executor_tests.rs::pine_shadow_success_payload_projects_engine_metadata_and_counts` | P2 | `partial`；Rust 验证 metadata/count projection，未覆盖 Go 配置 worker、Node 执行、repository/mode/status 与 plots 差异全量行为。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked --no-fail-fast -E 'test(run_script_maps_binary_request_and_order_intent_response) | test(run_script_maps_remote_unavailable_timeout_and_cancellation)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast -E 'test(pine_external_mode_parser_accepts_only_supported_values) | test(pine_shadow_error_payload_keeps_the_worker_failure_message) | test(pine_shadow_success_payload_projects_engine_metadata_and_counts)'
cargo fmt --check
git diff --check
```

本片结论：20/20 均保持 `partial`；精准验证为 strategy `104/104`、integration-pine `2/2`、engine Pine payload `3/3`，均通过。下一片为 strategy_pine partial 第 321–340 行。

## 第 130 批 12 切片三十三：strategy_pine 域 partial 第 321–340 行，20 行维持、零改判（2026-09-24）

范围：strategy_pine 域 `evidence_type=partial` 的第 321–340 行（PineTS payload/client/runtime、PineSpec
skill/spec、worker client、integration-pine endpoint/readiness/asset/process 与 engine runtime error summary），
按账本写入顺序逐条核对。本片没有 Rust 生产代码修改；跨 crate 引用仅按其直接断言字段计证据，不因存在同名主题而升为 exact。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 321 | `pine_ts_payload_test.go:131:TestPineTsPayloadEndpointAndTokenBoundaries` | `jftrade-integration-pine/src/execution/tests.rs::endpoint_and_token_boundaries_fail_closed` | P2 | `partial`；Rust 验证 endpoint/token 缺失与非法边界 fail-closed，但未覆盖 Go payload 的完整字段投影与差异摘要。 |
| 322 | `pine_ts_payload_test.go:190:TestPineTsPayloadWorkerFailureProjection` | `jftrade-engine/src/product_mcp_production_executor_tests.rs::pine_shadow_error_payload_keeps_the_worker_failure_message` | P2 | `partial`；Rust 保留 worker failure message，未逐项覆盖 Go 的 enabled、status、repository 与 diagnostics payload。 |
| 323 | `pine_ts_payload_test.go:207:TestPineTsPayloadRejectsNonLoopbackWorkerBeforeSpawn` | `jftrade-integration-pine/src/execution/tests.rs::validates_loopback_worker_boundary_before_spawn` | P1 | `partial`；Rust 验证非 loopback worker 在 spawn 前拒绝，未覆盖 Go 的环境清洗、端口组合与错误字段完整性。 |
| 324 | `pine_ts_payload_test.go:224:TestPineTsPayloadCapsWorkerStderrTail` | `jftrade-engine/src/runtime_dependencies_tests.rs::command_error_summary_keeps_output_tail_within_wire_budget` | P1 | `partial`；Rust 验证 stderr tail wire budget，未覆盖 Go payload 对 truncation 标记、status 和多行 stderr 的完整投影。 |
| 325 | `pine_ts_payload_test.go:236:TestPineTsPayloadReadinessShutdownAndJoin` | `jftrade-integration-pine/src/readiness.rs::monitor_shutdown_marks_state_and_joins_task` | P1 | `partial`；Rust 证明 readiness monitor shutdown/join 清理，未复现 Go 的 worker 进程退出码与 payload 生命周期字段。 |
| 326 | `pine_ts_runtime_test.go:12:TestPineTsRuntimeExecutesBundledWorkerSmoke` | `jftrade-integration-pine/tests/real_worker_smoke.rs::rust_client_executes_bundled_pinets_worker` (ignored) | P1 | `partial`；Rust 提供真实 bundled Node PineTS worker smoke 入口但默认 ignored，未形成常规 CI 等价证据。 |
| 327 | `pine_ts_runtime_test.go:31:TestPineTsRuntimeUsesEmbeddedBundleMetadata` | `jftrade-integration-pine/src/asset.rs::test_select_from_fs_returns_embedded_bundle_metadata` | P2 | `partial`；Rust 检查 embedded bundle metadata 存在，未覆盖 Go 的版本、license、入口文件和运行时可执行性全量断言。 |
| 328 | `pine_ts_runtime_test.go:58:TestPineTsRuntimeMapsProtocolBoundaryFailures` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_remote_unavailable_timeout_and_cancellation` | P1 | `partial`；Rust 覆盖 unavailable/timeout/cancellation mapping，未覆盖 Go malformed protocol、空响应和 worker close 顺序。 |
| 329 | `pinespec/skill_metadata_test.go:8:TestPineSpecSkillMetadataPayloadFields` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_freezes_go_owner_sections_and_key_payload_fields` | P2 | `partial`；Rust 冻结 owner sections/key payload fields，未覆盖 Go skill metadata 的全部资源、描述和版本字段。 |
| 330 | `pinespec/skill_metadata_test.go:80:TestPineSpecUnknownSectionUsesFallbackFormatting` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_freezes_go_owner_sections_and_key_payload_fields` | P2 | `partial`；Rust 冻结已知 section/key payload，未覆盖 unknown section fallback 文本、多 section 顺序、空值和资源链接组合。 |
| 331 | `pinespec/spec_test.go:14:TestPineSpecExamplesParseAndPlan` | `jftrade-strategy/tests/pine_mcp_contract.rs::native_pipeline_parses_lowers_and_plans_strategy_requirements` | P2 | `partial`；Rust 覆盖 native pipeline parse/lower/plan 基本路径，未逐项对齐 Go 每个 example 的语义节点、错误和计划字段。 |
| 332 | `pinespec/spec_test.go:28:TestPineSpecGoldenExamplesAnalyzeAndPlan` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_examples_section_includes_examples_when_selected_or_requested` | P2 | `partial`；Rust 验证 examples section 输出，未证明 Go golden 的完整 diagnostics、requirements 与 output 顺序。 |
| 333 | `pinespec/spec_test.go:59:TestPineSpecToolPayloadSectionsAndExamples` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_freezes_go_owner_sections_and_key_payload_fields` | P2 | `partial`；Rust 覆盖 payload section/example 子集，未逐字段迁移 Go tool payload 的所有 section 和示例内容。 |
| 334 | `pinespec/spec_test.go:108:TestPineSpecSupportMatrix` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_freezes_go_owner_sections_and_key_payload_fields` | P2 | `partial`；Rust 核对 support-matrix section 边界，未覆盖 Go matrix 的全部 feature 状态、原因和版本列。 |
| 335 | `pinespec/spec_test.go:248:TestPineSpecBrokerBoundary` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_freezes_go_owner_sections_and_key_payload_fields` | P2 | `partial`；Rust 验证 broker boundary 字段存在，未覆盖 Go broker capability/error payload 与 order lifecycle 说明。 |
| 336 | `pinespec/spec_test.go:285:TestPineSpecSkillResourcesAndExamples` | `jftrade-strategy/tests/pine_mcp_contract.rs::pine_spec_examples_section_includes_examples_when_selected_or_requested` | P2 | `partial`；Rust 检查 examples section 输出，未覆盖 Go 的资源数量、路径、内容摘要和 fallback 组合。 |
| 337 | `pineworker/client_test.go:12:TestPineWorkerClientMetadataDefaults` | `jftrade-integration-pine/src/asset.rs::test_select_from_fs_returns_embedded_bundle_metadata` | P2 | `partial`；Rust 覆盖 embedded bundle metadata，未覆盖 Go client 初始化时的完整 engine info、license 与 worker capability projection。 |
| 338 | `pineworker/client_test.go:41:TestPineWorkerClientRejectsInvalidRequestBeforeTransport` | `jftrade-integration-pine/src/execution/tests.rs::request_validation_rejects_every_incomplete_or_inconsistent_field` | P1 | `partial`；Rust 证明 invalid request 在 transport 前拒绝，未覆盖 Go 各类 request shape、字段路径和错误码文本。 |
| 339 | `pineworker/client_test.go:56:TestPineWorkerClientUsesExactJsonSize` | `jftrade-integration-pine/src/execution/tests.rs::grpc_request_message_limit_has_exact_encoded_boundaries` | P2 | `partial`；Rust 核对 encoded request size boundary，未覆盖 Go JSON byte size、unicode、嵌套 payload 与 framing/limit 组合。 |
| 340 | `pineworker/client_test.go:81:TestPineWorkerClientMapsTransportError` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_remote_unavailable_timeout_and_cancellation` | P1 | `partial`；Rust 映射 remote unavailable/timeout/cancel，未覆盖 Go EOF/reset 分类、可重试标记和 Close 后行为。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast -E 'test(command_error_summary_keeps_output_tail_within_wire_budget)'
cargo fmt --check
git diff --check
```

本片结论：20/20 均保持 `partial`；精准测试为 strategy `104/104`、integration-pine `50/50`（另 1 skipped）、
engine runtime `1/1`（另 1929 skipped），均通过。未发现可安全升为 `[x]` 的条目，也没有足够证据触发 Rust
生产功能修复。下一片为 strategy_pine partial 第 341–360 行。

## 第 130 批 12 切片三十四：strategy_pine 域 partial 第 341–370 行，30 行维持、零改判（2026-09-24）

范围：strategy_pine 域 `evidence_type=partial` 的第 341–370 行（Pine worker client/transport、manager
readiness/pool、payload size、Node worker bundle materialization/process lifecycle）。本批按 30 条处理以减少
审计与切片切换；没有 Rust 生产代码修改。共享 pool/readiness/asset 测试只按直接断言计证据，未把相邻生命周期
主题视为等价行为。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 341 | `pineworker/client_test.go:92:TestClientRunScriptMapsTimeout` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_remote_unavailable_timeout_and_cancellation` | P1 | `partial`；Rust 覆盖 timeout 映射，但未证明 Go 的 client 错误文案、deadline 分类与响应清理完全一致。 |
| 342 | `pineworker/client_test.go:103:TestClientRunScriptMapsWorkerError` | `jftrade-integration-pine/src/execution/tests.rs::run_script_rejects_worker_error_and_identity_mismatch` | P2 | `partial`；Rust 拒绝 worker error 并保留失败边界，未覆盖 Go compile failed 文案和 metadata 组合。 |
| 343 | `pineworker/client_test.go:114:TestClientRunScriptRejectsMismatchedJobID` | `jftrade-integration-pine/src/execution/tests.rs::run_script_rejects_worker_error_and_identity_mismatch` | P2 | `partial`；Rust 覆盖 identity mismatch，未单独证明 Go 的 job id 错误文本、空 metadata 与响应消费顺序。 |
| 344 | `pineworker/client_test.go:161:TestNewClientRequiresTransport` | `jftrade-integration-pine/src/execution/tests.rs::endpoint_and_token_boundaries_fail_closed` | P2 | `partial`；Rust 验证 endpoint/token 配置 fail-closed，但没有 Go `NewClient(nil)` 构造器错误的同形断言。 |
| 345 | `pineworker/grpc_dialer_test.go:9:TestGRPCDialerCreatesManagedTransport` | `jftrade-integration-pine/src/process.rs::grpc_probe_authenticates_and_rejects_endpoint_identity_mismatch` | P2 | `partial`；Rust 覆盖 gRPC probe/auth 与 endpoint identity，未证明 Go managed transport 的创建、Close 和 nil receiver 矩阵。 |
| 346 | `pineworker/grpc_transport_test.go:16:TestGRPCTransportRunScriptAndHealthCheck` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_binary_request_and_order_intent_response` + `jftrade-integration-pine/src/mock_worker.rs::health_probe_authenticates_and_reports_controllable_ok_flag` | P2 | `partial`；Rust 分别覆盖 RunScript response 与 health probe，未逐字段对齐 Go plots/order/metadata/strategy metrics 和 transport nil 边界。 |
| 347 | `pineworker/hardcut_audit_test.go:12:TestPineTSHardCutDoesNotExposeGoPineRuntime` | `jftrade-integration-pine/tests/real_worker_smoke.rs::rust_client_executes_bundled_pinets_worker` (ignored) | P2 | `partial`；Rust 只有 ignored bundled-worker smoke，未迁移 Go 的仓库扫描、前端/文档 hard-cut 与 CI 资产审计矩阵。 |
| 348 | `pineworker/manager_readiness_recovery_test.go:15:TestWorkerManagerReadinessFailuresCloseTransportAndRespectCancellation` | `jftrade-integration-pine/src/readiness.rs::monitor_shutdown_marks_state_and_joins_task` | P1 | `partial`；Rust 覆盖 monitor shutdown/join，未覆盖 Go readiness failure 关闭 transport、取消传播和重启状态组合。 |
| 349 | `pineworker/manager_test.go:14:TestWorkerManagerStartStopAndSnapshot` | `jftrade-integration-pine/src/pool.rs::health_results_and_restarts_project_into_the_snapshot` | P2 | `partial`；Rust 验证健康/重启 snapshot 投影，未覆盖 Go start-stop 全生命周期、worker id/version 与空快照断言。 |
| 350 | `pineworker/manager_test.go:200:TestWorkerManagerRunScriptRejectsWhenBusyIfConfigured` | `jftrade-integration-pine/src/pool.rs::healthy_workers_are_selected_in_rotation` | P2 | `partial`；Rust 只验证健康 worker 轮转，不覆盖 Go busy gate、并发 RunScript 拒绝和配置开关。 |
| 351 | `pineworker/manager_test.go:236:TestWorkerManagerCheckHealthRestartsFailedWorker` | `jftrade-integration-pine/src/pool.rs::health_results_and_restarts_project_into_the_snapshot` | P2 | `partial`；Rust 覆盖 restart count/health snapshot，未覆盖 Go 检查触发时序、重启后连接重建与所有错误字段。 |
| 352 | `pineworker/manager_test.go:262:TestWorkerManagerCheckHealthReportsRestartFailure` | `jftrade-integration-pine/src/pool.rs::health_results_and_restarts_project_into_the_snapshot` | P2 | `partial`；Rust 有 restart snapshot 证据，未覆盖 Go restart failure 的错误传播、LastError 文案和 manager 状态回滚。 |
| 353 | `pineworker/manager_test.go:281:TestWorkerManagerStartCleansUpAfterDialFailure` | `jftrade-integration-pine/src/process.rs::validates_loopback_worker_boundary_before_spawn` | P2 | `partial`；Rust 验证 spawn 前 loopback 边界，未覆盖 Go 多 worker dial 失败时全部 process stop 与空 snapshot 清理。 |
| 354 | `pineworker/manager_test.go:299:TestWorkerManagerStartDialFailureIncludesProcessDiagnostics` | `jftrade-integration-pine/src/process.rs::grpc_probe_authenticates_and_rejects_endpoint_identity_mismatch` | P2 | `partial`；Rust 覆盖 probe 身份失败，未覆盖 Go bundle/runtime/cwd/stderr diagnostics 拼接和失败 worker stop。 |
| 355 | `pineworker/manager_test.go:324:TestWorkerManagerStartRetriesDialUntilWorkerReady` | `jftrade-integration-pine/src/process.rs::readiness_policy_preserves_go_delay_and_supports_capped_backoff` | P2 | `partial`；Rust 验证 Go delay/backoff cap，未覆盖 Go 实际 dial attempt 次数、ready snapshot 和 retry 成功路径。 |
| 356 | `pineworker/manager_test.go:341:TestWorkerManagerStopReturnsFirstCloseError` | `jftrade-integration-pine/src/readiness.rs::monitor_shutdown_marks_state_and_joins_task` | P2 | `partial`；Rust 证明 monitor 可 shutdown/join，未覆盖 Go first close error 优先级和 Stop 后 pool 清空。 |
| 357 | `pineworker/manager_test.go:359:TestWorkerManagerRequiresDependenciesAndStart` | `jftrade-integration-pine/src/pool.rs::empty_pools_and_unknown_workers_fail_closed` | P1 | `partial`；Rust 覆盖 empty/unknown worker fail-closed，未覆盖 Go zero workers、缺 launcher 与未 Start 运行请求的构造器矩阵。 |
| 358 | `pineworker/payload_size_test.go:9:TestJSONSizeMatchesMarshalAcrossPayloadShapes` | `jftrade-integration-pine/src/execution/tests.rs::grpc_request_message_limit_has_exact_encoded_boundaries` | P2 | `partial`；Rust 核对 protobuf encoded boundary，不是 Go JSON marshal size 的 nil/map/unicode/array 全量矩阵。 |
| 359 | `pineworker/payload_size_test.go:26:TestEstimateRunScriptRequestJSONSizeHandlesNilAndEmptyCollections` | `jftrade-integration-pine/src/execution/tests.rs::grpc_request_message_limit_has_exact_encoded_boundaries` | P1 | `partial`；Rust 覆盖 request message size 限制，未证明 Go nil/empty collections 的 JSON byte size 精确相等。 |
| 360 | `pineworker/payload_size_test.go:49:TestEstimateCandleJSONSizeRejectsNonFiniteFields` | `jftrade-integration-pine/src/execution/tests.rs::request_validation_rejects_oversized_source_and_invalid_candle` | P2 | `partial`；Rust 拒绝 invalid candle，未逐项覆盖 Go NaN/+Inf/-Inf 的 candle 与 candle-list size helper 错误。 |
| 361 | `pineworker/process_launcher_boundaries_test.go:19:TestNodeWorkerLauncherDefaultsAndMaterializationBoundaries` | `jftrade-integration-pine/src/asset.rs::test_select_from_fs_returns_embedded_bundle_metadata` | P2 | `partial`；Rust 覆盖 embedded bundle metadata，未覆盖 Go launcher 默认 runtime/name/timeout、临时目录和 materialize bytes。 |
| 362 | `pineworker/process_launcher_boundaries_test.go:70:TestNodeWorkerLauncherRemovesMaterializedBundleWhenContextAlreadyCanceled` | `jftrade-integration-pine/src/asset.rs::checksum_is_verified_before_worker_asset_is_written` | P1 | `partial`；Rust 验证 checksum 在写文件前检查，未覆盖 Go 已取消 context 下启动失败和 materialized bundle 清理。 |
| 363 | `pineworker/process_launcher_boundaries_test.go:94:TestOSWorkerProcessDiagnosticsAndNilBoundaries` | `jftrade-engine/src/runtime_dependencies.rs::command_error_summary_keeps_output_tail_within_wire_budget` | P2 | `partial`；Rust 验证 command error tail wire budget，未覆盖 Go nil process、stdout 2000 字符尾部、writerString 和 exit helper。 |
| 364 | `pineworker/process_launcher_boundaries_test.go:118:TestOSWorkerProcessForcesTerminationOnTimeoutAndCancellation` | `jftrade-integration-pine/src/process.rs::readiness_policy_preserves_go_delay_and_supports_capped_backoff` | P1 | `partial`；Rust 只有 readiness backoff 证据，未覆盖 Go interrupt-ignoring process 的超时/取消强制终止及文件清理。 |
| 365 | `pineworker/process_launcher_boundaries_test.go:165:TestPineworkerInterruptIgnoringHelperProcess` | `jftrade-integration-pine/src/readiness.rs::monitor_shutdown_marks_state_and_joins_task` | P2 | `partial`；Rust monitor join 不等价于 Go helper process 忽略信号并验证终止路径，需后续进程级回归测试。 |
| 366 | `pineworker/process_launcher_test.go:16:TestNodeWorkerLauncherMaterializesBundleWithArgs` | `jftrade-integration-pine/src/asset.rs::checksum_is_verified_before_worker_asset_is_written` | P2 | `partial`；Rust 覆盖 bundle 写入完整性，未覆盖 Go cwd、stderr、worker args、extra args 与 mock 参数顺序。 |
| 367 | `pineworker/process_launcher_test.go:72:TestNodeWorkerLauncherRejectsBadChecksum` | `jftrade-integration-pine/src/asset.rs::checksum_is_verified_before_worker_asset_is_written` | P2 | `partial`；Rust 验证 checksum 先于发布，未逐项对齐 Go bad checksum 错误文本和启动前不残留文件。 |
| 368 | `pineworker/process_launcher_test.go:88:TestNodeWorkerLauncherStopKillsLongRunningProcess` | `jftrade-integration-pine/src/readiness.rs::monitor_shutdown_marks_state_and_joins_task` | P2 | `partial`；Rust 只覆盖 monitor task join，未覆盖 Go long-running worker 的 Stop timeout、kill 与进程回收。 |
| 369 | `pineworker/process_launcher_test.go:107:TestNewNodeWorkerLauncherRequiresBundle` | `jftrade-integration-pine/src/asset.rs::test_select_from_fs_treats_missing_and_empty_bundles_as_unavailable` | P2 | `partial`；Rust 覆盖 missing/empty bundle unavailable，未覆盖 Go launcher 构造器缺 bundle data 的错误边界。 |
| 370 | `pineworker/process_launcher_test.go:141:TestNodeWorkerLauncherReturnsStartErrorAndRemovesFile` | `jftrade-integration-pine/src/asset.rs::test_select_from_fs_treats_missing_and_empty_bundles_as_unavailable` | P2 | `partial`；Rust 覆盖 unavailable bundle，未覆盖 Go runtime start error 后 materialized file 删除和错误传播。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast -E 'test(command_error_summary_keeps_output_tail_within_wire_budget)'
cargo fmt --check
git diff --check
```

本片结论：30/30 均保持 `partial`；下一批固定为每批 30 条，下一片为 strategy_pine partial 第 371–400 行。
P1 的 timeout/cancellation、busy gate、retry/rollback、进程强制终止和精确 JSON size 缺口已保留为后续回归方向；
本片没有足够证据升格 `[x]`，也没有发现需要立即修改 Rust 生产代码的单一行为差异。

## 第 130 批 12 切片三十六：strategy_pine partial 第 241–260 行（2026-09-24）

本片按 20 条尾批逐项读取 Go 函数体并复核 Rust 断言粒度；没有把聚合测试或
worker-owned 旧解析助手升格为 `function_exact`。其中第 260 行发现真实的
`request.security` 内部 TA 校验缺口，先以失败回归测试复现，再在 semantic owner
拒绝不支持的 TA/参数组合。

| 序号 | Go 测试 | Rust 证据/结论 |
| ---: | --- | --- |
| 241 | `compiler_and_security_diagnostics_test.go:70:TestPineEditorRecoveryAndCapabilityEvidenceContracts` | `partial`；pinespec 能力矩阵有旁证，但 Rust 没有 Go 编辑器恢复助手与 analyzed=0.5 计分层。 |
| 242 | `compiler_rejection_contracts_test.go:9:TestUnsupportedSyntaxDiagnosticsDescribeUnsafeRequestSecurityContracts` | `partial`；静态 timeframe 与主要拒绝码有证据，malformed 参数、collection 文案和部分 tuple 专码仍不同。 |
| 243 | `compiler_rejection_contracts_test.go:79:TestTupleHelpersRejectMalformedIndicatorArityWithoutInventingAliases` | `partial`；Rust 覆盖 tuple arity/period 拒绝，缺 Go helper handoff 与 normalizationErr 传播。 |
| 244 | `compiler_rejection_contracts_test.go:180:TestTALoweringLeavesMalformedCallsUntouched` | `partial`；窗口缺省已在 planner 对齐，Rust 没有旧字符串 lowering 的原文保留层。 |
| 245 | `control_flow_reject_test.go:8:TestCompileRejectsInvalidControlFlowAndUDFContracts` | `partial`；for 可达性有证据，UDF 参数/多行体、break/continue 专文案与嵌套深度仍缺。 |
| 246 | `control_flow_reject_test.go:43:TestCompilePreservesDescendingAndConditionalLoopSemantics` | `partial`；Rust 当前不支持 while 与 continue/break 的 Go 语义，保留 P1 失败回归方向。 |
| 247 | `controlflow_object_collection_contracts_test.go:11:TestControlFlowParserRetainsUserFunctionAndCollectionLoopSemantics` | `boundary`；多行 UDF 与 collection loop 属 PineTS/旧 owner，Rust 无同形执行面。 |
| 248 | `controlflow_object_collection_contracts_test.go:85:TestObjectLifecycleParserCoversFieldsMultilineMethodsAndReceivers` | `boundary`；UDT 字段、方法重载和 receiver 降级不在 Rust engine owner。 |
| 249 | `controlflow_object_collection_contracts_test.go:157:TestCollectionParserKeepsHistoryAndExpressionContracts` | `boundary`；集合声明、历史读取和 collection namespace 仍由 worker 侧承担。 |
| 250 | `extended_ticker_test.go:5:TestExtendedTickerRequestSecuritySupportsCurrentSymbolOnly` | `partial`；Rust whitelist 与动态 symbol 诊断已测，但 requirement 键仍不是 Go 的 `security_source`。 |
| 251 | `language_execution_boundaries_test.go:12:TestRequestSecuritySupportedTAFamiliesKeepTheirExecutionContracts` | `partial`；timeframe 映射与部分 TA 已覆盖，MTF TA 键覆盖及 security_source 键空间仍有缺口。 |
| 252 | `language_execution_boundaries_test.go:99:TestCollectionExecutionParserKeepsReceiverAndErrorBoundaries` | `boundary`；collection parser/lowering 属旧 worker owner，Rust 没有同形入口。 |
| 253 | `language_execution_boundaries_test.go:179:TestTupleLoweringHelpersMaintainAliasAndArityContracts` | `partial`；tuple 宽度/拒绝有证据，typed IR 不保留 Go expressionAliases 与 MTF tuple helper。 |
| 254 | `language_execution_boundaries_test.go:287:TestCompilerHeadersAndLexicalHelpersRetainPineV6Rules` | `partial`；version/strategy header 拒绝一致，CRLF scanner、regexp cache、TA/color helper 无同形 Rust 断言。 |
| 255 | `language_execution_boundaries_test.go:364:TestObjectExecutionParserPreservesDeclaredTypeContracts` | `boundary`；UDT constructor/method overload 仍属于 worker-owned 旧执行面。 |
| 256 | `language_execution_boundaries_test.go:454:TestCollectionLexicalParserRejectsMalformedHistoryAndNamespaceReferences` | `boundary`；collection lexical namespace/history helper 未迁移。 |
| 257 | `language_execution_boundaries_test.go:531:TestUDFAndDynamicLoopHelpersProtectRuntimeBoundaries` | `partial`；Rust 缺 UDF 多行体与 runtime for/while，保留 P1 回归要求。 |
| 258 | `language_execution_boundaries_test.go:600:TestTALoweringHelpersKeepNativePineArgumentSemantics` | `partial`；planner 已覆盖窗口族缺省，旧 TA 字符串重写与高级 security helper 仍无同形层。 |
| 259 | `language_failure_contracts_test.go:9:TestObjectAndCollectionParserFailureContracts` | `boundary`；对象/集合失败契约由 PineTS worker 承担，Rust 只提供整体拒绝边界。 |
| 260 | `language_failure_contracts_test.go:149:TestRequestSecurityAndTupleContractsRejectUnsafeExpressions` | `partial`；新增 `request_security_rejects_unsupported_inner_ta_contracts`，修复 `ta.sum`、缺 multiplier 的 `ta.bb`、非法 correlation source 与日周期 `ta.obv` 误接受；Go helper 级纯度/lowering 仍无同形实现。 |

本片验证：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked -E 'test(request_security_rejects_unsupported_inner_ta_contracts) | test(request_security_rejects_unlisted_static_timeframe_strings) | test(request_security_tickers_follow_the_go_whitelist) | test(request_security_tuple_diagnostics_match_go_codes) | test(tuple_assignments_keep_the_go_alias_and_width_contract) | test(window_family_defaults_keep_the_go_source_and_period) | test(malformed_tuple_and_switch_scripts_are_rejected) | test(compile_supports_framework_language_features)'
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
cargo fmt --all -- --check
git diff --check
```

8 个精准 Rust 测试全部通过；本片 20/20 仍为 `partial` 或明确 `boundary`，没有新增
`function_exact`。下一片固定为 strategy_pine partial 第 261–280 行。

## 第 130 批 12 切片三十五：strategy_pine 域 partial 第 371–385 行，15 行维持、零改判（2026-09-24）

范围：strategy_pine partial 队列尾部第 371–385 行（process smoke、protobuf contract/mapping、runtime
boundaries、settings/types）。partial 队列共 385 条，因此本片为尾批 15 条；没有 Rust 生产代码修改。

| 序号 | Go 测试 | Rust 证据 | 级别 | 结论 |
| ---: | --- | --- | :---: | --- |
| 371 | `pineworker/process_smoke_test.go:20:TestWorkerManagerProcessSmokeWithNodeWorker` | `jftrade-integration-pine/tests/real_worker_smoke.rs::rust_client_executes_bundled_pinets_worker` (ignored) | P2 | `partial`；Rust 提供 bundled worker smoke 入口但默认 ignored，未形成 Go mock Node 进程 manager 的常规 CI 证据。 |
| 372 | `pineworker/process_smoke_test.go:34:TestWorkerManagerRealPineTSProcessSmoke` | `jftrade-integration-pine/tests/real_worker_smoke.rs::rust_client_executes_bundled_pinets_worker` (ignored) | P2 | `partial`；Rust smoke 依赖外部 bundle/runtime 且 ignored，未覆盖 Go 的 pinets 安装检查、版本 metadata 和 manager start/stop。 |
| 373 | `pineworker/proto_contract_test.go:14:TestPineWorkerProtoCompilesAndExposesContract` | `jftrade-integration-pine/src/execution/tests.rs::grpc_request_message_limit_has_exact_encoded_boundaries` | P2 | `partial`；Rust 验证 gRPC 编码边界，不是 Go protoc descriptor 的 package/import/service/message/field 全量契约审计。 |
| 374 | `pineworker/proto_mapping_test.go:15:TestProtoMappingRoundTripRequestAndResponse` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_binary_request_and_order_intent_response` | P2 | `partial`；Rust 映射 binary request/order response 子集，未逐字段覆盖 Go session/chart/alerts/visuals/diagnostics/metrics 与 nil mapping。 |
| 375 | `pineworker/proto_mapping_test.go:154:TestCandleBatchEncodingGoldenVector` | `jftrade-integration-pine/src/execution/tests.rs::grpc_request_message_limit_has_exact_encoded_boundaries` | P2 | `partial`；Rust 证明 encoded message limit，未覆盖 Go CandleBatch 固定 56-byte little-endian golden vector。 |
| 376 | `pineworker/proto_mapping_test.go:197:TestHealthFromProtoCopiesCapabilities` | `jftrade-integration-pine/src/mock_worker.rs::health_probe_authenticates_and_reports_controllable_ok_flag` | P2 | `partial`；Rust 验证 health probe/auth/ok 状态，未覆盖 Go capabilities 深拷贝、version/PineTS 字段与 nil health。 |
| 377 | `pineworker/runtime_boundaries_test.go:14:TestTailBufferRetainsOnlyConfiguredProcessLogTail` | `jftrade-engine/src/runtime_dependencies.rs::command_error_summary_keeps_output_tail_within_wire_budget` | P2 | `partial`；Rust 保证 command error tail 在 wire budget 内，未覆盖 Go 可配置 tail buffer、stdout/stderr 来源和精确 2000 字节截断。 |
| 378 | `pineworker/runtime_boundaries_test.go:66:TestWorkerConfigAndCandleTimeBoundaries` | `jftrade-settings/src/pine_worker.rs::worker_limits_and_nested_quotes_match_go_settings_owner` | P2 | `partial`；Rust 覆盖 settings limits/quotes，未覆盖 Go candle 时间顺序、zero/negative 边界和完整 WorkerConfig 默认值矩阵。 |
| 379 | `pineworker/runtime_boundaries_test.go:79:TestClientDefaultsAndResponseIdentityBoundaries` | `jftrade-integration-pine/src/execution/tests.rs::endpoint_and_token_boundaries_fail_closed` | P2 | `partial`；Rust 覆盖 endpoint/token fail-closed，未覆盖 Go client timeout/now 默认、response identity 和 nil client 行为。 |
| 380 | `pineworker/runtime_boundaries_test.go:107:TestGRPCTransportPropagatesRPCFailures` | `jftrade-integration-pine/src/execution/tests.rs::run_script_maps_remote_unavailable_timeout_and_cancellation` | P2 | `partial`；Rust 映射 unavailable/timeout/cancel，未覆盖 Go RunScript/HealthCheck 原始 RPC error 传播、nil transport 和 dialer defaults。 |
| 381 | `pineworker/runtime_boundaries_test.go:134:TestWorkerManagerSelectionCapacityAndErrorBoundaries` | `jftrade-integration-pine/src/pool.rs::live_sessions_are_pinned_and_open_failure_rolls_back` | P2 | `partial`；Rust 覆盖 live-session pin/rollback，未覆盖 Go capacity token、取消 acquire、未启动/无健康 worker 与 execution error 矩阵。 |
| 382 | `pineworker/runtime_boundaries_test.go:189:TestWorkerManagerDiagnosticErrorFormatting` | `jftrade-integration-pine/src/process.rs::grpc_probe_authenticates_and_rejects_endpoint_identity_mismatch` | P2 | `partial`；Rust 覆盖 probe identity mismatch，未覆盖 Go health/restart diagnostics 四种组合的稳定错误格式。 |
| 383 | `pineworker/types_test.go:10:TestNormalizeRuntimeMigratesLegacyRuntime` | `jftrade-settings/src/pine_worker.rs::worker_limits_and_nested_quotes_match_go_settings_owner` | P2 | `partial`；Rust settings owner 不证明 Go legacy runtime id 归一化与 unsupported runtime 拒绝。 |
| 384 | `pineworker/types_test.go:21:TestDefaultWorkerConfigScalesByCPU` | `jftrade-settings/src/pine_worker.rs::worker_limits_and_nested_quotes_match_go_settings_owner` | P2 | `partial`；Rust 只覆盖配置限制/嵌套 quotes，未覆盖 Go CPU=1/8 的 live/backtest/optimization worker scaling 及 timeout/message/candle defaults。 |
| 385 | `pineworker/types_test.go:164:TestRunScriptPayloadSizeRejectsNonFiniteCandle` | `jftrade-integration-pine/src/execution/tests.rs::non_finite_candle_values_are_rejected_before_transport` | P2 | `partial`；Rust 拒绝 non-finite candle before transport，未覆盖 Go JSON size helper 对 NaN/+Inf/-Inf 及 candle list 的完整错误矩阵。 |

验证命令：

```bash
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked --no-fail-fast
node scripts/quality/cargo-nextest.mjs run -p jftrade-settings --all-targets --locked --no-fail-fast -E 'test(worker_limits_and_nested_quotes_match_go_settings_owner)'
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast -E 'test(command_error_summary_keeps_output_tail_within_wire_budget)'
cargo fmt --check
git diff --check
```

本片结论：15/15 均保持 `partial`；strategy_pine partial 队列至此全部核对完毕（385/385）。ignored smoke、
descriptor/golden mapping、legacy runtime normalization 和 CPU scaling 缺口均保留为后续补测方向；没有足够证据
升格 `[x]`，也没有发现应立即修改 Rust 生产代码的单一行为差异。下一步应转入其他领域的 P0/P1 队列。

## 第 130 批 12 切片三十七：strategy_pine partial 第 261–280 行（2026-09-24）

本片按当前窗口逐条读取 20 个 Go 测试函数体，并将断言拆成 Rust 已有证据、
worker-owned 边界和待补功能三类。没有因为引用了 `framework_language_feature_tests`
这一聚合用例就升格为 `[x]`；Go 的 UDT/collection 降级、Pine v2.0–v2.3
语言表面和 benchmark corpus 仍保持 `partial`/`boundary`。

| 序号 | Go 测试 | Rust 证据/结论 |
| ---: | --- | --- |
| 261 | `object_collect_bounds_test.go:12:TestObjectNamedArgumentNormalizationBusinessBoundaries` | `boundary`；Go 断言 UDT 构造器/方法具名参数补默认值、位置参数混用、重复/未知/缺省参数；归一化 owner 是 PineTS worker，Rust 编译器没有同形入口。 |
| 262 | `object_collect_bounds_test.go:57:TestObjectMethodLoweringHandlesHistoryNamedDefaultsAndExpressionReceivers` | `boundary`；Go 的 `object_method(...)` 与 `history(box, 1)` 接收者降级由 worker 完成，Rust 仅有独立 history 引用证据，不能视为方法 lowering 等价。 |
| 263 | `object_collect_bounds_test.go:83:TestCollectionStatementsAndReadLoweringBusinessBoundaries` | `boundary`；typed collection statement、`collection_array_*` read lowering 和缺 target 错误不属于 Rust strategy IR owner。 |
| 264 | `object_collect_reject_test.go:9:TestCompileRejectsInvalidObjectAndCollectionContracts` | `boundary`；Go 25 个对象/集合拒绝文案由 worker parser 负责，Rust 只保留整体 unsupported declaration/call 边界。 |
| 265 | `object_declaration_contracts_test.go:10:TestObjectDeclarationContractsRejectMalformedDomainTypesAndMethods` | `boundary`；非法 type/method/receiver/default/递归契约没有 Rust parser 同形入口，不能用 MCP rejection 聚合测试代替。 |
| 266 | `object_declaration_contracts_test.go:120:TestObjectCallsPreserveNamedArgumentAndOverloadSafety` | `boundary`；对象构造/方法 overload 与具名参数安全由 worker 承担，Rust 不实现该调用重载层。 |
| 267 | `order_command_security_rejection_test.go:82:TestRequestSecurityTupleValidationKeepsParserBoundaries` | `partial`；tuple 匹配和宽度诊断已有 `request_security_tuple_diagnostics_match_go_codes`，未闭合 call/TA helper 的错误码仍与 Go helper 契约不同。 |
| 268 | `order_metadata_contracts_test.go:8:TestOrderMetadataRejectsAmbiguousInputsAndKeepsSupportedPositionals` | `partial`；Rust 已覆盖 close/close_all metadata 拒绝、单引号解码与 positional close 编译通过；`symbol_position_percent` 内部模式和旧 helper 级参数投影仍未同形。 |
| 269 | `parse_benchmark_business_test.go:5:TestBenchmarkBusinessScriptsCompileAsRegressionCases` | `partial`；Go 的 6 个 benchmark case（minimal、indicator-heavy、MTF、native indicators、UDF/static-for、orders）未迁移为 Rust corpus；指标、security、UDF/loop、order 子集有分散旁证，但不等价于逐 case `Compile`+`AnalyzeScript`+AST/Semantic 断言。 |
| 270 | `parse_collection_test.go:11:TestAnalyzeScriptIncludesV20CollectionAndDeclarationSemantics` | `partial`；Rust 对同脚本保持显式 unsupported diagnostics，但未提供 Go 的 typed collection/declaration semantic projection；待 worker/runtime owner 补齐后再加失败回归。 |
| 271 | `parse_collection_test.go:140:TestPineV20LanguageFoundationGate` | `boundary`；Go 的语言 foundation gate 是旧 parser 的整体版本闸门，Rust 按能力逐项校验；同脚本的 parse-only 诊断不能当作 gate 等价。 |
| 272 | `parse_collection_test.go:210:TestAnalyzeScriptReportsCollectionOperationSignatureDiagnostics` | `partial`；Go 的 collection signature diagnostics 在 Rust 仍落到 unsupported call，缺少逐 operation typed metadata 与稳定专码。 |
| 273 | `parse_collection_test.go:233:TestAnalyzeScriptIncludesCollectionMethodStyleOperations` | `partial`；Rust 不实现 `arr.push`/`arr.get`/`map.put`/`matrix.set` 的 collection runtime projection，不能由通用 framework test 覆盖。 |
| 274 | `parse_collection_test.go:274:TestAnalyzeScriptIncludesTypedCollectionDeclarations` | `partial`；Rust 保留 typed declaration 的拒绝边界，但没有 Go 的 declaration AST/collection operation 结构。 |
| 275 | `parse_collection_test.go:330:TestCompileSupportsV21ExecutableCollectionCore` | `partial`；Go 的 9 条可执行 collection statement 与 runtime values 缺失，Rust 同脚本仍拒绝，待 PineTS/执行 owner 补回归。 |
| 276 | `parse_collection_test.go:375:TestCompileSupportsV21CollectionAliases` | `partial`；alias mutation/read 的 collection target 传播不在 Rust IR，不能以普通 assignment 或 framework test 冒充。 |
| 277 | `parse_collection_test.go:400:TestCompileSupportsV21BBWAndCOG` | `partial`；Rust 有 v12/v13 指标 requirement 测试和 security timeframe 测试，但本 Go 场景的 `bbw`、`cog`、anchored `vwap` 及其 MTF requirement key 未形成同一条 Rust 断言链。 |
| 278 | `parse_collection_test.go:430:TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops` | `partial`；Rust tuple assignment、structured AST 和 static-for 各有专项测试，但 Go 的一般 tuple + dynamic `for`/`while`/`continue`/`break` 组合尚未完整对齐。 |
| 279 | `parse_collection_test.go:477:TestCompileSupportsV22PureUDTAndMethodSubset` | `partial`；Go 期望 UDT/method 可执行并投影 typed `ObjectStmt`，Rust parser 当前对该多行对象表面保留拒绝边界，执行 owner 是 PineTS。 |
| 280 | `parse_collection_test.go:515:TestCompileSupportsV23NamedObjectArgsAndPureMethodBody` | `partial`；Go 的 named constructor/method args 与 pure multi-line method body 尚无 Rust 同形 lowering，保持缺口，不把 framework 聚合测试升格。 |

本片精准验证（9/9 通过）：

```bash
node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked -E 'test(compile_supports_framework_language_features) | test(compile_accepts_expression_udf_and_static_for_unroll) | test(compile_supports_v12_advanced_indicators) | test(compile_supports_v12_advanced_indicators_in_static_intraday_security) | test(compile_supports_v13_migration_indicators) | test(compile_supports_v13_indicators_in_static_intraday_security) | test(request_security_tuple_diagnostics_match_go_codes) | test(compile_preserves_order_notification_metadata_and_immediate_close) | test(compile_supports_multi_bar_history_references)'
python3 scripts/compatibility/audit_test_parity.py --write-report
python3 scripts/compatibility/parity_anchor_reconcile.py
cargo fmt --all -- --check
git diff --check
```

本片结论：20/20 保持 `partial` 或明确 `boundary`，没有新增 Rust 生产修复；
benchmark corpus、collection runtime、UDT/method lowering 和完整动态控制流缺口
继续登记为后续补测方向。下一片固定为 strategy_pine partial 第 281–300 行。
