//! Go-parity regressions for the indicator-binding parse tables that the
//! Rust planner still shares: the OHLCV source whitelist, the positive-integer
//! period contract, the DSL time-unit table used by trailing indicator
//! arguments, the static Pine timeframe whitelist and the percentile
//! percentage range.
//!
//! Parity: `pkg/strategy/indicatorbinding/parse_test.go` and
//! `pkg/strategy/indicatorbinding/parse_semantics_test.go`.
//!
//! The reference implementation reads indicator sources through
//! `indicatorbinding.ParsePriceSource`, periods through `ParsePositiveInt`,
//! trailing time units through `ParseIndicatorTimeUnitValue` and
//! `request.security` timeframes through `pkg/strategy/pine::pineTimeframeUnit`.
//! The tests below pin the same accept/reject tables and key shapes through the
//! public compile pipeline.

use jftrade_strategy::pine::{DiagnosticSeverity, compile};

fn script(body: &str) -> String {
    format!("//@version=6\nstrategy(\"Parity\", overlay=true)\n{body}")
}

fn requirement_keys(body: &str) -> Vec<String> {
    let compilation = compile(&script(body));
    assert!(
        compilation.ok,
        "compile must succeed for {body}: {:?}",
        compilation
            .diagnostics
            .iter()
            .map(|diagnostic| format!("{}:{}", diagnostic.code, diagnostic.message))
            .collect::<Vec<_>>()
    );
    compilation
        .requirements
        .indicators
        .iter()
        .map(|indicator| indicator.key.clone())
        .collect()
}

