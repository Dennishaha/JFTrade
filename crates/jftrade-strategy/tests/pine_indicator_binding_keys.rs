use jftrade_strategy::pine::compile;

fn indicator_keys(source: &str) -> Vec<String> {
    let compilation = compile(source);
    assert!(
        compilation.ok,
        "script must compile: {:?}",
        compilation.diagnostics
    );
    compilation
        .requirements
        .indicators
        .iter()
        .map(|item| item.key.clone())
        .collect()
}

/// Go builds moving-average requirement keys as `ma:<TYPE>:<period>` with an
/// optional `:<time unit>` and `:<source>` suffix. The Rust planner produces
/// the same shape for the Pine call forms it supports, so the key asserts are
/// written against the Go expectations. Go's `MA` alias has no Pine
/// counterpart (`ta.ma` does not exist), which is why the Go table's
/// `ma:MA:20` case stays a documented gap instead of an assertion here.
///
/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:304 TestBuildMovingAverageKey
#[test]
fn moving_average_keys_carry_type_period_and_security_time_unit() {
    let minute_unit = indicator_keys(
        "//@version=6\nstrategy(\"Keys\")\nmtfFast = request.security(syminfo.tickerid, \"1\", ta.ema(close, 14))\nif mtfFast > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)\n",
    );
    assert!(
        minute_unit.contains(&"ma:EMA:14:minute".to_owned()),
        "{minute_unit:?}"
    );

    let day_unit = indicator_keys(
        "//@version=6\nstrategy(\"Keys\")\nmtfSlow = request.security(syminfo.tickerid, \"D\", ta.sma(close, 5))\nif mtfSlow > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)\n",
    );
    assert!(
        day_unit.contains(&"ma:SMA:5:day".to_owned()),
        "{day_unit:?}"
    );
}

/// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:114 TestParsePriceSourceAndBuildMovingAverageKeyWithSource
#[test]
fn moving_average_keys_append_the_requested_source_and_time_unit() {
    let default_source = indicator_keys(
        "//@version=6\nstrategy(\"Keys\")\nfast = ta.ema(close, 14)\nif fast > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)\n",
    );
    assert!(
        default_source.contains(&"ma:EMA:14".to_owned()),
        "a close source stays implicit: {default_source:?}"
    );

    let non_default_source = indicator_keys(
        "//@version=6\nstrategy(\"Keys\")\nslow = ta.sma(volume, 21)\nif slow > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)\n",
    );
    assert!(
        non_default_source.contains(&"ma:SMA:21:volume".to_owned()),
        "{non_default_source:?}"
    );

    let unit_and_source = indicator_keys(
        "//@version=6\nstrategy(\"Keys\")\nmtfFast = request.security(syminfo.tickerid, \"60\", ta.ema(hlc3, 14))\nif mtfFast > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)\n",
    );
    assert!(
        unit_and_source.contains(&"ma:EMA:14:hour:hlc3".to_owned()),
        "{unit_and_source:?}"
    );
}
