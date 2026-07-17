package pine

import (
	"errors"
	"strings"
	"testing"
)

func TestCoverage98OrderAndParserBoundaryContracts(t *testing.T) {
	t.Run("positional quantities retain Pine semantic modes", func(t *testing.T) {
		mode, quantity, ok := pineExplicitQuantity([]string{"strategy.equity * 25 / 100 / close"})
		if !ok || mode != "account_position_percent" || quantity != "25" {
			t.Fatalf("equity positional quantity = %q/%q/%v", mode, quantity, ok)
		}
		mode, quantity, ok = pineExplicitQuantity([]string{"contracts"})
		if !ok || mode != "shares" || quantity != "contracts" {
			t.Fatalf("share positional quantity = %q/%q/%v", mode, quantity, ok)
		}
	})

	t.Run("close metadata reports the original API boundary", func(t *testing.T) {
		if _, _, _, _, err := pineCloseMetadata(7, "strategy.close", []string{"disable_alert=maybe"}); err == nil || !strings.Contains(err.Error(), "strategy.close disable_alert") {
			t.Fatalf("pineCloseMetadata error = %v", err)
		}
		if _, _, _, _, err := pineCloseAllMetadata(8, []string{"disable_alert=maybe"}); err == nil || !strings.Contains(err.Error(), "strategy.close_all disable_alert") {
			t.Fatalf("pineCloseAllMetadata error = %v", err)
		}
	})

	t.Run("stale structured AST falls back to tokenized source", func(t *testing.T) {
		fallback := []parsedLine{{number: 1, trimmed: "//@version=6"}, {number: 2, trimmed: `strategy("Fallback")`}}
		ast := &AST{Nodes: []ASTNode{{Line: ASTLine{Line: 1}}, {Line: ASTLine{Line: 99}}}}
		got := parsedLinesFromStructuredAST(ast, fallback)
		if len(got) != len(fallback) || got[0].number != 1 || got[1].number != 2 {
			t.Fatalf("structured AST fallback = %#v, want %#v", got, fallback)
		}
	})

	t.Run("internal lowering rejects an empty executable source", func(t *testing.T) {
		if _, err := compileLoweredAST("", nil, nil); err == nil || !strings.Contains(err.Error(), "pine script is required") {
			t.Fatalf("compileLoweredAST empty error = %v", err)
		}
	})
}

func TestCoverage98DynamicForBoundsUseRuntimeFallback(t *testing.T) {
	state := newParseState("", nil, nil)
	for _, header := range []string{
		"for offset = close to 5",
		"for offset = 1 to 5 by int(close)",
	} {
		match, ok := parseStaticForLoopHeader(header)
		if !ok {
			t.Fatalf("parseStaticForLoopHeader(%q) did not match", header)
		}
		if _, err := state.parseStaticForLoopSpec(12, match); !errors.Is(err, errStaticForRuntimeFallback) {
			t.Fatalf("parseStaticForLoopSpec(%q) error = %v, want runtime fallback", header, err)
		}
	}
}

