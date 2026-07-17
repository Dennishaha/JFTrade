package pine

import (
	"strings"
	"testing"
)

func TestCoverage98UDFExpansionRejectsMalformedRecursiveAndDeepCalls(t *testing.T) {
	newState := func(body string) *parseState {
		state := newParseState("", nil, nil)
		state.udfs["f"] = pineUDF{Name: "f", Args: []string{"value"}, Body: body, Line: 1}
		return state
	}

	for _, tc := range []struct {
		name       string
		expression string
		body       string
		depth      int
		want       string
	}{
		{name: "unclosed call", expression: "f(", body: "value + 1", want: "invalid call"},
		{name: "missing argument", expression: "f()", body: "value + 1", want: "expects 1 arguments, got 0"},
		{name: "extra argument", expression: "f(1, 2)", body: "value + 1", want: "expects 1 arguments, got 2"},
		{name: "recursive body", expression: "f(1)", body: "f(value)", want: "recursive user-defined function"},
		{name: "excessive nesting", expression: "f(1)", body: "value + 1", depth: maxUDFExpansionDepth + 1, want: "expansion exceeded depth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newState(tc.body).expandUDFCalls(tc.expression, tc.depth, map[string]bool{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expandUDFCalls(%q) error = %v, want %q", tc.expression, err, tc.want)
			}
		})
	}
}

func TestCoverage98UDFExpansionSkipsMembersAndExpandsStandaloneCalls(t *testing.T) {
	state := newParseState("", nil, nil)
	state.udfs["f"] = pineUDF{Name: "f", Args: []string{"value"}, Body: "value + 1", Line: 1}

	if start := state.findUDFCallStart("obj.f(1)", "f"); start >= 0 {
		t.Fatalf("member call was treated as UDF invocation at %d", start)
	}
	expanded, err := state.expandUDFCalls("obj.f(1) + f(2)", 0, map[string]bool{})
	if err != nil {
		t.Fatalf("expandUDFCalls returned error: %v", err)
	}
	if expanded != "obj.f(1) + (2 + 1)" {
		t.Fatalf("expanded expression = %q", expanded)
	}
}
