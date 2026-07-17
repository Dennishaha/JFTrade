package pine

import (
	"errors"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCoverage98GeneralTupleAssignmentsKeepPineAliasContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{
			name: "too many aliases are rejected before execution",
			line: "[a, b, c, d, e, f, g, h, i] = [open, high, low, close, volume, hl2, hlc3, ohlc4, close]",
			want: "supports 2 to 8 aliases",
		},
		{
			name: "invalid alias cannot become a runtime variable",
			line: "[openValue, 4bad] = [open, close]",
			want: "invalid tuple alias",
		},
		{
			name: "tuple shape must match the returned values",
			line: "[first, second] = [close]",
			want: "tuple returns 1 values",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := newParseState("", nil, nil)
			statement, handled, err := state.parseGeneralTupleAssignment(parsedLine{number: 18, trimmed: tc.line})
			if statement != nil || !handled || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("tuple parse = %#v/%v/%v, want %q", statement, handled, err, tc.want)
			}
		})
	}

	state := newParseState("", nil, nil)
	statement, handled, err := state.parseGeneralTupleAssignment(parsedLine{number: 19, trimmed: "[current, previous] := [close, close[1]]"})
	if err != nil || !handled {
		t.Fatalf("tuple reassignment = %#v/%v/%v", statement, handled, err)
	}
	tuple, ok := statement.(*strategyir.TupleStmt)
	if !ok || tuple.Mode != strategyir.AssignmentModeReassign {
		t.Fatalf("tuple reassignment statement = %#v", statement)
	}

	state = newParseState("", nil, nil)
	state.normalizationErr = errors.New("unsupported tuple source")
	if _, handled, err := state.parseGeneralTupleAssignment(parsedLine{number: 20, trimmed: "[current, previous] = [close, close[1]]"}); !handled || err == nil || !strings.Contains(err.Error(), "unsupported tuple source") {
		t.Fatalf("tuple normalization failure = handled:%v err:%v", handled, err)
	}
}
