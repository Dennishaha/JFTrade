package pineruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestExecuteDynamicLoopsAndGeneralTuple(t *testing.T) {
	runtime := &strategyRuntime{persistentValues: map[string]any{}, bindingCache: map[*strategyir.LetStmt]cachedIndicatorBinding{}}
	scope := collectionTestScope(runtime, 100)
	statements := []strategyir.Statement{
		&strategyir.TupleStmt{
			Range:       strategyir.SourceRange{StartLine: 1},
			Names:       []string{"a", "b", "_", "d"},
			Expressions: []string{"1", "2", "3", "4"},
		},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "total", Expression: "0"},
		&strategyir.LoopStmt{
			Range:           strategyir.SourceRange{StartLine: 3},
			Variable:        "i",
			StartExpression: "0",
			EndExpression:   "3",
			StepExpression:  "1",
			MaxIterations:   10,
			Body: []strategyir.Statement{
				&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 4}, Name: "total", Expression: "total + i", Mode: strategyir.AssignmentModeReassign},
			},
		},
	}
	if _, err := runtime.executeStatements(statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if _, ok := scope.variables["_"]; ok {
		t.Fatal("discard tuple alias was stored")
	}
	if total, _ := coerceFloatValue(scope.variables["total"]); total != 6 {
		t.Fatalf("total = %v, want 6", total)
	}
}

func TestExecuteV26CollectionForLoop(t *testing.T) {
	runtime := &strategyRuntime{bindingCache: map[*strategyir.LetStmt]cachedIndicatorBinding{}, expressionCache: map[string]exprast.Node{}}
	scope := collectionTestScope(runtime, 100)
	scope.setVariable("values", &pineArray{values: []any{float64(1), float64(2), float64(3)}})
	statements := []strategyir.Statement{
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "total", Expression: "0"},
		&strategyir.LoopStmt{
			Range:         strategyir.SourceRange{StartLine: 2},
			IndexVariable: "i",
			Variable:      "value",
			Collection:    "values",
			MaxIterations: 10,
			Body: []strategyir.Statement{
				&strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 3}, Condition: "i == 2", Then: []strategyir.Statement{&strategyir.BreakStmt{Range: strategyir.SourceRange{StartLine: 4}}}},
				&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 5}, Name: "total", Expression: "total + value", Mode: strategyir.AssignmentModeReassign},
			},
		},
	}
	if _, err := runtime.executeStatements(statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if total, _ := coerceFloatValue(scope.variables["total"]); total != 3 {
		t.Fatalf("total = %v, want 3", total)
	}
}

func TestExecuteWhileLoopHonorsBreakAndLimit(t *testing.T) {
	runtime := &strategyRuntime{bindingCache: map[*strategyir.LetStmt]cachedIndicatorBinding{}}
	scope := collectionTestScope(runtime, 100)
	scope.setVariable("count", float64(0))
	loop := &strategyir.LoopStmt{
		Range:          strategyir.SourceRange{StartLine: 1},
		WhileCondition: "count < 5",
		MaxIterations:  10,
		Body: []strategyir.Statement{
			&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "count", Expression: "count + 1", Mode: strategyir.AssignmentModeReassign},
			&strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 3}, Condition: "count == 3", Then: []strategyir.Statement{&strategyir.BreakStmt{Range: strategyir.SourceRange{StartLine: 4}}}},
		},
	}
	if _, err := runtime.executeStatements([]strategyir.Statement{loop}, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if count, _ := coerceFloatValue(scope.variables["count"]); count != 3 {
		t.Fatalf("count = %v, want 3", count)
	}

	infinite := &strategyir.LoopStmt{Range: strategyir.SourceRange{StartLine: 8}, WhileCondition: "true", MaxIterations: 3}
	if _, err := runtime.executeStatements([]strategyir.Statement{infinite}, scope); err == nil || !strings.Contains(err.Error(), "exceeded 3 iterations") {
		t.Fatalf("infinite loop error = %v", err)
	}
}

