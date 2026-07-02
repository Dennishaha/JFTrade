package pine

import (
	"strings"
	"testing"
)

func TestRequestSecurityValidationExplainsMalformedAndUnsafeExpressions(t *testing.T) {
	cases := []struct {
		name string
		line string
		code string
	}{
		{name: "unclosed call", line: `x = request.security(syminfo.tickerid, "D", close`, code: "PINE_REQUEST_SECURITY_UNSUPPORTED"},
		{name: "missing expression", line: `x = request.security(syminfo.tickerid, "D")`, code: "PINE_REQUEST_SECURITY_UNSUPPORTED"},
		{name: "reassignment side effect", line: `x = request.security(syminfo.tickerid, "D", total := total + close)`, code: "PINE_REQUEST_SECURITY_SIDE_EFFECT"},
		{name: "collection mutation", line: `x = request.security(syminfo.tickerid, "D", values.push(close))`, code: "PINE_REQUEST_SECURITY_SIDE_EFFECT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic, ok := requestSecurityUnsupportedDiagnostic(parsedLine{number: 7, trimmed: tc.line})
			if !ok || diagnostic.Code != tc.code || diagnostic.Line != 7 {
				t.Fatalf("diagnostic = %#v/%v, want %s on line 7", diagnostic, ok, tc.code)
			}
		})
	}
}

func TestRequestSecurityExpressionTASubsetValidation(t *testing.T) {
	if requestSecurityExpressionHasUnsupportedTACall("close + open") {
		t.Fatal("plain price expression reported as unsupported TA call")
	}
	if requestSecurityExpressionHasUnsupportedTACall("ta.sma(close, 20) + ta.ema(close, 10)") {
		t.Fatal("supported moving averages reported as unsupported")
	}
	for _, expression := range []string{"ta.not_supported(close)", "ta.sma(close, 20"} {
		if !requestSecurityExpressionHasUnsupportedTACall(expression) {
			t.Fatalf("expression %q accepted, want unsupported", expression)
		}
	}
}

func TestRequestSecurityTupleAliasExtractionPreservesAssignmentContract(t *testing.T) {
	aliases, ok := requestSecurityTupleAliasesFromLine(`[dailyOpen, dailyClose] = request.security(syminfo.tickerid, "D", [open, close])`)
	if !ok || len(aliases) != 2 || aliases[0] != "dailyOpen" || aliases[1] != "dailyClose" {
		t.Fatalf("general tuple aliases = %#v/%v", aliases, ok)
	}
	aliases, ok = requestSecurityTupleAliasesFromLine(`[float fast, float slow] = request.security(syminfo.tickerid, "D", [ta.sma(close, 10), ta.sma(close, 20)])`)
	if !ok || len(aliases) != 2 {
		t.Fatalf("typed tuple aliases = %#v/%v", aliases, ok)
	}
	if aliases, ok := requestSecurityTupleAliasesFromLine(`value = close`); ok || aliases != nil {
		t.Fatalf("non-tuple aliases = %#v/%v, want nil/false", aliases, ok)
	}
}

func TestRejectUnsupportedReturnsRuntimeAndCollectionBusinessErrors(t *testing.T) {
	state := &parseState{}
	if err := state.rejectUnsupported(parsedLine{number: 4, trimmed: `runtime.error("risk limit exceeded")`}); err == nil || !strings.Contains(err.Error(), "risk limit exceeded") {
		t.Fatalf("runtime.error validation = %v", err)
	}
	if err := state.rejectUnsupported(parsedLine{number: 5, trimmed: `array.unsupported(values)`}); err == nil || !strings.Contains(err.Error(), "collection") {
		t.Fatalf("unsupported collection validation = %v", err)
	}
	if err := state.rejectUnsupported(parsedLine{number: 6, trimmed: `closeValue = close`}); err != nil {
		t.Fatalf("ordinary assignment rejected: %v", err)
	}
}

func TestStrategyDeclarationInvalidConstantsFallBackWithWarnings(t *testing.T) {
	metadata, warnings := parseStrategyDeclaration(`strategy(title="Risk managed", default_qty_type=strategy.unknown, default_qty_value=25, pyramiding=-1, initial_capital=0, commission_type=strategy.commission.unknown, commission_value=-1, slippage=-2, process_orders_on_close=maybe)`)
	if metadata.Name != "Risk managed" || metadata.DefaultQtyMode != "fixed" || metadata.DefaultQtyValue != "25" || metadata.Pyramiding != 1 {
		t.Fatalf("fallback metadata = %#v", metadata)
	}
	joined := strings.Join(warnings, "\n")
	for _, expected := range []string{
		"default_qty_type", "pyramiding", "initial_capital", "commission_type",
		"commission_value", "slippage", "process_orders_on_close",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("warnings = %#v, missing %q", warnings, expected)
		}
	}
}

func TestExpressionAndHistoryValidationRejectsInvalidBoundaries(t *testing.T) {
	if err := validateExpression(9, "entry condition", "close >"); err == nil || !strings.Contains(err.Error(), "pine line 9") {
		t.Fatalf("invalid expression error = %v", err)
	}
	if err := validateExpression(10, "entry condition", "close > open"); err != nil {
		t.Fatalf("valid expression error = %v", err)
	}
	if message := historyDiagnosticMessage("close[999999999999999999999999999999]"); !strings.Contains(message, "non-negative") {
		t.Fatalf("overflowing history message = %q", message)
	}
}

func TestPublicHelperGuardReturnsActionableMigrationErrors(t *testing.T) {
	if err := rejectPublicDisabledHelperCalls(3, `ta.ema(close, 20)`); err != nil {
		t.Fatalf("native Pine helper rejected: %v", err)
	}
	if call, ok := findPublicDisabledHelperCall(`log.info("history(close, 1) with \"quoted\" text")`); ok {
		t.Fatalf("helper name inside escaped string literal was treated as a call: %#v", call)
	}
	if err := rejectPublicDisabledHelperCalls(4, `value = history(close, 1)`); err == nil ||
		!strings.Contains(err.Error(), "history() is an internal JFTrade helper") ||
		!strings.Contains(err.Error(), "series[n]") {
		t.Fatalf("internal helper migration error = %v", err)
	}
	if err := rejectPublicDisabledHelperCalls(5, `value = ta.adx(14)`); err == nil ||
		!strings.Contains(err.Error(), "ta.adx() is a JFTrade-only shortcut") ||
		!strings.Contains(err.Error(), "ta.dmi") {
		t.Fatalf("TA shortcut migration error = %v", err)
	}
}
