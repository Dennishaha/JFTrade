package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCompileSupportsV27CollectionTimeframeAndMTFHelpers(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v27 helpers")
values = array.from(1, 2, 4, 8)
prevRange = values[1].range()
prevDev = values[1].stdev()
labels = map.new<string, float>()
labels.put("b", 2)
labels.put("a", 1)
total = 0
for key in labels.keys()
    total := total + labels.get(key)
grid = matrix.new<float>(2, 2, 0)
grid.set(1, 1, close)
cell = grid.get(1, 1)
rows = grid.rows()
cols = grid.columns()
seconds = timeframe.in_seconds("15")
mult = timeframe.multiplier
mtf = request.security(syminfo.tickerid, "15", str.length(str.format("{0}", close)) + timeframe.in_seconds("15"))
if nz(prevRange, 0) >= 0 and nz(prevDev, 0) >= 0 and total == 3 and rows == 2 and cols == 2 and cell > 0 and seconds == 900 and mult >= 1 and mtf > 0
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
			if typed.Collection == "collection_map_keys(labels)" && typed.Variable == "key" {
				foundCollectionLoop = true
			}
		case *strategyir.LetStmt:
			expressions.WriteString(typed.Expression + "\n")
		case *strategyir.CollectionStmt:
			collectionOps.WriteString(typed.Namespace + "." + typed.Operation + ":" + typed.Target + "\n")
		}
	}
	if !foundCollectionLoop {
		t.Fatalf("statements = %#v, want map.keys collection loop", analysis.Program.Hooks[0].Statements)
	}
	for _, fragment := range []string{"collection_array_range(history(values, 1))", "collection_array_stdev(history(values, 1))", "timeframe_in_seconds", "timeframe_multiplier", "str_length", "str_format"} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
	for _, fragment := range []string{"matrix.set:grid", "matrix.get:grid", "matrix.rows:grid", "matrix.columns:grid"} {
		if !strings.Contains(collectionOps.String(), fragment) {
			t.Fatalf("collection ops = %q, missing %q", collectionOps.String(), fragment)
		}
	}
}

func TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v28 object semantic")
type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new(close)
prevPrice = box[1].price
chained = box.identity().score(2)
export helper(float src) => src
export type ExportedBox
export method exportedScore(PriceBox self) => self.price
if nz(prevPrice, 0) >= 0 and chained > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var expressions strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			expressions.WriteString(let.Expression + "\n")
		}
	}
	if !strings.Contains(expressions.String(), "history(box, 1).price") || !strings.Contains(expressions.String(), `object_method("PriceBox", "score", object_method("PriceBox", "identity", box), 2)`) {
		t.Fatalf("expressions = %q, want object history and method chain lowering", expressions.String())
	}
	exports := map[string]string{}
	for _, declaration := range analysis.Semantic.Declarations {
		if declaration.Kind == "export" {
			exports[declaration.Name] = declaration.ExportedKind
		}
	}
	if exports["helper"] != "function" || exports["ExportedBox"] != "type" || exports["exportedScore"] != "method" {
		t.Fatalf("exports = %#v", exports)
	}
}

func TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v29 object history receiver")
type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1, float offset = 0) => self.price * factor + offset
box = PriceBox.new(close)
prevScore = box[1].score(factor=2, offset=1)
chained = box.identity().score(offset=1, factor=2)
mtfPrev = request.security(syminfo.tickerid, "15", box[1].price + box[1].score(offset=1, factor=2))
if nz(prevScore, 0) >= 0 and chained > 0 and mtfPrev >= 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var expressions strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			expressions.WriteString(let.Expression + "\n")
		}
	}
	for _, fragment := range []string{
		`object_method("PriceBox", "score", history(box, 1), 2, 1)`,
		`object_method("PriceBox", "score", object_method("PriceBox", "identity", box), 2, 1)`,
		`history(box, 1).price`,
	} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
}

