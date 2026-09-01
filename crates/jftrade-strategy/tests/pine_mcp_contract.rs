use jftrade_strategy::pine::{DiagnosticSeverity, ExprKind, compile, parse};
use jftrade_strategy::pinespec::{SECTIONS, build_tool_payload, validate_script};

const EMA_SCRIPT: &str = r#"//@version=6
strategy("EMA", overlay=true, pyramiding=2)
fast = ta.ema(close, 8)
slow = ta.sma(volume, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=(strategy.equity * 10 / 100) / close)
else
    strategy.close("Long")
"#;

#[test]
fn native_pipeline_parses_lowers_and_plans_strategy_requirements() {
    let compilation = compile(EMA_SCRIPT);
    assert!(compilation.ok, "diagnostics: {:?}", compilation.diagnostics);
    let program = compilation.program.expect("lowered program");
    assert_eq!(program.metadata.name, "EMA");
    assert_eq!(program.metadata.pyramiding, 2);
    assert_eq!(program.hooks.len(), 1);
    assert!(compilation.requirements.requires_position);
    assert!(compilation.requirements.requires_total_account_value);
    let keys = compilation
        .requirements
        .indicators
        .iter()
        .map(|item| item.key.as_str())
        .collect::<Vec<_>>();
    assert!(keys.contains(&"ma:EMA:8"));
    assert!(keys.contains(&"ma:SMA:21:volume"));
}

#[test]
fn parser_handles_strings_history_and_nested_calls_without_regex() {
    let program = parse(
        "//@version=6\nstrategy(\"History\")\nvalue = nz(close[1], close)\nlog.info(\"ready\")",
    )
    .expect("parse");
    assert_eq!(program.strategy.as_ref().expect("strategy").name, "History");
    let statement = &program.statements[0];
    let expression = match statement {
        jftrade_strategy::pine::Statement::Assignment { expression, .. } => expression,
        other => panic!("unexpected statement: {other:?}"),
    };
    assert!(matches!(expression.kind, ExprKind::Call { ref callee, .. } if callee == "nz"));
}

#[test]
fn semantic_checker_rejects_non_boolean_conditions_and_unsupported_declarations() {
    let compilation = compile(
        "//@version=6\nstrategy(\"Bad\")\nif close\n    strategy.entry(\"Long\", strategy.long)\nimport TradingView/ta/7",
    );
    assert!(!compilation.ok);
    assert!(
        compilation
            .diagnostics
            .iter()
            .any(|item| item.code == "PINE_CONDITION_NOT_BOOL")
    );
    assert!(
        compilation
            .diagnostics
            .iter()
            .any(|item| item.code == "PINE_DECLARATION_UNSUPPORTED")
    );
    assert!(
        compilation
            .diagnostics
            .iter()
            .all(|item| item.severity == DiagnosticSeverity::Error)
    );
}

#[test]
fn pine_spec_freezes_seven_sections_and_validation_defaults_requirements() {
    assert_eq!(
        SECTIONS,
        &[
            "overview",
            "syntax",
            "expressions",
            "indicators",
            "orders",
            "unsupported",
            "examples"
        ]
    );
    let payload = build_tool_payload(" examples ", false).expect("examples payload");
    assert_eq!(payload["selectedSection"], "examples");
    assert!(
        payload["examples"]
            .as_array()
            .is_some_and(|items| !items.is_empty())
    );
    assert!(build_tool_payload("support-matrix", false).is_err());
    let result = validate_script(EMA_SCRIPT, true, true);
    assert!(result.ok, "errors: {:?}", result.errors);
    assert!(result.requirements.is_some());
    assert!(result.ast.is_some());
}
