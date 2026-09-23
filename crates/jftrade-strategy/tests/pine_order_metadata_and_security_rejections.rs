//! Go-parity regressions for order metadata, request.security purity and
//! tuple/history diagnostics.
//!
//! Parity: `pkg/strategy/pine/order_metadata_contracts_test.go`,
//! `pkg/strategy/pine/language_failure_contracts_test.go`,
//! `pkg/strategy/pine/request_security_diagnostics_test.go` and
//! `pkg/strategy/pine/order_command_security_rejection_test.go`.
//!
//! Go rejects ambiguous order metadata (unknown arguments, non-boolean
//! `disable_alert`/`immediately`, missing ids, excess `strategy.close_all`
//! positionals), impure `request.security` expressions (`strategy.*` reads,
//! `log.*`, drawing objects and collection mutation) and tuple
//! `request.security` calls that are not assigned to matching aliases.  The
//! diagnostics below pin those contracts through the public compile pipeline.

use jftrade_strategy::pine::{AnalysisOptions, Compilation, analyze_script, compile};

fn script(body: &str) -> String {
    format!("//@version=6\nstrategy(\"Parity\", overlay=true)\n{body}")
}

fn compile_error(body: &str) -> (String, String) {
    let compilation = compile(&script(body));
    assert!(!compilation.ok, "compile must fail for {body}");
    let diagnostic = compilation
        .diagnostics
        .iter()
        .find(|diagnostic| diagnostic.severity == jftrade_strategy::pine::DiagnosticSeverity::Error)
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

/// Parity: go:452dea11:pkg/strategy/pine/order_metadata_contracts_test.go:8 TestOrderMetadataRejectsAmbiguousInputsAndKeepsSupportedPositionals
///
/// Parity: go:452dea11:pkg/strategy/pine/language_failure_contracts_test.go:239 TestOrderAndTupleHelperContractsKeepTradeInstructionsUnambiguous
#[test]
fn compile_rejects_ambiguous_order_metadata_and_missing_ids() {
    for (body, code, message) in [
        (
            r#"strategy.entry("Long", strategy.long, disable_alert=maybe)"#,
            "PINE_COMPILE_ERROR",
            "disable_alert must be true or false",
        ),
        (
            r#"strategy.entry("Long", strategy.long, immediately=true)"#,
            "PINE_COMPILE_ERROR",
            "strategy.entry argument immediately is not supported by JFTrade",
        ),
        (
            r#"strategy.order("Net", strategy.long, immediately=true)"#,
            "PINE_COMPILE_ERROR",
            "strategy.order argument immediately is not supported by JFTrade",
        ),
        (
            r#"strategy.exit("Exit", "Long", stop=98, immediately=true)"#,
            "PINE_COMPILE_ERROR",
            "strategy.exit argument immediately is not supported by JFTrade",
        ),
        (
            r#"strategy.close("Long", immediately=maybe)"#,
            "PINE_COMPILE_ERROR",
            "immediately must be true or false",
        ),
        (
            r#"strategy.close("Long", qty=1, qty_percent=20)"#,
            "PINE_ORDER_QTY_CONFLICT",
            "qty or qty_percent",
        ),
        (
            r#"strategy.close()"#,
            "PINE_COMPILE_ERROR",
            "requires an entry id",
        ),
        (
            r#"strategy.entry()"#,
            "PINE_COMPILE_ERROR",
            "requires at least two arguments",
        ),
        (
            r#"strategy.order("Net")"#,
            "PINE_COMPILE_ERROR",
            "requires at least two arguments",
        ),
        (
            r#"strategy.exit()"#,
            "PINE_COMPILE_ERROR",
            "requires an exit id",
        ),
        (
            r#"strategy.cancel()"#,
            "PINE_COMPILE_ERROR",
            "requires one order id",
        ),
        (
            r#"strategy.cancel("first", "second")"#,
            "PINE_COMPILE_ERROR",
            "requires one order id",
        ),
        (
            r#"strategy.close("Long", mystery=true)"#,
            "PINE_COMPILE_ERROR",
            "strategy.close argument mystery is not supported by JFTrade",
        ),
        (
            r#"strategy.entry("Long", strategy.long, mystery=1)"#,
            "PINE_COMPILE_ERROR",
            "strategy.entry argument mystery is not supported by JFTrade",
        ),
        (
            r#"strategy.exit("Exit", "Long", stop=98, mystery=1)"#,
            "PINE_COMPILE_ERROR",
            "strategy.exit argument mystery is not supported by JFTrade",
        ),
        (
            r#"strategy.close_all("maybe")"#,
            "PINE_COMPILE_ERROR",
            "strategy.close_all immediately must be true or false",
        ),
        (
            r#"strategy.close_all(true, "note", "alert", "maybe")"#,
            "PINE_COMPILE_ERROR",
            "strategy.close_all disable_alert must be true or false",
        ),
        (
            r#"strategy.close_all(true, "note", "alert", false, "extra")"#,
            "PINE_COMPILE_ERROR",
            "supports positional immediately, comment, alert_message, and disable_alert only",
        ),
        (
            r#"strategy.close_all(foo=1)"#,
            "PINE_COMPILE_ERROR",
            "strategy.close_all argument foo is not supported by JFTrade",
        ),
        (
            r#"strategy.entry("Long", strategy.long, trail_offset=2)"#,
            "PINE_COMPILE_ERROR",
            "strategy.entry argument trail_offset is not supported by JFTrade",
        ),
        (
            r#"strategy.exit("Exit", "Long", trail_offset=2)"#,
            "PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED",
            "advanced exit semantics",
        ),
        (
            r#"strategy.exit("Exit", "Long", trail_points=2, trail_price=3)"#,
            "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED",
            "accepts trail_points or trail_price, not both",
        ),
    ] {
        assert_error(body, code, message);
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/order_command_security_rejection_test.go:11 TestOrderCommandRejectionsPropagateToPineCallers
///
/// Go asserts every order command form returns the same actionable metadata
/// error: invalid `disable_alert` on entry, order, bracket exit and trailing
/// exit, an unsupported `strategy.close` argument, and a non-boolean
/// `immediately`.
#[test]
fn compile_propagates_order_metadata_rejections_to_every_order_form() {
    for (body, message) in [
        (
            r#"strategy.entry("long", strategy.long, disable_alert=maybe)"#,
            "disable_alert must be true or false",
        ),
        (
            r#"strategy.order("net", strategy.long, disable_alert=maybe)"#,
            "disable_alert must be true or false",
        ),
        (
            r#"strategy.exit("exit", "long", stop=close, disable_alert=maybe)"#,
            "disable_alert must be true or false",
        ),
        (
            r#"strategy.exit("trail", "long", trail_points=2, trail_offset=1, disable_alert=maybe)"#,
            "disable_alert must be true or false",
        ),
        (
            r#"strategy.close("long", unexpected=true)"#,
            "strategy.close argument unexpected is not supported by JFTrade",
        ),
        (
            r#"strategy.close("long", immediately=maybe)"#,
            "strategy.close immediately must be true or false",
        ),
    ] {
        let (code, found) = compile_error(body);
        assert_eq!(code, "PINE_COMPILE_ERROR", "code for {body}");
        assert!(
            found.contains(message),
            "message {found:?} for {body} must contain {message:?}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/order_metadata_contracts_test.go:8 TestOrderMetadataRejectsAmbiguousInputsAndKeepsSupportedPositionals
///
/// Go's parser rejects `immediately` on entry, order and exit through the
/// unsupported-named-argument path and keeps every other message at the order
/// metadata helper. It keeps the second positional `strategy.close` argument as a
/// `symbol_position_percent` quantity, keeps the four supported
/// `strategy.close_all` positionals and treats single-quoted text as a string
/// literal.
#[test]
fn compile_accepts_supported_order_positional_metadata() {
    for body in [
        r#"strategy.close("Long", 2)"#,
        r#"strategy.close_all(true, "flat", "done", false)"#,
        r#"strategy.close_all(immediately=true, comment="done")"#,
        r#"strategy.exit("Exit", "Long", trail_points=3, trail_offset=2)"#,
    ] {
        let compilation = compile(&script(body));
        assert!(
            compilation.ok,
            "diagnostics for {body} = {:?}",
            compilation.diagnostics
        );
    }

    let compilation = compile(&script(
        r#"strategy.close_all(true, 'close comment', 'close alert', true)"#,
    ));
    assert!(
        compilation.ok,
        "diagnostics = {:?}",
        compilation.diagnostics
    );
    let rendered = rendered_actions(&compilation);
    for wanted in ["\"close comment\"", "\"close alert\""] {
        assert!(
            rendered.contains(wanted),
            "close_all positional metadata {rendered:?} missing {wanted}"
        );
    }

    let compilation = compile(&script(r#"strategy.close("Long", comment='stop here')"#));
    assert!(
        compilation.ok,
        "diagnostics = {:?}",
        compilation.diagnostics
    );
    let rendered = rendered_actions(&compilation);
    assert!(
        rendered.contains("\"stop here\""),
        "single-quoted comment {rendered:?} must decode to stop here"
    );
    assert!(
        !rendered.contains("\\\'"),
        "single-quoted comment {rendered:?} must not keep its delimiters"
    );
}

fn rendered_actions(compilation: &Compilation) -> String {
    let program = compilation.program.as_ref().expect("lowered program");
    let lowered = serde_json::to_value(program).expect("lowered program json");
    lowered["hooks"][0]["statements"]
        .as_array()
        .expect("statements")
        .iter()
        .map(ToString::to_string)
        .collect::<Vec<_>>()
        .join(" | ")
}

/// Parity: go:452dea11:pkg/strategy/pine/request_security_diagnostics_test.go:8 TestRequestSecurityDiagnosticsRejectUnsafeOrAmbiguousInputs
#[test]
fn request_security_rejects_impure_member_and_visual_side_effects() {
    for body in [
        r#"value = request.security(syminfo.tickerid, "60", strategy.position_size)"#,
        r#"value = request.security(syminfo.tickerid, "60", close + strategy.position_size)"#,
        r#"value = request.security(syminfo.tickerid, "60", log.info("unsafe"))"#,
        r#"value = request.security(syminfo.tickerid, "60", line.new(bar_index, close, bar_index, open))"#,
        r#"value = request.security(syminfo.tickerid, "60", values.push(close))"#,
    ] {
        assert_error(
            body,
            "PINE_REQUEST_SECURITY_SIDE_EFFECT",
            "request.security() expression must be pure",
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/request_security_diagnostics_test.go:8 TestRequestSecurityDiagnosticsRejectUnsafeOrAmbiguousInputs
#[test]
fn request_security_merge_flags_are_rejected_in_named_and_positional_form() {
    for (body, code) in [
        (
            r#"value = request.security(syminfo.tickerid, "60", close, barmerge.gaps_off, barmerge.lookahead_on)"#,
            "PINE_REQUEST_SECURITY_LOOKAHEAD",
        ),
        (
            r#"value = request.security(syminfo.tickerid, "60", close, barmerge.gaps_on)"#,
            "PINE_REQUEST_SECURITY_GAPS",
        ),
        (
            r#"value = request.security(syminfo.tickerid, "60", close, lookahead=barmerge.lookahead_on)"#,
            "PINE_REQUEST_SECURITY_LOOKAHEAD",
        ),
        (
            r#"value = request.security(syminfo.tickerid, "60", close, gaps=barmerge.gaps_on)"#,
            "PINE_REQUEST_SECURITY_GAPS",
        ),
    ] {
        assert_error(body, code, "request.security()");
    }

    let compilation = compile(&script(
        r#"value = request.security(syminfo.tickerid, "60", close, barmerge.gaps_off, barmerge.lookahead_off)"#,
    ));
    assert!(
        compilation.ok,
        "supported merge flags must compile: {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/request_security_diagnostics_test.go:8 TestRequestSecurityDiagnosticsRejectUnsafeOrAmbiguousInputs
///
/// Parity: go:452dea11:pkg/strategy/pine/language_failure_contracts_test.go:149 TestRequestSecurityAndTupleContractsRejectUnsafeExpressions
///
/// Parity: go:452dea11:pkg/strategy/pine/order_command_security_rejection_test.go:82 TestRequestSecurityTupleValidationKeepsParserBoundaries
#[test]
fn request_security_tuple_diagnostics_match_go_codes() {
    assert_error(
        r#"value = request.security(syminfo.tickerid, "60", [close, open])"#,
        "PINE_REQUEST_SECURITY_TUPLE_ASSIGNMENT",
        "request.security() tuple expressions must be assigned with matching tuple aliases",
    );
    assert_error(
        r#"value = request.security(syminfo.tickerid, "60", [close])"#,
        "PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED",
        "request.security() tuple expressions support 2 to 8 values",
    );
    assert_error(
        r#"[only] = request.security(syminfo.tickerid, "60", [close, open])"#,
        "PINE_REQUEST_SECURITY_TUPLE_MISMATCH",
        "request.security() tuple returns 2 values but assignment has 1 aliases",
    );
    assert_error(
        r#"[left, right] = request.security(syminfo.tickerid, "60", [close])"#,
        "PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED",
        "request.security() tuple expressions support 2 to 8 values",
    );

    let compilation = compile(&script(
        r#"[left, right] = request.security(syminfo.tickerid, "60", [close, open])"#,
    ));
    assert!(
        compilation.ok,
        "supported request.security tuple diagnostics = {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/language_failure_contracts_test.go:149 TestRequestSecurityAndTupleContractsRejectUnsafeExpressions
#[test]
fn history_reference_overflow_is_rejected() {
    assert_error(
        "value = close[999999999999999999999999999999]",
        "PINE_HISTORY_REF_UNSUPPORTED",
        "history reference lookback must be a non-negative integer",
    );
    let analysis = analyze_script(
        &script("value = close[999999999999999999999999999999]"),
        AnalysisOptions::default(),
    );
    assert!(
        !analysis.ok,
        "analysis must reject the overflowing lookback"
    );
}
