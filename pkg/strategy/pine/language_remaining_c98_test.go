package pine

import (
	"errors"
	"strings"
	"testing"
)

func TestCoverage98ObjectAndCollectionParserFailureContracts(t *testing.T) {
	newObjectState := func(lines ...parsedLine) *parseState {
		return newParseState("", lines, nil)
	}

	t.Run("method declarations reject unnormalizable and malformed bodies", func(t *testing.T) {
		normalizationFailure := newObjectState(
			parsedLine{number: 1, trimmed: "type Quote"},
			parsedLine{number: 2, trimmed: "float value", indent: 4},
			parsedLine{number: 3, trimmed: "method score(Quote self) => close"},
		)
		if _, err := normalizationFailure.parseExecutableTypeDefinition(0); err != nil {
			t.Fatalf("prepare type: %v", err)
		}
		normalizationFailure.normalizationErr = errors.New("method normalization failed")
		if _, err := normalizationFailure.parseExecutableMethodDefinition(2); err == nil || !strings.Contains(err.Error(), "method normalization failed") {
			t.Fatalf("method normalization error = %v", err)
		}

		for _, tc := range []struct {
			name string
			line string
			want string
		}{
			{name: "invalid body", line: "method score(Quote self) => close >", want: "invalid method body"},
			{name: "invalid parameter default", line: "method score(Quote self, float threshold=close >) => close", want: "method parameter default"},
			{name: "multiline invalid body", line: "method score(Quote self) =>", want: "method score"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				lines := []parsedLine{
					{number: 10, trimmed: "type Quote"},
					{number: 11, trimmed: "float value", indent: 4},
					{number: 12, trimmed: tc.line},
				}
				if tc.name == "multiline invalid body" {
					lines = append(lines, parsedLine{number: 13, trimmed: "strategy.entry(\"x\", strategy.long)", indent: 4})
				}
				state := newObjectState(lines...)
				if _, err := state.parseExecutableTypeDefinition(0); err != nil {
					t.Fatalf("prepare type: %v", err)
				}
				if _, err := state.parseExecutableMethodDefinition(2); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("method error = %v, want %q", err, tc.want)
				}
			})
		}
	})

	t.Run("object assignment leaves unknown receivers for ordinary expression parsing", func(t *testing.T) {
		state := newObjectCollectionBoundaryParseState()
		statement, handled, err := state.parseObjectFieldAssignment(parsedLine{number: 20, trimmed: "unknown.price := close"})
		if err != nil || handled || statement != nil {
			t.Fatalf("unknown field receiver = %#v/%v/%v", statement, handled, err)
		}
		statement, handled, err = state.parseAssignedObjectStatement(parsedLine{number: 21, trimmed: "result = unknown.score(close)"})
		if err != nil || handled || statement != nil {
			t.Fatalf("unknown method receiver = %#v/%v/%v", statement, handled, err)
		}
		if values, err := state.normalizeObjectConstructorArguments(22, nil, state.udtTypes["pricebox"].Fields); err != nil || len(values) != 0 {
			t.Fatalf("empty constructor arguments = %#v/%v", values, err)
		}
		if values, err := state.normalizeObjectMethodArguments(23, nil, state.udtMethods["pricebox.score"][0].Parameters); err != nil || len(values) != 0 {
			t.Fatalf("empty method arguments = %#v/%v", values, err)
		}
	})

	t.Run("object method scanners ignore incomplete and unrelated calls", func(t *testing.T) {
		state := newObjectCollectionBoundaryParseState()
		if _, _, _, _, _, found := state.nextObjectHistoryMethodCall("box[1].score("); found {
			t.Fatal("unterminated history method was accepted")
		}
		if _, _, _, _, _, found := state.nextObjectMethodExpressionReceiverCall(`object_method("PriceBox", "identity", box) text`); found {
			t.Fatal("non-method expression receiver was accepted")
		}
		if _, _, _, _, found := state.nextObjectMethodCall("box.unknown(1)"); found {
			t.Fatal("unknown object method was accepted")
		}
		if result, err := state.lowerObjectMethodCalls("box.unknown(1)"); err != nil || result != "box.unknown(1)" {
			t.Fatalf("unrelated object call lowered=%q err=%v", result, err)
		}
	})

	t.Run("typed collections preserve namespace safety before execution", func(t *testing.T) {
		for _, tc := range []struct {
			line string
			want string
		}{
			{line: "array<float> values = close", want: "require an executable collection constructor"},
			{line: "array<float> values = array.get(values, 0)", want: "require an executable collection constructor"},
			{line: "array<float> values = map.new<string, float>()", want: "cannot be initialized"},
			{line: "matrix<float> grid = matrix.new<float>(2, 2, close)", want: ""},
		} {
			state := newObjectCollectionBoundaryParseState()
			statement, handled, err := state.parseTypedCollectionStatement(parsedLine{number: 30, trimmed: tc.line})
			if tc.want == "" {
				if err != nil || !handled || statement == nil {
					t.Fatalf("typed collection %q = %#v/%v/%v", tc.line, statement, handled, err)
				}
				continue
			}
			if err == nil || !handled || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("typed collection %q err=%v handled=%v", tc.line, err, handled)
			}
		}
	})

	t.Run("collection calls require an assignable target when they are reads", func(t *testing.T) {
		state := newObjectCollectionBoundaryParseState()
		if _, handled, err := state.parseStandaloneCollectionStatement(parsedLine{number: 40, trimmed: "values.get(0)"}); !handled || err == nil || !strings.Contains(err.Error(), "must be assigned") {
			t.Fatalf("standalone read err=%v handled=%v", err, handled)
		}
		if _, handled, err := state.parseStandaloneCollectionStatement(parsedLine{number: 41, trimmed: "array.new_float(2)"}); !handled || err == nil || !strings.Contains(err.Error(), "must be assigned") {
			t.Fatalf("standalone constructor err=%v handled=%v", err, handled)
		}
		if _, handled, err := state.parseAssignedCollectionStatement(parsedLine{number: 42, trimmed: "answer = values.push(close)"}); err != nil || !handled {
			t.Fatalf("assigned mutation = %v/%v", handled, err)
		}
		if _, handled, err := state.parseAssignedCollectionStatement(parsedLine{number: 43, trimmed: "answer = close"}); err != nil || handled {
			t.Fatalf("ordinary expression claimed by collection parser = %v/%v", handled, err)
		}
	})

	t.Run("history scanning does not inspect quoted pseudo-code", func(t *testing.T) {
		state := newObjectCollectionBoundaryParseState()
		call, found := findCollectionHistoryCall(`"values[1].get(0)" + values[2].last()`)
		if !found || call.name != "values" || call.lookback != "2" || call.operation != "last" {
			t.Fatalf("history call = %#v found=%v", call, found)
		}
		if args := collectionHistoryCallArgs("values[1].last()"); args != "" {
			t.Fatalf("last args = %q", args)
		}
		if replacement := collectionHistoryReplacement(collectionHistoryCall{name: "values", lookback: "3", operation: "get", args: "0"}); replacement != "collection_array_get(history(values, 3), 0)" {
			t.Fatalf("history replacement = %q", replacement)
		}
		if _, err := state.lowerCollectionHistoryReadCalls("values[1].join(',')"); err != nil {
			t.Fatalf("supported collection history join: %v", err)
		}
	})
}

