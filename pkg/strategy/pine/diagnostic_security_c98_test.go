package pine

import (
	"errors"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCoverage98CompilerDiagnosticsPreserveActionablePlannerAndRemoteErrors(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("planner rejection")
slow = ta.sma(close, 0)`, AnalysisOptions{IncludeSemantic: true})
	if analysis.OK {
		t.Fatal("zero-length moving average was accepted by the planner")
	}
	matchedPlannerError := false
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Severity == DiagnosticSeverityError && strings.Contains(diagnostic.Message, "period must be a positive integer") {
			matchedPlannerError = true
		}
	}
	if !matchedPlannerError {
		t.Fatalf("planner failure was not projected as a diagnostic: %#v", analysis.Diagnostics)
	}

	broker := diagnosticFromError(errors.New("pine line 9: broker emulator partial fill needs intrabar behavior"))
	if broker.Code != "PINE_BROKER_EMULATOR_OUT_OF_SCOPE" || broker.Line != 9 || !strings.Contains(broker.Message, "partial fill") {
		t.Fatalf("broker emulator diagnostic = %#v", broker)
	}
	if malformed := diagnosticFromWarning("upstream warning pine line 9"); malformed.Line != 1 || malformed.Message != "upstream warning pine line 9" {
		t.Fatalf("malformed warning line was rewritten: %#v", malformed)
	}
	if malformed := diagnosticFromError(errors.New("pine line 0: invalid remote compiler location")); malformed.Line != 1 || !strings.Contains(malformed.Message, "pine line 0") {
		t.Fatalf("invalid remote error line was accepted: %#v", malformed)
	}

	// Semantic projection is invoked independently by editor and analysis paths;
	// an incomplete compilation must remain safe to display.
	markExecutableSemanticSurface(nil, &strategyir.Program{})
}

func TestCoverage98RequestSecurityPurityCoversOptionalAndBuiltinRecovery(t *testing.T) {
	if _, ok := lowerRequestSecurityTACall("stoch", []string{"close", "high", "low"}, "minute"); ok {
		t.Fatal("stochastic request-security lowering accepted an incomplete argument list")
	}
	if _, ok := lowerPureRequestSecurityExpression(`"ta.unresolved"`, "minute"); ok {
		t.Fatal("unresolved request-security token was accepted after lowering")
	}

	for _, expression := range []string{
		"len(close)",
		"min(unknown_runtime_call(1), close)",
		"collection_array_get(unknown_runtime_call(1))",
		"[close, high]",
	} {
		if requestSecurityLoweredASTIsPure(expression) {
			t.Fatalf("unsafe lowered expression was accepted: %q", expression)
		}
	}
	if !requestSecurityLoweredASTIsPure("market?.snapshot.close") {
		t.Fatal("optional read-only member chain was rejected")
	}
	if !requestSecurityLoweredASTIsPure("min(close, open)") {
		t.Fatal("allowed builtin call with pure inputs was rejected")
	}
}

func TestCoverage98PineEditorRecoveryAndCapabilityEvidenceContracts(t *testing.T) {
	if got := normalizeTernaryExpression("close ? : open"); got != "close ? : open" {
		t.Fatalf("incomplete ternary was rewritten to %q", got)
	}
	if got := rewriteOutsideStringLiterals(`close[1] + "unfinished`, func(segment string) string {
		return normalizeHistoryReferences(segment)
	}); got != `history(close, 1) + "unfinished` {
		t.Fatalf("unfinished editor string recovery = %q", got)
	}

	ast, _ := parseAST([]parsedLine{
		{number: 1, indent: 2, trimmed: ""},
		{number: 2, trimmed: "[first, second] := ta.macd(close, 12, 26, 9)"},
	})
	if len(ast.Lines) != 2 || ast.Lines[0].EndColumn <= ast.Lines[0].Column || ast.Lines[1].Mode != strategyir.AssignmentModeReassign {
		t.Fatalf("editor AST recovery = %#v", ast.Lines)
	}
	lineDiagnostic := diagnosticForLine(DiagnosticSeverityError, "PINE_EDITOR", "incomplete input", parsedLine{number: 3, indent: 4})
	if lineDiagnostic.EndColumn <= lineDiagnostic.Column {
		t.Fatalf("empty editor diagnostic lost its visible range: %#v", lineDiagnostic)
	}

	if capabilityStatusValue(CapabilityAnalyzed) != 0.5 {
		t.Fatal("analyzed capability must contribute partial compatibility credit")
	}
	if layers := capabilityLayers(capabilityDefinition{status: CapabilityAnalyzed}); !layers.Spec || !layers.Parser || layers.Planner || layers.Runtime {
		t.Fatalf("analyzed capability layers = %#v", layers)
	}
	if ids := capabilityTestIDsRaw("syntax.arrays.unavailable"); ids != nil {
		t.Fatalf("unsupported capability advertised golden-test evidence: %#v", ids)
	}
	if ids := existingCapabilityTestIDs(nil); ids != nil {
		t.Fatalf("empty evidence set = %#v, want nil", ids)
	}
}
