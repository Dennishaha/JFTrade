package pine

import (
	"strings"
	"testing"
)

func TestCoverage98IncompleteColorAndRequestCallsRemainRecoverable(t *testing.T) {
	for _, expression := range []string{"color.new(", "color.rgb("} {
		if got := replaceColorFunctions(expression); got != expression {
			t.Fatalf("incomplete color call %q was rewritten to %q", expression, got)
		}
	}
	if args, ok := requestSecurityCallArgs("request.security("); ok || args != nil {
		t.Fatalf("incomplete request.security call = %#v/%v", args, ok)
	}
}

func TestCoverage98UDFExpansionRetainsWhitespaceAndUnwindsRejectedArguments(t *testing.T) {
	zeroArgument := newParseState("", nil, nil)
	zeroArgument.udfs["ready"] = pineUDF{Name: "ready", Body: "close > open", Line: 1}
	expanded, err := zeroArgument.expandUDFCalls("ready ()", 0, map[string]bool{})
	if err != nil || !strings.Contains(expanded, "close > open") {
		t.Fatalf("zero-argument UDF expansion = %q/%v", expanded, err)
	}
	expanded, err = zeroArgument.expandUDFCalls("ready( )", 0, map[string]bool{})
	if err != nil || !strings.Contains(expanded, "close > open") {
		t.Fatalf("whitespace-only UDF arguments = %q/%v", expanded, err)
	}

	state := newParseState("", nil, nil)
	state.collectionNamespaces["values"] = "array"
	state.udfs["score"] = pineUDF{Name: "score", Args: []string{"value"}, Body: "value + 1", Line: 2}
	stack := map[string]bool{}
	if _, err := state.expandUDFCalls("score(values[1].push(close))", 0, stack); err == nil || !strings.Contains(err.Error(), "supports only read operations") {
		t.Fatalf("historical collection mutation error = %v", err)
	}
	if stack["score"] {
		t.Fatal("failed UDF argument normalization left recursion tracking behind")
	}
}

func TestCoverage98MethodAndControlFlowFailuresKeepSourceContracts(t *testing.T) {
	t.Run("multiline method forwards UDF body errors", func(t *testing.T) {
		state := newParseState("", []parsedLine{
			{number: 1, trimmed: "type Quote"},
			{number: 2, trimmed: "float price", indent: 4},
			{number: 3, trimmed: "method score(Quote self) =>"},
			{number: 4, trimmed: "if close > open", indent: 4},
			{number: 5, trimmed: "close", indent: 8},
		}, nil)
		if _, err := state.parseExecutableTypeDefinition(0); err != nil {
			t.Fatalf("parse type: %v", err)
		}
		if _, err := state.parseExecutableMethodDefinition(2); err == nil || !strings.Contains(err.Error(), "requires else") {
			t.Fatalf("multiline method error = %v", err)
		}
	})

	t.Run("method defaults reject historical collection mutations", func(t *testing.T) {
		state := newParseState("", []parsedLine{
			{number: 10, trimmed: "type Quote"},
			{number: 11, trimmed: "float price", indent: 4},
			{number: 12, trimmed: "method score(Quote self, float factor=values[1].push(close)) => self.price"},
		}, nil)
		state.collectionNamespaces["values"] = "array"
		if _, err := state.parseExecutableTypeDefinition(0); err != nil {
			t.Fatalf("parse type: %v", err)
		}
		if _, err := state.parseExecutableMethodDefinition(2); err == nil || !strings.Contains(err.Error(), "supports only read operations") {
			t.Fatalf("method default error = %v", err)
		}
	})

	t.Run("assignments and conditions reject incomplete expressions", func(t *testing.T) {
		state := newParseState("", nil, nil)
		if _, handled, err := state.parseAssignment(parsedLine{number: 20, trimmed: "value = close >"}); !handled || err == nil || !strings.Contains(err.Error(), "assignment expression") {
			t.Fatalf("incomplete assignment = handled:%v err:%v", handled, err)
		}

		state = newParseState("", []parsedLine{{number: 21, trimmed: "if close >"}}, nil)
		if _, _, err := state.parseIfStatement(0, state.lines[0]); err == nil || !strings.Contains(err.Error(), "if condition") {
			t.Fatalf("incomplete if condition error = %v", err)
		}

		state = newParseState("", []parsedLine{{number: 22, trimmed: "if values[1].push(close)"}}, nil)
		state.collectionNamespaces["values"] = "array"
		if _, _, err := state.parseIfStatement(0, state.lines[0]); err == nil || !strings.Contains(err.Error(), "supports only read operations") {
			t.Fatalf("historical collection condition error = %v", err)
		}
	})

	t.Run("invalid else blocks preserve their original parser error", func(t *testing.T) {
		state := newParseState("", []parsedLine{
			{number: 25, trimmed: "if close > open"},
			{number: 26, trimmed: "value = close", indent: 4},
			{number: 27, trimmed: "else"},
			{number: 28, trimmed: "break", indent: 4},
		}, nil)
		if _, _, err := state.parseIfStatement(0, state.lines[0]); err == nil || !strings.Contains(err.Error(), "static for loops") {
			t.Fatalf("invalid else block error = %v", err)
		}
	})

	t.Run("switch conditions reject historical mutations", func(t *testing.T) {
		state := newParseState("", nil, nil)
		state.collectionNamespaces["values"] = "array"
		_, err := state.lowerSwitchExpression(30, []switchArm{{
			condition: "values[1].push(close)",
			value:     "close",
			line:      parsedLine{number: 31},
		}})
		if err == nil || !strings.Contains(err.Error(), "supports only read operations") {
			t.Fatalf("switch condition error = %v", err)
		}
	})
}