func TestCoverage98NormalizationPreservesInvalidUserSyntaxAndErrors(t *testing.T) {
	t.Run("object and collection argument errors reach the compiler", func(t *testing.T) {
		state := newObjectCollectionBoundaryParseState()
		for _, tc := range []struct {
			expression string
			want       string
		}{
			{expression: "box.score(missing=1)", want: "unknown method argument missing"},
			{expression: "array.get()", want: "collection operation array.get is not valid in an expression"},
		} {
			_, err := state.normalizeExpressionDepth(tc.expression, 0, map[string]bool{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("normalizeExpressionDepth(%q) error = %v, want %q", tc.expression, err, tc.want)
			}
		}
	})

	t.Run("user functions retain no-argument and nested validation contracts", func(t *testing.T) {
		state := newParseState("", nil, nil)
		state.udfs["zero"] = pineUDF{Name: "zero", Body: "close", Line: 1}
		state.udfs["identity"] = pineUDF{Name: "identity", Args: []string{"value"}, Body: "value", Line: 2}

		got, err := state.normalizeExpressionDepth("zero()", 0, map[string]bool{})
		if err != nil || got != "close" {
			t.Fatalf("zero-argument UDF = %q, %v", got, err)
		}
		if _, err := state.normalizeExpressionDepth("identity(ta.ema(close)[1])", 0, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "history references are supported only") {
			t.Fatalf("nested UDF history reference error = %v", err)
		}
	})
}

func TestCoverage98MalformedTAExpressionsRemainVisibleForValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{
			name: "vwap with an unclosed call",
			got:  replaceTASourceOptionalFunction("ta.vwap(close", "vwap", "vwap", "hlc3"),
			want: "ta.vwap(close",
		},
		{
			name: "anchored vwap with an unclosed call",
			got:  replaceTAAnchoredVWAP(`ta.vwap(close, timeframe.change("D")`),
			want: `ta.vwap(close, timeframe.change("D")`,
		},
		{
			name: "anchored vwap requires one timeframe argument",
			got:  replaceTAAnchoredVWAP("ta.vwap(close, timeframe.change())"),
			want: "ta.vwap(close, timeframe.change())",
		},
		{
			name: "highest bars with an unclosed call",
			got:  replaceTAExtremaBarsFunction("ta.highestbars(close", "highestbars"),
			want: "ta.highestbars(close",
		},
		{
			name: "stochastic with an unclosed call",
			got:  replaceTAStoch("ta.stoch(close, high"),
			want: "ta.stoch(close, high",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("malformed Pine expression was rewritten to %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestCoverage98ControlFlowErrorsRemainActionableBeforeRuntime(t *testing.T) {
	if _, err := compileUDFBody([]parsedLine{{number: 1, indent: 1, trimmed: ""}}); err == nil || !strings.Contains(err.Error(), "requires a return expression") {
		t.Fatalf("empty UDF body error = %v", err)
	}

	t.Run("dynamic for bounds retain normalization errors", func(t *testing.T) {
		for _, header := range []string{
			"for i = 0 to array.get()",
			"for i = 0 to 1 by array.get()",
		} {
			state := newObjectCollectionBoundaryParseState()
			match, ok := parseStaticForLoopHeader(header)
			if !ok {
				t.Fatalf("dynamic for header did not match: %q", header)
			}
			state.lines = []parsedLine{{number: 12, trimmed: header + ":", indent: 0}}
			if _, _, err := state.parseRuntimeForLoop(0, match); err == nil || !strings.Contains(err.Error(), "collection operation array.get") {
				t.Fatalf("parseRuntimeForLoop(%q) error = %v", header, err)
			}
		}
	})

	t.Run("collection and while loops restore state when their body is invalid", func(t *testing.T) {
		collectionHeader := "for item in values"
		match := collectionForLoopPattern.FindStringSubmatch(collectionHeader)
		if match == nil {
			t.Fatal("collection loop header did not match")
		}
		state := newParseState("", nil, nil)
		state.runtimeLoopDepth = maxRuntimeLoopDepth
		state.lines = []parsedLine{{number: 20, trimmed: collectionHeader, indent: 0}}
		if _, _, err := state.parseCollectionForLoop(0, match); err == nil || !strings.Contains(err.Error(), "dynamic loop nesting") {
			t.Fatalf("collection nesting error = %v", err)
		}

		state = newObjectCollectionBoundaryParseState()
		invalidCollectionHeader := "for item in array.get()"
		match = collectionForLoopPattern.FindStringSubmatch(invalidCollectionHeader)
		state.lines = []parsedLine{{number: 21, trimmed: invalidCollectionHeader, indent: 0}}
		if _, _, err := state.parseCollectionForLoop(0, match); err == nil || !strings.Contains(err.Error(), "collection operation array.get") {
			t.Fatalf("collection source normalization error = %v", err)
		}

		state = newParseState("", nil, nil)
		state.lines = []parsedLine{
			{number: 22, trimmed: collectionHeader, indent: 0},
			{number: 23, trimmed: "else", indent: 1},
		}
		if _, _, err := state.parseCollectionForLoop(0, matchForCollectionLoop(t, collectionHeader)); err == nil || !strings.Contains(err.Error(), "else must follow") {
			t.Fatalf("collection body error = %v", err)
		}
		if state.loopVariables["item"] {
			t.Fatal("collection-loop variable leaked after a rejected body")
		}

		state = newParseState("", nil, nil)
		state.lines = []parsedLine{
			{number: 24, trimmed: "while true:", indent: 0},
			{number: 25, trimmed: "else", indent: 1},
		}
		if _, _, err := state.parseRuntimeWhileLoop(0); err == nil || !strings.Contains(err.Error(), "else must follow") {
			t.Fatalf("while body error = %v", err)
		}
	})

	t.Run("switch lowering does not hide invalid source", func(t *testing.T) {
		state := newObjectCollectionBoundaryParseState()
		if _, err := state.lowerSwitchExpression(30, []switchArm{{line: parsedLine{number: 31}, value: "box.score(missing=1)"}}); err == nil || !strings.Contains(err.Error(), "unknown method argument") {
			t.Fatalf("switch value normalization error = %v", err)
		}
		state = newParseState("", nil, nil)
		if _, err := state.lowerSwitchStatement(32, []switchArm{{line: parsedLine{number: 33}, condition: "array.get()", value: "value = close"}}); err == nil || !strings.Contains(err.Error(), "collection operation array.get") {
			t.Fatalf("switch condition normalization error = %v", err)
		}
		if statement, err := state.lowerSwitchStatement(34, []switchArm{{line: parsedLine{number: 35}, value: "value = close"}}); err != nil || statement == nil {
			t.Fatalf("default-only switch statement = %#v, %v", statement, err)
		}
		if _, err := state.parseInlineSwitchStatement(switchArm{line: parsedLine{number: 36}, value: "unsupported()"}); err == nil || !strings.Contains(err.Error(), "unsupported switch statement") {
			t.Fatalf("unsupported switch statement error = %v", err)
		}
	})

	t.Run("empty if branches do not materialize a no-op runtime statement", func(t *testing.T) {
		state := newParseState("", nil, nil)
		state.lines = []parsedLine{
			{number: 40, trimmed: "if true:", indent: 0},
			{number: 41, trimmed: "else", indent: 0},
		}
		statement, next, err := state.parseIfStatement(0, state.lines[0])
		if err != nil || statement != nil || next != len(state.lines) {
			t.Fatalf("empty if/else = %#v next=%d err=%v", statement, next, err)
		}
	})
}

func matchForCollectionLoop(t *testing.T, header string) []string {
	t.Helper()
	match := collectionForLoopPattern.FindStringSubmatch(header)
	if match == nil {
		t.Fatalf("collection loop header did not match: %q", header)
	}
	return match
}