func TestExecuteWhileLoopHonorsContinueBeforeConditionExit(t *testing.T) {
	statements := []strategyir.Statement{
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "count", Expression: "0"},
		&strategyir.LoopStmt{
			Range:          strategyir.SourceRange{StartLine: 2},
			WhileCondition: "count < 2",
			MaxIterations:  10,
			Body: []strategyir.Statement{
				&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 3}, Name: "count", Expression: "count + 1", Mode: strategyir.AssignmentModeReassign},
				&strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 4}, Condition: "count == 2", Then: []strategyir.Statement{&strategyir.ContinueStmt{Range: strategyir.SourceRange{StartLine: 5}}}},
				&strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 6}, Condition: "count >= 2", Then: []strategyir.Statement{&strategyir.BreakStmt{Range: strategyir.SourceRange{StartLine: 7}}}},
			},
		},
	}
	runtime := &strategyRuntime{
		program: &strategyir.Program{Hooks: []strategyir.HookBlock{{
			Kind:       strategyir.HookKLineClose,
			Statements: statements,
		}}},
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
	}
	runtime.ifScopePlans = buildIfScopePlans(runtime.program)
	scope := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if count, _ := coerceFloatValue(scope.variables["count"]); count != 2 {
		t.Fatalf("count = %v, want 2", count)
	}
}

func TestCompiledWhileLoopHonorsContinueBeforeConditionExit(t *testing.T) {
	script := `//@version=6
strategy("compiled while")
count = 0
while count < 2
    count := count + 1
    if count == 2
        continue
    if count >= 2
        break`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	var compiledLoop *strategyir.LoopStmt
	for _, statement := range compilation.Program.Hooks[0].Statements {
		if loop, ok := statement.(*strategyir.LoopStmt); ok {
			compiledLoop = loop
			break
		}
	}
	if compiledLoop == nil || len(compiledLoop.Body) != 3 {
		t.Fatalf("compiled loop = %#v", compiledLoop)
	}
	if let, ok := compiledLoop.Body[0].(*strategyir.LetStmt); !ok || let.Name != "count" || let.Expression != "count + 1" || let.Mode != strategyir.AssignmentModeReassign {
		t.Fatalf("compiled loop first body statement = %#v", compiledLoop.Body[0])
	}
	if let, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.LetStmt); !ok || let.Name != "count" || let.Expression != "0" || let.Mode != strategyir.AssignmentModeLet {
		t.Fatalf("compiled first statement = %#v", compilation.Program.Hooks[0].Statements[0])
	}
	runtime, err := newStrategyRuntime(context.Background(), &Strategy{
		Script:   script,
		Symbol:   "US.AAPL",
		Interval: types.Interval1m,
	}, compilation.Program, compilation.Requirements, nil, nil)
	if err != nil {
		t.Fatalf("newStrategyRuntime() error = %v", err)
	}
	testScope := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, testScope); err != nil {
		t.Fatalf("execute compiled statements with test scope error = %v", err)
	}
	if err := runtime.runHookLocked(strategyir.HookKLineClose, nil, market.SessionRegular); err != nil {
		t.Fatalf("runHookLocked() error = %v", err)
	}
}

func TestExecutePureUDTConstructorAndMethod(t *testing.T) {
	program := &strategyir.Program{
		Types: []strategyir.TypeDefinition{{
			Name: "PriceBox",
			Fields: []strategyir.ObjectField{
				{Name: "price", Type: "float", Default: "close"},
				{Name: "bars", Type: "int", Default: "1"},
			},
		}},
		Methods: []strategyir.MethodDefinition{{
			Name:         "score",
			ReceiverType: "PriceBox",
			ReceiverName: "self",
			Parameters:   []strategyir.ObjectParameter{{Name: "factor", Type: "float", Default: "2"}},
			Body:         "self.price * factor + self.bars",
		}},
	}
	runtime := &strategyRuntime{
		program:         program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
	}
	scope := collectionTestScope(runtime, 100)
	statements := []strategyir.Statement{
		&strategyir.ObjectStmt{Range: strategyir.SourceRange{StartLine: 1}, Operation: "constructor", TypeName: "PriceBox", ResultName: "box"},
		&strategyir.ObjectStmt{Range: strategyir.SourceRange{StartLine: 2}, Operation: "method", TypeName: "PriceBox", Method: "score", Target: "box", ResultName: "value"},
	}
	if _, err := runtime.executeStatements(statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if value, _ := coerceFloatValue(scope.variables["value"]); value != 201 {
		t.Fatalf("value = %v, want 201", value)
	}
}

func TestExecuteV23PureUDTNamedConstructorAndMethodBody(t *testing.T) {
	script := `//@version=6
strategy("v23 objects")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + self.bars + offset
box = PriceBox.new(bars=3, price=close)
value = box.score(offset=2, factor=2)`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:         compilation.Program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
	}
	scope := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if value, _ := coerceFloatValue(scope.variables["value"]); value != 205 {
		t.Fatalf("value = %v, want 205", value)
	}
}

