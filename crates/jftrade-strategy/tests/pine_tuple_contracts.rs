use jftrade_strategy::pine::compile;

fn script(body: &str) -> String {
    format!("//@version=6\nstrategy(\"Tuple contracts\", overlay=true)\n{body}")
}

fn rejection(body: &str) -> String {
    let compilation = compile(&script(body));
    assert!(
        !compilation.ok,
        "script must be rejected: {body}\ndiagnostics = {:?}",
        compilation.diagnostics
    );
    compilation
        .diagnostics
        .iter()
        .map(|diagnostic| diagnostic.message.clone())
        .collect::<Vec<_>>()
        .join("\n")
}

/// Parity: go:452dea11:pkg/strategy/pine/tuple_assignment_contracts_test.go:11
/// TestGeneralTupleAssignmentsKeepPineAliasContract
///
/// Go rejects more than eight aliases, a damaged alias, and a tuple whose value
/// count differs from the alias count, while `:=` stays a reassignment.
#[test]
fn tuple_assignments_keep_the_go_alias_and_width_contract() {
    let messages = rejection(
        "[a, b, c, d, e, f, g, h, i] = [open, high, low, close, volume, hl2, hlc3, ohlc4, close]",
    );
    assert!(
        messages.contains("supports 2 to 8 aliases"),
        "messages = {messages}"
    );
    let messages = rejection("[openValue, 4bad] = [open, close]");
    assert!(
        messages.contains("invalid tuple alias"),
        "messages = {messages}"
    );
    let messages = rejection("[first, second] = [close]");
    assert!(
        messages.contains("tuple returns 1 values"),
        "messages = {messages}"
    );

    let compilation = compile(&script("[current, previous] := [close, close[1]]"));
    assert!(
        compilation.ok,
        "tuple reassignment must compile: {:?}",
        compilation.diagnostics
    );
    let program = compilation.program.expect("lowered program");
    let lowered = serde_json::to_value(&program).expect("lowered program json");
    let statement = &lowered["hooks"][0]["statements"][0];
    assert_eq!(statement["kind"], "tuple", "statement = {statement}");
    assert_eq!(statement["names"][0], "current");
    assert_eq!(statement["names"][1], "previous");
    assert_eq!(statement["mode"], "reassign");
}

/// Parity: go:452dea11:pkg/strategy/pine/tuple_switch_reject_test.go:8
/// TestCompileRejectsMalformedSwitchAndTupleContracts
///
/// The eight tuple bodies keep the Go message contract. The switch bodies are
/// rejected as well; the switch language family itself has no Rust runtime yet,
/// so their diagnostics differ from the Go switch messages.
#[test]
fn malformed_tuple_and_switch_scripts_are_rejected() {
    for (body, message) in [
        (
            "[fast, slow] = [close, open, high]",
            "tuple returns 3 values",
        ),
        ("[fast, slow] = [close >, open]", "unexpected token"),
        (
            "[basis, upper, lower] = ta.bb(close, 20)",
            "ta.bb expects ta.bb(source, length, mult)",
        ),
        (
            "[plus, minus, adx] = ta.dmi(14)",
            "ta.dmi expects ta.dmi(diLength, adxSmoothing)",
        ),
        (
            "[trend, direction] = ta.supertrend(3)",
            "ta.supertrend expects ta.supertrend(factor, atrPeriod)",
        ),
        (
            "[basis, upper, lower] = ta.kc(close, 20)",
            "ta.kc expects ta.kc(source, length, mult, useTrueRange?)",
        ),
        (
            "[macd, signal, histogram] = ta.macd(close, 12, 26)",
            "ta.macd expects ta.macd(source, fast, slow, signal)",
        ),
        (
            "[first, second, third] = ta.rsi(close, 14)",
            "tuple assignment is supported only",
        ),
    ] {
        let messages = rejection(body);
        assert!(
            messages.contains(message),
            "body = {body}\nmessages = {messages}"
        );
    }

    let messages = rejection("signal = switch");
    assert!(
        messages.contains("switch requires at least one arm"),
        "messages = {messages}"
    );
    for body in [
        "switch",
        "signal = switch\n    close",
        "signal = switch\n    close > open => close >",
        "switch\n    close > => log.info(\"bad\")",
        "switch\n    close > open => broker.submit()",
    ] {
        let compilation = compile(&script(body));
        assert!(
            !compilation.ok,
            "switch body must be rejected: {body}\ndiagnostics = {:?}",
            compilation.diagnostics
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/parser_loop_boundaries_test.go:38
/// TestStaticLoopBoundsRejectNonTerminatingUserRanges
///
/// Go unrolls constant `for` ranges and rejects a zero step, a range that never
/// lands on the end value, and more than `maxStaticForIterations = 100` values,
/// while a descending range stays valid.
#[test]
fn static_for_ranges_reject_non_terminating_bounds() {
    for (body, message) in [
        (
            "sum = 0\nfor i = 0 to 3 by 0\n    sum := sum + i",
            "step cannot be 0",
        ),
        (
            "sum = 0\nfor i = 3 to 0\n    sum := sum + i",
            "does not reach",
        ),
        (
            "sum = 0\nfor i = 0 to 3 by 2\n    sum := sum + i",
            "does not reach",
        ),
        ("sum = 0\nfor i = 0 to 100\n    sum := sum + i", "more than"),
    ] {
        let messages = rejection(body);
        assert!(
            messages.contains(message),
            "body = {body}\nmessages = {messages}"
        );
    }

    let compilation = compile(&script("sum = 0\nfor i = 3 to 0 by -1\n    sum := sum + i"));
    assert!(
        compilation.ok,
        "descending static range must compile: {:?}",
        compilation.diagnostics
    );
}

/// Parity: go:452dea11:pkg/strategy/pine/public_lowering_test.go:10
/// TestParseScriptPublicEntryReturnsProgramAndPropagatesErrors
///
/// The public parse entry returns the lowered program with the declared name and
/// a buy order inside the if branch, and a disabled internal helper call fails
/// the parse with the Go "internal JFTrade helper" wording.
#[test]
fn public_entry_returns_program_and_propagates_helper_errors() {
    let compilation = compile(
        "//@version=6\nstrategy(\"Public Parse\", overlay=true)\nif close > open\n    strategy.entry(\"Long\", strategy.long, qty=1)",
    );
    assert!(
        compilation.ok,
        "public entry must compile: {:?}",
        compilation.diagnostics
    );
    let program = compilation.program.expect("lowered program");
    assert_eq!(program.metadata.name, "Public Parse");
    assert_eq!(program.hooks.len(), 1, "exactly one hook");
    let lowered = serde_json::to_value(&program).expect("lowered program json");
    let statements = lowered["hooks"][0]["statements"]
        .as_array()
        .cloned()
        .unwrap_or_default();
    assert_eq!(statements.len(), 1, "statements = {statements:?}");
    assert_eq!(statements[0]["kind"], "if", "statement = {}", statements[0]);
    let then_body = statements[0]["then_body"]
        .as_array()
        .cloned()
        .unwrap_or_default();
    let order = then_body.first().expect("then statement");
    assert_eq!(order["kind"], "action", "order = {order}");
    assert_eq!(order["call"], "strategy.entry");
    assert_eq!(order["arguments"][0]["value"], "Long");
    assert_eq!(order["arguments"][2]["left"]["name"], "qty");

    let messages = rejection("value = ma(EMA, 20)");
    assert!(
        messages.contains("internal JFTrade helper"),
        "messages = {messages}"
    );
}
