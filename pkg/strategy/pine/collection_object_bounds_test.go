package pine

import (
	"reflect"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCollectionHelperBusinessBoundaries(t *testing.T) {
	for _, operation := range []string{"join", "median", "variance"} {
		if !collectionHistoryReadOperation(operation) {
			t.Fatalf("history read operation %s was rejected", operation)
		}
	}
	for _, operation := range []string{"set", "clear", "push"} {
		if collectionHistoryReadOperation(operation) {
			t.Fatalf("mutating history operation %s was accepted", operation)
		}
	}

	if !collectionConstructorOperation("array", "from") || collectionConstructorOperation("map", "from") {
		t.Fatalf("array.from constructor handling changed")
	}
	if functionCallNameText(" array.get(values, 0) ") != "array.get" || functionCallNameText("close") != "" {
		t.Fatalf("functionCallNameText no longer isolates executable calls")
	}

	resultNamespaces := map[string]string{
		collectionResultNamespace("map", "keys"):           "array",
		collectionResultNamespace("map", "copy"):           "map",
		collectionResultNamespace("matrix", "remove_row"):  "array",
		collectionResultNamespace("matrix", "copy"):        "matrix",
		collectionResultNamespace("array", "sort_indices"): "array",
	}
	for got, want := range resultNamespaces {
		if got != want {
			t.Fatalf("collection result namespace = %q, want %q", got, want)
		}
	}
	if namespace, operation, typeArgs, ok := parseExecutableCollectionCall("matrix.new<float>"); !ok || namespace != "matrix" || operation != "new" || typeArgs != "float" {
		t.Fatalf("parseExecutableCollectionCall matrix.new<float> = %q/%q/%q/%v", namespace, operation, typeArgs, ok)
	}
	if _, _, _, ok := parseExecutableCollectionCall("array.unsupported"); ok {
		t.Fatal("unsupported collection call parsed as executable")
	}
	for _, item := range []struct {
		typeName string
		want     string
	}{
		{"array<float>", "array"},
		{"map<string, float>", "map"},
		{"matrix<int>", "matrix"},
		{"line", ""},
	} {
		if got := collectionNamespaceFromType(item.typeName); got != item.want {
			t.Fatalf("collectionNamespaceFromType(%q) = %q, want %q", item.typeName, got, item.want)
		}
	}

	supportedLines := []string{
		"array<float> values = array.new_float(2, close)",
		"latest = array.get(values, 0) + array.last(values)",
		"array.push(values, close)",
	}
	for _, line := range supportedLines {
		if !supportedExecutableCollectionLine(line) {
			t.Fatalf("supported collection line rejected: %s", line)
		}
	}
	for _, line := range []string{"array.new_float(2)", "latest = array.unknown(values)"} {
		if supportedExecutableCollectionLine(line) {
			t.Fatalf("unsupported collection line accepted: %s", line)
		}
	}
	if !lineContainsOnlySupportedCollectionCalls("x = array.get(values, 0) + matrix.rows(grid)") {
		t.Fatal("supported expression collection calls rejected")
	}
	if lineContainsOnlySupportedCollectionCalls("x = array.get(values, 0) + array.unknown(values)") {
		t.Fatal("unsupported expression collection call accepted")
	}
}

func TestCollectionObjectFieldAndHistoryBoundaries(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()

	for _, item := range []struct {
		line      string
		result    string
		namespace string
		operation string
		target    string
	}{
		{"map<string, float> labels = map.new<string, float>()", "labels", "map", "new", ""},
		{"keys = labels.keys()", "keys", "map", "keys", "labels"},
		{"matrix<float> grid = matrix.new<float>(2, 2, close)", "grid", "matrix", "new", ""},
		{"row = grid.remove_row(0)", "row", "matrix", "remove_row", "grid"},
		{"box.values.push(close)", "", "array", "push", "box.values"},
	} {
		stmt, handled, err := state.parseCollectionStatement(parsedLine{number: 40, trimmed: item.line})
		if err != nil || !handled {
			t.Fatalf("parseCollectionStatement(%q) handled=%v err=%v", item.line, handled, err)
		}
		collection, ok := stmt.(*strategyir.CollectionStmt)
		if !ok || collection.ResultName != item.result || collection.Namespace != item.namespace ||
			collection.Operation != item.operation || collection.Target != item.target {
			t.Fatalf("collection statement for %q = %#v", item.line, stmt)
		}
	}
	if state.collectionNamespaces["keys"] != "array" || state.collectionNamespaces["row"] != "array" {
		t.Fatalf("derived collection namespaces = %#v", state.collectionNamespaces)
	}

	lowered, err := state.lowerCollectionHistoryReadCalls(`values[1].median() + values[2].variance()`)
	if err != nil {
		t.Fatalf("lowerCollectionHistoryReadCalls: %v", err)
	}
	for _, fragment := range []string{
		"collection_array_median(history(values, 1))",
		"collection_array_variance(history(values, 2))",
	} {
		if !strings.Contains(lowered, fragment) {
			t.Fatalf("history collection expression = %q, missing %q", lowered, fragment)
		}
	}
	for _, item := range []struct {
		expression string
		message    string
	}{
		{`missing[1].get(0)`, "known collection variable"},
		{`labels[1].size()`, "supported only for arrays"},
		{`values[1].push(close)`, "supports only read operations"},
	} {
		if _, err := state.lowerCollectionHistoryReadCalls(item.expression); err == nil || !strings.Contains(err.Error(), item.message) {
			t.Fatalf("history expression %q err = %v, want %q", item.expression, err, item.message)
		}
	}
}

func TestObjectDefinitionAndMethodArgumentBoundaries(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	state.lines = []parsedLine{
		{number: 1, trimmed: "type TradeBox", indent: 0},
		{number: 2, trimmed: "float price = close", indent: 4},
		{number: 3, trimmed: "int bars = 1", indent: 4},
		{number: 4, trimmed: "method carry(TradeBox self)", indent: 0},
		{number: 5, trimmed: "self", indent: 4},
	}

	next, err := state.parseExecutableTypeDefinition(0)
	if err != nil || next != 3 {
		t.Fatalf("parseExecutableTypeDefinition next=%d err=%v", next, err)
	}
	if got := state.udtTypes["tradebox"]; got.Name != "TradeBox" || len(got.Fields) != 2 || got.Fields[0].Default != "close" {
		t.Fatalf("registered type = %#v", got)
	}
	next, err = state.parseExecutableMethodDefinition(3)
	if err != nil || next != 5 {
		t.Fatalf("parseExecutableMethodDefinition next=%d err=%v", next, err)
	}
	if got := state.udtMethods["tradebox.carry"]; len(got) != 1 || got[0].Body != "self" {
		t.Fatalf("registered method = %#v", got)
	}

	priceBox := state.udtTypes["pricebox"]
	if _, err := state.normalizeObjectConstructorArguments(10, []string{"price=close >"}, priceBox.Fields); err == nil || !strings.Contains(err.Error(), "invalid constructor argument") {
		t.Fatalf("invalid constructor expression err = %v", err)
	}
	if _, err := state.normalizeObjectMethodArguments(11, []string{"factor=close >"}, state.udtMethods["pricebox.score"][0].Parameters); err == nil || !strings.Contains(err.Error(), "invalid method argument") {
		t.Fatalf("invalid method expression err = %v", err)
	}

	if got := objectMethodExpressionReceiverType(`object_method("PriceBox", "identity", box)`); got != "PriceBox" {
		t.Fatalf("objectMethodExpressionReceiverType = %q", got)
	}
	for _, expression := range []string{`object_method()`, `box.score(1)`} {
		if got := objectMethodExpressionReceiverType(expression); got != "" {
			t.Fatalf("objectMethodExpressionReceiverType(%q) = %q, want empty", expression, got)
		}
	}
	if args, err := reorderObjectMethodCallArguments([]string{"2", "offset=3"}, state.udtMethods["pricebox.score"][0].Parameters); err != nil || !reflect.DeepEqual(args, []string{"2", "3"}) {
		t.Fatalf("mixed reorder args = %#v err=%v", args, err)
	}
	if _, err := reorderObjectMethodCallArguments([]string{"factor=1", "offset=2", "3"}, state.udtMethods["pricebox.score"][0].Parameters); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("too many reorder args err = %v", err)
	}
}
