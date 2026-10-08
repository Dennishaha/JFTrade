use jftrade_strategy::pine::{
    AnalysisOptions, DiagnosticSeverity, analyze, analyze_script, compile, parse,
};
use jftrade_strategy::pinespec::validate_script;

const BAD_EMA: &str =
    "//@version=6\nstrategy(\"Bad Signature\", overlay=true)\nfast = ta.ema(close)";

// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:180 TestAnalyzeScriptReportsSemanticSignatureDiagnostics
#[test]
fn analysis_classifies_missing_ema_length_as_a_semantic_signature_error() {
    let result = analyze_script(BAD_EMA, AnalysisOptions { include_ast: true });
    assert!(!result.ok);
    let wire = serde_json::to_value(&result).unwrap();
    assert!(wire["semantic"].is_object());
    let first = result.diagnostics.first().expect("signature diagnostic");
    assert_eq!(first.code, "PINE_SEMANTIC_SIGNATURE");
    assert_eq!(first.line, 3);
    assert_eq!(first.severity, DiagnosticSeverity::Error);
    assert_eq!(first.message, "ta.ema expects ta.ema(source, length)");
    assert_eq!(result.semantic.diagnostics.first(), Some(first));
    assert!(result.program.is_none());
    assert!(result.requirements.indicators.is_empty());
}

#[test]
fn moving_average_arity_errors_are_reported_before_lowering_and_planning() {
    let mut failures = Vec::new();
    for family in ["ema", "sma", "rma", "wma", "hma", "vwma"] {
        for arguments in ["", "close", "close, 14, 2"] {
            let script = format!(
                "//@version=6\nstrategy(\"MA arity\")\n\n\n\n\nx = ta.{family}({arguments})"
            );
            let semantic = analyze(&parse(&script).unwrap());
            let compilation = compile(&script);
            let expected = format!("ta.{family} expects ta.{family}(source, length)");
            let signature_matches = semantic.diagnostics.first().is_some_and(|d| {
                d.code == "PINE_SEMANTIC_SIGNATURE" && d.line == 7 && d.message == expected
            });
            if !signature_matches
                || compilation.ok
                || compilation.program.is_some()
                || !compilation.requirements.indicators.is_empty()
            {
                failures.push(format!(
                    "ta.{family}({arguments}): semantic={:?}, ok={}, executable={}, requirements={:?}",
                    semantic.diagnostics, compilation.ok, compilation.program.is_some(), compilation.requirements.indicators
                ));
            }
        }
    }
    assert!(failures.is_empty(), "{}", failures.join("\n"));
}

#[test]
fn moving_averages_with_source_and_length_keep_their_planned_requirements() {
    for family in ["ema", "sma", "rma", "wma", "hma", "vwma"] {
        let source = format!("//@version=6\nstrategy(\"MA valid\")\nx = ta.{family}(close, 14)");
        let result = compile(&source);
        assert!(result.ok, "{family}: {:?}", result.diagnostics);
        assert!(result.semantic.diagnostics.is_empty());
        assert_eq!(result.requirements.indicators.len(), 1);
        assert_eq!(
            result.requirements.indicators[0].key,
            format!("ma:{}:14", family.to_ascii_uppercase())
        );
        assert!(result.program.is_some());
    }
}

#[test]
fn immutable_input_lengths_and_aliases_plan_chart_and_higher_timeframe_averages() {
    for declaration in ["len = input.int(8, \"Length\")", "len = 8"] {
        let source = format!(
            "//@version=6\nstrategy(\"Static input\")\n{declaration}\nperiod = len\nfast = ta.ema(close, period)\nslow = request.security(syminfo.tickerid, \"15\", ta.ema(close, period))"
        );
        let result = compile(&source);
        assert!(result.ok, "{:?}", result.diagnostics);
        assert_eq!(
            result
                .requirements
                .indicators
                .iter()
                .map(|i| i.key.as_str())
                .collect::<Vec<_>>(),
            ["ma:EMA:8", "ma:EMA:8:15m"]
        );
        // Planning resolves defaults without replacing the input or alias in IR.
        let program = serde_json::to_value(result.program.unwrap()).unwrap();
        assert_eq!(
            program["hooks"][0]["statements"][1]["expression"]["name"],
            "len"
        );
    }
}

