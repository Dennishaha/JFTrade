package pine

import (
	"errors"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParserHelperContractsCoverEmptyHeadersAndLexicalEdges(t *testing.T) {
	state := newParseState("", []parsedLine{{number: 1, trimmed: "value = close", indent: 4}}, nil)
	headers, err := state.scanCompilationHeaders(&strategyir.Program{})
	if err != nil || headers.executableStart != 0 || headers.versionSeen || headers.strategySeen {
		t.Fatalf("indented header scan = %#v/%v", headers, err)
	}
	empty := newParseState("", nil, nil)
	headers, err = empty.scanCompilationHeaders(&strategyir.Program{})
	if err != nil || headers.executableStart != 0 {
		t.Fatalf("empty header scan = %#v/%v", headers, err)
	}

	result := buildCompilationResult(&strategyir.Program{}, newParseState("", nil, nil), nil)
	if len(result.Program.Hooks) != 1 || len(result.Program.Hooks[0].Statements) != 1 {
		t.Fatalf("empty compilation result = %#v", result)
	}
	if _, ok := result.Program.Hooks[0].Statements[0].(*strategyir.LogStmt); !ok {
		t.Fatalf("empty program fallback statement = %#v", result.Program.Hooks[0].Statements[0])
	}

	state.lines = []parsedLine{
		{number: 10, trimmed: "type PriceBox"},
		{number: 11, trimmed: "float price", indent: 4},
		{number: 12, trimmed: "value = close"},
	}
	if got := state.skipDeclarationBlock(0); got != 2 {
		t.Fatalf("skipDeclarationBlock = %d, want 2", got)
	}
	if got := state.skipDeclarationBlock(-1); got != 0 {
		t.Fatalf("skipDeclarationBlock invalid index = %d, want 0", got)
	}

	for _, tc := range []struct {
		line string
		want string
	}{
		{line: "value", want: ""},
		{line: "call(one, two)", want: "one, two"},
		{line: "call(one", want: "one"},
	} {
		if got := callArgs(tc.line); got != tc.want {
			t.Fatalf("callArgs(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
	if got := firstStringArgument("runtime.error()"); got != "" {
		t.Fatalf("firstStringArgument without argument = %q", got)
	}
	if got := unquote(`"broken`); got != `"broken` {
		t.Fatalf("unquote malformed literal = %q", got)
	}
	if callName("not_a_call") != "not_a_call" || callName("strategy.entry(\"Long\")") != "strategy.entry" {
		t.Fatalf("callName parsing changed")
	}
}

func TestValidationAndDynamicWhileHelpersKeepDiagnosticsActionable(t *testing.T) {
	for _, tc := range []struct {
		line string
		code string
	}{
		{line: `x = request.security(syminfo.tickerid, "15", [close])`, code: "PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED"},
		{line: `x = request.security(syminfo.tickerid, "15", [open, close])`, code: "PINE_REQUEST_SECURITY_TUPLE_ASSIGNMENT"},
		{line: `[open] = request.security(syminfo.tickerid, "15", [open, close])`, code: "PINE_REQUEST_SECURITY_TUPLE_MISMATCH"},
	} {
		if diagnostic, ok := requestSecurityUnsupportedDiagnostic(parsedLine{number: 20, trimmed: tc.line}); !ok || diagnostic.Code != tc.code {
			t.Fatalf("requestSecurityUnsupportedDiagnostic(%q) = %#v/%v, want %s", tc.line, diagnostic, ok, tc.code)
		}
	}
	for _, tc := range []struct {
		expression string
		want       bool
	}{
		{expression: "ta.obv", want: false},
		{expression: "ta.ema(close, 20", want: true},
		{expression: "ta.ema(close, 20) + ta.rsi(close, 14)", want: false},
	} {
		if got := requestSecurityExpressionHasUnsupportedTACall(tc.expression); got != tc.want {
			t.Fatalf("requestSecurityExpressionHasUnsupportedTACall(%q) = %v, want %v", tc.expression, got, tc.want)
		}
	}

	state := newParseState("", []parsedLine{
		{number: 30, trimmed: "while close > open", indent: 0},
		{number: 31, trimmed: "value = close", indent: 4},
	}, nil)
	statement, next, err := state.parseRuntimeWhileLoop(0)
	if err != nil || next != 2 {
		t.Fatalf("parseRuntimeWhileLoop next=%d err=%v", next, err)
	}
	loop, ok := statement.(*strategyir.LoopStmt)
	if !ok || loop.WhileCondition != "close > open" || len(loop.Body) != 1 || state.runtimeLoopDepth != 0 {
		t.Fatalf("while loop = %#v, state depth=%d", statement, state.runtimeLoopDepth)
	}
	state.runtimeLoopDepth = maxRuntimeLoopDepth
	if _, _, err := state.parseRuntimeWhileLoop(0); err == nil || !strings.Contains(err.Error(), "dynamic loop nesting") {
		t.Fatalf("while loop depth guard = %v", err)
	}
}

func TestRuntimeLoopAndTupleParserErrorContracts(t *testing.T) {
	loopMatch, ok := parseStaticForLoopHeader("for i = start to finish by step")
	if !ok {
		t.Fatal("runtime loop header was not parsed")
	}
	newLoopState := func(header, body string) *parseState {
		return newParseState("", []parsedLine{
			{number: 40, trimmed: header},
			{number: 41, trimmed: body, indent: 4},
		}, nil)
	}
	normalizationFailure := newLoopState("for i = start to finish by step", "value = i")
	normalizationFailure.normalizationErr = errors.New("loop normalization failure")
	if _, _, err := normalizationFailure.parseRuntimeForLoop(0, loopMatch); err == nil || !strings.Contains(err.Error(), "loop normalization failure") {
		t.Fatalf("runtime for normalization error = %v", err)
	}
	invalidExpression := newLoopState("for i = close > to finish by step", "value = i")
	match, ok := parseStaticForLoopHeader(invalidExpression.lines[0].trimmed)
	if !ok {
		t.Fatal("invalid-expression loop header was not parsed")
	}
	if _, _, err := invalidExpression.parseRuntimeForLoop(0, match); err == nil || !strings.Contains(err.Error(), "for start") {
		t.Fatalf("runtime for invalid start = %v", err)
	}
	bodyFailure := newLoopState("for i = start to finish by step", "broker.submit()")
	if _, _, err := bodyFailure.parseRuntimeForLoop(0, loopMatch); err == nil || !strings.Contains(err.Error(), "unsupported executable statement") {
		t.Fatalf("runtime for body error = %v", err)
	}

	whileNormalization := newParseState("", []parsedLine{{number: 50, trimmed: "while close > open"}}, nil)
	whileNormalization.normalizationErr = errors.New("while normalization failure")
	if _, _, err := whileNormalization.parseRuntimeWhileLoop(0); err == nil || !strings.Contains(err.Error(), "while normalization failure") {
		t.Fatalf("while normalization error = %v", err)
	}
	invalidWhile := newParseState("", []parsedLine{{number: 51, trimmed: "while close >"}}, nil)
	if _, _, err := invalidWhile.parseRuntimeWhileLoop(0); err == nil || !strings.Contains(err.Error(), "while condition") {
		t.Fatalf("invalid while condition = %v", err)
	}

	state := newObjectCollectionBoundaryParseState()
	state.expressionAliases = map[string]string{}
	line := parsedLine{number: 60, trimmed: "[first, second, third] = value"}
	if _, handled, err := state.parseRequestSecurityTupleAssignment(line, []string{"first", "second"}, "ta.ema(close, 20)"); handled || err != nil {
		t.Fatalf("non-request tuple handled=%v err=%v", handled, err)
	}
	if _, handled, err := state.parseRequestSecurityTupleAssignment(line, []string{"first"}, `request.security(syminfo.tickerid, "15", [open, close])`); !handled || err == nil || !strings.Contains(err.Error(), "returns 2 values") {
		t.Fatalf("request tuple width error handled=%v err=%v", handled, err)
	}
	for _, tc := range []struct {
		name  string
		parse func() (strategyir.Statement, bool, error)
	}{
		{
			name: "Bollinger MTF tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseBollingerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"20", "2", `"15m"`, "close"}, `bollinger(20, 2, "15m", close)`)
			},
		},
		{
			name: "Supertrend MTF tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseSupertrendTupleAssignment(line, []string{"trend", "direction"}, []string{"3", "10", `"15m"`}, `supertrend(3, 10, "15m")`)
			},
		},
		{
			name: "Keltner MTF tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseKeltnerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"close", "20", "2", "true", `"15m"`}, `kc(close, 20, 2, true, "15m")`)
			},
		},
		{
			name: "MACD MTF tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseMACDTupleAssignment(line, []string{"macd", "signal", "histogram"}, []string{"12", "26", "9", `"15m"`, "close"}, `macd(12, 26, 9, "15m", close)`)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statement, handled, err := tc.parse()
			if err != nil || !handled || statement == nil {
				t.Fatalf("tuple helper handled=%v statement=%#v err=%v", handled, statement, err)
			}
		})
	}
}

