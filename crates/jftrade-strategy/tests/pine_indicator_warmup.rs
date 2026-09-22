use jftrade_strategy::pine::compile;

fn warmup_bars(script: &str, interval: &str, symbol: &str, extended: bool) -> usize {
    let compilation = compile(script);
    assert!(
        compilation.ok,
        "script must compile: {:?}",
        compilation.diagnostics
    );
    compilation
        .requirements
        .try_derived_warmup_bars_with_session(symbol, interval, extended)
        .unwrap_or_else(|error| panic!("warmup for {symbol}@{interval} failed: {error}"))
}

const MIXED_PLAN: &str = r#"//@version=6
strategy("Warmup Max", overlay=true)
fast = ta.sma(close, 5)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(fast, signal)
    alert("go")
"#;

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:12 TestWarmupBarsFromPlanUsesLargestIndicatorRequirement
#[test]
fn warmup_bars_use_the_largest_indicator_requirement() {
    let compilation = compile(MIXED_PLAN);
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    let keys = compilation
        .requirements
        .indicators
        .iter()
        .map(|item| item.key.as_str())
        .collect::<Vec<_>>();
    assert!(keys.contains(&"ma:SMA:20:day"), "{keys:?}");

    // The daily 20-period SMA needs 20 * 390 one-minute bars, which is larger
    // than the 5-bar SMA and the MACD's 26 + 9 warmup.
    assert_eq!(warmup_bars(MIXED_PLAN, "1m", "US.AAPL", false), 20 * 390);
}

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:40 TestWarmupBarsFromPlanForSymbolUsesMarketTradingProfiles
#[test]
fn warmup_bars_follow_market_trading_profiles() {
    assert_eq!(warmup_bars(MIXED_PLAN, "1m", "US.AAPL", false), 20 * 390);
    assert_eq!(warmup_bars(MIXED_PLAN, "1m", "HK.00700", false), 20 * 330);
    assert_eq!(warmup_bars(MIXED_PLAN, "1m", "SH.600519", false), 20 * 240);
    assert_eq!(warmup_bars(MIXED_PLAN, "1m", "SZ.000001", false), 20 * 240);
}

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:74 TestWarmupBarsFromPlanForSymbolUsesExtendedTradingDayWhenEnabled
#[test]
fn warmup_bars_use_the_extended_trading_day_when_enabled() {
    const SCRIPT: &str = r#"//@version=6
strategy("Warmup Extended", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 5))
"#;
    assert_eq!(warmup_bars(SCRIPT, "1m", "US.AAPL", true), 5 * 24 * 60);
}

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_plan_test.go:98 TestWarmupBarsFromPlanDoesNotApplyRuntimeSeriesFloor
#[test]
fn warmup_bars_omit_a_runtime_series_floor() {
    const SCRIPT: &str = r#"//@version=6
strategy("Warmup Small", overlay=true)
fast = ta.sma(close, 5)
"#;
    assert_eq!(warmup_bars(SCRIPT, "1m", "US.AAPL", false), 5);
}

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:14 TestWarmupBarsFromScriptMatchesPlanWithExtendedHours
#[test]
fn warmup_bars_from_a_script_match_the_plan_for_extended_hours() {
    const SCRIPT: &str = r#"//@version=6
strategy("Warmup Script", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(close, slow)
    strategy.entry("Long", strategy.long, qty=1)
"#;
    // Both Go entry points compute the same value; Rust derives requirements
    // from the script and computes the warmup from that plan.
    assert_eq!(warmup_bars(SCRIPT, "1m", "US.AAPL", true), 20 * 24 * 60);
}

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:51 TestWarmupBarsFromScriptFallsBackToGenericTradingCalendarForUnknownSymbol
#[test]
fn warmup_bars_fall_back_to_the_generic_calendar_for_unknown_symbols() {
    const SCRIPT: &str = r#"//@version=6
strategy("Unknown Symbol Warmup", overlay=true)
monthly = request.security(syminfo.tickerid, "M", ta.sma(close, 1))
"#;
    // Go's generic calendar is 390 minutes per day and 20 days per month.
    assert_eq!(warmup_bars(SCRIPT, "5m", "CRYPTO.BTC", false), 390 * 20 / 5);
}

/// Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_script_test.go:67 TestRequestSecurityTimeframeRequirementsValidateAgainstStrategyInterval
#[test]
fn request_security_timeframes_validate_against_the_strategy_interval() {
    const SCRIPT: &str = r#"//@version=6
strategy("MTF Validation", overlay=true)
fast = request.security(syminfo.tickerid, "15", ta.ema(close, 20))
if close > fast
    strategy.entry("Long", strategy.long, qty=1)
"#;
    let compilation = compile(SCRIPT);
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    let keys = compilation
        .requirements
        .indicators
        .iter()
        .map(|item| item.key.as_str())
        .collect::<Vec<_>>();
    assert_eq!(keys, vec!["ma:EMA:20:15m"], "planned keys = {keys:?}");

    assert_eq!(warmup_bars(SCRIPT, "1m", "US.AAPL", false), 20 * 15);
    for interval in ["1h", "1d"] {
        let error = compilation
            .requirements
            .try_derived_warmup_bars_with_session("US.AAPL", interval, false)
            .expect_err("a lower security timeframe must be rejected");
        assert!(
            error.contains("fixed timeframe 15m is lower than strategy interval"),
            "{interval} error = {error}"
        );
    }
}
