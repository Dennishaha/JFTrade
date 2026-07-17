package pine

import (
	"errors"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestControlFlowParserRetainsUserFunctionAndCollectionLoopSemantics(t *testing.T) {
	state := newParseState("", []parsedLine{
		{number: 1, trimmed: "score(source, factor) =>"},
		{number: 2, trimmed: "base = source * factor", indent: 4},
		{number: 3, trimmed: "if base > 0", indent: 4},
		{number: 4, trimmed: "base", indent: 8},
		{number: 5, trimmed: "else", indent: 4},
		{number: 6, trimmed: "0", indent: 8},
	}, nil)
	handled, next, err := state.parseUDFDefinition(0)
	if err != nil || !handled || next != 6 {
		t.Fatalf("parseUDFDefinition handled=%v next=%d err=%v", handled, next, err)
	}
	udf := state.udfs["score"]
	if len(udf.Args) != 2 || !strings.Contains(udf.Body, "ifelse") || !strings.Contains(udf.Body, "source * factor") {
		t.Fatalf("compiled user function = %#v", udf)
	}

	for _, tc := range []struct {
		name  string
		lines []parsedLine
		want  string
	}{
		{
			name:  "nested functions remain unsupported",
			lines: []parsedLine{{number: 10, trimmed: "outer() => inner() => close"}},
			want:  "nested user-defined functions",
		},
		{
			name: "disabled helpers are rejected inside a multiline body",
			lines: []parsedLine{
				{number: 11, trimmed: "unsafe() =>"},
				{number: 12, trimmed: "history(close, 1)", indent: 4},
			},
			want: "internal JFTrade helper",
		},
		{
			name:  "function body needs an expression",
			lines: []parsedLine{{number: 13, trimmed: "empty() =>"}},
			want:  "requires one expression body",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := newParseState("", tc.lines, nil)
			_, _, parseErr := invalid.parseUDFDefinition(0)
			if parseErr == nil || !strings.Contains(parseErr.Error(), tc.want) {
				t.Fatalf("parse UDF error = %v, want %q", parseErr, tc.want)
			}
		})
	}

	loopState := newParseState("", []parsedLine{
		{number: 20, trimmed: "for [position, value] in values"},
		{number: 21, trimmed: "weighted = value + position", indent: 4},
		{number: 22, trimmed: "after = close"},
	}, nil)
	loopState.loopVariables["position"] = true // Preserve an outer loop variable after the nested loop closes.
	loopStatements, next, err := loopState.parseStaticForLoop(0)
	if err != nil || next != 2 || len(loopStatements) != 1 {
		t.Fatalf("parse collection loop statements=%#v next=%d err=%v", loopStatements, next, err)
	}
	loop, ok := loopStatements[0].(*strategyir.LoopStmt)
	if !ok || loop.IndexVariable != "position" || loop.Variable != "value" || loop.Collection != "values" || len(loop.Body) != 1 || !loopState.loopVariables["position"] || loopState.loopVariables["value"] {
		t.Fatalf("collection loop = %#v variables=%#v", loop, loopState.loopVariables)
	}

	for _, header := range []string{"for [index, value] values", "for index in close >"} {
		invalid := newParseState("", []parsedLine{{number: 30, trimmed: header}}, nil)
		if _, _, parseErr := invalid.parseStaticForLoop(0); parseErr == nil {
			t.Fatalf("invalid loop header %q was accepted", header)
		}
	}
}

