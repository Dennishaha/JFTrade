package pine

import (
	"errors"
	"strings"
	"testing"
)

func TestCoverage98UnsupportedSyntaxDiagnosticsDescribeUnsafeRequestSecurityContracts(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{
			name: `request.security requires a parsable argument list`,
			line: `value = request.security(`,
			want: "could not be parsed",
		},
		{
			name: `request.security requires symbol timeframe and expression`,
			line: `value = request.security(syminfo.tickerid, "60")`,
			want: "requires symbol, timeframe, and expression",
		},
		{
			name: `request.security rejects dynamic symbols`,
			line: `value = request.security(otherSymbol, "60", close)`,
			want: "only syminfo.tickerid",
		},
		{
			name: `request.security rejects dynamic timeframes`,
			line: `value = request.security(syminfo.tickerid, "2", close)`,
			want: "only static timeframe strings",
		},
		{
			name: `request.security rejects nested reads`,
			line: `value = request.security(syminfo.tickerid, "60", request.security(syminfo.tickerid, "15", close))`,
			want: "nested request.security",
		},
		{
			name: `request.security rejects side effects`,
			line: `value = request.security(syminfo.tickerid, "60", strategy.entry("long", strategy.long))`,
			want: "expression must be pure",
		},
		{
			name: `request.security rejects unsupported tuple assignments`,
			line: `[onlyOne] = request.security(syminfo.tickerid, "60", [close, high])`,
			want: "tuple returns 2 values but assignment has 1 aliases",
		},
		{
			name: `collection mutation stays non executable`,
			line: `array.unknown(values)`,
			want: "collection namespaces",
		},
		{
			name: `function result history must first be assigned`,
			line: `value = ta.sma(close, 20)[1]`,
			want: "assign the function result first",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic, ok := unsupportedSyntaxDiagnostic(parsedLine{number: 42, trimmed: tc.line})
			if !ok || !strings.Contains(diagnostic.Message, tc.want) {
				t.Fatalf("unsupportedSyntaxDiagnostic(%q) = %#v/%v, want %q", tc.line, diagnostic, ok, tc.want)
			}
		})
	}

	state := newParseState("", nil, nil)
	state.normalizationErr = errors.New("normalizer rejected request-security expression")
	if err := state.rejectUnsupported(parsedLine{number: 77, trimmed: `value = request.security(syminfo.tickerid, "60", close)`}); err == nil || !strings.Contains(err.Error(), "normalizer rejected") {
		t.Fatalf("request.security normalization error = %v", err)
	}
	if err := newParseState("", nil, nil).rejectUnsupported(parsedLine{number: 78, trimmed: `value = request.security(syminfo.tickerid, selectedTimeframe, close)`}); err != nil {
		t.Fatalf("timeframe alias request-security fallback error = %v", err)
	}
}