func TestExecuteV23LocalObjectFieldReassignment(t *testing.T) {
	script := `//@version=6
strategy("v23 object fields")
type PriceBox
    float price = close
box = PriceBox.new()
box.price := close + 5
value = box.price`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:         compilation.Program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
	}
	scope := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if value, _ := coerceFloatValue(scope.variables["value"]); value != 105 {
		t.Fatalf("value = %v, want 105", value)
	}
}

func TestEvaluateV23ObjectMethodExpression(t *testing.T) {
	program := &strategyir.Program{
		Types: []strategyir.TypeDefinition{{
			Name: "PriceBox",
			Fields: []strategyir.ObjectField{
				{Name: "price", Type: "float", Default: "close"},
			},
		}},
		Methods: []strategyir.MethodDefinition{{
			Name:         "score",
			ReceiverType: "PriceBox",
			ReceiverName: "self",
			Parameters:   []strategyir.ObjectParameter{{Name: "factor", Type: "float", Default: "1"}},
			Body:         "self.price * factor",
		}},
	}
	runtime := &strategyRuntime{program: program, expressionCache: map[string]exprast.Node{}}
	scope := collectionTestScope(runtime, 100)
	scope.setVariable("box", map[string]any{"__type": "PriceBox", "price": float64(100)})
	value, err := evaluateExpression(`object_method("PriceBox", "score", box, 3)`, scope)
	if err != nil {
		t.Fatalf("evaluateExpression() error = %v", err)
	}
	if got, _ := coerceFloatValue(value); got != 300 {
		t.Fatalf("value = %v, want 300", got)
	}
}

func TestExecuteV24PersistentObjectFieldReassignment(t *testing.T) {
	script := `//@version=6
strategy("v24 persistent object")
type PriceBox
    float price = close
var box = PriceBox.new()
box.price := close + 5
value = box.price`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:          compilation.Program,
		persistentValues: map[string]any{},
		bindingCache:     map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache:  map[string]exprast.Node{},
	}
	first := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, first); err != nil {
		t.Fatalf("first executeStatements() error = %v", err)
	}
	if value, _ := coerceFloatValue(first.variables["value"]); value != 105 {
		t.Fatalf("first value = %v, want 105", value)
	}
	second := collectionTestScope(runtime, 110)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, second); err != nil {
		t.Fatalf("second executeStatements() error = %v", err)
	}
	if value, _ := coerceFloatValue(second.variables["value"]); value != 115 {
		t.Fatalf("second value = %v, want 115", value)
	}
	persistent, ok := runtime.persistentValues["box"].(map[string]any)
	if !ok {
		t.Fatalf("persistent box = %#v", runtime.persistentValues["box"])
	}
	if value, _ := coerceFloatValue(persistent["price"]); value != 115 {
		t.Fatalf("persistent price = %v, want 115", value)
	}
}

func TestExecuteV26ObjectCollectionFieldMutation(t *testing.T) {
	script := `//@version=6
strategy("v26 object collection")
type Box
    array<float> values
box = Box.new(array.new_float())
box.values.push(close)
fieldSize = box.values.size()`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:          compilation.Program,
		persistentValues: map[string]any{},
		bindingCache:     map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache:  map[string]exprast.Node{},
	}
	runtime.ifScopePlans = buildIfScopePlans(runtime.program)
	scope := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if fieldSize, _ := coerceFloatValue(scope.variables["fieldSize"]); fieldSize != 1 {
		t.Fatalf("fieldSize = %v, want 1", fieldSize)
	}
}