func TestCollectionParserHelperErrorsPreserveExecutableBoundaries(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	line := parsedLine{number: 70, trimmed: "value = array.get()"}
	if _, _, err := state.parseCollectionConstructorAssignment(line, "values", "array", "new_float", "", []string{"close >"}, strategyir.AssignmentModeLet); err == nil || !strings.Contains(err.Error(), "invalid collection argument") {
		t.Fatalf("collection constructor invalid argument = %v", err)
	}
	if _, _, err := state.parseCollectionOperationAssignment(line, "value", "array.get()", "array", "get", "", nil, strategyir.AssignmentModeLet); err == nil || !strings.Contains(err.Error(), "requires a collection argument") {
		t.Fatalf("collection operation missing receiver = %v", err)
	}
	if _, _, err := state.parseCollectionOperationAssignment(line, "value", "array.get(values, close >)", "array", "get", "", []string{"values", "close >"}, strategyir.AssignmentModeLet); err == nil || !strings.Contains(err.Error(), "invalid collection argument") {
		t.Fatalf("collection operation invalid argument = %v", err)
	}
	if _, _, err := state.parseStandaloneCollectionStatement(parsedLine{number: 71, trimmed: "array.push()"}); err == nil || !strings.Contains(err.Error(), "requires a collection argument") {
		t.Fatalf("standalone mutation missing receiver = %v", err)
	}

	for _, call := range []string{"box", "box.unknown"} {
		if _, _, _, ok := state.resolveExecutableCollectionCall(call); ok {
			t.Fatalf("non-executable collection call %q was resolved", call)
		}
	}
	fieldResolution := newObjectCollectionBoundaryParseState()
	fieldResolution.collectionNamespaces = map[string]string{}
	if got := fieldResolution.collectionNamespaceForTargetExpression("box.values"); got != "array" {
		t.Fatalf("object field collection namespace = %q", got)
	}
	if target, args, err := state.collectionTargetAndArguments("array", "array.get", []string{"values", "0"}); err != nil || target != "values" || len(args) != 1 || args[0] != "0" {
		t.Fatalf("namespace collection receiver = %q/%#v/%v", target, args, err)
	}
	if target, args, err := state.collectionTargetAndArguments("array", "unknown.get", []string{"0"}); err != nil || target != "unknown" || len(args) != 1 {
		t.Fatalf("method collection receiver fallback = %q/%#v/%v", target, args, err)
	}
}
