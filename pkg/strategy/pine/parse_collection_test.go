package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestAnalyzeScriptIncludesV20CollectionAndDeclarationSemantics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v2 Foundation", overlay=true)
arr = array.new_float(0)
prices = map.new<string, float>()
grid = matrix.new<float>(1, 1)
array.push(arr, close)
latest = array.get(arr, 0)
map.put(prices, "last", latest)
matrix.set(grid, 0, 0, latest)
type TradeBox
    float price = close
    int bars = 0
method reset(TradeBox box, float limit = 0) =>
    box
box = TradeBox.new(close, 0)
resetBox = box.reset(10)
import TradingView/ta/7 as tav7
export helper(float src, int length = 1) => src
library("JFTradeFoundation")`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want unsupported diagnostics for v2 parse-only surfaces")
	}
	if analysis.Semantic == nil {
		t.Fatal("Semantic is nil")
	}
	if len(analysis.Declarations) == 0 || len(analysis.Declarations) != len(analysis.Semantic.Declarations) {
		t.Fatalf("analysis declarations = %#v, semantic declarations = %#v", analysis.Declarations, analysis.Semantic.Declarations)
	}
	codes := map[string]bool{}
	for _, diagnostic := range analysis.Diagnostics {
		codes[diagnostic.Code] = true
	}
	if codes["PINE_COLLECTION_UNSUPPORTED"] || !codes["PINE_DECLARATION_UNSUPPORTED"] {
		t.Fatalf("diagnostic codes = %#v", codes)
	}
	namespaces := map[string]SemanticDeclaration{}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Semantic.Declarations {
		if declaration.Kind == "collection" {
			namespaces[declaration.Namespace] = declaration
			continue
		}
		declarations[declaration.Kind] = declaration
	}
	if namespaces["array"].Call != "array.new_float" || namespaces["array"].Name != "arr" {
		t.Fatalf("array declaration = %#v", namespaces["array"])
	}
	if namespaces["map"].Call != "map.new" || namespaces["map"].TypeArgs != "string, float" || namespaces["map"].Name != "prices" {
		t.Fatalf("map declaration = %#v", namespaces["map"])
	}
	if namespaces["matrix"].Call != "matrix.new" || namespaces["matrix"].TypeArgs != "float" || namespaces["matrix"].Name != "grid" {
		t.Fatalf("matrix declaration = %#v", namespaces["matrix"])
	}
	if declarations["type"].Name != "TradeBox" || declarations["method"].Name != "reset" || declarations["import"].Name != "TradingView/ta/7" || declarations["import"].Alias != "tav7" || declarations["export"].Name != "helper" || declarations["library"].Name != "JFTradeFoundation" {
		t.Fatalf("declarations = %#v", declarations)
	}
	if len(declarations["type"].Fields) != 2 || declarations["type"].Fields[0].Type != "float" || declarations["type"].Fields[0].Name != "price" || declarations["type"].Fields[0].Default != "close" || declarations["type"].Fields[1].Type != "int" || declarations["type"].Fields[1].Name != "bars" {
		t.Fatalf("type fields = %#v", declarations["type"].Fields)
	}
	if declarations["method"].Receiver == nil || declarations["method"].Receiver.Type != "TradeBox" || declarations["method"].Receiver.Name != "box" {
		t.Fatalf("method receiver = %#v", declarations["method"].Receiver)
	}
	if len(declarations["method"].Parameters) != 2 || declarations["method"].Parameters[1].Type != "float" || declarations["method"].Parameters[1].Name != "limit" || declarations["method"].Parameters[1].Default != "0" {
		t.Fatalf("method parameters = %#v", declarations["method"].Parameters)
	}
	if declarations["import"].ImportPath != "TradingView/ta/7" || declarations["import"].Version != "7" || declarations["import"].Alias != "tav7" {
		t.Fatalf("import declaration = %#v", declarations["import"])
	}
	if len(declarations["export"].Parameters) != 2 || declarations["export"].Parameters[0].Type != "float" || declarations["export"].Parameters[0].Name != "src" || declarations["export"].Parameters[1].Default != "1" {
		t.Fatalf("export declaration = %#v", declarations["export"])
	}
	if len(analysis.CollectionOperations) != 7 || len(analysis.CollectionOperations) != len(analysis.Semantic.CollectionOperations) {
		t.Fatalf("collection operations = %#v, semantic = %#v", analysis.CollectionOperations, analysis.Semantic.CollectionOperations)
	}
	operations := map[string]SemanticCollectionOperation{}
	for _, operation := range analysis.CollectionOperations {
		operations[operation.Call] = operation
	}
	if operations["array.new_float"].Target != "arr" || !operations["array.new_float"].Mutates {
		t.Fatalf("array.new_float operation = %#v", operations["array.new_float"])
	}
	if operations["array.push"].Target != "arr" || !operations["array.push"].Mutates || !operations["array.push"].Supported || operations["array.push"].Signature != "array.push(id, value)" || len(operations["array.push"].Arguments) != 2 {
		t.Fatalf("array.push operation = %#v", operations["array.push"])
	}
	if operations["array.get"].Target != "arr" || operations["array.get"].Mutates {
		t.Fatalf("array.get operation = %#v", operations["array.get"])
	}
	if operations["map.put"].Target != "prices" || !operations["map.put"].Mutates {
		t.Fatalf("map.put operation = %#v", operations["map.put"])
	}
	if operations["matrix.set"].Target != "grid" || !operations["matrix.set"].Mutates {
		t.Fatalf("matrix.set operation = %#v", operations["matrix.set"])
	}
	for _, operation := range analysis.CollectionOperations {
		if !operation.Executable {
			t.Fatalf("collection operation = %#v, want executable", operation)
		}
	}
	if len(analysis.ObjectOperations) != 2 || len(analysis.ObjectOperations) != len(analysis.Semantic.ObjectOperations) {
		t.Fatalf("object operations = %#v, semantic = %#v", analysis.ObjectOperations, analysis.Semantic.ObjectOperations)
	}
	objectOperations := map[string]SemanticObjectOperation{}
	for _, operation := range analysis.ObjectOperations {
		objectOperations[operation.Kind] = operation
	}
	if objectOperations["constructor"].Type != "TradeBox" || objectOperations["constructor"].Call != "TradeBox.new" || objectOperations["constructor"].Target != "box" || objectOperations["constructor"].Signature != "TradeBox.new(float price = close, int bars = 0)" || len(objectOperations["constructor"].Arguments) != 2 || objectOperations["constructor"].Executable {
		t.Fatalf("constructor object operation = %#v", objectOperations["constructor"])
	}
	if objectOperations["method"].Type != "TradeBox" || objectOperations["method"].Method != "reset" || objectOperations["method"].Call != "box.reset" || objectOperations["method"].Target != "box" || objectOperations["method"].Signature != "reset(TradeBox box, float limit = 0)" || len(objectOperations["method"].Arguments) != 1 || !objectOperations["method"].Supported || objectOperations["method"].Executable {
		t.Fatalf("method object operation = %#v", objectOperations["method"])
	}
	symbolKinds := map[string]SemanticValueKind{}
	for _, symbol := range analysis.Semantic.Symbols {
		symbolKinds[symbol.Name] = symbol.ValueKind
	}
	if symbolKinds["arr"] != SemanticValueObject || symbolKinds["prices"] != SemanticValueObject || symbolKinds["grid"] != SemanticValueObject || symbolKinds["box"] != SemanticValueObject {
		t.Fatalf("symbol kinds = %#v", symbolKinds)
	}
	if symbolKinds["latest"] != SemanticValueUnknown {
		t.Fatalf("latest semantic kind = %#v", symbolKinds["latest"])
	}
	for _, line := range analysis.AST.Lines {
		if strings.HasPrefix(line.Text, "arr = ") && line.Kind != NodeKindCollection {
			t.Fatalf("array AST kind = %q, want collection", line.Kind)
		}
	}
}

func TestPineV20LanguageFoundationGate(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v2 Foundation Gate", overlay=true)
var array<float> arr = array.new_float(0)
map<string, float> prices = map.new<string, float>()
matrix<float> grid = matrix.new<float>(1, 1)
arr.push(close)
prices.put("last", close)
grid.set(0, 0, close)
type TradeBox
    float price = close
method reset(TradeBox box, float limit = 0) =>
    box
box = TradeBox.new(close)
updated = box.reset(10)
import TradingView/ta/7 as tav7
library("JFTradeFoundation")
lbl = label.new(bar_index, close, "Entry")
tbl = table.new(position.top_right, 1, 1)
table.cell(tbl, 0, 0, "Ready")
plot(close, title="Close")`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want parse-only diagnostics for v2 foundation surfaces")
	}
	if analysis.AST == nil || analysis.Semantic == nil {
		t.Fatalf("analysis missing AST/semantic payload: %#v", analysis)
	}
	if len(analysis.Declarations) != 7 {
		t.Fatalf("declarations = %#v, want three collections plus type/method/import/library", analysis.Declarations)
	}
	if len(analysis.CollectionOperations) != 6 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	if len(analysis.ObjectOperations) != 2 {
		t.Fatalf("object operations = %#v", analysis.ObjectOperations)
	}
	if len(analysis.Visuals) != 4 {
		t.Fatalf("visuals = %#v", analysis.Visuals)
	}
	codes := map[string]bool{}
	for _, diagnostic := range analysis.Diagnostics {
		codes[diagnostic.Code] = true
		if strings.HasPrefix(diagnostic.Code, "PINE_SEMANTIC_") {
			t.Fatalf("valid v2 foundation script returned semantic diagnostic: %#v", diagnostic)
		}
	}
	if codes["PINE_COLLECTION_UNSUPPORTED"] || !codes["PINE_DECLARATION_UNSUPPORTED"] {
		t.Fatalf("diagnostic codes = %#v, want explicit parse-only execution boundaries", codes)
	}
	capabilities := map[string]Capability{}
	for _, capability := range CapabilityRegistry() {
		capabilities[capability.ID] = capability
	}
	if capability := capabilities["syntax.arrays_maps_matrices"]; capability.Status != CapabilityPartial || !capability.Layers.Parser || !capability.Layers.Runtime || !capability.Layers.Planner {
		t.Fatalf("collection capability = %#v, want v2.1 partial runtime surface", capability)
	}
	if capability := capabilities["syntax.methods_types_libraries"]; capability.Status != CapabilityPartial || !capability.Layers.Parser || !capability.Layers.Runtime || !capability.Layers.Planner {
		t.Fatalf("declaration capability = %#v, want v2.2 partial runtime surface", capability)
	}
	if capability := capabilities["visual.noop_calls"]; capability.Status != CapabilityWarning || !capability.Layers.Parser || !capability.Layers.Frontend {
		t.Fatalf("visual capability = %#v", capability)
	}
	if capability := capabilities["tooling.visual_metadata_output"]; capability.Status != CapabilitySupported || !capability.Layers.Frontend {
		t.Fatalf("visual metadata capability = %#v", capability)
	}
	if capability := capabilities["order.full_tv_broker_emulator"]; capability.Status != CapabilityUnsupported {
		t.Fatalf("broker emulator capability = %#v, want explicitly separate unsupported roadmap", capability)
	}
}

