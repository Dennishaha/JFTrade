package pine

import (
	"strings"
	"testing"
)

func TestCompileRejectsInvalidControlFlowAndUDFContracts(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "break outside loop", body: "break", message: "break is not supported"},
		{name: "continue outside loop", body: "continue", message: "continue is not supported"},
		{name: "orphan else", body: "else", message: "else must follow an if block"},
		{name: "unknown executable", body: "broker.submit()", message: "unsupported executable statement"},
		{name: "nested udf", body: "if close > open\n    helper(x) => x", message: "supported only at top level"},
		{name: "reserved udf name", body: "ta(x) => x", message: "conflicts with a JFTrade/Pine built-in"},
		{name: "invalid udf argument", body: "helper(x + 1) => x", message: "invalid user-defined function argument"},
		{name: "duplicate udf argument", body: "helper(x, x) => x", message: "duplicate user-defined function argument"},
		{name: "missing udf body", body: "helper(x) =>", message: "requires one expression body"},
		{name: "nested udf expression", body: "helper(x) => inner(y) => y", message: "nested user-defined functions"},
		{name: "udf if missing else", body: "helper(x) =>\n    if x > 0\n        x", message: "requires else"},
		{name: "udf local reassignment", body: "helper(x) =>\n    value = x\n    value := value + 1\n    value", message: "local reassignment is not supported"},
		{name: "udf return not last", body: "helper(x) =>\n    x\n    x + 1", message: "return expression must be the last"},
		{name: "malformed for header", body: "for i from 1 to 3\n    log.info(\"loop\")", message: "for loop must use"},
		{name: "unreachable ascending loop", body: "for i = 3 to 1 by 1\n    log.info(\"loop\")", message: "step does not reach the end value"},
		{name: "unreachable descending loop", body: "for i = 1 to 3 by -1\n    log.info(\"loop\")", message: "step does not reach the end value"},
		{name: "nested loop depth", body: "for i = 0 to 1\n    for j = 0 to 1\n        for k = 0 to 1\n            log.info(\"loop\")", message: "nested for loops deeper than"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := "//@version=6\nstrategy(\"Control flow rejection\", overlay=true)\n" + tc.body
			_, err := Compile(script)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("Compile() error = %v, want message containing %q", err, tc.message)
			}
		})
	}
}

func TestCompilePreservesDescendingAndConditionalLoopSemantics(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Loop semantics", overlay=true)
for i = 3 to 1 by -1
    if close > open
        continue
    strategy.entry("Long", strategy.long, qty=i)
while close > open
    break`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks) != 1 || len(compilation.Program.Hooks[0].Statements) == 0 {
		t.Fatalf("compiled hooks = %#v", compilation.Program.Hooks)
	}
}
