package pine

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCoverage98ParserRejectsMalformedUDFAndStatementBoundaries(t *testing.T) {
	state := newParseState("", []parsedLine{{number: 10, trimmed: "history(close, 1)"}}, nil)
	if _, _, err := state.parseStatement(0); err == nil || !strings.Contains(err.Error(), "internal JFTrade helper") {
		t.Fatalf("disabled helper statement error = %v", err)
	}

	state = newParseState("", []parsedLine{{number: 11, trimmed: `value = request.security(syminfo.tickerid, "15", [close])`}}, nil)
	if _, _, err := state.parseStatement(0); err == nil || !strings.Contains(err.Error(), "request.security") {
		t.Fatalf("unsupported request.security statement error = %v", err)
	}

	for _, tc := range []struct {
		name  string
		lines []parsedLine
		want  string
	}{
		{
			name:  "inline disabled helper",
			lines: []parsedLine{{number: 20, trimmed: "unsafe() => history(close, 1)"}},
			want:  "internal JFTrade helper",
		},
		{
			name:  "control-flow cannot be an inline return expression",
			lines: []parsedLine{{number: 21, trimmed: "unsafe() => if close > open"}},
			want:  "single expression body",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newParseState("", tc.lines, nil)
			_, _, err := invalid.parseUDFDefinition(0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseUDFDefinition error = %v, want %q", err, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name  string
		lines []parsedLine
		want  string
	}{
		{
			name: "unexpected nested body indentation",
			lines: []parsedLine{
				{number: 30, trimmed: "base = close"},
				{number: 31, trimmed: "base", indent: 4},
			},
			want: "unexpected indentation",
		},
		{
			name: "missing else branch return",
			lines: []parsedLine{
				{number: 31, trimmed: "if close > open"},
				{number: 32, trimmed: "close", indent: 4},
				{number: 33, trimmed: "else"},
			},
			want: "if/else branch requires an expression",
		},
		{
			name: "return after final if else",
			lines: []parsedLine{
				{number: 34, trimmed: "if close > open"},
				{number: 35, trimmed: "close", indent: 4},
				{number: 36, trimmed: "else"},
				{number: 37, trimmed: "open", indent: 4},
				{number: 38, trimmed: "hl2"},
			},
			want: "final if/else return",
		},
		{
			name: "nested branch block",
			lines: []parsedLine{
				{number: 39, trimmed: "if close > open"},
				{number: 40, trimmed: "close", indent: 4},
				{number: 41, trimmed: "open", indent: 8},
			},
			want: "nested blocks in UDF branches",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compileUDFBody(tc.lines)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("compileUDFBody error = %v, want %q", err, tc.want)
			}
		})
	}

	if !isReservedUDFName("history") || isReservedUDFName("tradeScore") {
		t.Fatal("reserved UDF name checks no longer protect internal helper names")
	}
}

func TestCoverage98ObjectArgumentRecoveryRejectsOverrunAndPreservesDefaults(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	fields := state.udtTypes["pricebox"].Fields[:2]
	got, err := state.normalizeObjectConstructorArguments(50, []string{"price=1", "2"}, fields)
	if err != nil || !reflect.DeepEqual(got, []string{"1", "2"}) {
		t.Fatalf("mixed constructor arguments = %#v/%v", got, err)
	}
	if _, err := state.normalizeObjectConstructorArguments(51, []string{"price=1", "bars=2", "3"}, fields); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("constructor overrun error = %v", err)
	}

	parameters := []strategyir.ObjectParameter{
		{Name: "factor", Type: "float", Default: "1"},
		{Name: "offset", Type: "float", Default: "0"},
	}
	got, err = state.normalizeObjectMethodArguments(52, []string{"offset=4"}, parameters)
	if err != nil || !reflect.DeepEqual(got, []string{"1", "4"}) {
		t.Fatalf("method defaults = %#v/%v", got, err)
	}
	got, err = state.normalizeObjectMethodArguments(53, []string{"factor=2", "3"}, parameters)
	if err != nil || !reflect.DeepEqual(got, []string{"2", "3"}) {
		t.Fatalf("mixed method arguments = %#v/%v", got, err)
	}
	if _, err := state.normalizeObjectMethodArguments(54, []string{"factor=2", "offset=3", "4"}, parameters); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("method overrun error = %v", err)
	}

	state.normalizationErr = errors.New("object argument normalization failed")
	if _, err := state.normalizeObjectMethodArguments(55, []string{"factor=2"}, parameters); err == nil || !strings.Contains(err.Error(), "object argument normalization failed") {
		t.Fatalf("method normalization error = %v", err)
	}
	state.normalizationErr = errors.New("object field normalization failed")
	if _, _, err := state.parseObjectFieldAssignment(parsedLine{number: 56, trimmed: "box.price := close"}); err == nil || !strings.Contains(err.Error(), "object field normalization failed") {
		t.Fatalf("field normalization error = %v", err)
	}

	if _, _, _, _, _, found := state.nextObjectHistoryMethodCall("box[1].missing(1)"); found {
		t.Fatal("unknown historic object method was treated as executable")
	}
	if _, _, _, _, _, found := state.nextObjectMethodExpressionReceiverCall(`object_method("PriceBox", "identity", box`); found {
		t.Fatal("unterminated object_method receiver was treated as executable")
	}
	if _, _, _, _, found := state.nextObjectMethodCall("box.score("); found {
		t.Fatal("unterminated object method call was treated as executable")
	}
}

func TestCoverage98CollectionScannerRejectsDamagedAndUnknownCalls(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	state.normalizationErr = errors.New("typed collection normalization failed")
	if _, handled, err := state.parseTypedCollectionStatement(parsedLine{number: 70, trimmed: "array<float> samples = array.new_float(2)"}); !handled || err == nil || !strings.Contains(err.Error(), "typed collection normalization failed") {
		t.Fatalf("typed collection normalization = handled:%v err:%v", handled, err)
	}

	call, found := findCollectionHistoryCall(`values[1].get(0) + "values[2].last()" + values[3].last()`)
	if !found || call.name != "values" || call.lookback != "1" || call.operation != "get" {
		t.Fatalf("first collection history call = %#v/%v", call, found)
	}
	if got := collectionHistoryCallArgs("values[1].get("); got != "" {
		t.Fatalf("damaged collection call arguments = %q", got)
	}

	for _, tc := range []struct {
		call string
	}{
		{call: "box.values."},
		{call: "box.values.unknown"},
	} {
		if _, _, _, ok := state.resolveExecutableCollectionCall(tc.call); ok {
			t.Fatalf("unsupported collection receiver %q was accepted", tc.call)
		}
	}
	if supportedExecutableCollectionLine("close > open") {
		t.Fatal("non-collection expression was accepted as a collection statement")
	}

	for _, tc := range []struct {
		expression string
		wantFound  bool
	}{
		{expression: "array.get(values,", wantFound: false},
		{expression: "values.get(", wantFound: false},
		{expression: "box.unknown.get(0)", wantFound: false},
		{expression: "box.values.get(", wantFound: false},
	} {
		if _, _, _, _, found := nextCollectionReadCall(tc.expression, state.collectionNamespaces); found != tc.wantFound {
			t.Fatalf("nextCollectionReadCall(%q) found=%v, want %v", tc.expression, found, tc.wantFound)
		}
	}
}