func TestCoverage98NestedLoopStateAndImportRecoveryRemainStable(t *testing.T) {
	collectionState := newParseState("", []parsedLine{
		{number: 1, trimmed: "for item in values"},
		{number: 2, trimmed: "total = item", indent: 4},
	}, nil)
	collectionState.loopVariables["item"] = true
	collectionMatch := collectionForLoopPattern.FindStringSubmatch(collectionState.lines[0].trimmed)
	if collectionMatch == nil {
		t.Fatal("collection loop header did not parse")
	}
	if _, _, err := collectionState.parseCollectionForLoop(0, collectionMatch); err != nil {
		t.Fatalf("parse nested collection loop: %v", err)
	}
	if !collectionState.loopVariables["item"] {
		t.Fatal("outer collection loop variable was not restored")
	}

	staticState := newParseState("", []parsedLine{
		{number: 10, trimmed: "for i = 0 to 0"},
		{number: 11, trimmed: "total = i", indent: 4},
	}, nil)
	staticState.valueAliases["i"] = "outer"
	staticState.loopVariables["i"] = true
	match, ok := parseStaticForLoopHeader(staticState.lines[0].trimmed)
	if !ok {
		t.Fatal("static loop header did not parse")
	}
	spec, err := staticState.parseStaticForLoopSpec(staticState.lines[0].number, match)
	if err != nil {
		t.Fatalf("static loop spec: %v", err)
	}
	if _, _, err := staticState.expandStaticForLoop(0, staticState.lines[0], match, spec); err != nil {
		t.Fatalf("expand static loop: %v", err)
	}
	if staticState.valueAliases["i"] != "outer" || !staticState.loopVariables["i"] {
		t.Fatalf("outer static loop state was not restored: aliases=%#v loop=%#v", staticState.valueAliases, staticState.loopVariables)
	}

	if version := importVersion(""); version != "" {
		t.Fatalf("empty import version = %q", version)
	}
}

func TestCoverage98OrderAndTupleErrorsPreserveExecutableBoundaries(t *testing.T) {
	newStateWithValues := func() *parseState {
		state := newParseState("", nil, nil)
		state.collectionNamespaces["values"] = "array"
		return state
	}

	for _, line := range []parsedLine{
		{number: 40, trimmed: `strategy.entry("Long", strategy.long, qty=values[1].push(close))`},
		{number: 41, trimmed: `strategy.entry("Long", strategy.long, limit=values[1].push(close))`},
	} {
		if _, err := newStateWithValues().parseStrategyEntryCall(line); err == nil || !strings.Contains(err.Error(), "supports only read operations") {
			t.Fatalf("strategy.entry mutation contract for %q = %v", line.trimmed, err)
		}
	}

	for _, tc := range []struct {
		line parsedLine
		want string
	}{
		{
			line: parsedLine{number: 42, trimmed: `strategy.exit("Exit", "Long")`},
			want: "advanced exit semantics",
		},
		{
			line: parsedLine{number: 43, trimmed: `strategy.exit("Exit", "Long", stop=close, qty=1, qty_percent=50)`},
			want: "supports qty or qty_percent",
		},
		{
			line: parsedLine{number: 44, trimmed: `strategy.exit("Trail", "Long", trail_points=1, trail_offset=1, when=values[1].push(close))`},
			want: "supports only read operations",
		},
	} {
		if _, err := newStateWithValues().parseStrategyExit(tc.line); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("strategy.exit contract for %q = %v, want %q", tc.line.trimmed, err, tc.want)
		}
	}

	state := newParseState("", nil, nil)
	if _, err := state.normalizeTupleExpressions(50, []string{"close >"}); err == nil || !strings.Contains(err.Error(), "assignment expression") {
		t.Fatalf("malformed tuple expression error = %v", err)
	}
}

func TestCoverage98OrderCallsRejectUnknownNamedArgumentsBeforePlanning(t *testing.T) {
	state := newParseState("", nil, nil)
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{
			name: "entry",
			call: func() error {
				_, err := state.parseStrategyEntryCall(parsedLine{number: 60, trimmed: `strategy.entry("Long", strategy.long, unknown=1)`})
				return err
			},
		},
		{
			name: "exit",
			call: func() error {
				_, err := state.parseStrategyExit(parsedLine{number: 61, trimmed: `strategy.exit("Exit", "Long", stop=close, unknown=1)`})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), "argument unknown is not supported") {
				t.Fatalf("unknown named argument error = %v", err)
			}
		})
	}
}

func TestCoverage98SourceAnnotationsAndCommentsKeepTheirSeparateRoles(t *testing.T) {
	state := newParseState("// @entry_policy allow\nstrategy.entry(\"Long\", strategy.long)", nil, nil)
	if policy := state.readEntryPolicyForLine(2); policy != "allow" {
		t.Fatalf("entry policy annotation = %q", policy)
	}
	if policy := state.readEntryPolicyForLine(3); policy != "same_direction" {
		t.Fatalf("unannotated entry policy = %q", policy)
	}

	lines := tokenizeScript("// ordinary user comment\n//@version=6\nvalue = close")
	if len(lines) != 2 || lines[0].trimmed != "//@version=6" || lines[1].trimmed != "value = close" {
		t.Fatalf("tokenized comment boundary = %#v", lines)
	}
}