func TestAnalyzeScriptReportsCollectionOperationSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Collection", overlay=true)
arr = array.new_float(0)
array.push(arr)
matrix.set(grid, 0, 0)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want collection diagnostics")
	}
	if len(analysis.CollectionOperations) != 3 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	signatureDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_SIGNATURE" {
			signatureDiagnostics++
		}
	}
	if signatureDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two collection signature diagnostics", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptIncludesCollectionMethodStyleOperations(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Collection Methods", overlay=true)
arr = array.new_float(0)
prices = map.new<string, float>()
grid = matrix.new<float>(1, 1)
arr.push(close)
latest = arr.get(0)
prices.put("last", latest)
grid.set(0, 0, latest)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.CollectionOperations) != 7 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	methodOperations := map[string]SemanticCollectionOperation{}
	for _, operation := range analysis.CollectionOperations {
		if !strings.HasPrefix(operation.Operation, "new") {
			methodOperations[operation.Target+"."+operation.Operation] = operation
		}
	}
	if methodOperations["arr.push"].Call != "array.push" || methodOperations["arr.push"].Signature != "array.push(id, value)" || len(methodOperations["arr.push"].Arguments) != 2 || methodOperations["arr.push"].Arguments[0] != "arr" || !methodOperations["arr.push"].Mutates {
		t.Fatalf("arr.push operation = %#v", methodOperations["arr.push"])
	}
	if methodOperations["arr.get"].Call != "array.get" || methodOperations["arr.get"].Target != "arr" || len(methodOperations["arr.get"].Arguments) != 2 || methodOperations["arr.get"].Mutates {
		t.Fatalf("arr.get operation = %#v", methodOperations["arr.get"])
	}
	if methodOperations["prices.put"].Call != "map.put" || methodOperations["prices.put"].Target != "prices" || len(methodOperations["prices.put"].Arguments) != 3 || !methodOperations["prices.put"].Mutates {
		t.Fatalf("prices.put operation = %#v", methodOperations["prices.put"])
	}
	if methodOperations["grid.set"].Call != "matrix.set" || methodOperations["grid.set"].Target != "grid" || len(methodOperations["grid.set"].Arguments) != 4 || !methodOperations["grid.set"].Mutates {
		t.Fatalf("grid.set operation = %#v", methodOperations["grid.set"])
	}
	for _, operation := range analysis.CollectionOperations {
		if !operation.Executable {
			t.Fatalf("operation = %#v, want executable", operation)
		}
	}
}

