use jftrade_strategy::pine::{DiagnosticSeverity, ExprKind, compile, parse};
use jftrade_strategy::pinespec::{SECTIONS, build_tool_payload, validate_script};
use serde_json::to_value;

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
    let external_engine = payload["externalEngine"]
        .as_object()
        .expect("external engine");
    assert_eq!(external_engine["engine"], "pinets-shadow");
    assert_eq!(external_engine["mode"], "off");
    assert_eq!(external_engine["enabled"], false);
    assert_eq!(external_engine["status"], "disabled");
    assert_eq!(external_engine["license"], "AGPL-3.0-only");
    assert_eq!(external_engine["package"], "pinets@0.9.31");
    assert_eq!(
        external_engine["repository"],
        "https://github.com/LuxAlgo/PineTS"
    );
    assert_eq!(external_engine["worker"], "scripts/pinets-worker.mjs");
    assert_eq!(payload["compatibilityScore"], 0);
    assert_eq!(payload["scoreModelVersion"], "native-rust-v1");
    assert!(
        payload["capabilities"]
            .as_array()
            .is_some_and(|items| !items.is_empty())
    );
    let golden_scripts = payload["goldenScripts"].as_array().expect("golden scripts");
    assert!(!golden_scripts.is_empty());
    for script in golden_scripts {
        let source = script["script"].as_str().expect("golden script source");
        assert!(compile(source).ok, "golden script failed: {source}");
    }
    let section_content = payload["sectionContent"]
        .as_object()
        .expect("section content");
    assert_eq!(section_content["id"], "examples");
    assert!(
        section_content["details"]
            .as_array()
            .is_some_and(|items| !items.is_empty())
    );
    assert!(payload["brokerBoundary"].as_array().is_some_and(|items| {
        items.iter().any(|item| {
            item["status"] == "out_of_scope"
                && item["diagnosticCodes"].as_array().is_some_and(|codes| {
                    codes
                        .iter()
                        .any(|code| code == "PINE_BROKER_EMULATOR_OUT_OF_SCOPE")
                })
        })
    }));
    let result = validate_script(EMA_SCRIPT, true, true);
    assert!(result.ok, "errors: {:?}", result.errors);
    assert!(result.requirements.is_some());
    assert!(result.ast.is_some());
    assert_eq!(result.external_engine["engine"], "pinets-shadow");
    assert_eq!(result.external_engine["status"], "disabled");
    assert!(result.save_hint.is_none());
    let serialized = to_value(&result).expect("validation payload");
    assert!(serialized.get("externalEngine").is_some());
    assert!(
        serialized
            .get("saveHint")
            .is_some_and(|hint| hint.is_null())
    );
}

#[test]
fn validation_payload_matches_save_hint_and_rejection_contract() {
    let empty = validate_script(" \n\t ", true, false);
    assert!(!empty.ok);
    assert_eq!(empty.errors, ["script 是必填项"]);
    let empty_hint = empty.save_hint.expect("empty save hint");
    assert_eq!(empty_hint.spec_tool, "strategy.pine_spec");
    assert_eq!(
        empty_hint.resource_files,
        [
            "references/pine-v6-spec.md".to_owned(),
            "references/pine-v6-examples.md".to_owned()
        ]
    );
    assert!(empty_hint.message.contains(&empty_hint.skeleton));
    assert!(
        empty_hint
            .skeleton
            .starts_with("//@version=6\nstrategy(\"Minimal Draft\"")
    );

    let invalid = validate_script(
        "//@version=6\nstrategy(\"Bad\")\nimport TradingView/ta/7",
        true,
        false,
    );
    assert!(!invalid.ok);
    assert!(invalid.requirements.is_none());
    assert!(invalid.save_hint.is_some());
    assert_eq!(invalid.external_engine["status"], "disabled");
}