func TestAnalyzeScriptReportsV29RequestSecurityDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		body string
		code string
	}{
		{name: "dynamic symbol", body: `x = request.security("NASDAQ:AAPL", "D", close)`, code: "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL"},
		{name: "dynamic timeframe", body: `tf = input.timeframe("15", "TF")
x = request.security(syminfo.tickerid, tf + "", close)`, code: "PINE_REQUEST_SECURITY_DYNAMIC_TIMEFRAME"},
		{name: "nested", body: `x = request.security(syminfo.tickerid, "D", request.security(syminfo.tickerid, "15", close))`, code: "PINE_REQUEST_SECURITY_NESTED"},
		{name: "side effect", body: `x = request.security(syminfo.tickerid, "D", alert("no side effects"))`, code: "PINE_REQUEST_SECURITY_SIDE_EFFECT"},
		{name: "lookahead", body: `x = request.security(syminfo.tickerid, "D", close, lookahead=barmerge.lookahead_on)`, code: "PINE_REQUEST_SECURITY_LOOKAHEAD"},
		{name: "gaps", body: `x = request.security(syminfo.tickerid, "D", close, gaps=barmerge.gaps_on)`, code: "PINE_REQUEST_SECURITY_GAPS"},
		{name: "calc_bars_count", body: `x = request.security(syminfo.tickerid, "D", close, calc_bars_count=100)`, code: "PINE_REQUEST_SECURITY_CALC_BARS_COUNT"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			analysis := AnalyzeScript(`//@version=6
strategy("request diagnostics")
`+item.body, AnalysisOptions{IncludeAST: true})
			if analysis.OK {
				t.Fatalf("AnalyzeScript().OK = true, want false")
			}
			found := false
			for _, diagnostic := range analysis.Diagnostics {
				if diagnostic.Code == item.code {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %#v, missing %s", analysis.Diagnostics, item.code)
			}
		})
	}
}