func TestObjectLifecycleParserCoversFieldsMultilineMethodsAndReceivers(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()

	statement, handled, err := state.parseObjectFieldAssignment(parsedLine{number: 40, trimmed: "box.price := hl2"})
	if err != nil || !handled {
		t.Fatalf("parse object field assignment handled=%v err=%v", handled, err)
	}
	fieldUpdate, ok := statement.(*strategyir.ObjectStmt)
	if !ok || fieldUpdate.Operation != "field_set" || fieldUpdate.TypeName != "PriceBox" || fieldUpdate.Method != "price" || fieldUpdate.Arguments[0] != "hl2" {
		t.Fatalf("object field update = %#v", statement)
	}
	for _, tc := range []struct {
		line string
		want string
	}{
		{line: "box.price = close", want: "must use :="},
		{line: "box.unknown := close", want: "has no field"},
		{line: "box.price := close >", want: "object field assignment"},
	} {
		_, _, parseErr := state.parseObjectFieldAssignment(parsedLine{number: 41, trimmed: tc.line})
		if parseErr == nil || !strings.Contains(parseErr.Error(), tc.want) {
			t.Fatalf("field assignment %q error=%v, want %q", tc.line, parseErr, tc.want)
		}
	}

	normalizationFailure := newParseState("", []parsedLine{
		{number: 50, trimmed: "type Quote"},
		{number: 51, trimmed: "float price = close", indent: 4},
	}, nil)
	normalizationFailure.normalizationErr = errors.New("field normalization failed")
	if _, parseErr := normalizationFailure.parseExecutableTypeDefinition(0); parseErr == nil || !strings.Contains(parseErr.Error(), "field normalization failed") {
		t.Fatalf("type default normalization error = %v", parseErr)
	}

	multiline := newParseState("", []parsedLine{
		{number: 60, trimmed: "type Quote"},
		{number: 61, trimmed: "float price", indent: 4},
		{number: 62, trimmed: "method adjusted(Quote self, float factor = 2) =>"},
		{number: 63, trimmed: "scaled = self.price * factor", indent: 4},
		{number: 64, trimmed: "scaled", indent: 4},
	}, nil)
	if next, parseErr := multiline.parseExecutableTypeDefinition(0); parseErr != nil || next != 2 {
		t.Fatalf("prepare multiline method next=%d err=%v", next, parseErr)
	}
	if next, parseErr := multiline.parseExecutableMethodDefinition(2); parseErr != nil || next != 5 {
		t.Fatalf("parse multiline method next=%d err=%v", next, parseErr)
	}
	method := multiline.udtMethods["quote.adjusted"][0]
	if method.Body != "self.price * factor" || len(method.Parameters) != 1 || method.Parameters[0].Default != "2" {
		t.Fatalf("multiline method = %#v", method)
	}

	parameters := []strategyir.ObjectParameter{
		{Name: "factor", Type: "float", Default: "1"},
		{Name: "offset", Type: "float", Default: "0"},
	}
	if ordered, orderErr := reorderObjectMethodCallArguments([]string{"offset=4"}, parameters); orderErr != nil || strings.Join(ordered, ",") != "1,4" {
		t.Fatalf("reordered optional method args = %#v/%v", ordered, orderErr)
	}
	if _, orderErr := reorderObjectMethodCallArguments([]string{"unknown=4"}, parameters); orderErr == nil || !strings.Contains(orderErr.Error(), "unknown method argument") {
		t.Fatalf("unknown named method argument error = %v", orderErr)
	}

	lowered, lowerErr := state.lowerObjectMethodCalls(`box[1].score(offset=3) + object_method("PriceBox", "identity", box).score(2)`)
	if lowerErr != nil || strings.Count(lowered, `object_method("PriceBox", "score",`) != 2 || !strings.Contains(lowered, "history(box, 1)") {
		t.Fatalf("lowered historical and expression-receiver methods = %q/%v", lowered, lowerErr)
	}
	if _, _, _, _, _, found := state.nextObjectHistoryMethodCall("missing[1].score(2)"); found {
		t.Fatal("unknown historical object receiver was treated as a known object")
	}
}

func TestCollectionParserKeepsHistoryAndExpressionContracts(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	state.collectionNamespaces["mapping"] = "map"

	loweredHistory, err := state.lowerCollectionHistoryReadCalls("values[2].get(1) + values[1].size()")
	if err != nil || loweredHistory != "collection_array_get(history(values, 2), 1) + collection_array_size(history(values, 1))" {
		t.Fatalf("lowered collection history = %q/%v", loweredHistory, err)
	}
	for _, tc := range []struct {
		expression string
		want       string
	}{
		{expression: "unknown[1].get(0)", want: "requires a known collection"},
		{expression: "mapping[1].get(0)", want: "only for arrays"},
		{expression: "values[1].push(close)", want: "supports only read operations"},
	} {
		if _, lowerErr := state.lowerCollectionHistoryReadCalls(tc.expression); lowerErr == nil || !strings.Contains(lowerErr.Error(), tc.want) {
			t.Fatalf("history expression %q error=%v, want %q", tc.expression, lowerErr, tc.want)
		}
	}

	loweredRead, err := state.lowerCollectionReadCalls("array.get(values, 0) + values.size() + box.values.avg()")
	if err != nil || !strings.Contains(loweredRead, "collection_array_get(values, 0)") || !strings.Contains(loweredRead, "collection_array_size(values)") || !strings.Contains(loweredRead, "collection_array_avg(box.values)") {
		t.Fatalf("lowered collection reads = %q/%v", loweredRead, err)
	}
	if _, lowerErr := state.lowerCollectionReadCalls("array.get()"); lowerErr == nil || !strings.Contains(lowerErr.Error(), "not valid in an expression") {
		t.Fatalf("targetless collection read error = %v", lowerErr)
	}

	for _, tc := range []struct {
		line string
		ok   bool
	}{
		{line: "array<float> values = array.new_float(2)", ok: true},
		{line: "array.push(values, close)", ok: true},
		{line: "array.unknown(values)", ok: false},
		{line: "value = array.unknown(values)", ok: false},
	} {
		if got := supportedExecutableCollectionLine(tc.line); got != tc.ok {
			t.Fatalf("supportedExecutableCollectionLine(%q)=%v, want %v", tc.line, got, tc.ok)
		}
	}

	if namespace, operation, _, ok := state.resolveExecutableCollectionCall("box.values.size"); !ok || namespace != "array" || operation != "size" {
		t.Fatalf("object collection call = %q/%q/%v", namespace, operation, ok)
	}
	if got := collectionResultNamespace("map", "keys"); got != "array" {
		t.Fatalf("map.keys result namespace = %q", got)
	}
}