func TestExecuteV26CollectionHistorySnapshot(t *testing.T) {
	script := `//@version=6
strategy("v26 collection history")
values = array.from(close)
previousFirst = values[1].get(0)`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:         compilation.Program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
		historyTargets:  collectProgramHistoryTargets(compilation.Program),
	}
	runtime.ifScopePlans = buildIfScopePlans(runtime.program)
	first := collectionTestScope(runtime, 10)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, first); err != nil {
		t.Fatalf("first executeStatements() error = %v", err)
	}
	runtime.recordHistorySnapshots(first)
	second := collectionTestScope(runtime, 20)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, second); err != nil {
		t.Fatalf("second executeStatements() error = %v", err)
	}
	if previous, _ := coerceFloatValue(second.variables["previousFirst"]); previous != 10 {
		t.Fatalf("previousFirst = %v, want 10", previous)
	}
}

func TestEvaluateV24ObjectMethodExpressionNamedDefaults(t *testing.T) {
	script := `//@version=6
strategy("v24 named method expression")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + offset + self.bars
box = PriceBox.new(price=close, bars=2)
value = box.score(offset=3, factor=2)`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:         compilation.Program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
	}
	scope := collectionTestScope(runtime, 100)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, scope); err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if value, _ := coerceFloatValue(scope.variables["value"]); value != 205 {
		t.Fatalf("value = %v, want 205", value)
	}
}

func TestExecuteV28ObjectHistoryAndMethodChain(t *testing.T) {
	script := `//@version=6
strategy("v28 object history")
type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new(close)
previous = box[1].price
chained = box.identity().score(2)`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:         compilation.Program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
		historyTargets:  collectProgramHistoryTargets(compilation.Program),
	}
	runtime.ifScopePlans = buildIfScopePlans(runtime.program)
	first := collectionTestScope(runtime, 10)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, first); err != nil {
		t.Fatalf("first executeStatements() error = %v", err)
	}
	runtime.recordHistorySnapshots(first)
	second := collectionTestScope(runtime, 20)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, second); err != nil {
		t.Fatalf("second executeStatements() error = %v", err)
	}
	if previous, _ := coerceFloatValue(second.variables["previous"]); previous != 10 {
		t.Fatalf("previous = %v, want 10", previous)
	}
	if chained, _ := coerceFloatValue(second.variables["chained"]); chained != 40 {
		t.Fatalf("chained = %v, want 40", chained)
	}
}

func TestExecuteV29ObjectHistoryMethodReceiverAndNamedChain(t *testing.T) {
	script := `//@version=6
strategy("v29 object history method")
type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1, float offset = 0) => self.price * factor + offset
box = PriceBox.new(close)
previousScore = box[1].score(factor=2, offset=1)
chained = box.identity().score(offset=1, factor=2)`
	compilation, err := strategypine.Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	runtime := &strategyRuntime{
		program:         compilation.Program,
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		expressionCache: map[string]exprast.Node{},
		historyTargets:  collectProgramHistoryTargets(compilation.Program),
	}
	runtime.ifScopePlans = buildIfScopePlans(runtime.program)
	first := collectionTestScope(runtime, 10)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, first); err != nil {
		t.Fatalf("first executeStatements() error = %v", err)
	}
	runtime.recordHistorySnapshots(first)
	second := collectionTestScope(runtime, 20)
	if _, err := runtime.executeStatements(compilation.Program.Hooks[0].Statements, second); err != nil {
		t.Fatalf("second executeStatements() error = %v", err)
	}
	if previousScore, _ := coerceFloatValue(second.variables["previousScore"]); previousScore != 21 {
		t.Fatalf("previousScore = %v, want 21", previousScore)
	}
	if chained, _ := coerceFloatValue(second.variables["chained"]); chained != 41 {
		t.Fatalf("chained = %v, want 41", chained)
	}
}
