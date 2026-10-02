use jftrade_strategy::pine::compile;

const DYNAMIC_SYMBOL_SCRIPT: &str = r#"//@version=6
strategy("Unsupported", overlay=true)
x = request.security("NASDAQ:AAPL", "D", close)"#;

/// Parity: go:452dea11:pkg/strategy/pine/parse_test.go:1014 TestAnalyzeScriptReturnsStructuredUnsupportedDiagnostics
#[test]
fn unsupported_security_symbols_report_the_original_line() {
    let compilation = compile(DYNAMIC_SYMBOL_SCRIPT);
    assert!(!compilation.ok, "dynamic symbols must fail the analysis");
    let first = compilation
        .diagnostics
        .first()
        .expect("a structured diagnostic");
    assert_eq!(first.code, "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL");
    assert!(
        first.message.contains("request.security"),
        "message = {}",
        first.message
    );
    // The call sits on the third script line, so the diagnostic keeps the
    // original line instead of the normalized offset.
    assert_eq!(first.line, 3);
}

/// Parity: go:452dea11:pkg/strategy/pine/parse_test.go:1029 TestAnalyzeScriptPreservesOriginalLineNumbers
#[test]
fn blank_lines_before_the_script_keep_later_diagnostic_lines() {
    // Go keeps the original line numbers, so a script that starts with blank
    // lines must still report the loop on the line the author wrote.
    for (leading_blank_lines, expected_line) in [(0usize, 3usize), (1, 4), (2, 5)] {
        let script = format!(
            "{}//@version=6\nstrategy(\"Loop\", overlay=true)\nfor i = 0 to 3 by 0\n    log.info(\"nope\")",
            "\n".repeat(leading_blank_lines)
        );
        let compilation = compile(&script);
        assert!(!compilation.ok, "a zero step must fail");
        let first = compilation
            .diagnostics
            .first()
            .expect("a structured diagnostic");
        assert_eq!(first.code, "PINE_LOOP_LIMIT_UNSUPPORTED");
        assert_eq!(
            first.line, expected_line,
            "leading blank lines = {leading_blank_lines}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pine/parser_and_lowering_recovery_test.go:119 TestNestedLoopStateAndImportRecoveryRemainStable
#[test]
fn unsupported_while_and_import_statements_fail_with_stable_diagnostics() {
    let while_script = r#"//@version=6
strategy("While", overlay=true)
while close > open
    log.info("unsupported")"#;
    let while_compilation = compile(while_script);
    assert!(
        !while_compilation.ok,
        "while loops are outside the executable subset"
    );
    let while_diagnostic = while_compilation
        .diagnostics
        .first()
        .expect("while diagnostic");
    assert_eq!(while_diagnostic.code, "PINE_INDENT_UNEXPECTED");
    assert_eq!(while_diagnostic.line, 4);

    let import_script = r#"//@version=6
strategy("Import", overlay=true)
import TradingView/ta/7"#;
    let import_compilation = compile(import_script);
    assert!(
        !import_compilation.ok,
        "imports are not executable worker statements"
    );
    let import_diagnostic = import_compilation
        .diagnostics
        .first()
        .expect("import diagnostic");
    assert_eq!(import_diagnostic.code, "PINE_DECLARATION_UNSUPPORTED");
    assert_eq!(import_diagnostic.line, 3);
}
