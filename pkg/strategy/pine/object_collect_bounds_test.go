package pine

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestObjectNamedArgumentNormalizationBusinessBoundaries(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	priceBox := state.udtTypes["pricebox"]

	constructorArgs, err := state.normalizeObjectConstructorArguments(10, []string{"bars=3"}, priceBox.Fields)
	if err != nil {
		t.Fatalf("constructor named args: %v", err)
	}
	if want := []string{"close", "3"}; !reflect.DeepEqual(constructorArgs, want) {
		t.Fatalf("constructor args = %#v, want %#v", constructorArgs, want)
	}
	constructorArgs, err = state.normalizeObjectConstructorArguments(11, []string{"1", "bars=2"}, priceBox.Fields)
	if err != nil {
		t.Fatalf("constructor mixed args: %v", err)
	}
	if want := []string{"1", "2"}; !reflect.DeepEqual(constructorArgs, want) {
		t.Fatalf("mixed constructor args = %#v, want %#v", constructorArgs, want)
	}
	if _, err := state.normalizeObjectConstructorArguments(12, []string{"price=close", "price=open"}, priceBox.Fields); err == nil || !strings.Contains(err.Error(), "duplicate constructor argument price") {
		t.Fatalf("duplicate constructor err = %v", err)
	}
	if _, err := state.normalizeObjectConstructorArguments(13, []string{"missing=1"}, priceBox.Fields); err == nil || !strings.Contains(err.Error(), "unknown constructor field missing") {
		t.Fatalf("unknown constructor err = %v", err)
	}

	scoreParams := state.udtMethods["pricebox.score"][0].Parameters
	methodArgs, err := state.normalizeObjectMethodArguments(20, []string{"offset=2", "factor=1.5"}, scoreParams)
	if err != nil {
		t.Fatalf("method named args: %v", err)
	}
	if want := []string{"1.5", "2"}; !reflect.DeepEqual(methodArgs, want) {
		t.Fatalf("method args = %#v, want %#v", methodArgs, want)
	}
	requiredParams := []strategyir.ObjectParameter{{Name: "factor", Type: "float"}, {Name: "offset", Type: "float", Default: "0"}}
	if _, err := state.normalizeObjectMethodArguments(21, []string{"offset=2"}, requiredParams); err == nil || !strings.Contains(err.Error(), "method argument factor is required") {
		t.Fatalf("missing required method arg err = %v", err)
	}
	if _, err := state.normalizeObjectMethodArguments(22, []string{"factor=1", "factor=2"}, scoreParams); err == nil || !strings.Contains(err.Error(), "duplicate method argument factor") {
		t.Fatalf("duplicate method arg err = %v", err)
	}
	if _, err := state.normalizeObjectMethodArguments(23, []string{"extra=1"}, scoreParams); err == nil || !strings.Contains(err.Error(), "unknown method parameter extra") {
		t.Fatalf("unknown method arg err = %v", err)
	}
}

func TestObjectMethodLoweringHandlesHistoryNamedDefaultsAndExpressionReceivers(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	expression := `box.score(offset=3, factor=2) + box[1].score(2) + object_method("PriceBox", "identity", box).score(offset=4)`

	lowered, err := state.lowerObjectMethodCalls(expression)
	if err != nil {
		t.Fatalf("lowerObjectMethodCalls: %v", err)
	}
	for _, fragment := range []string{
		`object_method("PriceBox", "score", box, 2, 3)`,
		`object_method("PriceBox", "score", history(box, 1), 2)`,
		`object_method("PriceBox", "score", object_method("PriceBox", "identity", box), 1, 4)`,
	} {
		if !strings.Contains(lowered, fragment) {
			t.Fatalf("lowered expression = %q, missing %q", lowered, fragment)
		}
	}

	if _, err := reorderObjectMethodCallArguments([]string{"missing=1"}, state.udtMethods["pricebox.score"][0].Parameters); err == nil || !strings.Contains(err.Error(), "unknown method argument missing") {
		t.Fatalf("unknown reorder arg err = %v", err)
	}
	if _, err := reorderObjectMethodCallArguments([]string{"offset=2"}, []strategyir.ObjectParameter{{Name: "factor"}, {Name: "offset", Default: "0"}}); err == nil || !strings.Contains(err.Error(), "method argument factor is required") {
		t.Fatalf("missing reorder arg err = %v", err)
	}
}