func TestCoverage98TupleHelpersRejectMalformedIndicatorArityWithoutInventingAliases(t *testing.T) {
	newState := func() *parseState {
		return newParseState("", nil, nil)
	}
	line := parsedLine{number: 15, trimmed: "[first, second, third] = value"}
	for _, tc := range []struct {
		name  string
		parse func(*parseState) (any, bool, error)
		want  string
	}{
		{
			name: "Bollinger requires source length and multiplier",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseBollingerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"close", "20"}, "ta.bb(close, 20)")
				return statement, handled, err
			},
			want: "requires three arguments",
		},
		{
			name: "MTF Bollinger keeps its four argument shape",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseBollingerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"20", "2", `"60"`}, `bollinger(20, 2, "60")`)
				return statement, handled, err
			},
			want: "requires length, multiplier, static timeframe, and source",
		},
		{
			name: "DMI requires both smoothing inputs",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseDMITupleAssignment(line, []string{"plus", "minus", "adx"}, []string{"14"}, "ta.dmi(14)")
				return statement, handled, err
			},
			want: "requires two arguments",
		},
		{
			name: "Supertrend requires factor and ATR period",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseSupertrendTupleAssignment(line, []string{"trend", "direction"}, []string{"3"}, "ta.supertrend(3)")
				return statement, handled, err
			},
			want: "requires two arguments",
		},
		{
			name: "Keltner rejects missing multiplier",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseKeltnerTupleAssignment(line, []string{"basis", "upper", "lower"}, []string{"close", "20"}, "ta.kc(close, 20)")
				return statement, handled, err
			},
			want: "requires three or four arguments",
		},
		{
			name: "MACD requires all source and period inputs",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseMACDTupleAssignment(line, []string{"macd", "signal", "histogram"}, []string{"close", "12", "26"}, "ta.macd(close, 12, 26)")
				return statement, handled, err
			},
			want: "requires four arguments",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statement, handled, err := tc.parse(newState())
			if statement != nil || !handled || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("tuple rejection = %#v/%v/%v, want %q", statement, handled, err, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name  string
		parse func(*parseState) (any, bool, error)
	}{
		{
			name: "unrelated expression is left to ordinary assignment parser",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseBollingerTupleAssignment(line, []string{"basis"}, nil, "custom_indicator(close)")
				return statement, handled, err
			},
		},
		{
			name: "unrelated DMI expression is not claimed",
			parse: func(state *parseState) (any, bool, error) {
				statement, handled, err := state.parseDMITupleAssignment(line, []string{"plus"}, nil, "custom_indicator(close)")
				return statement, handled, err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statement, handled, err := tc.parse(newState())
			if statement != nil || handled || err != nil {
				t.Fatalf("ordinary expression parser handoff = %#v/%v/%v", statement, handled, err)
			}
		})
	}

	state := newState()
	state.normalizationErr = errors.New("tuple normalization failed")
	if _, handled, err := state.parseRequestSecurityTupleAssignment(line, []string{"first", "second"}, `request.security(syminfo.tickerid, "60", [close, high])`); !handled || err == nil || !strings.Contains(err.Error(), "tuple normalization failed") {
		t.Fatalf("request-security tuple normalization = handled=%v err=%v", handled, err)
	}
}

func TestCoverage98TALoweringLeavesMalformedCallsUntouched(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  func() string
		want string
	}{
		{name: "generic TA function unmatched parenthesis", got: func() string { return replaceTAFunction("ta.rsi(close", "rsi", "rsi(${period})") }, want: "ta.rsi(close"},
		{name: "Bollinger unmatched parenthesis", got: func() string { return replaceTABollinger("ta.bb(close") }, want: "ta.bb(close"},
		{name: "moving average unmatched parenthesis", got: func() string { return replaceTAMovingAverageFunction("ta.ema(close", "ema", "EMA") }, want: "ta.ema(close"},
		{name: "source length unmatched parenthesis", got: func() string { return replaceTASourceLengthFunction("ta.rsi(close", "rsi", "rsi", "close", "14") }, want: "ta.rsi(close"},
		{name: "optional source has too many arguments", got: func() string { return replaceTASourceOptionalFunction("ta.obv(close, high)", "obv", "obv", "close") }, want: "ta.obv(close, high)"},
		{name: "anchored VWAP rejects unsupported anchor", got: func() string { return replaceTAAnchoredVWAP(`ta.vwap(close, timeframe.change("60"))`) }, want: `ta.vwap(close, timeframe.change("60"))`},
		{name: "source required unmatched parenthesis", got: func() string { return replaceTASourceRequiredFunction("ta.mfi(close", "mfi", "mfi") }, want: "ta.mfi(close"},
		{name: "state function unmatched parenthesis", got: func() string { return replaceTAStateFunction("ta.cum(close", "cum") }, want: "ta.cum(close"},
		{name: "extrema requires exactly two arguments", got: func() string { return replaceTAExtremaBarsFunction("ta.highestbars(close)", "highestbars") }, want: "ta.highestbars(close)"},
		{name: "stoch requires exactly four arguments", got: func() string { return replaceTAStoch("ta.stoch(close, high, low)") }, want: "ta.stoch(close, high, low)"},
		{name: "window unmatched parenthesis", got: func() string { return replaceTAWindowFunction("ta.highest(close", "highest") }, want: "ta.highest(close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got(); got != tc.want {
				t.Fatalf("malformed lowering = %q, want %q", got, tc.want)
			}
		})
	}
	if got := replaceTAFunction("ta.rsi(hl2, 9)", "rsi", "rsi(${period})"); got != "rsi(9)" {
		t.Fatalf("source-aware generic TA period lowering = %q", got)
	}
}
