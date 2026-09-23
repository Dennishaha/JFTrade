//! Go-parity regressions for `strategy.risk.*` validation, trailing exits,
//! indented block reassignments and the shared Pine structure corpus.
//!
//! Parity: `pkg/strategy/pine/strategy_business_test.go`,
//! `pkg/strategy/pine/strategy_call_bounds_test.go`,
//! `pkg/strategy/pine/runtime_and_parser_boundaries_test.go` and
//! `pkg/strategy/pine/shared_structure_corpus_test.go`.
//!
//! Go validates risk declarations while it parses (`allow_entry_in` direction,
//! positive constants, drawdown amount type, single-argument position size),
//! keeps indented block bodies attached to their `if`/`else`/`for` headers and
//! projects the shared corpus into statement kinds, branches, branch targets
//! and maximum nesting depth. The tests below pin the same contracts through
//! the public compile pipeline.

use std::collections::BTreeMap;

use jftrade_strategy::pine::{DiagnosticSeverity, compile};
use serde_json::Value;

fn script(body: &str) -> String {
    format!("//@version=6\nstrategy(\"Parity\", overlay=true)\n{body}")
}

fn compile_error(body: &str) -> (String, String) {
    let compilation = compile(&script(body));
    assert!(!compilation.ok, "compile must fail for {body}");
    let diagnostic = compilation
        .diagnostics
        .iter()
        .find(|diagnostic| diagnostic.severity == DiagnosticSeverity::Error)
        .unwrap_or_else(|| panic!("missing error diagnostic for {body}"));
    (diagnostic.code.clone(), diagnostic.message.clone())
}

