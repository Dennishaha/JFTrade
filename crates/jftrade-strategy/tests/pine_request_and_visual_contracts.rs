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
dailyClose = request.security(syminfo.tickerid, "D", close)
dailyHlc3 = request.security(syminfo.tickerid, "D", hlc3)
dailyHlc3Ema = request.security(syminfo.tickerid, "D", ta.ema(hlc3, 20))
tf = input.timeframe("15", "MTF")
fifteenClose = request.security(syminfo.tickerid, tf, close)
fourHourHlc3 = request.security(syminfo.tickerid, "240", hlc3)
dailyPreviousClose = request.security(syminfo.tickerid, "D", close[1])
fifteenHlc3Ema = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 20), gaps=barmerge.gaps_off, lookahead=barmerge.lookahead_off)"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_request_test.go:10 TestCompileSupportsMovingAverageRequestSecuritySubset
#[test]
fn request_security_moving_average_keys_keep_type_period_source_and_time_unit() {
    // Go's MTF matrix keeps moving-average and plain source requirements in
    // separate catalog families, including source history lookbacks.
    let keys = planned_keys(MTF_MA_SCRIPT);
    for wanted in [
        "ma:EMA:5:day",
        "ma:SMA:20:hour",
        "security_source:day:close",
        "security_source:day:hlc3",
        "ma:EMA:20:day:hlc3",
        "security_source:15m:close",
        "security_source:240m:hlc3",
        "security_source:day:close:1",
        "ma:EMA:20:15m:hlc3",
    ] {
        assert!(keys.iter().any(|key| key == wanted), "{wanted} in {keys:?}");
    }
}

#[test]
fn request_security_common_ta_keys_keep_the_inner_family_and_time_unit() {
    let script = r#"//@version=6
strategy("v1.5 MTF common TA", overlay=true)
signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 14), 50) > 50 and nz(ta.macd(close, 12, 26, 9).diff, 0) > 0 and nz(ta.atr(14), 0) > 0 and nz(ta.bb(close, 20, 2).upper, close) > close and nz(ta.supertrend(3, 10).direction, 0) > 0)
spread = ta.range(close, 5)
modeValue = ta.mode(close, 5)
if signal
    strategy.entry("Long", strategy.long, qty=1)"#;
    let compilation = compile(script);
    assert!(
        compilation.ok,
        "diagnostics = {:?}",
        compilation.diagnostics
    );
    // The Rust public lowered program preserves typed AST expressions rather
    // than Go's rendered IR strings; requirement keys are the stable parity
    // surface for this production owner.
    let keys = compilation
        .requirements
        .indicators
        .iter()
        .map(|item| item.key.clone())
        .collect::<Vec<_>>();
    for wanted in [
        "security_source:15m:close",
        "rsi:close:14:15m",
        "macd:close:12:26:9:15m",
        "atr:14:15m",
        "bollinger:close:20:2:15m",
        "supertrend:3:10:15m",
        "range:close:5",
        "mode:close:5",
    ] {
        assert!(keys.iter().any(|key| key == wanted), "{wanted} in {keys:?}");
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/parse_request_test.go:141
/// TestCompileSupportsV14WindowMomentumAndStatefulIndicators
#[test]
fn compile_v14_window_momentum_fixture_keeps_go_requirement_keys() {
    let script = r#"//@version=6
strategy("v1.4 window state", overlay=true)
dev = ta.stdev(close, 5)
variance = ta.variance(close, 5)
hb = ta.highestbars(high, 5)
lb = ta.lowestbars(low, 5)
delta = ta.change(close)
momentum = ta.mom(close, 3)
rate = ta.roc(close, 3)
up = ta.rising(close, 3)
down = ta.falling(close, 3)
bars = ta.barssince(close > open)
value = ta.valuewhen(close > open, close, 0)
trTrue = ta.tr(true)
trFalse = ta.tr(false)
if up and not down and nz(bars, 999) < 5 and nz(value, close) > 0 and trTrue >= trFalse
    strategy.entry("Long", strategy.long, qty=1)"#;
    let compilation = compile(script);
    assert!(
        compilation.ok,
        "Go V14 fixture must compile in Rust: {:?}",
        compilation.diagnostics
    );
    let keys = compilation
        .requirements
        .indicators
        .iter()
        .map(|requirement| requirement.key.as_str())
        .collect::<Vec<_>>();
    for expected in [
        "stdev:5",
        "variance:close:5",
        "highestbars:high:5",
        "lowestbars:low:5",
        "change:close:1",
        "mom:close:3",
        "roc:close:3",
        "rising:close:3",
        "falling:close:3",
    ] {
        assert!(
            keys.contains(&expected),
            "Go V14 fixture requirements {keys:?} missing {expected}"
        );
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
wr = ta.wpr(14)
[basis, upper, lower] = ta.bb(close, 20, 2)
if trendUp and close > hh and close < upper and wr < -20
    strategy.entry("Long", strategy.long, qty=1)"#;

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
        "bollinger:20:2",
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