func TestAnalyzeScriptIncludesTypedCollectionDeclarations(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Typed Collections", overlay=true)
var array<float> arr = array.new_float(0)
map<string, float> prices = na
matrix<float> grid = matrix.new<float>(1, 1)
arr.push(close)
prices.put("last", close)
grid.set(0, 0, close)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want parse-only collection diagnostics")
	}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Declarations {
		if declaration.Kind == "collection" {
			declarations[declaration.Name] = declaration
		}
	}
	if declarations["arr"].Namespace != "array" || declarations["arr"].TypeArgs != "float" || declarations["arr"].Call != "array.new_float" {
		t.Fatalf("array declaration = %#v", declarations["arr"])
	}
	if declarations["prices"].Namespace != "map" || declarations["prices"].TypeArgs != "string, float" || declarations["prices"].Call != "" {
		t.Fatalf("map declaration = %#v", declarations["prices"])
	}
	if declarations["grid"].Namespace != "matrix" || declarations["grid"].TypeArgs != "float" || declarations["grid"].Call != "matrix.new" {
		t.Fatalf("matrix declaration = %#v", declarations["grid"])
	}
	if len(analysis.CollectionOperations) != 5 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	methodOperations := map[string]SemanticCollectionOperation{}
	for _, operation := range analysis.CollectionOperations {
		if !strings.HasPrefix(operation.Operation, "new") {
			methodOperations[operation.Target+"."+operation.Operation] = operation
		}
	}
	if methodOperations["arr.push"].Call != "array.push" || len(methodOperations["arr.push"].Arguments) != 2 {
		t.Fatalf("arr.push operation = %#v", methodOperations["arr.push"])
	}
	if methodOperations["prices.put"].Call != "map.put" || len(methodOperations["prices.put"].Arguments) != 3 {
		t.Fatalf("prices.put operation = %#v", methodOperations["prices.put"])
	}
	if methodOperations["grid.set"].Call != "matrix.set" || len(methodOperations["grid.set"].Arguments) != 4 {
		t.Fatalf("grid.set operation = %#v", methodOperations["grid.set"])
	}
	foundTypedAST := false
	for _, line := range analysis.AST.Lines {
		if line.Name == "arr" && line.Type == "array<float>" && line.Kind == NodeKindCollection {
			foundTypedAST = true
		}
	}
	if !foundTypedAST {
		t.Fatalf("typed AST lines = %#v", analysis.AST.Lines)
	}
}

