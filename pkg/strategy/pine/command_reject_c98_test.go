package pine

import (
	"strings"
	"testing"
)

// These are parser-level contracts rather than helper-only checks: callers must
// receive the same actionable error when invalid order metadata reaches each
// Pine command form.
func TestCoverage98OrderCommandRejectionsPropagateToPineCallers(t *testing.T) {
	state := newParseState("", nil, nil)

	for _, tc := range []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "entry invalid disable alert",
			call: func() error {
				_, err := state.parseStrategyEntryCall(parsedLine{number: 10, trimmed: `strategy.entry("long", strategy.long, disable_alert=maybe)`})
				return err
			},
			want: "disable_alert must be true or false",
		},
		{
			name: "order invalid disable alert",
			call: func() error {
				_, err := state.parseStrategyOrderCall(parsedLine{number: 11, trimmed: `strategy.order("net", strategy.long, disable_alert=maybe)`})
				return err
			},
			want: "disable_alert must be true or false",
		},
		{
			name: "close unsupported argument",
			call: func() error {
				_, err := state.parseStrategyCloseCall(parsedLine{number: 12, trimmed: `strategy.close("long", unexpected=true)`})
				return err
			},
			want: "argument unexpected is not supported",
		},
		{
			name: "close invalid immediate",
			call: func() error {
				_, err := state.parseStrategyCloseCall(parsedLine{number: 13, trimmed: `strategy.close("long", immediately=maybe)`})
				return err
			},
			want: "immediately must be true or false",
		},
		{
			name: "bracket exit invalid metadata",
			call: func() error {
				_, err := state.parseStrategyBracketExit(
					parsedLine{number: 14, trimmed: `strategy.exit("exit", "long", stop=close, disable_alert=maybe)`},
					"exit", "long", "long", []string{"stop=close", "disable_alert=maybe"}, "shares", "1", "close", "", "", "",
				)
				return err
			},
			want: "disable_alert must be true or false",
		},
		{
			name: "trailing exit invalid metadata",
			call: func() error {
				_, err := state.parseStrategyTrailingExit(
					parsedLine{number: 15, trimmed: `strategy.exit("trail", "long", trail_points=2, trail_offset=1, disable_alert=maybe)`},
					"trail", "long", "long", []string{"trail_points=2", "trail_offset=1", "disable_alert=maybe"}, "shares", "1",
				)
				return err
			},
			want: "disable_alert must be true or false",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCoverage98RequestSecurityTupleValidationKeepsParserBoundaries(t *testing.T) {
	validTuple := parsedLine{number: 20, trimmed: `[openValue, closeValue] = request.security(syminfo.tickerid, "60", [open, close])`}
	if diagnostic, ok := requestSecurityTupleDiagnostic(validTuple, "[open, close]"); ok || diagnostic != (Diagnostic{}) {
		t.Fatalf("matching tuple should not produce a diagnostic: %#v/%v", diagnostic, ok)
	}

	if _, ok := requestSecurityArgsFromLine("request.security("); ok {
		t.Fatal("unterminated request.security call must not be accepted")
	}

	if requestSecurityExpressionHasUnsupportedTACall("ta.sma(close, 5") != true {
		t.Fatal("unterminated TA call must be rejected in request.security expression")
	}
}
