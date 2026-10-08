use jftrade_strategy::pine::{Compilation, DiagnosticSeverity, analyze, compile, parse};

fn script(body: &str) -> String {
    format!("//@version=6\nstrategy(\"reject helpers\", overlay=true)\n{body}")
}

fn rejected(body: &str) -> Compilation {
    let compilation = compile(&script(body));
    assert!(!compilation.ok, "{body}: {:?}", compilation.diagnostics);
    assert!(compilation.program.is_none(), "no executable IR for {body}");
    assert!(
        compilation.diagnostics.iter().any(|diagnostic| {
            diagnostic.severity == DiagnosticSeverity::Error && diagnostic.line == 3
        }),
        "line 3 error for {body}: {:?}",
        compilation.diagnostics
    );
    compilation
}

/// Parity: go:452dea11:pkg/strategy/pine/parse_test.go:54 TestCompileRejectsPublicInternalHelperCalls
#[test]
fn public_helpers_reject_each_frozen_input_with_its_original_message() {
    for (body, message) in [
        ("fast = ma(EMA, 14)", "ma() is an internal JFTrade helper"),
        (
            "daily = security_source(close, day)",
            "security_source() is an internal JFTrade helper",
        ),
        (
            "band = bollinger(20, 2)",
            "bollinger() is an internal JFTrade helper",
        ),
        (
            "bars = barssince(close > open)",
            "barssince() is an internal JFTrade helper",
        ),
        (
            "last = valuewhen(close > open, close, 0)",
            "valuewhen() is an internal JFTrade helper",
        ),
        (
            "prev = history(close, 1)",
            "history() is an internal JFTrade helper",
        ),
        (
            "picked = ifelse(close > open, close, open)",
            "ifelse() is an internal JFTrade helper",
        ),
        (
            "x = cross_over(fast, slow)",
            "cross_over() is an internal JFTrade helper",
        ),
        (
            "x = cross_under(fast, slow)",
            "cross_under() is an internal JFTrade helper",
        ),
        (
            "notify(\"hello\")",
            "notify() is an internal JFTrade helper",
        ),
        ("adx = ta.adx(14)", "ta.adx() is a JFTrade-only shortcut"),
    ] {
        let compilation = rejected(body);
        let code = if body == "adx = ta.adx(14)" {
            "PINE_PUBLIC_TA_SHORTCUT"
        } else {
            "PINE_INTERNAL_HELPER_PUBLIC"
        };
        assert!(
            compilation.diagnostics.iter().any(|diagnostic| {
                diagnostic.code == code && diagnostic.message.contains(message)
            }),
            "{body}: {:?}",
            compilation.diagnostics
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:99 TestSemanticHelpersReportMalformedScriptBoundaries
#[test]
fn tuple_semantics_report_the_original_duplicate_alias_before_lowering() {
    let body = "[fast, fast] = ta.macd(close, 12, 26, 9)";
    let ast = parse(&script(body)).expect("original MACD tuple parses");
    let summary = analyze(&ast);
    assert!(
        summary.diagnostics.iter().any(|diagnostic| {
            diagnostic.code == "PINE_SEMANTIC_TUPLE"
                && diagnostic.message == "tuple assignment repeats fast"
                && diagnostic.line == 3
                && diagnostic.severity == DiagnosticSeverity::Error
        }),
        "original semantic diagnostics: {:?}",
        summary.diagnostics
    );
    let compilation = rejected(body);
    assert_eq!(compilation.diagnostics, summary.diagnostics);
}

// Supplemental valid-width cases prevent MACD width validation from hiding the duplicate.
/// Parity: go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:99 TestSemanticHelpersReportMalformedScriptBoundaries
#[test]
fn duplicate_tuple_aliases_fail_for_declarations_and_reassignments() {
    for body in [
        "[fast, fast, histogram] = ta.macd(close, 12, 26, 9)",
        "[fast, fast] = [close, open]",
        "[fast, fast] := [close, open]",
        "[fast, slow, fast, slow, fast] = [close, open, high, low, volume]",
    ] {
        let compilation = rejected(body);
        let message = if body.contains("[fast, slow,") {
            "tuple assignment repeats fast, slow, fast"
        } else {
            "tuple assignment repeats fast"
        };
        assert!(
            compilation.diagnostics.iter().any(|diagnostic| {
                diagnostic.code == "PINE_SEMANTIC_TUPLE" && diagnostic.message == message
            }),
            "{body}: {:?}",
            compilation.diagnostics
        );
    }
}

// Positive controls: duplicate discard placeholders are permitted; concrete names stay distinct.
/// Parity: go:452dea11:pkg/strategy/pine/semantic_helper_boundaries_test.go:99 TestSemanticHelpersReportMalformedScriptBoundaries
#[test]
fn tuple_discard_placeholders_and_unique_aliases_remain_executable() {
    for body in [
        "[_, _, histogram] = ta.macd(close, 12, 26, 9)",
        "[fast, slow, histogram] = ta.macd(close, 12, 26, 9)",
        "[fast, Fast] = [close, open]",
        "[fast, slow] := [close, open]",
    ] {
        let compilation = compile(&script(body));
        assert!(compilation.ok, "{body}: {:?}", compilation.diagnostics);
        assert!(compilation.program.is_some(), "executable IR for {body}");
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/parser_loop_boundaries_test.go:63 TestTupleIndicatorsExposeUnsupportedCallHistory
#[test]
fn tuple_indicator_parameters_reject_call_result_history_in_each_original_position() {
    for body in [
        "[basis, upper, lower] = ta.bb(close, ta.sma(close, 2)[1], 2)",
        "[plus, minus, adx] = ta.dmi(ta.sma(close, 2)[1], 14)",
        "[trend, direction] = ta.supertrend(ta.sma(close, 2)[1], 14)",
        "[basis, upper, lower] = ta.kc(close, 20, 2, ta.sma(close, 2)[1])",
        "[macd, signal, histogram] = ta.macd(close, ta.sma(close, 2)[1], 26, 9)",
    ] {
        let compilation = rejected(body);
        assert!(
            compilation.diagnostics.iter().any(|diagnostic| {
                diagnostic.code == "PINE_HISTORY_REF_UNSUPPORTED"
                    && diagnostic.message.contains("history references")
            }),
            "{body}: {:?}",
            compilation.diagnostics
        );
    }
    // The frozen function uses call-result history, never a blanket ban on series history.
    let compilation = compile(&script("[current, previous] = [close, close[1]]"));
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    assert!(compilation.program.is_some());
}

/// Parity: go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:29 TestRequestSecurityExpressionTASubsetValidation
#[test]
fn security_inner_expressions_accept_prices_and_averages_but_reject_unknown_or_unclosed_ta() {
    for expression in ["close + open", "ta.sma(close, 20) + ta.ema(close, 10)"] {
        let body = format!("daily = request.security(syminfo.tickerid, \"D\", {expression})");
        let compilation = compile(&script(&body));
        assert!(compilation.ok, "{body}: {:?}", compilation.diagnostics);
        assert!(compilation.program.is_some());
    }
    for expression in ["ta.not_supported(close)", "ta.sma(close, 20"] {
        let body = format!("daily = request.security(syminfo.tickerid, \"D\", {expression})");
        let compilation = rejected(&body);
        assert!(compilation.normalized_script.contains(expression));
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/runtime_and_parser_boundaries_test.go:94 TestMalformedTAExpressionsRemainVisibleForValidation
#[test]
fn malformed_ta_inputs_remain_visible_in_normalized_source_and_fail_before_ir() {
    for expression in [
        "ta.vwap(close",
        "ta.vwap(close, timeframe.change(\"D\")",
        "ta.vwap(close, timeframe.change())",
        "ta.highestbars(close",
        "ta.stoch(close, high",
    ] {
        let body = format!("value = {expression}");
        let compilation = rejected(&body);
        assert_eq!(compilation.normalized_script, script(&body));
    }
}