func TestAnalyzeScriptReportsV32RequestSecurityDiagnosticMatrix(t *testing.T) {
	cases := []struct {
		name string
		body string
		code string
	}{
		{name: "tuple expression requires tuple assignment", body: `x = request.security(syminfo.tickerid, "15", [close, open])`, code: "PINE_REQUEST_SECURITY_TUPLE_ASSIGNMENT"},
		{name: "tuple alias mismatch", body: `[mtfClose, mtfOpen] = request.security(syminfo.tickerid, "15", [close, open, high])`, code: "PINE_REQUEST_SECURITY_TUPLE_MISMATCH"},
		{name: "tuple width unsupported", body: `[a, b, c, d, e, f, g, h, i] = request.security(syminfo.tickerid, "15", [open, high, low, close, volume, hl2, hlc3, ohlc4, close])`, code: "PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED"},
		{name: "unsupported pure expression", body: `x = request.security(syminfo.tickerid, "15", ta.not_supported(close))`, code: "PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			analysis := AnalyzeScript(`//@version=6
strategy("v32 request diagnostics")
`+item.body, AnalysisOptions{IncludeAST: true})
			if analysis.OK {
				t.Fatalf("AnalyzeScript().OK = true, want false")
			}
			found := false
			for _, diagnostic := range analysis.Diagnostics {
				if diagnostic.Code == item.code {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %#v, missing %s", analysis.Diagnostics, item.code)
			}
		})
	}
}

func TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v30 semantic")
type PriceBox
    float price = close
method score(PriceBox self, float factor = 1) => self.price * factor
varip count = 0
box = PriceBox.new(close)
score = box.score(2)
count := count + 1
export helper(float src) => src`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Warnings) == 0 || !strings.Contains(strings.Join(analysis.Warnings, "\n"), "varip uses closed-bar var semantics") {
		t.Fatalf("warnings = %#v, want varip policy warning", analysis.Warnings)
	}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Semantic.Declarations {
		declarations[declaration.Kind+":"+declaration.Name] = declaration
	}
	if declarations["type:PriceBox"].Signature != "PriceBox.new(float price = close)" || !declarations["type:PriceBox"].Executable || declarations["type:PriceBox"].UnsupportedReason != "" {
		t.Fatalf("type declaration = %#v", declarations["type:PriceBox"])
	}
	if declarations["method:score"].Signature != "score(PriceBox self, float factor = 1)" || !declarations["method:score"].Executable || declarations["method:score"].UnsupportedReason != "" {
		t.Fatalf("method declaration = %#v", declarations["method:score"])
	}
	if declarations["export:helper"].Signature != "export helper(float src)" || declarations["export:helper"].UnsupportedReason == "" {
		t.Fatalf("export declaration = %#v", declarations["export:helper"])
	}

	importAnalysis := AnalyzeScript(`//@version=6
strategy("v30 import")
import TradingView/ta/7 as tools`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if importAnalysis.Semantic == nil || len(importAnalysis.Semantic.Declarations) == 0 {
		t.Fatalf("import semantic = %#v", importAnalysis.Semantic)
	}
	importDeclaration := importAnalysis.Semantic.Declarations[0]
	if importDeclaration.Signature != "import TradingView/ta/7 as tools" || importDeclaration.Version != "7" || importDeclaration.UnsupportedReason == "" {
		t.Fatalf("import declaration = %#v", importDeclaration)
	}
}

func TestAnalyzeScriptReportsCollectionTypeDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Collection Types", overlay=true)
array<float, int> tooMany = na
map<string> missingValue = na
matrix<float, int> badMatrix = matrix.new<float, int>(1, 1)
array<float> wrongNamespace = map.new<string, float>()
array<int> wrongElement = array.new_float(0)
map<string, float> wrongMap = map.new<string, int>()`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want collection type diagnostics")
	}
	typeDiagnostics := make([]Diagnostic, 0)
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_TYPE" {
			typeDiagnostics = append(typeDiagnostics, diagnostic)
		}
	}
	if len(typeDiagnostics) != 7 {
		t.Fatalf("diagnostics = %#v, want seven collection type diagnostics", analysis.Diagnostics)
	}
	messageParts := make([]string, 0, len(typeDiagnostics))
	for _, diagnostic := range typeDiagnostics {
		messageParts = append(messageParts, diagnostic.Message)
	}
	messages := strings.Join(messageParts, "\n")
	for _, fragment := range []string{
		"type annotation requires 1 type argument(s), got 2",
		"type annotation requires 2 type argument(s), got 1",
		"matrix.new requires 1 type argument(s), got 2",
		"array declaration cannot be initialized with map.new",
		"wrongElement type arguments <int> do not match array.new_float element types <float>",
		"wrongMap type arguments <string, float> do not match map.new element types <string, int>",
	} {
		if !strings.Contains(messages, fragment) {
			t.Fatalf("diagnostic messages = %q, missing %q", messages, fragment)
		}
	}
}

func TestAnalyzeScriptReportsCollectionMethodStyleSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Collection Methods", overlay=true)
arr = array.new_float(0)
grid = matrix.new<float>(1, 1)
arr.push()
grid.set(0, 0)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want collection method diagnostics")
	}
	if len(analysis.CollectionOperations) != 4 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	signatureDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_SIGNATURE" {
			signatureDiagnostics++
		}
	}
	if signatureDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two collection method signature diagnostics", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptReportsDeclarationSemanticDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Declaration", overlay=true)
type TradeBox
    float price = close
    int price = 0
method reset(TradeBox box, float limit, int limit) =>
    box`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want declaration diagnostics")
	}
	declarationDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			declarationDiagnostics++
		}
	}
	if declarationDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two declaration diagnostics", analysis.Diagnostics)
	}
	if analysis.Semantic == nil || len(analysis.Semantic.Declarations) != 2 {
		t.Fatalf("semantic declarations = %#v", analysis.Semantic)
	}
	if len(analysis.Semantic.Declarations[0].Fields) != 2 {
		t.Fatalf("type fields = %#v", analysis.Semantic.Declarations[0].Fields)
	}
}

func TestAnalyzeScriptReportsTypeAndMethodRegistryDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Declaration Registry", overlay=true)
type TradeBox
    float price
type TradeBox
    int bars
method missing() =>
    na
method untyped(box) =>
    box
method haunt(Ghost ghost) =>
    ghost
