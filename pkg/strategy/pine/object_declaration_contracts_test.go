package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestObjectDeclarationContractsRejectMalformedDomainTypesAndMethods(t *testing.T) {
	newState := func(lines ...parsedLine) *parseState {
		return newParseState("", lines, nil)
	}

	for _, tc := range []struct {
		name  string
		lines []parsedLine
		parse func(*parseState) error
		want  string
	}{
		{
			name:  "type needs a valid identifier",
			lines: []parsedLine{{number: 1, trimmed: "type 123Quote"}},
			parse: func(state *parseState) error {
				_, err := state.parseExecutableTypeDefinition(0)
				return err
			},
			want: "invalid type name",
		},
		{
			name:  "type needs fields",
			lines: []parsedLine{{number: 2, trimmed: "type Quote"}},
			parse: func(state *parseState) error {
				_, err := state.parseExecutableTypeDefinition(0)
				return err
			},
			want: "requires at least one field",
		},
		{
			name: "type field must be typed",
			lines: []parsedLine{
				{number: 3, trimmed: "type Quote"},
				{number: 4, trimmed: "close", indent: 4},
			},
			parse: func(state *parseState) error {
				_, err := state.parseExecutableTypeDefinition(0)
				return err
			},
			want: "type field requires",
		},
		{
			name: "type fields cannot repeat",
			lines: []parsedLine{
				{number: 5, trimmed: "type Quote"},
				{number: 6, trimmed: "float price = close", indent: 4},
				{number: 7, trimmed: "int PRICE = 1", indent: 4},
			},
			parse: func(state *parseState) error {
				_, err := state.parseExecutableTypeDefinition(0)
				return err
			},
			want: "repeats field",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.parse(newState(tc.lines...))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	state := newState(
		parsedLine{number: 10, trimmed: "type Quote"},
		parsedLine{number: 11, trimmed: "float price = close", indent: 4},
		parsedLine{number: 12, trimmed: "int bars = 20", indent: 4},
		parsedLine{number: 13, trimmed: "method scaled(Quote self, float factor = 1) => self.price * factor"},
	)
	if next, err := state.parseExecutableTypeDefinition(0); err != nil || next != 3 {
		t.Fatalf("valid type declaration next=%d err=%v", next, err)
	}
	if _, err := state.parseExecutableTypeDefinition(0); err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("duplicate type error = %v", err)
	}
	if next, err := state.parseExecutableMethodDefinition(3); err != nil || next != 4 {
		t.Fatalf("valid method declaration next=%d err=%v", next, err)
	}
	method := state.udtMethods["quote.scaled"][0]
	if method.Body != "self.price * factor" || len(method.Parameters) != 1 || method.Parameters[0].Default != "1" {
		t.Fatalf("stored method = %#v", method)
	}

	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{name: "missing typed receiver", line: "method bad(self) => self", want: "requires a typed receiver"},
		{name: "unknown receiver type", line: "method bad(Missing self) => close", want: "is not declared"},
		{name: "missing body", line: "method bad(Quote self)", want: "requires one pure expression body"},
		{name: "recursive method", line: "method bad(Quote self) => bad(self)", want: "side-effect-free and non-recursive"},
		{name: "invalid default", line: "method bad(Quote self, float factor = close >) => close", want: "method parameter default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := newState(
				parsedLine{number: 20, trimmed: "type Quote"},
				parsedLine{number: 21, trimmed: "float price = close", indent: 4},
				parsedLine{number: 22, trimmed: tc.line},
			)
			if _, err := copy.parseExecutableTypeDefinition(0); err != nil {
				t.Fatalf("prepare type: %v", err)
			}
			if _, err := copy.parseExecutableMethodDefinition(2); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("method error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestObjectCallsPreserveNamedArgumentAndOverloadSafety(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	fields := state.udtTypes["pricebox"].Fields
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown constructor field", args: []string{"unknown=1"}, want: "unknown constructor field"},
		{name: "duplicate constructor field", args: []string{"price=1", "price=2"}, want: "duplicate constructor argument"},
		{name: "invalid constructor expression", args: []string{"price=close >"}, want: "constructor argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := state.normalizeObjectConstructorArguments(60, tc.args, fields)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("constructor error = %v, want %q", err, tc.want)
			}
		})
	}
	if _, _, err := state.parseObjectConstructorAssignment(
		parsedLine{number: 61, trimmed: "box = PriceBox.new(1, 2, 3, 4)"},
		"box",
		state.udtTypes["pricebox"],
		[]string{"1", "2", "3", "4"},
		strategyir.AssignmentModeLet,
	); err == nil || !strings.Contains(err.Error(), "expects at most") {
		t.Fatalf("too many constructor values error = %v", err)
	}

	parameters := []strategyir.ObjectParameter{
		{Name: "factor", Type: "float"},
		{Name: "offset", Type: "float", Default: "0"},
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown method parameter", args: []string{"unknown=1"}, want: "unknown method parameter"},
		{name: "duplicate method parameter", args: []string{"factor=1", "factor=2"}, want: "duplicate method argument"},
		{name: "invalid method expression", args: []string{"factor=close >"}, want: "method argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := state.normalizeObjectMethodArguments(70, tc.args, parameters)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("method error = %v, want %q", err, tc.want)
			}
		})
	}
	if _, _, err := state.parseObjectMethodAssignment(
		parsedLine{number: 71, trimmed: "result = box.score(1, 2, 3)"},
		"result",
		"box",
		"score",
		[]string{"1", "2", "3"},
		strategyir.AssignmentModeLet,
	); err == nil || !strings.Contains(err.Error(), "has no overload") {
		t.Fatalf("too many method arguments error = %v", err)
	}

	ordered, err := reorderObjectMethodCallArguments(
		[]string{"offset=3", "factor=2"},
		parameters,
	)
	if err != nil || strings.Join(ordered, ",") != "2,3" {
		t.Fatalf("reordered named arguments = %#v/%v", ordered, err)
	}
	if _, err := reorderObjectMethodCallArguments([]string{"offset=3"}, parameters); err == nil || !strings.Contains(err.Error(), "factor is required") {
		t.Fatalf("missing required named argument error = %v", err)
	}

	state.udtMethods["pricebox.score"] = []strategyir.MethodDefinition{{
		Name: "score", ReceiverType: "PriceBox", ReceiverName: "self", Parameters: parameters,
	}}
	lowered, err := state.lowerObjectMethodCalls("box.score(offset=3, factor=2) + box[1].score(4)")
	if err != nil {
		t.Fatalf("lowerObjectMethodCalls: %v", err)
	}
	if strings.Count(lowered, `object_method("PriceBox", "score",`) != 2 || !strings.Contains(lowered, ", 2, 3)") {
		t.Fatalf("lowered methods = %q", lowered)
	}
}