func TestCompileSupportsV21ExecutableCollectionCore(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Executable Collections", overlay=true)
var array<float> values = array.new_float(0)
var map<string, float> prices = map.new<string, float>()
var matrix<float> grid = matrix.new<float>(1, 1, 0)
values.push(close)
prices.put("last", close)
grid.set(0, 0, close)
latest = values.last()
known = prices.contains("last")
cell = grid.get(0, 0)
if values.size() > 0 and known and cell == latest
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.Program == nil || len(analysis.Program.Hooks) != 1 {
		t.Fatalf("program = %#v", analysis.Program)
	}
	collectionStatements := 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if _, ok := statement.(*strategyir.CollectionStmt); ok {
			collectionStatements++
		}
	}
	if collectionStatements != 9 {
		t.Fatalf("collection statements = %d, program = %#v", collectionStatements, analysis.Program.Hooks[0].Statements)
	}
	for _, operation := range analysis.CollectionOperations {
		if !operation.Executable {
			t.Fatalf("operation = %#v, want executable", operation)
		}
	}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Declarations {
		declarations[declaration.Name] = declaration
	}
	for _, name := range []string{"values", "prices", "grid"} {
		if !declarations[name].Executable {
			t.Fatalf("declaration %s = %#v, want executable", name, declarations[name])
		}
	}
}