#[test]
fn dynamic_rewritten_shadowed_and_invalid_lengths_cannot_reuse_a_static_default() {
    for body in [
        "x = ta.ema(close, len)\nlen = input.int(8)",
        "len = input.int(8)\nlen := close\nx = ta.ema(close, len)",
        "len = input.int(8)\nx = ta.ema(close, len)\nlen := 20",
        "len = input.int(8)\nif close > open\n    len := 20\nx = ta.ema(close, len)",
        "len = input.int(8)\nfor len = 1 to 2\n    log.info(\"iteration\")\nx = ta.ema(close, len)",
        "len = input.int(8)\nf(len) => len\nx = ta.ema(close, len)",
        "if close > open\n    len = input.int(8)\nx = ta.ema(close, len)",
        "len = input.int(0)\nx = ta.ema(close, len)",
        "len = input.int(2.5)\nx = ta.ema(close, len)",
        "len = input.int(close)\nx = ta.ema(close, len)",
        "len = input.int(8)\nx = ta.ema(close, \"len\")",
        "x = request.security(syminfo.tickerid, \"15\", ta.ema(close, 0))",
        "x = request.security(syminfo.tickerid, \"15\", ta.ema(close, missing))",
    ] {
        let source = format!("//@version=6\nstrategy(\"Unsafe period\")\n{body}");
        let result = compile(&source);
        assert!(!result.ok, "{body}: {:?}", result.requirements);
        assert!(result.requirements.indicators.is_empty(), "{body}");
        assert!(
            result
                .diagnostics
                .iter()
                .any(|d| d.code == "PINE_REQUIREMENTS_INVALID"
                    && d.message.contains("positive integer")),
            "{body}: {:?}",
            result.diagnostics
        );
    }
}

#[test]
fn pine_validation_does_not_offer_requirements_or_hooks_for_invalid_ema_calls() {
    let payload = validate_script(BAD_EMA, true, true);
    assert!(!payload.ok);
    assert_eq!(payload.normalized_script, BAD_EMA);
    assert_eq!(payload.errors, ["ta.ema expects ta.ema(source, length)"]);
    assert!(payload.requirements.is_none());
    assert!(payload.hooks.is_empty());
    assert!(payload.save_hint.is_some());
}

// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:10 TestAnalyzeScriptIncludesV17SemanticSummary
#[test]
fn semantic_analysis_of_input_ema_and_mtf_tuple_preserves_declared_symbols() {
    let source = r#"//@version=6
strategy("v1.7 Semantic", overlay=true)
len = input.int(8, "Length")
fast = ta.ema(close, len)
[mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5)])
if mtfClose > mtfFast and fast > fast[1]
    strategy.entry("Long", strategy.long, qty=1)"#;
    let result = analyze_script(source, AnalysisOptions { include_ast: true });
    assert!(result.ok, "{:?}", result.diagnostics);
    assert!(result.semantic.diagnostics.is_empty());
    for name in ["len", "fast", "mtfClose", "mtfFast"] {
        assert!(result.semantic.symbols.contains_key(name), "{name}");
        assert!(result.semantic.declarations.iter().any(|d| d.name == name));
    }
    let program = result.program.expect("executable program");
    assert_eq!(program.hooks.len(), 1);
    assert_eq!(program.hooks[0].statements.len(), 4);
    // Value-kind, function signature and tuple return metadata from the
    // reference summary still have no matching fields in the native summary.
}

// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:55 TestAnalyzeScriptReportsSupportedTASemanticSignatures
#[test]
fn analysis_of_window_channel_and_cross_calls_plans_the_original_ta_script() {
    let source = r#"//@version=6
strategy("TA Semantic", overlay=true)
hh = ta.highest(high, 20)
lb = ta.lowestbars(low, 5)
delta = ta.change(close)
rank = ta.percentrank(close, 14)
width = ta.kcw(close, 20, 1.5)
almaValue = ta.alma(close, 9, 0.85, 6)
cciValue = ta.cci(hlc3, 20)
if ta.crossover(close, open) and ta.barssince(close > open) > 2
    strategy.entry("Long", strategy.long, qty=1)"#;
    let result = analyze_script(source, AnalysisOptions { include_ast: true });
    assert!(result.ok, "{:?}", result.diagnostics);
    assert!(result.semantic.diagnostics.is_empty());
    for name in [
        "hh",
        "lb",
        "delta",
        "rank",
        "width",
        "almaValue",
        "cciValue",
    ] {
        assert!(result.semantic.symbols.contains_key(name), "{name}");
    }
    assert!(result.program.is_some());
    assert!(!result.requirements.indicators.is_empty());
}

// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:98 TestAnalyzeScriptReportsSupportedUtilitySemanticSignatures
#[test]
fn unsupported_string_helpers_in_the_original_utility_script_prevent_planning() {
    let source = r#"//@version=6
strategy("Utility Semantic", overlay=true)
plain = input(5, "Plain")
lengthInput = input.int(14, "Length")
factor = input.float(2.0, "Factor")
enabled = input.bool(true, "Enabled")
label = input.string("alpha", "Label")
src = input.source(close, "Source")
startTime = input.time(timestamp(2026, 1, 1), "Start")
tf = input.timeframe("15", "TF")
lineColor = input.color(color.green, "Line")
average = math.avg(close, open)
rounded = math.round_to_mintick(average)
wide = math.max(close, open)
narrow = math.min(close, open)
root = math.sqrt(4)
distance = math.abs(wide - narrow)
roundedDistance = math.round(distance)
ceiling = math.ceil(root)
score = math.sign(wide - narrow) + math.floor(root) + ceiling + math.pow(2, 3) + math.log(10)
upper = str.upper(label)
lower = str.lower(label)
length = str.length(label)
asText = str.tostring(length)
position = str.pos(upper, "A")
part = str.substring(upper, 0, 2)
clean = str.replace(part, "A", lower)
text = str.format("{0}:{1}", clean, length)
if enabled and str.contains(text, "a") and rounded > 0 and roundedDistance >= 0 and position >= 0 and score > 0
    strategy.entry("Long", strategy.long, qty=1)"#;
    let result = analyze_script(source, AnalysisOptions { include_ast: true });
    assert!(!result.ok);
    assert!(result.program.is_none());
    assert!(result.requirements.indicators.is_empty());
    for (line, name) in [
        (21, "str.upper"),
        (22, "str.lower"),
        (23, "str.length"),
        (24, "str.tostring"),
        (25, "str.pos"),
        (26, "str.substring"),
        (27, "str.replace"),
        (28, "str.format"),
        (29, "str.contains"),
    ] {
        assert!(
            result
                .diagnostics
                .iter()
                .any(|d| d.code == "PINE_CALL_UNSUPPORTED"
                    && d.line == line
                    && d.message.contains(name)),
            "{name}: {:?}",
            result.diagnostics
        );
    }
}

// Parity: go:452dea11:pkg/strategy/pine/compiler_rejection_contracts_test.go:79 TestTupleHelpersRejectMalformedIndicatorArityWithoutInventingAliases
#[test]
fn malformed_indicator_tuples_are_rejected_at_the_source_line_without_executable_ir() {
    for (body, signature) in [
        (
            "[basis, upper, lower] = ta.bb(close, 20)",
            "ta.bb(source, length, mult)",
        ),
        (
            "[plus, minus, adx] = ta.dmi(14)",
            "ta.dmi(diLength, adxSmoothing)",
        ),
        (
            "[trend, direction] = ta.supertrend(3)",
            "ta.supertrend(factor, atrPeriod)",
        ),
        (
            "[basis, upper, lower] = ta.kc(close, 20)",
            "ta.kc(source, length, mult, useTrueRange?)",
        ),
        (
            "[macd, signal, histogram] = ta.macd(close, 12, 26)",
            "ta.macd(source, fast, slow, signal)",
        ),
    ] {
        let source = format!(
            "//@version=6\nstrategy(\"Tuple arity\")\n{}{body}",
            "\n".repeat(12)
        );
        let result = compile(&source);
        assert!(!result.ok, "{body}");
        let first = result.diagnostics.first().expect("tuple rejection");
        assert_eq!(first.code, "PINE_TUPLE_ARITY", "{body}");
        assert_eq!(first.line, 15);
        assert!(first.message.contains(signature), "{}", first.message);
        assert!(result.program.is_none());
        assert!(result.requirements.indicators.is_empty());
    }
}