fn assert_error(body: &str, code: &str, message: &str) {
    let (found_code, found_message) = compile_error(body);
    assert_eq!(found_code, code, "code for {body}");
    assert!(
        found_message.contains(message),
        "message {found_message:?} for {body} must contain {message:?}"
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/strategy_business_test.go:130 TestValidateScriptReportsRiskDeclarationBoundaryErrors
///
/// Parity: go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:120 TestParseStrategyCallRejectsInvalidTradingExpressions
#[test]
fn compile_rejects_invalid_risk_declarations() {
    for (body, message) in [
        (
            "strategy.risk.allow_entry_in(strategy.direction.both)",
            "direction",
        ),
        (
            "strategy.risk.max_drawdown(10, strategy.fixed)",
            "not supported",
        ),
        (
            "strategy.risk.max_intraday_loss(10, strategy.fixed)",
            "not supported",
        ),
        (
            "strategy.risk.max_intraday_filled_orders(0)",
            "positive constant integer",
        ),
        (
            "strategy.risk.max_cons_loss_days(0)",
            "positive constant integer",
        ),
        (
            "strategy.risk.max_position_size(1, 2)",
            "requires one argument",
        ),
        ("strategy.risk.allow_entry_in()", "requires one argument"),
        (
            "strategy.risk.max_drawdown()",
            "requires at least two arguments",
        ),
        (
            "strategy.risk.max_drawdown(10)",
            "requires at least two arguments",
        ),
        (
            "strategy.risk.max_drawdown(0, strategy.cash)",
            "positive constant number",
        ),
        (
            "strategy.risk.max_drawdown(-1, strategy.cash)",
            "positive constant number",
        ),
        (
            "strategy.risk.max_intraday_filled_orders()",
            "requires at least one argument",
        ),
        (
            "strategy.risk.max_cons_loss_days(-2)",
            "positive constant integer",
        ),
        ("strategy.risk.max_position_size()", "requires one argument"),
        (
            "strategy.risk.max_position_size(-1)",
            "positive constant number",
        ),
        (
            "strategy.risk.max_position_size(close)",
            "positive constant number",
        ),
    ] {
        assert_error(body, "PINE_COMPILE_ERROR", message);
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:11 TestParseStrategyCallCoversOrderLifecycleBusinessBoundaries
#[test]
fn compile_keeps_valid_risk_declarations_and_projects_their_limits() {
    let body = "strategy.risk.allow_entry_in(strategy.direction.long)\n\
        strategy.risk.max_drawdown(12.5, strategy.cash, \"drawdown\")\n\
        strategy.risk.max_intraday_loss(8, strategy.percent_of_equity, alert_message=\"day loss\")\n\
        strategy.risk.max_intraday_filled_orders(5, \"fills\")\n\
        strategy.risk.max_position_size(2.5)\n\
        strategy.risk.max_cons_loss_days(3, alert_message=\"loss days\")";
    let compilation = compile(&script(body));
    assert!(compilation.ok, "errors: {:?}", compilation.diagnostics);
    let metadata = serde_json::to_value(
        compilation
            .program
            .as_ref()
            .expect("lowered program")
            .metadata
            .clone(),
    )
    .expect("metadata json");
    assert_eq!(metadata["allowedEntryDirection"], "long");
    assert_eq!(metadata["maxDrawdownValue"], "12.5");
    assert_eq!(metadata["maxDrawdownType"], "cash");
    assert_eq!(metadata["maxDrawdownAlert"], "drawdown");
    assert_eq!(metadata["maxIntradayLossValue"], "8");
    assert_eq!(metadata["maxIntradayLossType"], "percent_of_equity");
    assert_eq!(metadata["maxIntradayLossAlert"], "day loss");
    assert_eq!(metadata["maxIntradayFilledOrders"], 5);
    assert_eq!(metadata["maxIntradayFilledOrdersAlert"], "fills");
    assert_eq!(metadata["maxPositionSize"], "2.5");
    assert_eq!(metadata["maxConsLossDays"], 3);
    assert_eq!(metadata["maxConsLossDaysAlert"], "loss days");
}

/// Parity: go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:98 TestParseStrategyCallRejectsUnsupportedOrderBoundaries
#[test]
fn compile_requires_trailing_offset_for_trailing_exits() {
    for body in [
        r#"strategy.exit("NoOffset", "Long", trail_points=10)"#,
        r#"strategy.exit("PriceOnly", "Long", trail_price=high)"#,
    ] {
        assert_error(
            body,
            "PINE_COMPILE_ERROR",
            "trailing stop requires trail_offset",
        );
    }
    for body in [
        r#"strategy.exit("Trail", "Long", trail_points=10, trail_offset=2)"#,
        r#"strategy.exit("TrailPrice", "Long", trail_price=high, trail_offset=2)"#,
    ] {
        let compilation = compile(&script(body));
        assert!(
            compilation.ok,
            "errors for {body}: {:?}",
            compilation.diagnostics
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:62 TestNormalizationPreservesInvalidUserSyntaxAndErrors
///
/// Parity: go:452dea11:pkg/strategy/pine/shared_structure_corpus_test.go:35 TestSharedPineStructureCorpusMatchesBackendIR
#[test]
fn compile_keeps_indented_block_assignments_inside_their_blocks() {
    let body = "var armed = false\n\
        if close > 105\n\
        \x20   armed := true\n\
        else\n\
        \x20   if close < 95\n\
        \x20       armed := false\n\
        \x20       log.info(\"breakout reset\")\n\
        \x20       strategy.close_all(immediately=true)\n\
        for i = 0 to 3\n\
        \x20   total := total + close";
    let compilation = compile(&script(body));
    assert!(compilation.ok, "errors: {:?}", compilation.diagnostics);
    let program = compilation.program.as_ref().expect("lowered program");
    let program = serde_json::to_value(program).expect("program json");
    let statements = program["hooks"][0]["statements"]
        .as_array()
        .expect("statements");
    assert_eq!(statement_kinds(statements), ["let", "if", "for"]);
    let nested = statements[1]["then_body"].as_array().expect("then body");
    assert_eq!(statement_kinds(nested), ["let"]);
    assert_eq!(nested[0]["mode"], "reassign");
    let nested_else = statements[1]["else_body"].as_array().expect("else body");
    assert_eq!(statement_kinds(nested_else), ["if"]);
    let inner = nested_else[0]["then_body"].as_array().expect("inner body");
    assert_eq!(statement_kinds(inner), ["let", "action", "action"]);
    assert_eq!(inner[0]["mode"], "reassign");
    let loop_body = statements[2]["body"].as_array().expect("loop body");
    assert_eq!(statement_kinds(loop_body), ["let"]);
    assert_eq!(loop_body[0]["mode"], "reassign");
}

/// Parity: go:452dea11:pkg/strategy/pine/shared_structure_corpus_test.go:35 TestSharedPineStructureCorpusMatchesBackendIR
#[test]
fn shared_structure_corpus_projects_statement_kinds_and_branches() {
    let raw = include_str!("../../../tests/fixtures/pine-structure-corpus.json");
    let corpus: Value = serde_json::from_str(raw).expect("corpus json");
    let cases = corpus.as_array().expect("corpus cases");
    assert_eq!(cases.len(), 8, "shared corpus case count");
    let mut checked = 0;
    for case in cases {
        let id = case["id"].as_str().expect("case id");
        let source = case["source"].as_str().expect("case source");
        let compilation = compile(source);
        if id == "mtf-derived-and-collection-state" {
            // Pinned Rust gap: `array.from(...).median()` needs the collection
            // namespace that the worker runtime still owns. Once the namespace
            // lands this case must compile like the other seven.
            assert!(!compilation.ok, "{id} unexpectedly compiled");
            let code = compilation
                .diagnostics
                .iter()
                .map(|diagnostic| diagnostic.code.as_str())
                .collect::<Vec<_>>();
            assert_eq!(code, ["PINE_CALL_INVALID"], "gap code for {id}");
            continue;
        }
        assert!(
            compilation.ok,
            "errors for {id}: {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.as_ref().expect("lowered program");
        let program = serde_json::to_value(program).expect("program json");
        let statements = program["hooks"][0]["statements"]
            .as_array()
            .expect("statements");
        assert_eq!(
            projected_statement_kinds(statements),
            expected_counts(&case["expectedStatementKinds"]),
            "statement kinds for {id}"
        );
        let (branches, branch_targets, max_depth) = summarize_branches(statements, 0);
        assert_eq!(
            branches,
            expected_counts(&case["expectedBranches"]),
            "branches for {id}"
        );
        assert_eq!(
            branch_targets,
            expected_counts(&case["expectedBranchTargets"]),
            "branch targets for {id}"
        );
        assert_eq!(
            max_depth,
            case["expectedMaxIfDepth"].as_u64().expect("max if depth") as usize,
            "max if depth for {id}"
        );
        checked += 1;
    }
    assert_eq!(checked, 7, "corpus cases covered by the Rust pipeline");
}

/// Parity: go:452dea11:pkg/strategy/pine/shared_structure_corpus_test.go:35 TestSharedPineStructureCorpusMatchesBackendIR
#[test]
fn compile_plans_the_ta_catalog_calls_used_by_the_shared_corpus() {
    for (body, expected) in [
        ("x = ta.cum(close)", "cum:close"),
        ("x = ta.highestbars(close, 14)", "highestbars:close:14"),
        ("x = ta.lowestbars(close, 7)", "lowestbars:close:7"),
        ("x = ta.stoch(close, high, low, 14)", "stoch:close:14"),
        (
            "x = ta.stoch(close, high, low, 14, \"D\")",
            "stoch:close:14:day",
        ),
    ] {
        let compilation = compile(&script(body));
        assert!(
            compilation.ok,
            "errors for {body}: {:?}",
            compilation.diagnostics
        );
        let keys: Vec<String> = compilation
            .requirements
            .indicators
            .iter()
            .map(|indicator| indicator.key.clone())
            .collect();
        assert_eq!(keys, [expected], "requirement keys for {body}");
    }
    for body in [
        "x = ta.barssince(close > 520)",
        "x = ta.valuewhen(close > 520, close, 0)",
    ] {
        let compilation = compile(&script(body));
        assert!(
            compilation.ok,
            "errors for {body}: {:?}",
            compilation.diagnostics
        );
        assert!(
            compilation.requirements.indicators.is_empty(),
            "{body} is a state sequence without a catalog binding"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/strategy_call_bounds_test.go:120 TestParseStrategyCallRejectsInvalidTradingExpressions
#[test]
fn compile_rejects_every_truncated_order_expression() {
    for body in [
        r#"strategy.order("Net")"#,
        r#"strategy.entry("Long", strategy.long, qty=close >)"#,
        r#"strategy.entry("Long", strategy.long, limit=close >)"#,
        r#"strategy.entry("Long", strategy.long, stop=close >)"#,
        r#"strategy.entry("Long", strategy.long, when=close >)"#,
        r#"strategy.order("Net", strategy.long, qty=close >)"#,
        r#"strategy.order("Net", strategy.long, limit=close >)"#,
        r#"strategy.order("Net", strategy.long, stop=close >)"#,
        r#"strategy.order("Net", strategy.long, when=close >)"#,
        r#"strategy.close("Long", qty=close >)"#,
        r#"strategy.close("Long", limit=close >)"#,
        r#"strategy.close("Long", stop=close >)"#,
        r#"strategy.close("Long", when=close >)"#,
        r#"strategy.exit("Exit", "Long", qty=close >, stop=low)"#,
        r#"strategy.exit("Exit", "Long", stop=close >)"#,
        r#"strategy.exit("Exit", "Long", limit=close >)"#,
        r#"strategy.exit("Exit", "Long", profit=close >)"#,
        r#"strategy.exit("Exit", "Long", loss=close >)"#,
        r#"strategy.exit("Exit", "Long", stop=low, when=close >)"#,
        r#"strategy.exit("Trail", "Long", trail_points=10, trail_offset=close >)"#,
        r#"strategy.exit("Trail", "Long", trail_points=close >, trail_offset=2)"#,
        r#"strategy.exit("Trail", "Long", trail_price=close >, trail_offset=2)"#,
    ] {
        let compilation = compile(&script(body));
        assert!(!compilation.ok, "compile must fail for {body}");
        let diagnostic = compilation
            .diagnostics
            .iter()
            .find(|diagnostic| diagnostic.severity == DiagnosticSeverity::Error)
            .unwrap_or_else(|| panic!("missing error diagnostic for {body}"));
        assert_eq!(diagnostic.line, 3, "diagnostic line for {body}");
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:108 TestPublicHelperGuardReturnsActionableMigrationErrors
#[test]
fn compile_guards_disabled_helpers_and_ignores_helper_names_in_strings() {
    let native = compile(&script("value = ta.ema(close, 20)"));
    assert!(
        native.ok,
        "native Pine helper must stay executable: {:?}",
        native.diagnostics
    );
    let quoted = compile(&script(
        r#"log.info("history(close, 1) with \"quoted\" text")"#,
    ));
    assert!(
        quoted.ok,
        "helper names inside string literals are not calls: {:?}",
        quoted.diagnostics
    );
    for (body, code, wanted) in [
        (
            "value = history(close, 1)",
            "PINE_INTERNAL_HELPER_PUBLIC",
            ["history() is an internal JFTrade helper", "series[n]"],
        ),
        (
            "value = ta.adx(14)",
            "PINE_PUBLIC_TA_SHORTCUT",
            ["ta.adx() is a JFTrade-only shortcut", "ta.dmi"],
        ),
    ] {
        assert_error(body, code, wanted[0]);
        assert_error(body, code, wanted[1]);
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:86 TestStrategyDeclarationWithoutArgumentsUsesBusinessDefaults
#[test]
fn compile_uses_business_defaults_for_declarations_without_a_title() {
    for (body, name) in [
        ("//@version=6\nstrategy()", "Pine Strategy"),
        ("//@version=6\nstrategy(overlay=true)", "Pine Strategy"),
        (
            "//@version=6\nstrategy(title=\"Risk managed\", overlay=true)",
            "Risk managed",
        ),
        ("//@version=6\nstrategy(\"Plain\")", "Plain"),
    ] {
        let compilation = compile(body);
        assert!(
            compilation.ok,
            "errors for {body}: {:?}",
            compilation.diagnostics
        );
        assert!(
            compilation.warnings.is_empty(),
            "warnings for {body}: {:?}",
            compilation.warnings
        );
        let metadata = compilation
            .program
            .as_ref()
            .expect("lowered program")
            .metadata
            .clone();
        assert_eq!(metadata.name, name, "declared strategy name for {body}");
        assert_eq!(metadata.default_qty_mode, "fixed");
        assert_eq!(metadata.default_qty_value, "1");
        assert_eq!(metadata.pyramiding, 1);
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:70 TestStrategyDeclarationInvalidConstantsFallBackWithWarnings
#[test]
fn compile_warns_when_strategy_constants_fall_back_to_defaults() {
    let declaration = "//@version=6\nstrategy(title=\"Risk managed\", default_qty_type=strategy.unknown, default_qty_value=25, pyramiding=-1, initial_capital=0, commission_type=strategy.commission.unknown, commission_value=-1, slippage=-2, process_orders_on_close=maybe)";
    let compilation = compile(declaration);
    assert!(compilation.ok, "errors: {:?}", compilation.diagnostics);
    let joined = compilation.warnings.join("\n");
    for expected in [
        "pine strategy default_qty_type \"strategy.unknown\" is not supported by JFTrade; using strategy.fixed",
        "pine strategy pyramiding \"-1\" is not a supported constant integer; using 1",
        "pine strategy initial_capital \"0\" must be a positive constant number",
        "pine strategy commission_type \"strategy.commission.unknown\" is not supported by JFTrade",
        "pine strategy commission_value \"-1\" must be a non-negative constant number",
        "pine strategy slippage \"-2\" must be a non-negative constant integer",
        "pine strategy process_orders_on_close \"maybe\" must be true or false",
    ] {
        assert!(
            joined.contains(expected),
            "warnings {joined:?} must contain {expected:?}"
        );
    }
    assert_eq!(compilation.warnings.len(), 7, "warnings: {joined:?}");
    let metadata = compilation
        .program
        .as_ref()
        .expect("lowered program")
        .metadata
        .clone();
    assert_eq!(metadata.name, "Risk managed");
    assert_eq!(metadata.default_qty_mode, "fixed");
    assert_eq!(metadata.default_qty_value, "25");
    assert_eq!(metadata.pyramiding, 1);
}

fn statement_kinds(statements: &[Value]) -> Vec<String> {
    statements
        .iter()
        .map(|statement| statement["kind"].as_str().unwrap_or("?").to_owned())
        .collect()
}

/// Go folds `strategy.risk.*` declarations into metadata instead of keeping a
/// statement, so the Rust projection drops its `strategy.risk.*` actions and
/// reports the business kind of every remaining call.
fn projected_statement_kinds(statements: &[Value]) -> BTreeMap<String, usize> {
    let mut counts = BTreeMap::new();
    for statement in statements {
        match statement["kind"].as_str().unwrap_or("") {
            "let" | "tuple" => *counts.entry("let".to_owned()).or_default() += 1,
            "if" => {
                *counts.entry("if".to_owned()).or_default() += 1;
                merge(
                    &mut counts,
                    projected_statement_kinds(array(&statement["then_body"])),
                );
                merge(
                    &mut counts,
                    projected_statement_kinds(array(&statement["else_body"])),
                );
            }
            "for" => {
                *counts.entry("for".to_owned()).or_default() += 1;
                merge(
                    &mut counts,
                    projected_statement_kinds(array(&statement["body"])),
                );
            }
            "action" => {
                let call = statement["call"].as_str().unwrap_or("");
                if let Some(kind) = business_statement_kind(call) {
                    *counts.entry(kind.to_owned()).or_default() += 1;
                }
            }
            other => *counts.entry(other.to_owned()).or_default() += 1,
        }
    }
    counts
}

fn business_statement_kind(call: &str) -> Option<&'static str> {
    let call = call.to_ascii_lowercase();
    if call.starts_with("strategy.risk.") {
        return None;
    }
    if call.starts_with("log.") {
        return Some("log");
    }
    if call == "alert" || call == "alertcondition" {
        return Some("notify");
    }
    match call.as_str() {
        "strategy.entry" | "strategy.order" | "strategy.close" | "strategy.close_all" => {
            Some("order")
        }
        "strategy.exit" => Some("exit"),
        "strategy.cancel" | "strategy.cancel_all" => Some("cancel"),
        _ => None,
    }
}

/// Mirrors Go's `summarizeBranches`: every `if` counts a taken branch when its
/// body is non-empty, and branch targets use the first statement kind.
fn summarize_branches(
    statements: &[Value],
    depth: usize,
) -> (BTreeMap<String, usize>, BTreeMap<String, usize>, usize) {
    let mut branches = BTreeMap::new();
    let mut targets = BTreeMap::new();
    let mut max_depth = 0;
    for statement in statements {
        match statement["kind"].as_str().unwrap_or("") {
            "if" => {
                let if_depth = depth + 1;
                max_depth = max_depth.max(if_depth);
                let then_body = array(&statement["then_body"]);
                if let Some(head) = then_body.first() {
                    *branches.entry("true".to_owned()).or_default() += 1;
                    *targets
                        .entry(format!("true:{}", first_kind(head)))
                        .or_default() += 1;
                }
                let else_body = array(&statement["else_body"]);
                if let Some(head) = else_body.first() {
                    *branches.entry("false".to_owned()).or_default() += 1;
                    *targets
                        .entry(format!("false:{}", first_kind(head)))
                        .or_default() += 1;
                }
                let (then_branches, then_targets, then_depth) =
                    summarize_branches(then_body, if_depth);
                merge(&mut branches, then_branches);
                merge(&mut targets, then_targets);
                let (else_branches, else_targets, else_depth) =
                    summarize_branches(else_body, if_depth);
                merge(&mut branches, else_branches);
                merge(&mut targets, else_targets);
                max_depth = max_depth.max(then_depth).max(else_depth);
            }
            "for" => {
                let (body_branches, body_targets, body_depth) =
                    summarize_branches(array(&statement["body"]), depth);
                merge(&mut branches, body_branches);
                merge(&mut targets, body_targets);
                max_depth = max_depth.max(body_depth);
            }
            _ => {}
        }
    }
    (branches, targets, max_depth)
}

fn first_kind(statement: &Value) -> String {
    match statement["kind"].as_str().unwrap_or("") {
        "action" => business_statement_kind(statement["call"].as_str().unwrap_or(""))
            .unwrap_or("action")
            .to_owned(),
        "let" | "tuple" => "let".to_owned(),
        other => other.to_owned(),
    }
}

fn array(value: &Value) -> &[Value] {
    value.as_array().map(Vec::as_slice).unwrap_or(&[])
}

fn merge(target: &mut BTreeMap<String, usize>, source: BTreeMap<String, usize>) {
    for (key, count) in source {
        *target.entry(key).or_default() += count;
    }
}

fn expected_counts(value: &Value) -> BTreeMap<String, usize> {
    value
        .as_object()
        .expect("expected counts object")
        .iter()
        .filter(|(_, count)| count.as_u64().unwrap_or(0) > 0)
        .map(|(key, count)| (key.clone(), count.as_u64().unwrap_or(0) as usize))
        .collect()
}
