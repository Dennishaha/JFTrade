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

const LEGACY_SOURCE_KEYS: &str = r#"//@version=6
strategy("Source Keys", overlay=true)
closeSma = ta.sma(close, 20)
volumeSma = ta.sma(volume, 20)
hlc3Ema = ta.ema(hlc3, 20)
closeRsi = ta.rsi(close, 14)
hlc3Rsi = ta.rsi(hlc3, 14)
legacyCci = ta.cci(hlc3, 20)
closeCci = ta.cci(close, 20)
if close > closeSma and volume > volumeSma and hlc3Ema > 0 and closeRsi > hlc3Rsi and closeCci > legacyCci
    strategy.entry("Long", strategy.long, qty=1)
"#;

/// Parity: go:452dea11:pkg/strategy/ir/planner_test.go:204 TestPlanRequirementsPreservesLegacyCloseKeysAndSourceAwareKeys
#[test]
fn legacy_and_explicit_sources_keep_the_planned_keys() {
    let keys = planned_keys(LEGACY_SOURCE_KEYS);
    assert_eq!(
        keys,
        vec![
            "cci:20",
            "cci:close:20",
            "ma:EMA:20:hlc3",
            "ma:SMA:20",
            "ma:SMA:20:volume",
            "rsi:14",
            "rsi:hlc3:14",
        ]
    );
    // The literal `close` of a legacy key and the non-legacy `hlc3` of the CCI
    // key are both dropped, so these duplicate shapes must never appear.
    for unexpected in ["ma:SMA:20:close", "rsi:close:14", "cci:hlc3:20"] {
        assert!(!keys.iter().any(|key| key == unexpected), "{keys:?}");
    }
}

const WINDOW_AND_OSCILLATOR_KEYS: &str = r#"//@version=6
strategy("Window Keys", overlay=true)
delta = ta.change(close, 1)
up = ta.rising(close, 3)
down = ta.falling(close, 3)
momentum = ta.mom(close, 5)
rate = ta.roc(close, 12)
total = ta.sum(volume, 20)
deviation = ta.stdev(close, 20)
sourceDeviation = ta.stdev(hlc3, 11)
cciValue = ta.cci(hlc3, 20)
rsiValue = ta.rsi(close, 14)
upper = ta.highest(high, 20)
lower = ta.lowest(low, 10)
williams = ta.wpr(14)
"#;

/// Parity: go:452dea11:pkg/strategy/ir/planner_test.go:122 TestPlanRequirementsIndicatorKeysMatchRuntimeBindingParity
#[test]
fn window_and_oscillator_keys_keep_the_requested_source() {
    let keys = planned_keys(WINDOW_AND_OSCILLATOR_KEYS);
    assert_eq!(
        keys,
        vec![
            "cci:20",
            "change:close:1",
            "falling:close:3",
            "highest:high:20",
            "lowest:low:10",
            "mom:close:5",
            "rising:close:3",
            "roc:close:12",
            "rsi:14",
            "stdev:20",
            "stdev:hlc3:11",
            "sum:volume:20",
            "williamsr:14",
        ]
    );
}

const POSITION_VARIABLES: &str = r#"//@version=6
strategy("Position Variables", overlay=true)
stopPrice = strategy.position_avg_price * 0.95
if strategy.position_size > 0
    alert("position")
"#;

/// Parity: go:452dea11:pkg/strategy/ir/planner_test.go:100 TestPlanRequirementsDetectsPositionVariablesInExpressions
#[test]
fn position_variables_in_expressions_require_position_data() {
    let compilation = compile(POSITION_VARIABLES);
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    assert!(compilation.requirements.requires_position);
}

const ACCOUNT_VALUE_STATEMENTS: &str = r#"//@version=6
strategy("Account Value", overlay=true)
balance = strategy.equity
if balance > 0
    strategy.entry("Long", strategy.long, qty=1)
"#;

/// Parity: go:452dea11:pkg/strategy/ir/planner_business_boundary_test.go:42 TestPlanRequirementsCollectsLoopObjectAndExitExpressions
#[test]
fn account_value_usage_in_statements_requires_total_account_value() {
    let compilation = compile(ACCOUNT_VALUE_STATEMENTS);
    assert!(compilation.ok, "{:?}", compilation.diagnostics);
    assert!(compilation.requirements.requires_total_account_value);
    assert!(compilation.requirements.requires_position);
}