func TestCompileSupportsV21CollectionAliases(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("collection aliases")
var values = array.new_float()
alias = values
alias.push(close)
latest = alias.last()
if latest > 0
    strategy.entry("Long", strategy.long)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	var push *strategyir.CollectionStmt
	for _, statement := range compilation.Program.Hooks[0].Statements {
		collection, ok := statement.(*strategyir.CollectionStmt)
		if ok && collection.Operation == "push" {
			push = collection
			break
		}
	}
	if push == nil || push.Namespace != "array" || push.Target != "alias" {
		t.Fatalf("alias push = %#v", push)
	}
}

func TestCompileSupportsV21BBWAndCOG(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("v21 ta")
width = ta.bbw(close, 5, 2)
gravity = ta.cog(hlc3, 5)
weeklyVWAP = ta.vwap(hlc3, timeframe.change("W"))
mtfWidth = request.security(syminfo.tickerid, "15", ta.bbw(close, 5, 2))
mtfGravity = request.security(syminfo.tickerid, "15", ta.cog(hlc3, 5))
if width >= 0 and gravity <= 0 and weeklyVWAP > 0 and mtfWidth >= 0 and mtfGravity <= 0
    strategy.entry("Long", strategy.long)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	keys := map[string]bool{}
	for _, requirement := range compilation.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"bbw:close:5:2",
		"cog:hlc3:5",
		"anchored_vwap:week:hlc3",
		"bbw:close:5:2:15m",
		"cog:hlc3:5:15m",
	} {
		if !keys[key] {
			t.Fatalf("requirements = %#v, missing %s", compilation.Requirements.Indicators, key)
		}
	}
}

func TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v22 structured")
[a, b, _, d] = [open, high, low, close]
[mtfOpen, mtfHigh, mtfLow, mtfClose] = request.security(syminfo.tickerid, "15", [open, high, low, close])
limit = bar_index % 3
total = 0
for i = 0 to limit
    total := total + i
