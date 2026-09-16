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
