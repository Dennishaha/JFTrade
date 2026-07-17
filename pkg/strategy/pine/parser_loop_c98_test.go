package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCoverage98MalformedNamespaceCallsRemainVisibleForValidation(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		lower      func(string) string
	}{
		{name: "input member without call", expression: "input.float", lower: lowerInputCalls},
		{name: "input with unclosed call", expression: "input.float(10", lower: lowerInputCalls},
		{name: "string tostring with unclosed call", expression: "str.tostring(close", lower: replaceStringNamespace},
		{name: "string helper with unclosed call", expression: "str.upper(title", lower: replaceStringNamespace},
		{name: "timeframe helper with unclosed call", expression: "timeframe.change(\"D\"", lower: replaceTimeframeNamespace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.lower(test.expression); got != test.expression {
				t.Fatalf("malformed source was changed: got %q want %q", got, test.expression)
			}
		})
	}

	if got := lowerInputCalls("input()"); got != "na" {
		t.Fatalf("input() default = %q, want na", got)
	}
	if got := lowerInputCalls("input.float(defval = close)"); got != "close" {
		t.Fatalf("input.float(defval = close) = %q, want close", got)
	}
}

func TestCoverage98StaticLoopBoundsRejectNonTerminatingUserRanges(t *testing.T) {
	for _, test := range []struct {
		name       string
		start, end int
		step       int
		message    string
	}{
		{name: "zero step", start: 0, end: 3, step: 0, message: "step cannot be 0"},
		{name: "opposite direction", start: 3, end: 0, step: 1, message: "does not reach"},
		{name: "non-divisible range", start: 0, end: 3, step: 2, message: "does not reach"},
		{name: "too many iterations", start: 0, end: maxStaticForIterations, step: 1, message: "more than"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := expandStaticForLoopValues(12, test.start, test.end, test.step); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expandStaticForLoopValues(%d, %d, %d) = %v", test.start, test.end, test.step, err)
			}
		})
	}

	values, err := expandStaticForLoopValues(13, 3, 0, -1)
	if err != nil || len(values) != 4 || values[0] != 3 || values[len(values)-1] != 0 {
		t.Fatalf("descending static range = %#v, %v", values, err)
	}
}

func TestCoverage98TupleIndicatorsExposeUnsupportedCallHistory(t *testing.T) {
	line := parsedLine{number: 42, trimmed: "[value, aux] = ta.dmi(close, 14)"}
	history := "ta.sma(close, 2)[1]"
	tests := []struct {
		name  string
		parse func(*parseState) (strategyir.Statement, bool, error)
	}{
		{
			name: "Bollinger",
			parse: func(state *parseState) (strategyir.Statement, bool, error) {
				return state.parseBollingerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"close", history, "2"}, "ta.bb(close, 20, 2)")
			},
		},
		{
			name: "DMI",
			parse: func(state *parseState) (strategyir.Statement, bool, error) {
				return state.parseDMITupleAssignment(line, []string{"plus", "minus", "adx"}, []string{history, "14"}, "ta.dmi(close, 14)")
			},
		},
		{
			name: "Supertrend",
			parse: func(state *parseState) (strategyir.Statement, bool, error) {
				return state.parseSupertrendTupleAssignment(line, []string{"trend", "direction"}, []string{history, "14"}, "ta.supertrend(3, 14)")
			},
		},
		{
			name: "Keltner",
			parse: func(state *parseState) (strategyir.Statement, bool, error) {
				return state.parseKeltnerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"close", "20", "2", history}, "ta.kc(close, 20, 2, true)")
			},
		},
		{
			name: "MACD",
			parse: func(state *parseState) (strategyir.Statement, bool, error) {
				return state.parseMACDTupleAssignment(line, []string{"macd", "signal", "histogram"}, []string{"close", history, "26", "9"}, "ta.macd(close, 12, 26, 9)")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement, handled, err := test.parse(newParseState("", nil, nil))
			if statement != nil || !handled || err == nil || !strings.Contains(err.Error(), "history references") {
				t.Fatalf("tuple parse statement=%#v handled=%v err=%v", statement, handled, err)
			}
		})
	}

	state := newParseState("", nil, nil)
	if _, err := state.normalizeTupleExpressions(line.number, []string{history}); err == nil || !strings.Contains(err.Error(), "history references") {
		t.Fatalf("normalizeTupleExpressions did not expose unsupported history: %v", err)
	}
}