fn compile_error_message(body: &str) -> String {
    let compilation = compile(&script(body));
    assert!(!compilation.ok, "compile must fail for {body}");
    let diagnostic = compilation
        .diagnostics
        .iter()
        .find(|diagnostic| diagnostic.severity == DiagnosticSeverity::Error)
        .unwrap_or_else(|| panic!("missing error diagnostic for {body}"));
    diagnostic.message.clone()
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:509 TestParsePositiveInt
#[test]
fn indicator_periods_require_positive_integer_literals() {
    assert_eq!(requirement_keys("x = ta.sma(close, 3)"), vec!["ma:SMA:3"]);
    assert_eq!(
        requirement_keys("x = ta.highest(high, 5)"),
        vec!["highest:high:5"]
    );
    for body in [
        "x = ta.sma(close, 0)",
        "x = ta.sma(close, -3)",
        "x = ta.sma(close, 2.5)",
        "x = ta.highest(high, 0)",
    ] {
        let message = compile_error_message(body);
        assert!(
            message.contains("must be a positive integer"),
            "message {message:?} for {body}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:114 TestParsePriceSourceAndBuildMovingAverageKeyWithSource
#[test]
fn indicator_sources_follow_the_shared_ohlcv_whitelist() {
    assert_eq!(requirement_keys("x = ta.rsi(hl2, 14)"), vec!["rsi:hl2:14"]);
    assert_eq!(
        requirement_keys("x = ta.sma(volume, 5)"),
        vec!["ma:SMA:5:volume"]
    );
    assert_eq!(
        requirement_keys("x = ta.stoch(close, high, low, 14)"),
        vec!["stoch:close:14"]
    );

    let message = compile_error_message("x = ta.rsi(typical, 14)");
    assert!(
        message.contains("source \"typical\" is not supported")
            && message.contains("open/high/low/close/volume/hl2/hlc3/ohlc4"),
        "message {message:?} must name the allowed sources"
    );

    // `pkg/strategy/ir::parseStochSource` drops `volume` even though the
    // shared whitelist message still lists it.
    let message = compile_error_message("x = ta.stoch(volume, high, low, 14)");
    assert!(
        message.contains("source \"volume\" is not supported"),
        "message {message:?}"
    );
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:114 TestParsePriceSourceAndBuildMovingAverageKeyWithSource
///
/// The reference implementation expands a simple source alias before it reads
/// an indicator source (`src = hl2` turns `ta.sma(src, 5)` into `ma:SMA:5:hl2`).
#[test]
fn indicator_sources_resolve_local_ohlcv_aliases() {
    assert_eq!(
        requirement_keys("src = hl2\nx = ta.sma(src, 5)"),
        vec!["ma:SMA:5:hl2"]
    );
    assert_eq!(
        requirement_keys("base = close\nalias = base\nx = ta.ema(alias, 9)"),
        vec!["ma:EMA:9"]
    );

    // A local that does not resolve to an OHLCV source is kept verbatim
    // instead of being rejected, because the value is unknown at plan time.
    assert_eq!(
        requirement_keys("picked = high\nx = ta.mom(picked, 5)"),
        vec!["mom:high:5"]
    );
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:20 TestParseIndicatorTimeUnitValueSupportsQuotedAndMinuteCountInputs
#[test]
fn trailing_indicator_time_units_follow_the_dsl_parser() {
    for (unit, suffix) in [
        ("\"minute\"", "minute"),
        ("\"60m\"", "hour"),
        ("\"15m\"", "15m"),
        ("\"001m\"", "minute"),
        ("\"30m\"", "30m"),
    ] {
        assert_eq!(
            requirement_keys(&format!("x = ta.stoch(close, high, low, 14, {unit})")),
            vec![format!("stoch:close:14:{suffix}")],
            "unit {unit}"
        );
    }
    for unit in ["\"15\"", "\"1D\"", "\"0m\"", "\"badm\""] {
        let message = compile_error_message(&format!("x = ta.stoch(close, high, low, 14, {unit})"));
        assert!(
            message.contains("time unit") && message.contains("is not supported"),
            "message {message:?} for unit {unit}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:235 TestParseIndicatorTimeUnitValue
#[test]
fn trailing_indicator_time_units_accept_word_and_bar_forms() {
    for (unit, suffix) in [
        ("m", "minute"),
        ("min", "minute"),
        ("minutes", "minute"),
        ("h", "hour"),
        ("hours", "hour"),
        ("day", "day"),
        ("days", "day"),
        ("week", "week"),
        ("mo", "month"),
        ("months", "month"),
    ] {
        assert_eq!(
            requirement_keys(&format!("x = ta.stoch(close, high, low, 14, {unit})")),
            vec![format!("stoch:close:14:{suffix}")],
            "unit {unit}"
        );
    }
    // `bar`/`bars` mean the chart period, which keeps the empty suffix.
    assert_eq!(
        requirement_keys("x = ta.stoch(close, high, low, 14, bar)"),
        vec!["stoch:close:14:"]
    );

    let message = compile_error_message("x = ta.stoch(close, high, low, 14, year)");
    assert!(message.contains("is not supported"), "message {message:?}");
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:283 TestNormalizeIndicatorTimeUnit
///
/// The static Pine timeframe whitelist is narrower than the DSL table: only
/// `1`, `5`, `15`, `30`, `45`, `60`, `120`, `240`, `D`, `W` and `M` may be
/// pinned to a `request.security` call.
#[test]
fn security_timeframes_use_the_static_pine_whitelist() {
    assert_eq!(
        requirement_keys("x = request.security(syminfo.tickerid, \"15\", ta.ema(close, 5))"),
        vec!["ma:EMA:5:15m"]
    );
    assert_eq!(
        requirement_keys("x = request.security(syminfo.tickerid, \"60\", ta.ema(close, 5))"),
        vec!["ma:EMA:5:hour"]
    );
    assert_eq!(
        requirement_keys("x = request.security(syminfo.tickerid, \"D\", close)"),
        vec!["security_source:day:close"]
    );

    for timeframe in ["\"15m\"", "\"60m\"", "\"1m\"", "\"0m\"", "\"2\""] {
        let message = compile_error_message(&format!(
            "x = request.security(syminfo.tickerid, {timeframe}, ta.ema(close, 5))"
        ));
        assert!(
            message.contains("only static timeframe strings"),
            "message {message:?} for timeframe {timeframe}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:574 TestParsePercentage
#[test]
fn percentile_percentage_is_bounded_and_canonicalized() {
    assert_eq!(
        requirement_keys("x = ta.percentile_linear_interpolation(close, 10, 50)"),
        vec!["percentile_linear_interpolation:close:10:50"]
    );
    assert_eq!(
        requirement_keys("x = ta.percentile_nearest_rank(hl2, 10, 50.0)"),
        vec!["percentile_nearest_rank:hl2:10:50"]
    );
    assert_eq!(
        requirement_keys("x = ta.percentile_linear_interpolation(close, 10, 0)"),
        vec!["percentile_linear_interpolation:close:10:0"]
    );
    for percentage in ["101", "-1", "abc"] {
        let message = compile_error_message(&format!(
            "x = ta.percentile_linear_interpolation(close, 10, {percentage})"
        ));
        assert!(
            message.contains("percentage must be between 0 and 100"),
            "message {message:?} for percentage {percentage}"
        );
    }
}
