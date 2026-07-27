package pine

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestRequestSecuritySupportedTAFamiliesKeepTheirExecutionContracts(t *testing.T) {
	timeUnit := `"15m"`
	cases := []struct {
		name string
		call string
		args []string
		want string
	}{
		{name: "default SMA window", call: "sma", want: `ma(SMA, 20, "15m")`},
		{name: "source aware WMA", call: "wma", args: []string{"hl2", "9"}, want: `ma(LWMA, 9, "15m", hl2)`},
		{name: "default source RSI", call: "rsi", args: []string{"7"}, want: `rsi(close, 7, "15m")`},
		{name: "source aware RSI", call: "rsi", args: []string{"hlc3", "14"}, want: `rsi(hlc3, 14, "15m")`},
		{name: "MACD", call: "macd", args: []string{"close", "12", "26", "9"}, want: `macd(12, 26, 9, "15m", close)`},
		{name: "ATR period", call: "atr", args: []string{"7"}, want: `atr(7, "15m")`},
		{name: "Bollinger source", call: "bb", args: []string{"hl2", "20", "2"}, want: `bollinger(20, 2, "15m", hl2)`},
		{name: "Supertrend", call: "supertrend", args: []string{"3", "10"}, want: `supertrend(3, 10, "15m")`},
		{name: "Linear regression", call: "linreg", args: []string{"close", "20", "0"}, want: `linreg(close, 20, 0, "15m")`},
		{name: "OBV property", call: "obv", want: `obv(close, "15m")`},
		{name: "ALMA", call: "alma", args: []string{"hl2", "9", "0.85", "6"}, want: `alma(hl2, 9, 0.85, 6, "15m")`},
		{name: "Bollinger bandwidth", call: "bbw", args: []string{"close", "20", "2"}, want: `bbw(close, 20, 2, "15m")`},
		{name: "Center of gravity", call: "cog", args: []string{"close", "10"}, want: `cog(close, 10, "15m")`},
		{name: "Chande momentum", call: "cmo", args: []string{"hlc3", "14"}, want: `cmo(hlc3, 14, "15m")`},
		{name: "Deviation", call: "dev", args: []string{"close", "20"}, want: `dev(close, 20, "15m")`},
		{name: "Median", call: "median", args: []string{"close", "20"}, want: `median(close, 20, "15m")`},
		{name: "Linear percentile", call: "percentile_linear_interpolation", args: []string{"close", "20", "80"}, want: `percentile_linear_interpolation(close, 20, 80, "15m")`},
		{name: "Percent rank", call: "percentrank", args: []string{"close", "20"}, want: `percentrank(close, 20, "15m")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lowerRequestSecurityTACall(tc.call, tc.args, timeUnit)
			if !ok || got != tc.want {
				t.Fatalf("lowerRequestSecurityTACall(%q, %#v) = %q/%v, want %q/true", tc.call, tc.args, got, ok, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		call string
		args []string
	}{
		{call: "macd", args: []string{"close", "12", "26"}},
		{call: "bb", args: []string{"strategy.position_size", "20", "2"}},
		{call: "supertrend", args: []string{"3"}},
		{call: "atr", args: []string{"7", "extra"}},
		{call: "stoch", args: []string{"close", "low", "high", "14"}},
		{call: "not_a_pine_indicator", args: []string{"close"}},
	} {
		if got, ok := lowerRequestSecurityTACall(tc.call, tc.args, timeUnit); ok || got != "" {
			t.Fatalf("unsupported request.security TA call %s(%#v) = %q/%v", tc.call, tc.args, got, ok)
		}
	}

	for _, tc := range []struct {
		args   []string
		source string
		period string
	}{
		{source: "close", period: "20"},
		{args: []string{"7"}, source: "close", period: "7"},
		{args: []string{"hl2", "9", "ignored"}, source: "hl2", period: "9"},
	} {
		source, period := pineSourceLengthArgs(tc.args, "close", "20")
		if source != tc.source || period != tc.period {
			t.Fatalf("pineSourceLengthArgs(%#v) = %q/%q, want %q/%q", tc.args, source, period, tc.source, tc.period)
		}
	}

	for _, tc := range []struct {
		value string
		want  string
		ok    bool
	}{
		{value: "1", want: "minute", ok: true},
		{value: "60", want: "hour", ok: true},
		{value: "D", want: "day", ok: true},
		{value: "w", want: "week", ok: true},
		{value: "M", want: "month", ok: true},
		{value: "15", want: `"15m"`, ok: true},
		{value: "2", ok: false},
	} {
		got, ok := pineTimeframeUnit(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("pineTimeframeUnit(%q) = %q/%v, want %q/%v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCollectionExecutionParserKeepsReceiverAndErrorBoundaries(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	line := func(number int, text string) parsedLine {
		return parsedLine{number: number, trimmed: text}
	}

	for _, tc := range []struct {
		name    string
		text    string
		message string
	}{
		{name: "typed declaration requires constructor", text: "var array<float> values = close", message: "executable collection constructor"},
		{name: "typed declaration namespace must match", text: "array<float> values = map.new<string, float>()", message: "array declaration cannot be initialized"},
		{name: "standalone read is rejected", text: "values.get(0)", message: "must be assigned or used"},
		{name: "invalid collection argument is rejected", text: "values.push(close >)", message: "invalid collection argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, handled, err := state.parseCollectionStatement(line(10, tc.text))
			if !handled || err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("parseCollectionStatement(%q) handled=%v err=%v, want %q", tc.text, handled, err, tc.message)
			}
		})
	}

	statement, handled, err := state.parseCollectionStatement(line(20, "values = array.from(close, open, high)"))
	if err != nil || !handled {
		t.Fatalf("array.from constructor handled=%v err=%v", handled, err)
	}
	collection, ok := statement.(*strategyir.CollectionStmt)
	if !ok || collection.Namespace != "array" || collection.Operation != "from" || collection.ResultName != "values" || !reflect.DeepEqual(collection.Arguments, []string{"close", "open", "high"}) {
		t.Fatalf("array.from statement = %#v", statement)
	}
	if state.collectionNamespaces["values"] != "array" {
		t.Fatalf("array.from did not retain its namespace: %#v", state.collectionNamespaces)
	}

	if got, err := state.lowerCollectionReadCalls("array.get()"); err == nil || got != "array.get()" || !strings.Contains(err.Error(), "not valid in an expression") {
		t.Fatalf("targetless collection read = %q/%v", got, err)
	}
	if got, err := state.lowerCollectionReadCalls("values.sort()"); err != nil || got != "values.sort()" {
		t.Fatalf("mutating collection call should remain a statement: %q/%v", got, err)
	}
	if got, err := state.lowerCollectionReadCalls("array.get(values, 0) + values.size()"); err != nil || got != "collection_array_get(values, 0) + collection_array_size(values)" {
		t.Fatalf("collection read lowering = %q/%v", got, err)
	}

	for _, tc := range []struct {
		expression string
		found      bool
	}{
		{expression: `"values[1].get(0)"`, found: false},
		{expression: "values[2].get(1)", found: true},
		{expression: "values[-1].get(0)", found: false},
		{expression: "values[1].get(", found: false},
	} {
		_, found := findCollectionHistoryCall(tc.expression)
		if found != tc.found {
			t.Fatalf("findCollectionHistoryCall(%q) = %v, want %v", tc.expression, found, tc.found)
		}
	}

	for _, tc := range []struct {
		call      string
		namespace string
		op        string
		typeArgs  string
		ok        bool
	}{
		{call: "array.new_int", namespace: "array", op: "new_int", ok: true},
		{call: "map.new<string, float>", namespace: "map", op: "new", typeArgs: "string, float", ok: true},
		{call: "values.get", ok: false},
		{call: "matrix.unknown", ok: false},
	} {
		namespace, operation, typeArgs, ok := parseExecutableCollectionCall(tc.call)
		if namespace != tc.namespace || operation != tc.op || typeArgs != tc.typeArgs || ok != tc.ok {
			t.Fatalf("parseExecutableCollectionCall(%q) = %q/%q/%q/%v", tc.call, namespace, operation, typeArgs, ok)
		}
	}
}

func TestTupleLoweringHelpersMaintainAliasAndArityContracts(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	state.expressionAliases = map[string]string{}
	line := func(number int) parsedLine {
		return parsedLine{number: number, trimmed: "[first, second, third] = value"}
	}

	type tupleCase struct {
		name  string
		parse func() (strategyir.Statement, bool, error)
		alias string
		want  string
	}
	cases := []tupleCase{
		{
			name: "Bollinger tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseBollingerTupleAssignment(line(30), []string{"basis", "upper", "lower"}, []string{"close", "20", "2"}, "ta.bb(close, 20, 2)")
			},
			alias: "upper", want: "basis.upper",
		},
		{
			name: "DMI tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseDMITupleAssignment(line(31), []string{"plus", "minus", "adx"}, []string{"14", "14"}, "ta.dmi(14, 14)")
			},
			alias: "minus", want: "plus.minus",
		},
		{
			name: "Supertrend tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseSupertrendTupleAssignment(line(32), []string{"trend", "direction"}, []string{"3", "10"}, "ta.supertrend(3, 10)")
			},
			alias: "direction", want: "trend.direction",
		},
		{
			name: "Keltner tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseKeltnerTupleAssignment(line(33), []string{"basis", "upper", "lower"}, []string{"close", "20", "2", "false"}, "ta.kc(close, 20, 2, false)")
			},
			alias: "lower", want: "basis.lower",
		},
		{
			name: "MACD tuple",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseMACDTupleAssignment(line(34), []string{"macd", "signal", "histogram"}, []string{"close", "12", "26", "9"}, "ta.macd(close, 12, 26, 9)")
			},
			alias: "histogram", want: "macd.histogram",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statement, handled, err := tc.parse()
			if err != nil || !handled {
				t.Fatalf("tuple helper handled=%v err=%v", handled, err)
			}
			let, ok := statement.(*strategyir.LetStmt)
			if !ok || let.Name == "" || let.Expression == "" {
				t.Fatalf("tuple helper statement = %#v", statement)
			}
			if got := state.expressionAliases[tc.alias]; got != tc.want {
				t.Fatalf("tuple alias %q = %q, want %q", tc.alias, got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name  string
		parse func() (strategyir.Statement, bool, error)
	}{
		{
			name: "Bollinger MTF arity",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseBollingerTupleAssignment(line(40), []string{"basis", "upper", "lower"}, []string{"20", "2", `"15m"`}, "bollinger(20, 2, \"15m\")")
			},
		},
		{
			name: "Supertrend MTF arity",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseSupertrendTupleAssignment(line(41), []string{"trend", "direction"}, []string{"3", "10"}, "supertrend(3, 10)")
			},
		},
		{
			name: "Keltner MTF arity",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseKeltnerTupleAssignment(line(42), []string{"basis", "upper", "lower"}, []string{"close", "20", "2", "true"}, "kc(close, 20, 2, true)")
			},
		},
		{
			name: "MACD MTF arity",
			parse: func() (strategyir.Statement, bool, error) {
				return state.parseMACDTupleAssignment(line(43), []string{"macd", "signal", "histogram"}, []string{"12", "26", "9", `"15m"`}, "macd(12, 26, 9, \"15m\")")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, handled, err := tc.parse()
			if !handled || err == nil || !strings.Contains(err.Error(), "requires") {
				t.Fatalf("invalid MTF tuple handled=%v err=%v", handled, err)
			}
		})
	}

	if got, values := tupleAssignmentExpressionParts(`request.security(syminfo.tickerid, "15", [open, close])`); got != `request.security(syminfo.tickerid, "15", [open, close])` || !reflect.DeepEqual(values, []string{"syminfo.tickerid", `"15"`, "[open, close]"}) {
		t.Fatalf("tuple assignment expression parts = %q/%#v", got, values)
	}
}

func TestCompilerHeadersAndLexicalHelpersRetainPineV6Rules(t *testing.T) {
	validLines := tokenizeScript("//@version=6\r\nstrategy(\"Header\", overlay=true)\r\nplot(close) // ignored visual\r\nvalue = close // keep assignment")
	state := newParseState("", validLines, nil)
	program := &strategyir.Program{}
	headers, err := state.scanCompilationHeaders(program)
	if err != nil || !headers.versionSeen || !headers.strategySeen || headers.executableStart != 3 {
		t.Fatalf("scanCompilationHeaders = %#v/%v", headers, err)
	}
	if err := validateCompilationHeaders(headers); err != nil {
		t.Fatalf("valid headers error = %v", err)
	}
	if len(state.warnings) != 1 || !strings.Contains(state.warnings[0], "visual-only") {
		t.Fatalf("visual header warning = %#v", state.warnings)
	}

	for _, tc := range []struct {
		state   compilationHeaderState
		message string
	}{
		{state: compilationHeaderState{}, message: "//@version=6"},
		{state: compilationHeaderState{versionSeen: true}, message: "strategy"},
	} {
		if err := validateCompilationHeaders(tc.state); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("validateCompilationHeaders(%#v) = %v, want %q", tc.state, err, tc.message)
		}
	}

	for _, script := range []string{
		"//@version=5\nstrategy(\"Wrong version\")",
		"//@version=6\nindicator(\"Indicator\")",
		"//@version=6\nlibrary(\"Library\")",
	} {
		if _, err := Compile(script); err == nil {
			t.Fatalf("Compile(%q) succeeded for a non-executable header", script)
		}
	}

	var nilState *parseState
	if got := nilState.cachedRegexp("identifier", "[a-z]+").FindString("Pine"); got != "ine" {
		t.Fatalf("nil parseState regexp = %q", got)
	}
	state = &parseState{}
	first := state.cachedRegexp("identifier", "[a-z]+")
	second := state.cachedRegexp("identifier", "[A-Z]+")
	if first != second || second.FindString("Pine") != "ine" {
		t.Fatalf("cached regexp was rebuilt or replaced: %p/%p", first, second)
	}

	if name, args, ok := parseTACall("ta.ema(close, 20)"); !ok || name != "ema" || !reflect.DeepEqual(args, []string{"close", "20"}) {
		t.Fatalf("parseTACall valid = %q/%#v/%v", name, args, ok)
	}
	for _, expression := range []string{"ema(close, 20)", "ta.ema(close, 20", "ta.ema(close, 20) + close"} {
		if _, _, ok := parseTACall(expression); ok {
			t.Fatalf("parseTACall accepted %q", expression)
		}
	}

	if got := replaceColorFunctions("color.new(color.red, 50) + color.rgb(-2, 260, 16)"); got != `color.red + "#00ff10"` {
		t.Fatalf("color lowering = %q", got)
	}
	for _, tc := range []struct {
		input string
		value int
		ok    bool
	}{
		{input: "128", value: 128, ok: true},
		{input: "-1", value: 0, ok: true},
		{input: "256", value: 255, ok: true},
		{input: "bad", ok: false},
	} {
		value, ok := parsePineColorComponent(tc.input)
		if value != tc.value || ok != tc.ok {
			t.Fatalf("parsePineColorComponent(%q) = %d/%v, want %d/%v", tc.input, value, ok, tc.value, tc.ok)
		}
	}
}

func TestObjectExecutionParserPreservesDeclaredTypeContracts(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	state.expressionAliases = map[string]string{}
	state.objectPersistent = map[string]bool{}
	line := func(number int, text string) parsedLine { return parsedLine{number: number, trimmed: text} }

	statement, handled, err := state.parseObjectFieldAssignment(line(50, "box.price := close"))
	if err != nil || !handled {
		t.Fatalf("object field update handled=%v err=%v", handled, err)
	}
	fieldUpdate, ok := statement.(*strategyir.ObjectStmt)
	if !ok || fieldUpdate.Operation != "field_set" || fieldUpdate.TypeName != "PriceBox" || fieldUpdate.Method != "price" || fieldUpdate.Target != "box" || !reflect.DeepEqual(fieldUpdate.Arguments, []string{"close"}) {
		t.Fatalf("object field update = %#v", statement)
	}

	for _, tc := range []struct {
		text    string
		handled bool
		message string
	}{
		{text: "box.price = close", handled: true, message: "must use :="},
		{text: "missing.price := close", handled: false},
		{text: "box.unknown := close", handled: true, message: "has no field"},
		{text: "box.price := close >", handled: true, message: "invalid object field assignment"},
	} {
		_, handled, err := state.parseObjectFieldAssignment(line(51, tc.text))
		if handled != tc.handled || (tc.message == "" && err != nil) || (tc.message != "" && (err == nil || !strings.Contains(err.Error(), tc.message))) {
			t.Fatalf("parseObjectFieldAssignment(%q) handled=%v err=%v", tc.text, handled, err)
		}
	}

	definition := state.udtTypes["pricebox"]
	statement, handled, err = state.parseObjectConstructorAssignment(line(60, "var replacement = PriceBox.new(bars=3)"), "replacement", definition, []string{"bars=3"}, strategyir.AssignmentModeVar)
	if err != nil || !handled {
		t.Fatalf("object constructor handled=%v err=%v", handled, err)
	}
	constructor, ok := statement.(*strategyir.ObjectStmt)
	if !ok || constructor.TypeName != "PriceBox" || constructor.ResultName != "replacement" || constructor.Mode != strategyir.AssignmentModeVar || !state.objectPersistent["replacement"] || state.collectionNamespaces["replacement.values"] != "array" {
		t.Fatalf("object constructor = %#v, state=%#v", statement, state)
	}

	for _, tc := range []struct {
		name     string
		receiver string
		member   string
		args     []string
		handled  bool
		message  string
	}{
		{name: "unknown receiver", receiver: "missing", member: "score", handled: false},
		{name: "unknown method", receiver: "box", member: "missing", handled: false},
		{name: "unknown named method argument", receiver: "box", member: "score", args: []string{"missing=1"}, handled: true, message: "unknown method parameter"},
		{name: "method argument expression", receiver: "box", member: "score", args: []string{"factor=close >"}, handled: true, message: "invalid method argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, handled, err := state.parseObjectMethodAssignment(line(70, "result = box.score()"), "result", tc.receiver, tc.member, tc.args, strategyir.AssignmentModeLet)
			if handled != tc.handled || (tc.message == "" && err != nil) || (tc.message != "" && (err == nil || !strings.Contains(err.Error(), tc.message))) {
				t.Fatalf("parseObjectMethodAssignment handled=%v err=%v", handled, err)
			}
		})
	}

	method, ok := selectExecutableMethod([]strategyir.MethodDefinition{
		{Name: "one", Parameters: []strategyir.ObjectParameter{{Name: "required"}}},
		{Name: "two", Parameters: []strategyir.ObjectParameter{{Name: "required"}, {Name: "optional", Default: "0"}}},
	}, 2)
	if !ok || method.Name != "two" {
		t.Fatalf("selectExecutableMethod overload = %#v/%v", method, ok)
	}
	if _, ok := selectExecutableMethod([]strategyir.MethodDefinition{{Name: "one", Parameters: []strategyir.ObjectParameter{{Name: "required"}}}}, 0); ok {
		t.Fatal("selectExecutableMethod accepted a missing required argument")
	}

	for _, expression := range []string{
		`object_method("PriceBox", "identity", box)`,
		`object_method("PriceBox", "identity", box).`,
		`object_method("PriceBox", "identity", box).score(`,
		`object_method("Unknown", "identity", box).score()`,
	} {
		if _, _, _, _, _, ok := state.nextObjectMethodExpressionReceiverCall(expression); ok {
			t.Fatalf("invalid object-method receiver expression was accepted: %q", expression)
		}
	}
	for _, expression := range []string{"box.score(missing=1)", "box.score(1, 2, 3)"} {
		if _, err := state.lowerObjectMethodCalls(expression); err == nil {
			t.Fatalf("lowerObjectMethodCalls(%q) accepted an invalid overload", expression)
		}
	}
}

func TestCollectionLexicalParserRejectsMalformedHistoryAndNamespaceReferences(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	state.normalizationErr = errors.New("injected normalization failure")
	if _, err := state.normalizeCollectionArguments(80, []string{"close"}); err == nil || !strings.Contains(err.Error(), "injected normalization failure") {
		t.Fatalf("normalizeCollectionArguments normalization error = %v", err)
	}

	state = newObjectCollectionBoundaryParseState()
	statement, handled, err := state.parseCollectionStatement(parsedLine{number: 81, trimmed: "map labels = map.new<string, float>()"})
	if err != nil || !handled {
		t.Fatalf("type-argument inference handled=%v err=%v", handled, err)
	}
	collection, ok := statement.(*strategyir.CollectionStmt)
	if !ok || collection.TypeArgs != "string, float" || collection.Namespace != "map" {
		t.Fatalf("inferred map declaration = %#v", statement)
	}

	for _, tc := range []struct {
		segment string
		found   bool
	}{
		{segment: "[1].get(0)", found: false},
		{segment: "values[1", found: false},
		{segment: "values[-1].get(0)", found: false},
		{segment: "values[1] no_method", found: false},
		{segment: "values[1].()", found: false},
		{segment: "values[1].get(", found: false},
		{segment: "values[1].get(0)", found: true},
	} {
		_, _, ok := parseCollectionHistoryCallAt(tc.segment, 0, 0)
		if ok != tc.found {
			t.Fatalf("parseCollectionHistoryCallAt(%q) found=%v, want %v", tc.segment, ok, tc.found)
		}
	}
	if got := collectionHistoryCallArgs("values[1].get()"); got != "" {
		t.Fatalf("empty history arguments = %q", got)
	}
	if got := collectionHistoryCallArgs("values[1].get(1, 2)"); got != "1, 2" {
		t.Fatalf("history arguments = %q", got)
	}

	for _, tc := range []struct {
		target string
		want   string
	}{
		{target: "values", want: "array"},
		{target: "box.values", want: "array"},
		{target: "unknown.values", want: ""},
		{target: "box.unknown", want: ""},
		{target: "not_a_member", want: ""},
	} {
		if got := state.collectionNamespaceForTargetExpression(tc.target); got != tc.want {
			t.Fatalf("collectionNamespaceForTargetExpression(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line string
		want bool
	}{
		{line: "plain = close", want: false},
		{line: "array.push(values, close); matrix.set(grid, 0, 0, close)", want: true},
		{line: "array.push(values, close); array.unknown(values)", want: false},
		{line: "values.push(close)", want: false},
		{line: "values.get(0)", want: false},
	} {
		if got := supportedExecutableCollectionLine(tc.line); got != tc.want {
			t.Fatalf("supportedExecutableCollectionLine(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
	if _, _, _, ok := state.resolveExecutableCollectionCall("box.unknown"); ok {
		t.Fatal("unknown object collection operation was resolved")
	}
	if namespace, operation, _, ok := state.resolveExecutableCollectionCall("box.values.last"); !ok || namespace != "array" || operation != "last" {
		t.Fatalf("object field collection call = %q/%q/%v", namespace, operation, ok)
	}
}

func TestUDFAndDynamicLoopHelpersProtectRuntimeBoundaries(t *testing.T) {
	validBody := []parsedLine{
		{number: 1, trimmed: "scaled = value * 2", indent: 4},
		{number: 2, trimmed: "if scaled > 0", indent: 4},
		{number: 3, trimmed: "scaled", indent: 8},
		{number: 4, trimmed: "else", indent: 4},
		{number: 5, trimmed: "0", indent: 8},
	}
	compiled, err := compileUDFBody(validBody)
	if err != nil || !strings.Contains(compiled, "ifelse") || !strings.Contains(compiled, "value * 2") {
		t.Fatalf("compileUDFBody valid branch = %q/%v", compiled, err)
	}
	for _, tc := range []struct {
		name  string
		lines []parsedLine
		want  string
	}{
		{name: "empty body", want: "requires a return expression"},
		{name: "unexpected nested indentation", lines: []parsedLine{{number: 1, trimmed: "value", indent: 4}, {number: 2, trimmed: "extra", indent: 8}}, want: "return expression must be the last"},
		{name: "local reassignment", lines: []parsedLine{{number: 1, trimmed: "value := 1", indent: 4}}, want: "local reassignment"},
		{name: "missing else", lines: []parsedLine{{number: 1, trimmed: "if value > 0", indent: 4}, {number: 2, trimmed: "value", indent: 8}}, want: "requires else"},
		{name: "branch missing return", lines: []parsedLine{{number: 1, trimmed: "if value > 0", indent: 4}, {number: 2, trimmed: "temp = value", indent: 8}, {number: 3, trimmed: "else", indent: 4}, {number: 4, trimmed: "0", indent: 8}}, want: "branch requires a return"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compileUDFBody(tc.lines)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("compileUDFBody error = %v, want %q", err, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{args: nil, ok: true},
		{args: []string{"source", "length"}, ok: true},
		{args: []string{"source", "source"}, ok: false},
		{args: []string{"source + 1"}, ok: false},
	} {
		_, err := parseUDFArgs(90, strings.Join(tc.args, ","))
		if (err == nil) != tc.ok {
			t.Fatalf("parseUDFArgs(%#v) err=%v, want ok=%v", tc.args, err, tc.ok)
		}
	}

	state := newParseState("", []parsedLine{
		{number: 100, trimmed: "for i = start to finish by step", indent: 0},
		{number: 101, trimmed: "value = i", indent: 4},
	}, nil)
	match, ok := parseStaticForLoopHeader("for i = start to finish by step")
	if !ok {
		t.Fatal("runtime loop header was not parsed")
	}
	statements, next, err := state.parseRuntimeForLoop(0, match)
	if err != nil || next != 2 || len(statements) != 1 {
		t.Fatalf("parseRuntimeForLoop = %#v/%d/%v", statements, next, err)
	}
	runtimeLoop, ok := statements[0].(*strategyir.LoopStmt)
	if !ok || runtimeLoop.Variable != "i" || runtimeLoop.StartExpression != "start" || runtimeLoop.EndExpression != "finish" || runtimeLoop.StepExpression != "step" || len(runtimeLoop.Body) != 1 || state.loopVariables["i"] {
		t.Fatalf("runtime loop = %#v, state=%#v", runtimeLoop, state.loopVariables)
	}

	state.runtimeLoopDepth = maxRuntimeLoopDepth
	if _, _, err := state.parseRuntimeForLoop(0, match); err == nil || !strings.Contains(err.Error(), "dynamic loop nesting") {
		t.Fatalf("runtime loop depth guard = %v", err)
	}
}

func TestTALoweringHelpersKeepNativePineArgumentSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{name: "generic period function", got: replaceTAFunction("ta.atr(7) + ta.atr()", "atr", "atr(${period})"), want: "atr(7) + atr(14)"},
		{name: "generic multi-source function", got: replaceTAFunction("ta.correlation(close, volume, 20)", "correlation", "correlation(${left}, ${right}, ${third})"), want: "correlation(close, volume, 20)"},
		{name: "MACD", got: replaceTAMacd("ta.macd(close, 12, 26, 9)"), want: "macd(12, 26, 9)"},
		{name: "Bollinger", got: replaceTABollinger("ta.bb(hl2, 20, 2)"), want: "bollinger(20, 2)"},
		{name: "moving average", got: replaceTAMovingAverageFunction("ta.ema(hl2, 9)", "ema", "EMA"), want: "ma(EMA, 9, hl2)"},
		{name: "source length", got: replaceTASourceLengthFunction("ta.rsi(hlc3, 14)", "rsi", "rsi", "close", "14"), want: "rsi(hlc3, 14)"},
		{name: "optional source", got: replaceTASourceOptionalFunction("ta.vwap(hlc3)", "vwap", "vwap", "close"), want: "vwap(hlc3)"},
		{name: "anchored VWAP", got: replaceTAAnchoredVWAP(`ta.vwap(, timeframe.change("D"))`), want: "anchored_vwap(hlc3, day)"},
		{name: "required source", got: replaceTASourceRequiredFunction("ta.cum(volume)", "cum", "cum"), want: "cum(volume)"},
		{name: "stateful indicator", got: replaceTAStateFunction("ta.obv(close)", "obv"), want: "obv(close)"},
		{name: "extrema bars", got: replaceTAExtremaBarsFunction("ta.highestbars(high, 10)", "highestbars"), want: "highestbars(high, 10)"},
		{name: "stochastic", got: replaceTAStoch("ta.stoch(close, high, low, 14)"), want: "stoch(close, high, low, 14)"},
		{name: "true range property and call", got: replaceTATr("ta.tr + ta.tr(true)"), want: "tr() + tr()"},
		{name: "window function", got: replaceTAWindowFunction("ta.highest(10) + ta.roc(close, 5)", "highest"), want: "highest(high, 10) + ta.roc(close, 5)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("lowered Pine TA expression = %q, want %q", tc.got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{name: "unclosed generic call", got: replaceTAFunction("ta.atr(7", "atr", "atr(${period})"), want: "ta.atr(7"},
		{name: "unclosed MACD", got: replaceTAMacd("ta.macd(close, 12"), want: "ta.macd(close, 12"},
		{name: "wrong optional source arity", got: replaceTASourceOptionalFunction("ta.vwap(close, volume)", "vwap", "vwap", "close"), want: "ta.vwap(close, volume)"},
		{name: "invalid anchored VWAP timeframe", got: replaceTAAnchoredVWAP(`ta.vwap(close, timeframe.change("15"))`), want: `ta.vwap(close, timeframe.change("15"))`},
		{name: "wrong required source arity", got: replaceTASourceRequiredFunction("ta.cum(close, open)", "cum", "cum"), want: "ta.cum(close, open)"},
		{name: "wrong extrema arity", got: replaceTAExtremaBarsFunction("ta.highestbars(high)", "highestbars"), want: "ta.highestbars(high)"},
		{name: "wrong stochastic arity", got: replaceTAStoch("ta.stoch(close, high, low)"), want: "ta.stoch(close, high, low)"},
		{name: "unclosed true range property remains normalized", got: replaceTATr("ta.tr("), want: "tr()("},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("invalid Pine TA expression changed to %q, want %q", tc.got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		call   string
		args   []string
		source string
		period string
	}{
		{name: "highest defaults", call: "highest", source: "high", period: "14"},
		{name: "lowest length", call: "lowest", args: []string{"5"}, source: "low", period: "5"},
		{name: "momentum source", call: "mom", args: []string{"hl2"}, source: "hl2", period: "14"},
		{name: "explicit window", call: "rising", args: []string{"close", "7"}, source: "close", period: "7"},
	} {
		source, period := pineWindowFunctionArgs(tc.call, tc.args)
		if source != tc.source || period != tc.period {
			t.Fatalf("pineWindowFunctionArgs(%q, %#v) = %q/%q, want %q/%q", tc.call, tc.args, source, period, tc.source, tc.period)
		}
	}

	if got, ok := lowerSupportedRequestSecurityInner("ta.sma(strategy.position_size, 2)", `"15m"`); ok || got != "" {
		t.Fatalf("unsafe moving average request.security lowering = %q/%v", got, ok)
	}
	for _, tc := range []struct {
		name string
		call string
		args []string
	}{
		{name: "OBV too many arguments", call: "obv", args: []string{"close", "extra"}},
		{name: "pivot missing strengths", call: "pivothigh", args: []string{"close"}},
		{name: "Keltner wrong arity", call: "kc", args: []string{"close", "20"}},
		{name: "ALMA wrong arity", call: "alma", args: []string{"close", "9"}},
		{name: "TSI wrong arity", call: "tsi", args: []string{"close", "13"}},
		{name: "correlation invalid second source", call: "correlation", args: []string{"close", "bad", "20"}},
		{name: "SWMA wrong arity", call: "swma", args: []string{"close", "extra"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := lowerAdvancedRequestSecurity(tc.call, tc.args, `"15m"`); ok || got != "" {
				t.Fatalf("lowerAdvancedRequestSecurity(%s, %#v) = %q/%v", tc.name, tc.args, got, ok)
			}
		})
	}
}
