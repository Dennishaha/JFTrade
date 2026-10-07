use jftrade_strategy::pine::{AnalysisOptions, analyze_script, compile};

// Parity: go:452dea11:pkg/strategy/pine/parse_test.go:838 TestValidateScriptReportsUnsupportedUDFAndStaticForCases
#[test]
fn compiler_rejects_original_udf_and_static_loop_failures_before_planning() {
    let cases = [
        (
            "argument mismatch",
            "//@version=6\nstrategy(\"UDF\", overlay=true)\nf(x) => x\ny = f(close, open)",
            "expects 1 arguments, got 2",
        ),
        (
            "recursive udf",
            "//@version=6\nstrategy(\"UDF\", overlay=true)\nf(x) => f(x)\ny = f(close)",
            "recursive user-defined function",
        ),
        (
            "zero step",
            "//@version=6\nstrategy(\"Loop\", overlay=true)\nfor i = 0 to 3 by 0\n    log.info(\"nope\")",
            "for loop step cannot be 0",
        ),
        (
            "too many iterations",
            "//@version=6\nstrategy(\"Loop\", overlay=true)\nfor i = 0 to 100\n    log.info(\"nope\")",
            "for loop expands to more than 100 iterations",
        ),
        (
            "loop var readonly",
            "//@version=6\nstrategy(\"Loop\", overlay=true)\nfor i = 0 to 3\n    i := 1",
            "loop variable \"i\" is read-only",
        ),
        (
            "call history in unrolled loop",
            "//@version=6\nstrategy(\"Loop\", overlay=true)\nfor i = 0 to 3\n    x = ta.sma(close, 20)[i]",
            "assign the function result first",
        ),
    ];
    let mut missing = Vec::new();
    for (name, script, message) in cases {
        let compilation = compile(script);
        if compilation.ok
            || !compilation
                .diagnostics
                .iter()
                .any(|diagnostic| diagnostic.message.contains(message))
        {
            missing.push(format!(
                "{name}: ok={}, diagnostics={:?}",
                compilation.ok, compilation.diagnostics
            ));
        } else {
            assert!(
                compilation.program.is_none(),
                "{name}: invalid script must not produce executable IR"
            );
            assert!(
                compilation.requirements.indicators.is_empty(),
                "{name}: invalid script must not plan indicators"
            );
        }
    }
    assert!(
        missing.is_empty(),
        "original failures still accepted or misclassified: {}",
        missing.join("\n")
    );
}

// Parity: go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:206 TestOrderCallsRejectUnknownNamedArgumentsBeforePlanning
#[test]
fn analysis_rejects_unknown_entry_and_exit_arguments_at_original_source_lines() {
    for (line, call) in [
        (60, "strategy.entry(\"Long\", strategy.long, unknown=1)"),
        (
            61,
            "strategy.exit(\"Exit\", \"Long\", stop=close, unknown=1)",
        ),
    ] {
        let script = format!(
            "//@version=6\nstrategy(\"unknown argument\")\n{}{call}",
            "\n".repeat(line - 3)
        );
        let result = analyze_script(&script, AnalysisOptions { include_ast: true });
        assert!(!result.ok);
        assert!(result.program.is_none());
        assert!(result.requirements.indicators.is_empty());
        assert!(
            result
                .diagnostics
                .iter()
                .any(|diagnostic| diagnostic.line == line
                    && diagnostic
                        .message
                        .contains("argument unknown is not supported")),
            "{:?}",
            result.diagnostics
        );
    }
}

#[test]
fn repeated_function_body_text_preserves_expression_source_columns() {
    use jftrade_strategy::pine::{ExprKind, Statement, parse};
    for (source, column) in [
        ("f(x) => x", 9),
        ("f(x) => f(x)", 9),
        ("f(x) => x // x", 9),
        ("x = x", 5),
        ("if iffy\n    x = 1", 4),
    ] {
        let script = format!("//@version=6\nstrategy(\"scope\")\n{source}");
        let ast = parse(&script).expect(source);
        let expression = match &ast.statements[0] {
            Statement::Function { body, .. } => body,
            Statement::Assignment { expression, .. } => expression,
            Statement::If { condition, .. } => condition,
            other => panic!("{other:?}"),
        };
        assert_eq!(expression.range.start_line, 3);
        assert_eq!(expression.range.start_column, column, "{source}");
        if source == "f(x) => x" {
            assert_eq!(expression.kind, ExprKind::Identifier { name: "x".into() });
        }
    }
}

#[test]
fn udf_call_errors_keep_invocation_lines_and_prevent_indicator_planning() {
    for (script, code, message) in [
        (
            "f(x) => x\ny = f(close, open)",
            "PINE_UDF_SIGNATURE_UNSUPPORTED",
            "expects 1 arguments, got 2",
        ),
        (
            "f(x) => f(x)\ny = f(close)",
            "PINE_UDF_RECURSIVE_UNSUPPORTED",
            "recursive user-defined function",
        ),
        (
            "f(x) => g(x)\ng(x) => f(x)\ny = f(close)",
            "PINE_UDF_RECURSIVE_UNSUPPORTED",
            "recursive user-defined function",
        ),
        (
            "f(x) => x\ng(x) => f(x, x)\ny = g(close)",
            "PINE_UDF_SIGNATURE_UNSUPPORTED",
            "expects 1 arguments, got 2",
        ),
    ] {
        let line = script.lines().count() + 2;
        let result = compile(&format!("//@version=6\nstrategy(\"scope\")\n{script}"));
        assert!(!result.ok);
        assert!(result.program.is_none());
        assert!(result.requirements.indicators.is_empty());
        assert!(
            result
                .diagnostics
                .iter()
                .any(|d| d.code == code && d.line == line && d.message.contains(message)),
            "{:?}",
            result.diagnostics
        );
    }
}

#[test]
fn loop_bindings_are_readonly_until_their_own_scope_exits() {
    for body in [
        "for i = 0 to 1\n    i := 1",
        "for i = 0 to 1\n    if close > open\n        i = 1",
        "for i = 0 to 1\n    for i = 0 to 1\n        log.info(i)\n    i := 1",
        "for i = 0 to 1\n    [i, x] = ta.macd(close, 12, 26, 9)",
    ] {
        let result = compile(&format!("//@version=6\nstrategy(\"scope\")\n{body}"));
        assert!(!result.ok);
        assert!(
            result
                .diagnostics
                .iter()
                .any(|d| d.code == "PINE_LOOP_VARIABLE_READONLY"
                    && d.message.contains("loop variable \"i\" is read-only")),
            "{:?}",
            result.diagnostics
        );
        assert!(result.program.is_none());
    }
    let result = compile(
        "//@version=6\nstrategy(\"scope\")\nf(x) => x\ng(x) => f(x) + f(x)\ny = g(close)\nfor i = 0 to 1\n    for j = 0 to 1\n        log.info(j)\n    j = 3\ni = 4",
    );
    assert!(result.ok, "{:?}", result.diagnostics);
    assert!(result.program.is_some());
}
