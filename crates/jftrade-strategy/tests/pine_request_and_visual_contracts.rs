use jftrade_strategy::pine::compile;

fn planned_keys(script: &str) -> Vec<String> {
    let compilation = compile(script);
    assert!(
        compilation.ok,
        "script must compile: {:?}",
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

/// Parity: go:452dea11:pkg/strategy/pine/parse_object_test.go:136 TestAnalyzeScriptReportsV29RequestSecurityDiagnostics
#[test]
fn unsupported_request_security_forms_keep_the_go_diagnostic_codes() {
    // Go lists the seven request.security rejections; every one must keep its
    // stable code in Rust.
    for (body, code) in [
        (
            "x = request.security(\"NASDAQ:AAPL\", \"D\", close)",
            "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL",
        ),
        (
            "tf = input.timeframe(\"15\", \"TF\")\nx = request.security(syminfo.tickerid, tf + \"\", close)",
            "PINE_REQUEST_SECURITY_DYNAMIC_TIMEFRAME",
        ),
        (
            "x = request.security(syminfo.tickerid, \"D\", request.security(syminfo.tickerid, \"15\", close))",
            "PINE_REQUEST_SECURITY_NESTED",
        ),
        (
            "x = request.security(syminfo.tickerid, \"D\", alert(\"no side effects\"))",
            "PINE_REQUEST_SECURITY_SIDE_EFFECT",
        ),
        (
            "x = request.security(syminfo.tickerid, \"D\", close, lookahead=barmerge.lookahead_on)",
            "PINE_REQUEST_SECURITY_LOOKAHEAD",
        ),
        (
            "x = request.security(syminfo.tickerid, \"D\", close, gaps=barmerge.gaps_on)",
            "PINE_REQUEST_SECURITY_GAPS",
        ),
        (
            "x = request.security(syminfo.tickerid, \"D\", close, calc_bars_count=100)",
            "PINE_REQUEST_SECURITY_CALC_BARS_COUNT",
        ),
    ] {
        let script = format!("//@version=6\nstrategy(\"request diagnostics\")\n{body}");
        let compilation = compile(&script);
        assert!(!compilation.ok, "{code} must fail the analysis");
        assert!(
            compilation
                .diagnostics
                .iter()
                .any(|diagnostic| diagnostic.code == code),
            "missing {code}: {:?}",
            compilation.diagnostics
        );
    }
}

const STDEV_SCRIPT: &str = r#"//@version=6
strategy("Stdev", overlay=true)
dev = 2.0 * ta.stdev(close, 20)
if close > dev
    strategy.entry("Long", strategy.long, qty=1)"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_request_test.go:82 TestCompileSupportsPineStdev
#[test]
fn stdev_keeps_the_legacy_close_key() {
    // Go lowers `2.0 * ta.stdev(close, 20)` and plans the close-based legacy
    // key; Rust keeps the same requirement key.
    assert_eq!(planned_keys(STDEV_SCRIPT), vec!["stdev:20"]);
}

const VISUAL_SCRIPT: &str = r#"//@version=6
strategy("Visual", overlay=true)
plot(close)
alertcondition(close > open, "Up")
label.new(bar_index, close, "x")
if close > open
    plotshape(true)
strategy.entry("Long", strategy.long, qty=1)"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:377 TestCompileIgnoresVisualCallsWithWarning
#[test]
fn visual_calls_are_ignored_with_named_warnings() {
    let compilation = compile(VISUAL_SCRIPT);
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    assert_eq!(
        compilation.warnings.len(),
        4,
        "warnings = {:?}",
        compilation.warnings
    );
    let joined = compilation.warnings.join("\n");
    assert!(joined.contains("alertcondition"), "warnings = {joined}");
    assert!(joined.contains("label.new"), "warnings = {joined}");
}

const MTF_MA_SCRIPT: &str = r#"//@version=6
strategy("MTF MA", overlay=true)
fast = request.security(syminfo.tickerid, "D", ta.ema(close, 5))
slow = request.security(syminfo.tickerid, "60", ta.sma(close, 20))
dailyHlc3Ema = request.security(syminfo.tickerid, "D", ta.ema(hlc3, 20))
fifteenHlc3Ema = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 20))"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_request_test.go:10 TestCompileSupportsMovingAverageRequestSecuritySubset
#[test]
fn request_security_moving_average_keys_keep_type_period_source_and_time_unit() {
    // The moving-average subset of Go's MTF matrix maps onto the shared `ma`
    // key with the requested time unit; the plain source subset is covered by
    // the `security_source:` gap recorded in the parity ledger.
    let keys = planned_keys(MTF_MA_SCRIPT);
    for wanted in [
        "ma:EMA:5:day",
        "ma:SMA:20:hour",
        "ma:EMA:20:day:hlc3",
        "ma:EMA:20:15m:hlc3",
    ] {
        assert!(keys.iter().any(|key| key == wanted), "{wanted} in {keys:?}");
    }
}

const COMMON_TA_SCRIPT: &str = r#"//@version=6
strategy("Common TA", overlay=true)
hh = ta.highest(high, 20)
hhDefault = ta.highest(20)
ll = ta.lowest(low, 10)
delta = ta.change(close)
momentum = ta.mom(close, 5)
rate = ta.roc(close, 12)
trendUp = ta.rising(close, 3)
wr = ta.wpr(14)"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_request_test.go:97 TestCompileSupportsCommonTradingViewTAFunctions
#[test]
fn common_ta_window_keys_keep_the_requested_source() {
    let keys = planned_keys(COMMON_TA_SCRIPT);
    for wanted in [
        "highest:high:20",
        "highest:high:20",
        "lowest:low:10",
        "change:close:1",
        "mom:close:5",
        "roc:close:12",
        "rising:close:3",
        "williamsr:14",
    ] {
        assert!(keys.iter().any(|key| key == wanted), "{wanted} in {keys:?}");
    }
}

const VISUAL_METADATA_SCRIPT: &str = r#"//@version=6
strategy("Visual Metadata", overlay=true)
plot(close, title="Close")
alertcondition(close > open, "Up")
if close > open
    plotshape(true)
label.new(bar_index, close, "Entry")
lbl = label.new(bar_index, close, "Assigned")
tbl = table.new(position.top_right, 1, 1)
table.cell(tbl, 0, 0, "Value")
strategy.entry("Long", strategy.long, qty=1)"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:401 TestAnalyzeScriptReturnsVisualMetadata
#[test]
fn visual_metadata_lists_every_drawing_call() {
    let compilation = compile(VISUAL_METADATA_SCRIPT);
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    let visuals = &compilation.semantic.visuals;
    assert_eq!(visuals.len(), 7, "visuals = {visuals:?}");
    assert_eq!(visuals[0].kind, "plot");
    assert_eq!(visuals[0].call, "plot");
    assert_eq!(visuals[0].target.as_deref(), Some("close"));
    assert_eq!(visuals[5].call, "table.new");
    // Rust keeps the member of a qualified target (`top_right`) where Go
    // reports the dotted path (`position.top_right`).
    assert_eq!(visuals[5].target.as_deref(), Some("top_right"));
    assert_eq!(visuals[6].call, "table.cell");
}