func TestCoverage98RequestSecurityAndTupleContractsRejectUnsafeExpressions(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		ok   bool
	}{
		{name: "missing ticker", args: []string{"close", "60", "close"}},
		{name: "unsupported timeframe", args: []string{"syminfo.tickerid", "2", "close"}},
		{name: "merge on", args: []string{"syminfo.tickerid", "60", "close", "barmerge.gaps_on"}},
		{name: "supported defaults", args: []string{"syminfo.tickerid", "60", "close"}, ok: true},
		{name: "supported named merge", args: []string{"syminfo.tickerid", "60", "close", "gaps=barmerge.gaps_off", "lookahead=barmerge.lookahead_off"}, ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := lowerSupportedRequestSecurity(tc.args)
			if ok != tc.ok {
				t.Fatalf("lower request security %#v = %v, want %v", tc.args, ok, tc.ok)
			}
		})
	}

	for _, tc := range []struct {
		expression string
		ok         bool
	}{
		{expression: "close + high * 2", ok: true},
		{expression: "close := open"},
		{expression: "strategy.position_size"},
		{expression: "[close, high]"},
		{expression: "ta.unknown(close, 3)"},
		{expression: "ta.rsi(close, 3) + close[1]", ok: true},
	} {
		_, ok := lowerPureRequestSecurityExpression(tc.expression, "hour")
		if ok != tc.ok {
			t.Fatalf("pure request security %q = %v, want %v", tc.expression, ok, tc.ok)
		}
	}
	if requestSecurityLoweredASTIsPure("unknown_call(close)") || requestSecurityLoweredASTIsPure("close +") {
		t.Fatal("unknown or invalid runtime calls cannot be pure security ASTs")
	}
	if !requestSecurityLoweredASTIsPure("ma(EMA, 10, hour) + security_source(close, hour, 1)") {
		t.Fatal("known lowered calls must remain pure")
	}
	if pureRequestSecurityRuntimeCall("danger") || !pureRequestSecurityRuntimeCall("collection_array_get") {
		t.Fatal("runtime call allowlist mismatch")
	}

	for _, tc := range []struct {
		name string
		args []string
		ok   bool
	}{
		{name: "macd lacks source", args: []string{"1", "2", "3"}},
		{name: "stoch cannot use volume", args: []string{"volume", "high", "low", "3"}},
		{name: "advanced day rejected", args: []string{"close", "3"}},
		{name: "advanced minute works", args: []string{"close", "3"}, ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "macd"
			unit := "hour"
			if tc.name == "stoch cannot use volume" {
				name = "stoch"
			}
			if tc.name == "advanced day rejected" || tc.name == "advanced minute works" {
				name = "cmo"
				if tc.name == "advanced day rejected" {
					unit = "day"
				}
			}
			_, ok := lowerRequestSecurityTACall(name, tc.args, unit)
			if ok != tc.ok {
				t.Fatalf("%s lowering = %v, want %v", tc.name, ok, tc.ok)
			}
		})
	}

	line := parsedLine{number: 60, trimmed: "[one] = request.security(syminfo.tickerid, \"60\", [close, high])"}
	if diagnostic, ok := requestSecurityTupleDiagnostic(line, "[close, high]"); !ok || !strings.Contains(diagnostic.Message, "tuple returns 2 values but assignment has 1 aliases") {
		t.Fatalf("tuple alias diagnostic = %#v/%v", diagnostic, ok)
	}
	if diagnostic, ok := requestSecurityTupleDiagnostic(parsedLine{number: 61, trimmed: "[a, b] = request.security(syminfo.tickerid, \"60\", [close])"}, "[close]"); !ok || !strings.Contains(diagnostic.Message, "2 to 8") {
		t.Fatalf("tuple arity diagnostic = %#v/%v", diagnostic, ok)
	}
	if message := historyDiagnosticMessage("close[999999999999999999999999999999]"); !strings.Contains(message, "non-negative") {
		t.Fatalf("overflowing history diagnostic = %q", message)
	}
	if message := historyDiagnosticMessage("ta.sma(close, 2)[1]"); !strings.Contains(message, "assign the function result") {
		t.Fatalf("call history diagnostic = %q", message)
	}
}

