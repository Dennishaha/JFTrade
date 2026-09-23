use jftrade_strategy::pine::compile;

fn script(body: &str) -> String {
    format!("//@version=6\nstrategy(\"Boundaries\", overlay=true)\n{body}")
}

fn keys(body: &str) -> Vec<String> {
    let compilation = compile(&script(body));
    assert!(
        compilation.ok,
        "script must compile: {body}\ndiagnostics = {:?}",
        compilation.diagnostics
    );
    let mut keys = compilation
        .requirements
        .indicators
        .iter()
        .map(|item| item.key.clone())
        .collect::<Vec<_>>();
    keys.sort();
    keys
}

fn codes(body: &str) -> Vec<String> {
    compile(&script(body))
        .diagnostics
        .iter()
        .map(|item| item.code.clone())
        .collect()
}

/// Parity: go:452dea11:pkg/strategy/pine/expression_test.go:5
/// TestParseExpressionRejectsBlankInput
#[test]
fn expression_parser_rejects_blank_assignment() {
    let compilation = compile("//@version=6\nstrategy(\"Blank\")\nvalue =");
    assert!(
        !compilation.ok,
        "blank expression must not compile: {:?}",
        compilation.diagnostics
    );
    assert!(
        compilation
            .diagnostics
            .iter()
            .any(|item| item.code == "PINE_EXPRESSION_REQUIRED"),
        "diagnostics = {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/expression_test.go:11
/// TestParseExpressionParsesTrimmedExpressions
#[test]
fn expression_parser_accepts_trimmed_expressions() {
    let compilation = compile(&script("value = close + 1"));
    assert!(
        compilation.ok,
        "trimmed expression must compile: {:?}",
        compilation.diagnostics
    );
    assert!(compilation.program.is_some(), "lowered program");
}

/// Parity: go:452dea11:pkg/strategy/pine/expression_test.go:21
/// TestParseExpressionRejectsInvalidSyntax
#[test]
fn expression_parser_rejects_trailing_operator() {
    let compilation = compile(&script("value = close +"));
    assert!(
        !compilation.ok,
        "trailing operator must not compile: {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/extended_ticker_test.go:49
/// TestCompileAcceptsExtendedTickerAndChartFlags
#[test]
fn extended_ticker_and_chart_flags_keep_requirements() {
    let compilation = compile(
        "//@version=6\nstrategy(\"Extended ticker\")\nhaClose = request.security(ticker.heikinashi(syminfo.tickerid), \"60\", close)\nstandardClose = request.security(ticker.standard(), \"60\", close)\nsignal = chart.is_heikinashi ? haClose > standardClose : chart.is_standard\nif signal\n    strategy.entry(\"long\", strategy.long)",
    );
    assert!(
        compilation.ok,
        "extended ticker script must compile: {:?}",
        compilation.diagnostics
    );
    let program = compilation.program.expect("lowered program");
    assert!(!program.hooks.is_empty(), "hooks = {:?}", program.hooks);
    assert!(
        !program.hooks[0].statements.is_empty(),
        "statements = {:?}",
        program.hooks[0].statements
    );
    assert!(
        !compilation.requirements.indicators.is_empty(),
        "requirements = {:?}",
        compilation.requirements
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/extended_ticker_test.go:5
/// TestExtendedTickerRequestSecuritySupportsCurrentSymbolOnly
///
/// Go keeps the current-symbol tickers and rejects every other symbol with
/// `PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL`. Rust lowers the accepted tickers to
/// the opaque `security:` requirement instead of Go's `security_source` key,
/// so only the acceptance and the diagnostic code are compared here.
#[test]
fn request_security_tickers_follow_the_go_whitelist() {
    for ticker in [
        "syminfo.tickerid",
        "ticker.heikinashi(syminfo.tickerid)",
        "ticker.standard(syminfo.tickerid)",
        "ticker.standard()",
        "ticker.inherit(ticker.heikinashi(syminfo.tickerid), syminfo.tickerid)",
    ] {
        let compilation = compile(&script(&format!(
            "value = request.security({ticker}, \"60\", close)"
        )));
        assert!(
            compilation.ok,
            "ticker {ticker} must compile: {:?}",
            compilation.diagnostics
        );
    }
    for ticker in [
        "\"NASDAQ:AAPL\"",
        "ticker.heikinashi(\"NASDAQ:AAPL\")",
        "ticker.standard(otherTicker)",
        "ticker.inherit(syminfo.tickerid, \"NASDAQ:AAPL\")",
        "ticker.renko(syminfo.tickerid)",
    ] {
        let codes = codes(&format!(
            "value = request.security({ticker}, \"60\", close)"
        ));
        assert!(
            codes
                .iter()
                .any(|code| code == "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL"),
            "ticker {ticker} codes = {codes:?}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/compiler_and_security_diagnostics_test.go:11
/// TestCompilerDiagnosticsPreserveActionablePlannerAndRemoteErrors
///
/// Go projects the planner failure of `ta.sma(close, 0)` into an error
/// diagnostic that mentions the positive-integer period. The Go-only
/// `diagnosticFromError`/`diagnosticFromWarning` line mapping is not part of the
/// Rust compiler surface.
#[test]
fn moving_average_period_must_be_positive() {
    let compilation =
        compile("//@version=6\nstrategy(\"planner rejection\")\nslow = ta.sma(close, 0)");
    assert!(
        !compilation.ok,
        "zero-length moving average must be rejected"
    );
    assert!(
        compilation
            .diagnostics
            .iter()
            .any(|item| item.code == "PINE_REQUIREMENTS_INVALID"
                && item.message.contains("period must be a positive integer")),
        "diagnostics = {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/compiler_rejection_contracts_test.go:9
/// TestUnsupportedSyntaxDiagnosticsDescribeUnsafeRequestSecurityContracts
///
/// Go rejects `request.security(syminfo.tickerid, "2", close)` with the static
/// timeframe contract; Rust surfaces the same message through the planner.
#[test]
fn request_security_rejects_unlisted_static_timeframe_strings() {
    let compilation = compile(&script(
        "value = request.security(syminfo.tickerid, \"2\", close)",
    ));
    assert!(
        !compilation.ok,
        "unsupported static timeframe must be rejected"
    );
    assert!(
        compilation
            .diagnostics
            .iter()
            .any(|item| item.message.contains("only static timeframe strings")),
        "diagnostics = {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/language_execution_boundaries_test.go:600
/// TestTALoweringHelpersKeepNativePineArgumentSemantics
///
/// Go's `pineWindowFunctionArgs` keeps the high/low defaults for the extrema
/// calls, reads a lone argument as the period for `highest`/`lowest`, and reads
/// it as the source with a 14-bar default for the momentum calls. Rust plans the
/// same requirement keys; the TA string-rewrite helpers of that reference case
/// have no Rust counterpart.
#[test]
fn window_family_defaults_keep_the_go_source_and_period() {
    assert_eq!(keys("value = ta.highest(10)"), vec!["highest:high:10"]);
    assert_eq!(keys("value = ta.highest()"), vec!["highest:high:14"]);
    assert_eq!(keys("value = ta.lowest(5)"), vec!["lowest:low:5"]);
    assert_eq!(keys("value = ta.mom(hl2)"), vec!["mom:hl2:14"]);
    assert_eq!(keys("value = ta.mom()"), vec!["mom:close:14"]);
    assert_eq!(keys("value = ta.rising(close, 7)"), vec!["rising:close:7"]);
    assert_eq!(keys("value = ta.change(close)"), vec!["change:close:1"]);
}