count = 0
while count < 3
    count := count + 1
    if count == 2
        continue
    if count >= 3
        break
if d >= a and mtfClose >= mtfOpen and total >= 0 and count == 3
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.AST == nil || len(analysis.AST.Nodes) == 0 {
		t.Fatalf("structured AST = %#v", analysis.AST)
	}
	foundLoopChildren := false
	for _, node := range analysis.AST.Nodes {
		if (node.Line.Kind == NodeKindFor || node.Line.Kind == NodeKindWhile) && len(node.Children) > 0 {
			foundLoopChildren = true
		}
	}
	if !foundLoopChildren {
		t.Fatalf("AST nodes = %#v, want loop children", analysis.AST.Nodes)
	}
	tuples, loops := 0, 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch statement.(type) {
		case *strategyir.TupleStmt:
			tuples++
		case *strategyir.LoopStmt:
			loops++
		}
	}
	if tuples != 2 || loops != 2 {
		t.Fatalf("tuples=%d loops=%d statements=%#v", tuples, loops, analysis.Program.Hooks[0].Statements)
	}
}

func TestCompileSupportsV22PureUDTAndMethodSubset(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v22 objects")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1) => self.price * factor + self.bars
box = PriceBox.new(close, 2)
value = box.score(2)
if value > close
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Program.Types) != 1 || len(analysis.Program.Methods) != 1 {
		t.Fatalf("types=%#v methods=%#v", analysis.Program.Types, analysis.Program.Methods)
	}
	objects := 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if _, ok := statement.(*strategyir.ObjectStmt); ok {
			objects++
		}
	}
	if objects != 2 {
		t.Fatalf("object statements = %d, statements = %#v", objects, analysis.Program.Hooks[0].Statements)
	}
	for _, declaration := range analysis.Declarations {
		if (declaration.Kind == "type" || declaration.Kind == "method") && !declaration.Executable {
			t.Fatalf("declaration = %#v, want executable", declaration)
		}
	}
	for _, operation := range analysis.ObjectOperations {
		if !operation.Executable {
			t.Fatalf("object operation = %#v, want executable", operation)
		}
	}
}

func TestCompileSupportsV23NamedObjectArgsAndPureMethodBody(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v23 objects")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + self.bars + offset
box = PriceBox.new(bars=3, price=close)
value = box.score(offset=2, factor=2)
if value > close
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Program.Methods) != 1 {
		t.Fatalf("methods = %#v", analysis.Program.Methods)
	}
	if got, want := analysis.Program.Methods[0].Body, "(self.price * factor) + self.bars + offset"; got != want {
		t.Fatalf("method body = %q, want %q", got, want)
	}
	objectStatements := make([]*strategyir.ObjectStmt, 0)
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if object, ok := statement.(*strategyir.ObjectStmt); ok {
			objectStatements = append(objectStatements, object)
		}
	}
	if len(objectStatements) != 2 {
		t.Fatalf("object statements = %#v", objectStatements)
	}
	if got, want := strings.Join(objectStatements[0].Arguments, ","), "close,3"; got != want {
		t.Fatalf("constructor args = %q, want %q", got, want)
	}
	if got, want := strings.Join(objectStatements[1].Arguments, ","), "2,2"; got != want {
		t.Fatalf("method args = %q, want %q", got, want)
	}
}

func TestCompileSupportsV23LocalObjectFieldReassignment(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v23 object fields")
type PriceBox
    float price = close
box = PriceBox.new()
box.price := close + 1
if box.price > close
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	fieldSets := 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if object, ok := statement.(*strategyir.ObjectStmt); ok && object.Operation == "field_set" {
			fieldSets++
		}
	}
	if fieldSets != 1 {
		t.Fatalf("field sets = %d, statements = %#v", fieldSets, analysis.Program.Hooks[0].Statements)
	}

	persistent := AnalyzeScript(`//@version=6
strategy("v24 persistent object fields")
type PriceBox
    float price = close
var box = PriceBox.new()
box.price := close + 1`, AnalysisOptions{IncludeAST: true})
	if !persistent.OK {
		t.Fatalf("persistent object field reassignment diagnostics = %#v, want OK", persistent.Diagnostics)
	}
}

func TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions(t *testing.T) {
	state := &parseState{
		collectionNamespaces: map[string]string{"values": "array"},
		objectTypes:          map[string]string{},
		udtMethods:           map[string][]strategyir.MethodDefinition{},
	}
	normalized := state.normalizeExpression(`request.security(syminfo.tickerid, "15", values.last())`)
	if normalized != "collection_array_last(values)" {
		t.Fatalf("normalized collection MTF expression = %q", normalized)
	}

	analysis := AnalyzeScript(`//@version=6
strategy("v23 mtf object collection")
values = array.new_float(0)
values.push(close)
type PriceBox
    float price = close
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new()
mtfLast = request.security(syminfo.tickerid, "15", values.last())
mtfField = request.security(syminfo.tickerid, "15", box.price)
mtfScore = request.security(syminfo.tickerid, "15", box.score(2))
if mtfLast > 0 and mtfField > 0 and mtfScore > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	got := make([]string, 0)
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok && strings.HasPrefix(let.Name, "mtf") {
			got = append(got, let.Expression)
		}
	}
	joined := strings.Join(got, "\n")
	for _, fragment := range []string{"collection_array_last(values)", "box.price", "object_method"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("mtf expressions = %q, missing %q", joined, fragment)
		}
	}
}

func TestCompileSupportsV24CollectionExpansionAndMTFStoch(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v24 collections and stoch")
values = array.from(close, open, high)
values.sort(order.descending)
indices = values.sort_indices(order.ascending)
joined = indices.join(",")
lookup = values.binary_search(close)
middle = values.median()
spread = values.range()
prices = map.new<string, float>()
prices.put("b", close)
prices.put("a", open)
keys = prices.keys()
vals = prices.values()
mtfStoch = request.security(syminfo.tickerid, "15", ta.stoch(close, high, low, 14))
if mtfStoch >= 0 and middle >= 0 and spread >= 0 and lookup >= -1 and vals.size() >= 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var joined strings.Builder
	var collectionOps strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			joined.WriteString(let.Expression + "\n")
		}
		if collection, ok := statement.(*strategyir.CollectionStmt); ok {
			collectionOps.WriteString(collection.Namespace + "." + collection.Operation + "\n")
		}
	}
	for _, fragment := range []string{`stoch(close, high, low, 14, "15m")`} {
		if !strings.Contains(joined.String(), fragment) {
			t.Fatalf("compiled expressions = %q, missing %q", joined.String(), fragment)
		}
	}
	for _, fragment := range []string{"array.from", "array.median", "map.values"} {
		if !strings.Contains(collectionOps.String(), fragment) {
			t.Fatalf("collection ops = %q, missing %q", collectionOps.String(), fragment)
		}
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	if !keys["stoch:close:14:15m"] {
		t.Fatalf("requirements = %#v, missing stoch:close:14:15m", analysis.Requirements.Indicators)
	}
}

func TestCompileSupportsV24NamedObjectMethodExpressionAndRuntimeLoopFallback(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v24 object method and loop")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + offset + self.bars
box = PriceBox.new(price=close, bars=2)
value = box.score(offset=3, factor=2)
mtfValue = request.security(syminfo.tickerid, "15", box.score(offset=1, factor=2))
total = 0
for i = 0 to 5
    if i == 3
        break
    total := total + i
if value > 0 and mtfValue > 0 and total >= 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	foundLoop := false
	foundNamedMethod := false
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch typed := statement.(type) {
		case *strategyir.LoopStmt:
			foundLoop = true
		case *strategyir.ObjectStmt:
			if typed.Operation == "method" && typed.Method == "score" && strings.Join(typed.Arguments, ",") == "2,3" {
				foundNamedMethod = true
			}
		case *strategyir.LetStmt:
			if strings.Contains(typed.Expression, "object_method") && strings.Contains(typed.Expression, "2, 1") {
				foundNamedMethod = true
			}
		}
	}
	if !foundLoop {
		t.Fatalf("statements = %#v, want runtime loop fallback", analysis.Program.Hooks[0].Statements)
	}
	if !foundNamedMethod {
		t.Fatalf("statements = %#v, want named method expression lowering", analysis.Program.Hooks[0].Statements)
	}
}

func TestCompileSupportsV25ArrayStringAndTimeframeHelpers(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v25 helpers")
values = array.from(-2, 1, 2, 2, 5)
absValues = values.abs()
left = values.binary_search_leftmost(2)
right = values.binary_search_rightmost(2)
rank = values.percentrank(3)
p50 = values.percentile_nearest_rank(50)
p50lin = values.percentile_linear_interpolation(50)
dev = values.stdev()
variance = values.variance()
other = array.from(2, 4, 6, 8, 10)
cov = values.covariance(other)
labelText = str.format("{0}:{1}", str.upper("alpha"), str.length("beta"))
changed = timeframe.change("15")
tc = time_close
if absValues.size() == 5 and left >= 0 and right >= left and rank >= 0 and p50 >= 0 and p50lin >= 0 and dev >= 0 and variance >= 0 and cov >= 0 and str.contains(labelText, "ALPHA") and tc > time and changed
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var collectionOps strings.Builder
	var expressions strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if collection, ok := statement.(*strategyir.CollectionStmt); ok {
			collectionOps.WriteString(collection.Namespace + "." + collection.Operation + "\n")
		}
		if let, ok := statement.(*strategyir.LetStmt); ok {
			expressions.WriteString(let.Expression + "\n")
		}
	}
	for _, fragment := range []string{"array.abs", "array.binary_search_leftmost", "array.percentile_linear_interpolation", "array.covariance"} {
		if !strings.Contains(collectionOps.String(), fragment) {
			t.Fatalf("collection ops = %q, missing %q", collectionOps.String(), fragment)
		}
	}
	for _, fragment := range []string{"str_format", "str_upper", "str_length", "timeframe_change", "time_close"} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
}

func TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v26 collection foundation")
type Box
    array<float> values
values = array.from(1, 2, 3)
total = 0
for [i, value] in values
    if i == 2
        break
    total := total + value
previousFirst = values[1].get(0)
box = Box.new(array.new_float())
box.values.push(close)
fieldSize = box.values.size()
if total >= 3 and nz(previousFirst, 0) >= 0 and fieldSize > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	foundCollectionLoop := false
	var expressions strings.Builder
	var collectionOps strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch typed := statement.(type) {
		case *strategyir.LoopStmt:
			if typed.Collection == "values" && typed.IndexVariable == "i" && typed.Variable == "value" {
				foundCollectionLoop = true
			}
		case *strategyir.LetStmt:
			expressions.WriteString(typed.Expression + "\n")
		case *strategyir.CollectionStmt:
			collectionOps.WriteString(typed.Target + "." + typed.Operation + "\n")
		case *strategyir.ObjectStmt:
			expressions.WriteString(strings.Join(typed.Arguments, "\n") + "\n")
		}
	}
	if !foundCollectionLoop {
		t.Fatalf("statements = %#v, want collection for loop", analysis.Program.Hooks[0].Statements)
	}
	for _, fragment := range []string{"collection_array_get(history(values, 1), 0)", "collection_array_new_float()"} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
	if !strings.Contains(collectionOps.String(), "box.values.push") {
		t.Fatalf("collection ops = %q, missing box.values.push", collectionOps.String())
	}
	if !strings.Contains(collectionOps.String(), "box.values.size") {
		t.Fatalf("collection ops = %q, missing box.values.size", collectionOps.String())
	}
}