func TestCoverage98OrderAndTupleHelperContractsKeepTradeInstructionsUnambiguous(t *testing.T) {
	state := newParseState("", nil, nil)
	if _, _, _, err := pineOrderMetadata(1, "strategy.entry", []string{"disable_alert=maybe"}, false); err == nil || !strings.Contains(err.Error(), "true or false") {
		t.Fatalf("invalid order alert flag = %v", err)
	}
	if _, _, _, err := pineOrderMetadata(2, "strategy.entry", []string{"immediately=true"}, false); err == nil || !strings.Contains(err.Error(), "does not support immediately") {
		t.Fatalf("entry immediate flag = %v", err)
	}
	if _, _, _, _, err := pineCloseAllMetadata(3, []string{"maybe"}); err == nil || !strings.Contains(err.Error(), "immediately") {
		t.Fatalf("close-all positional immediate = %v", err)
	}
	if _, _, _, _, err := pineCloseAllMetadata(4, []string{"true", "note", "alert", "maybe"}); err == nil || !strings.Contains(err.Error(), "disable_alert") {
		t.Fatalf("close-all alert flag = %v", err)
	}
	if err := rejectConflictingQuantityArgs(5, "strategy.close", []string{"qty=1", "qty_percent=20"}); err == nil {
		t.Fatal("a close cannot set both absolute and percentage quantities")
	}
	if err := rejectUnsupportedNamedArgs(6, "strategy.entry", []string{"trail_offset=2"}, "qty"); err == nil {
		t.Fatal("unsupported order arguments must remain visible errors")
	}

	if _, err := state.parseStrategyCloseCall(parsedLine{number: 7, trimmed: "strategy.close()"}); err == nil || !strings.Contains(err.Error(), "requires an entry id") {
		t.Fatalf("missing close id = %v", err)
	}
	if _, err := state.parseStrategyCloseCall(parsedLine{number: 8, trimmed: "strategy.close(\"long\", qty=1, qty_percent=20)"}); err == nil || !strings.Contains(err.Error(), "qty or qty_percent") {
		t.Fatalf("conflicting close quantities = %v", err)
	}
	if _, err := state.parseStrategyTrailingExit(parsedLine{number: 9}, "exit", "entry", "long", nil, "shares", "1"); err == nil || !strings.Contains(err.Error(), "requires trail_offset") {
		t.Fatalf("missing trailing offset = %v", err)
	}
	if _, err := validateStrategyExitTriggers(10, []string{"trail_points=2", "trail_price=3"}); err == nil || !strings.Contains(err.Error(), "or trail_price") {
		t.Fatalf("conflicting trailing triggers = %v", err)
	}
	if _, err := validateStrategyExitTriggers(11, []string{"trail_offset=2"}); err == nil || !strings.Contains(err.Error(), "advanced exit") {
		t.Fatalf("missing exit trigger = %v", err)
	}
	if trailing, err := validateStrategyExitTriggers(12, []string{"trail_offset=2", "trail_points=3"}); err != nil || !trailing {
		t.Fatalf("valid trailing trigger = %v/%v", trailing, err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "bollinger aliases", args: []string{"a", "b", "c"}, want: "bollinger"},
		{name: "dmi aliases", args: []string{"a", "b", "c"}, want: "dmi"},
		{name: "macd aliases", args: []string{"a", "b", "c"}, want: "macd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var statementErr error
			switch tc.name {
			case "bollinger aliases":
				_, _, statementErr = state.parseBollingerTupleAssignment(parsedLine{number: 20}, tc.args, []string{"close", "2", "2"}, "ta.bb(close, 2, 2)")
			case "dmi aliases":
				_, _, statementErr = state.parseDMITupleAssignment(parsedLine{number: 21}, tc.args, []string{"2", "2"}, "ta.dmi(2, 2)")
			case "macd aliases":
				_, _, statementErr = state.parseMACDTupleAssignment(parsedLine{number: 22}, tc.args, []string{"close", "2", "3", "1"}, "ta.macd(close, 2, 3, 1)")
			}
			if statementErr != nil {
				t.Fatalf("%s tuple = %v", tc.name, statementErr)
			}
		})
	}
}