func TestCollectionStatementsAndReadLoweringBusinessBoundaries(t *testing.T) {
	state := newObjectCollectionBoundaryParseState()
	stmt, handled, err := state.parseCollectionStatement(parsedLine{number: 30, trimmed: "var array<float> values = array.new_float(2, close)"})
	if err != nil || !handled {
		t.Fatalf("typed array statement handled=%v err=%v", handled, err)
	}
	collection, ok := stmt.(*strategyir.CollectionStmt)
	if !ok || collection.Namespace != "array" || collection.Operation != "new_float" || collection.ResultName != "values" || collection.TypeArgs != "float" || collection.Mode != strategyir.AssignmentModeVar {
		t.Fatalf("typed array statement = %#v", stmt)
	}

	stmt, handled, err = state.parseCollectionStatement(parsedLine{number: 31, trimmed: "latest = values.get(0)"})
	if err != nil || !handled {
		t.Fatalf("method read statement handled=%v err=%v", handled, err)
	}
	collection, ok = stmt.(*strategyir.CollectionStmt)
	if !ok || collection.Target != "values" || collection.Operation != "get" || collection.ResultName != "latest" {
		t.Fatalf("method read statement = %#v", stmt)
	}
	stmt, handled, err = state.parseCollectionStatement(parsedLine{number: 32, trimmed: "values.push(close)"})
	if err != nil || !handled {
		t.Fatalf("method mutation statement handled=%v err=%v", handled, err)
	}
	collection, ok = stmt.(*strategyir.CollectionStmt)
	if !ok || collection.Target != "values" || collection.Operation != "push" || collection.ResultName != "" {
		t.Fatalf("method mutation statement = %#v", stmt)
	}

	lowered, err := state.lowerCollectionReadCalls(`array.get(values, 0) + values.last() + box.values.first()`)
	if err != nil {
		t.Fatalf("lowerCollectionReadCalls: %v", err)
	}
	for _, fragment := range []string{
		"collection_array_get(values, 0)",
		"collection_array_last(values)",
		"collection_array_first(box.values)",
	} {
		if !strings.Contains(lowered, fragment) {
			t.Fatalf("lowered collection expression = %q, missing %q", lowered, fragment)
		}
	}
	if unchanged, err := state.lowerCollectionReadCalls(`array.set(values, 0, close)`); err != nil || unchanged != `array.set(values, 0, close)` {
		t.Fatalf("mutating expression lower = %q err=%v, want unchanged without read lowering", unchanged, err)
	}
	if _, _, err := state.collectionTargetAndArguments("array", "array.get", nil); err == nil || !strings.Contains(err.Error(), "requires a collection argument") {
		t.Fatalf("missing collection target err = %v", err)
	}
}

func newObjectCollectionBoundaryParseState() *parseState {
	priceBox := strategyir.TypeDefinition{
		Name: "PriceBox",
		Fields: []strategyir.ObjectField{
			{Name: "price", Type: "float", Default: "close"},
			{Name: "bars", Type: "int", Default: "0"},
			{Name: "values", Type: "array<float>", Default: "array.new_float()"},
		},
	}
	return &parseState{
		collectionNamespaces: map[string]string{"values": "array", "box.values": "array"},
		udtTypes:             map[string]strategyir.TypeDefinition{"pricebox": priceBox},
		udtMethods: map[string][]strategyir.MethodDefinition{
			"pricebox.identity": {
				{Name: "identity", ReceiverType: "PriceBox", ReceiverName: "self"},
			},
			"pricebox.score": {
				{
					Name:         "score",
					ReceiverType: "PriceBox",
					ReceiverName: "self",
					Parameters: []strategyir.ObjectParameter{
						{Name: "factor", Type: "float", Default: "1"},
						{Name: "offset", Type: "float", Default: "0"},
					},
				},
			},
		},
		objectTypes:  map[string]string{"box": "PriceBox"},
		regexpCache:  map[string]*regexp.Regexp{},
		valueAliases: map[string]string{},
	}
}