method reset(TradeBox box, float limit) =>
    box
method reset(TradeBox target, float threshold = 0) =>
    target
method reset(TradeBox box, float limit, int bars = 0) =>
    box
method put(map<string, float> values, string key, float value = close) =>
    values
box = TradeBox.new(close)
updated = box.reset(10, 1)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want declaration registry diagnostics")
	}
	declarationDiagnostics := make([]Diagnostic, 0)
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			declarationDiagnostics = append(declarationDiagnostics, diagnostic)
		}
	}
	if len(declarationDiagnostics) != 5 {
		t.Fatalf("diagnostics = %#v, want five declaration registry diagnostics", analysis.Diagnostics)
	}
	messages := make([]string, 0, len(declarationDiagnostics))
	for _, diagnostic := range declarationDiagnostics {
		messages = append(messages, diagnostic.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, fragment := range []string{
		"type TradeBox is already declared",
		"method missing requires a receiver parameter",
		"method untyped requires a typed receiver",
		"method haunt receiver type Ghost is not declared",
		"method reset is already declared for receiver TradeBox with this signature",
	} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("diagnostic messages = %q, missing %q", joined, fragment)
		}
	}
	var collectionMethod SemanticDeclaration
	for _, declaration := range analysis.Declarations {
		if declaration.Kind == "method" && declaration.Name == "put" {
			collectionMethod = declaration
		}
	}
	if collectionMethod.Receiver == nil || collectionMethod.Receiver.Type != "map<string, float>" || collectionMethod.Receiver.Name != "values" || len(collectionMethod.Parameters) != 3 {
		t.Fatalf("collection method declaration = %#v", collectionMethod)
	}
	if len(analysis.ObjectOperations) != 2 {
		t.Fatalf("object operations = %#v", analysis.ObjectOperations)
	}
	methodOperation := analysis.ObjectOperations[1]
	if methodOperation.Kind != "method" || methodOperation.Signature != "reset(TradeBox box, float limit, int bars = 0)" || len(methodOperation.Arguments) != 2 {
		t.Fatalf("overloaded method operation = %#v", methodOperation)
	}
}

func TestAnalyzeScriptReportsImportAliasDeclarationDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Import Alias", overlay=true)
import TradingView/ta/7 as tools
import TradingView/math/1 as tools`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want declaration diagnostics")
	}
	if analysis.Semantic == nil || len(analysis.Semantic.Declarations) != 2 {
		t.Fatalf("semantic declarations = %#v", analysis.Semantic)
	}
	if analysis.Semantic.Declarations[0].ImportPath != "TradingView/ta/7" || analysis.Semantic.Declarations[0].Alias != "tools" || analysis.Semantic.Declarations[0].Version != "7" {
		t.Fatalf("first import declaration = %#v", analysis.Semantic.Declarations[0])
	}
	if analysis.Semantic.Declarations[1].ImportPath != "TradingView/math/1" || analysis.Semantic.Declarations[1].Alias != "tools" || analysis.Semantic.Declarations[1].Version != "1" {
		t.Fatalf("second import declaration = %#v", analysis.Semantic.Declarations[1])
	}
	aliasDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			aliasDiagnostics++
		}
	}
	if aliasDiagnostics != 1 {
		t.Fatalf("diagnostics = %#v, want one import alias diagnostic", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptReportsObjectOperationSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Object", overlay=true)
type TradeBox
    float price
    int bars = 0
method reset(TradeBox box, float limit, int bars = 0) =>
    box
box = TradeBox.new()
tooWide = TradeBox.new(close, 0, 1)
resetTooFew = box.reset()
resetTooWide = box.reset(10, 1, 2)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want object diagnostics")
	}
	if len(analysis.ObjectOperations) != 4 {
		t.Fatalf("object operations = %#v", analysis.ObjectOperations)
	}
	signatureDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_OBJECT_SIGNATURE" {
			signatureDiagnostics++
		}
	}
	if signatureDiagnostics != 4 {
		t.Fatalf("diagnostics = %#v, want four object signature diagnostics", analysis.Diagnostics)
	}
}
