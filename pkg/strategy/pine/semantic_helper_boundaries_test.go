package pine

import (
	"strings"
	"testing"
)

func TestSemanticHelpersHandleDeclarationAndParameterBoundaries(t *testing.T) {
	summary, diagnostics := analyzeSemantics(nil)
	if summary == nil || len(summary.Symbols) != 0 || len(diagnostics) != 0 {
		t.Fatalf("analyzeSemantics(nil) summary=%#v diagnostics=%#v", summary, diagnostics)
	}

	if got := semanticDeclarationSignature(SemanticDeclaration{Kind: "export", Name: "helper"}); got != "export helper" {
		t.Fatalf("export helper signature = %q", got)
	}
	if got := semanticDeclarationSignature(SemanticDeclaration{Kind: "library", Name: `JF"Trade`}); got != `library("JF\"Trade")` {
		t.Fatalf("library signature = %q", got)
	}
	if got := semanticDeclarationSignature(SemanticDeclaration{Kind: "declaration", Name: "raw"}); got != "raw" {
		t.Fatalf("default declaration signature = %q", got)
	}

	path, alias := parseImportDeclaration("TradingView/ta/7")
	if path != "TradingView/ta/7" || alias != "" {
		t.Fatalf("parseImportDeclaration path=%q alias=%q", path, alias)
	}
	path, alias = parseImportDeclaration("   ")
	if path != "" || alias != "" {
		t.Fatalf("blank parseImportDeclaration path=%q alias=%q", path, alias)
	}
	for input, want := range map[string]string{
		"TradingView/ta/7":  "7",
		"TradingView/ta/v7": "",
		"TradingView/ta/":   "",
		"":                  "",
	} {
		if got := importVersion(input); got != want {
			t.Fatalf("importVersion(%q) = %q, want %q", input, got, want)
		}
	}

	if name, receiver, params := parseMethodDeclaration("score"); name != "score" || receiver != nil || params != nil {
		t.Fatalf("parseMethodDeclaration without params name=%q receiver=%#v params=%#v", name, receiver, params)
	}
	if name, receiver, params := parseMethodDeclaration("score(PriceBox self"); name != "score" || receiver != nil || params != nil {
		t.Fatalf("parseMethodDeclaration unmatched name=%q receiver=%#v params=%#v", name, receiver, params)
	}
	params := parseSemanticParameters(`array<float> values, string label = "A,B", float threshold = math.max(close, open), ,`)
	if len(params) != 3 || params[0].Type != "array<float>" || params[1].Default != `"A,B"` || params[2].Default != "math.max(close, open)" {
		t.Fatalf("parseSemanticParameters = %#v", params)
	}
	if got := parseSemanticParameter(""); got != (SemanticParameter{}) {
		t.Fatalf("blank parseSemanticParameter = %#v", got)
	}
}

func TestSemanticHelpersReflectPineCollectionAndObjectContracts(t *testing.T) {
	line := ASTLine{Line: 10, Text: "array.unknown(arr)", Kind: NodeKindUnsupported}
	operations := semanticCollectionOperations(line, nil)
	if len(operations) != 1 || !collectionOperationsContainNonExecutable(operations) {
		t.Fatalf("unknown operations = %#v, want non-executable", operations)
	}
	if collectionOperationsContainNonExecutable([]SemanticCollectionOperation{{Executable: true}, {Executable: true}}) {
		t.Fatal("all executable collection operations reported as non-executable")
	}
	if got := collectionOperationReason("array", "push"); got != "" {
		t.Fatalf("array.push reason = %q, want executable", got)
	}
	if got := collectionOperationReason("array", "unknown"); !strings.Contains(got, "parse-only") {
		t.Fatalf("array.unknown reason = %q, want parse-only", got)
	}

	if namespace, args := collectionTypeAnnotationInfo("map"); namespace != "map" || args != "" {
		t.Fatalf("map annotation namespace=%q args=%q", namespace, args)
	}
	if count, ok := collectionTypeArgumentCount("queue"); ok || count != 0 {
		t.Fatalf("unknown collection arg count=%d ok=%v", count, ok)
	}
	if got := collectionTypeArguments("array<float>, map<string, float>, math.max(close, open)"); len(got) != 3 || got[1] != "map<string, float>" {
		t.Fatalf("collectionTypeArguments = %#v", got)
	}
	if got := collectionConstructorTypeArguments(SemanticCollectionOperation{Namespace: "array", Operation: "new_color"}); got != nil {
		t.Fatalf("unknown array constructor type args = %#v", got)
	}

	if isKnownMethodReceiverType("", nil) {
		t.Fatal("blank receiver type reported as known")
	}
	if isKnownMethodReceiverType("Ghost", nil) {
		t.Fatal("unknown receiver type reported as known")
	}
	line = ASTLine{Line: 20, Text: "notMethod"}
	if diagnostics, register := semanticMethodRegistryDiagnostics(line, SemanticDeclaration{Kind: "type", Name: "PriceBox"}, nil, nil); diagnostics != nil || register {
		t.Fatalf("non-method registry diagnostics=%#v register=%v", diagnostics, register)
	}
}

func TestSemanticHelpersReportMalformedScriptBoundaries(t *testing.T) {
	ast := &AST{Lines: []ASTLine{
		{Line: 1, Kind: NodeKindTupleAssignment, Text: "[fast, fast] = ta.macd(close, 12, 26, 9)", Expression: "ta.macd(close, 12, 26, 9)"},
		{Line: 2, Kind: NodeKindUnsupported, Text: "array.unknown(values)"},
	}}
	summary, diagnostics := analyzeSemantics(ast)
	if len(summary.TupleBindings) != 1 || summary.TupleBindings[0].ReturnCount != 3 {
		t.Fatalf("tuple summary = %#v", summary.TupleBindings)
	}
	joined := diagnosticsText(diagnostics)
	if !strings.Contains(joined, "tuple assignment repeats fast") || !strings.Contains(joined, "Pine collection namespaces") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}

	methodDiagnostics := semanticDeclarationDiagnostics(ASTLine{Line: 3}, SemanticDeclaration{
		Kind: "method", Name: "score", Parameters: []SemanticParameter{{Type: "float"}, {Name: "value"}, {Name: "value"}},
	})
	if len(methodDiagnostics) != 1 || !strings.Contains(methodDiagnostics[0].Message, "repeats parameter value") {
		t.Fatalf("method diagnostics = %#v", methodDiagnostics)
	}
	if got := semanticImportAliasDiagnostics(ASTLine{Line: 4}, SemanticDeclaration{Kind: "import", Alias: "   "}, map[string]bool{}); got != nil {
		t.Fatalf("blank import alias diagnostics = %#v", got)
	}
	if !isKnownMethodReceiverType("label", nil) {
		t.Fatal("label should be accepted as built-in method receiver")
	}
}

//nolint:funlen
func TestSemanticHelpersCoverObjectAndVisualFallbacks(t *testing.T) {
	if isCollectionGenericOpen("request<security>", len("request")) {
		t.Fatal("non-collection generic opener accepted")
	}
	if isCollectionGenericOpen("array.<float>", len("array.")) {
		t.Fatal("collection generic opener should not cross punctuation")
	}
	if got := parseSemanticParameter(" = close"); got != (SemanticParameter{}) {
		t.Fatalf("blank named default parameter = %#v", got)
	}
	if got := semanticCollectionMethodOperations(ASTLine{Line: 1, Text: "arr.new(1)"}, map[string]string{"arr": "array"}); len(got) != 0 {
		t.Fatalf("constructor-style collection method operations = %#v", got)
	}
	if got := semanticCollectionMethodOperations(ASTLine{Line: 2, Text: "arr.push("}, map[string]string{"arr": "array"}); len(got) != 0 {
		t.Fatalf("malformed collection method operations = %#v", got)
	}

	typeDecls := map[string]SemanticDeclaration{"pricebox": {Kind: "type", Name: "PriceBox"}}
	if got := semanticObjectOperations(ASTLine{Line: 3, Text: "Ghost.new()", Name: "ghost"}, typeDecls, nil, nil); len(got) != 0 {
		t.Fatalf("unknown constructor operations = %#v", got)
	}
	if got := semanticObjectOperations(ASTLine{Line: 4, Text: "box.score("}, typeDecls, nil, map[string]string{"box": "PriceBox"}); len(got) != 0 {
		t.Fatalf("malformed object method operations = %#v", got)
	}
	if got := semanticObjectOperations(ASTLine{Line: 5, Text: "box.score()"}, typeDecls, nil, map[string]string{"box": "PriceBox"}); len(got) != 0 {
		t.Fatalf("missing method declaration operations = %#v", got)
	}

	methodKey := semanticMethodSignatureKey(SemanticDeclaration{Name: "score", Receiver: &SemanticParameter{Name: "self"}, Parameters: []SemanticParameter{{Name: "self"}}})
	if !strings.Contains(methodKey, "(?)") {
		t.Fatalf("method signature key = %q, want unknown receiver type marker", methodKey)
	}
	if _, ok := resolveSemanticMethodDeclaration(nil, 1); ok {
		t.Fatal("empty method declarations resolved")
	}
	if got := objectOperationDiagnostics(ASTLine{Line: 6}, []SemanticObjectOperation{{Kind: "constructor", Type: "Missing"}}, nil, nil); len(got) != 0 {
		t.Fatalf("missing object declaration diagnostics = %#v", got)
	}
	if _, _, ok := objectOperationArgBounds(SemanticObjectOperation{Kind: "unknown"}, nil, nil); ok {
		t.Fatal("unknown object operation kind returned bounds")
	}
	if _, _, ok := objectOperationArgBounds(SemanticObjectOperation{Kind: "method", Type: "PriceBox", Method: "missing"}, nil, nil); ok {
		t.Fatal("missing method declarations returned bounds")
	}
	fallbackMethod, ok := semanticMethodDeclarationForOperation(
		[]SemanticDeclaration{{Name: "score", Parameters: []SemanticParameter{{Type: "PriceBox", Name: "self"}, {Type: "float", Name: "factor"}}}},
		SemanticObjectOperation{Method: "score", Signature: "score(PriceBox self, int factor)", Arguments: []string{"1"}},
	)
	if !ok || fallbackMethod.Name != "score" {
		t.Fatalf("fallback method declaration = %#v/%v", fallbackMethod, ok)
	}

	if got := collectionTypeArgumentDiagnostics(ASTLine{Line: 7}, "queue.new", "queue", "float"); got != nil {
		t.Fatalf("unknown collection type diagnostics = %#v", got)
	}
	if got := collectionConstructorTypeArguments(SemanticCollectionOperation{Namespace: "map", Operation: "new"}); got != nil {
		t.Fatalf("map constructor inferred array type args = %#v", got)
	}
	if equalCollectionTypeArguments([]string{"float"}, []string{"float", "int"}) {
		t.Fatal("mismatched collection type argument lengths reported equal")
	}

	if firstDeclarationName("") != "" || firstDeclarationName("helper(float src)") != "helper" {
		t.Fatalf("firstDeclarationName boundaries failed")
	}
	if got := semanticVisualCalls("plot(close"); len(got) != 0 {
		t.Fatalf("malformed visual calls = %#v", got)
	}
	if visualMetadataKind("bgcolor") != "color" || visualMetadataTarget("plot", nil) != "" || visualMetadataTarget("plot", []string{"series=close"}) != "close" {
		t.Fatalf("visual metadata fallback failed")
	}
	if visualMetadataTitle("plot", []string{"close", `"Close"`}, nil) != "Close" || visualMetadataTitle("hline", []string{"50", `"Mid"`}, nil) != "Mid" {
		t.Fatalf("visual metadata titles failed")
	}

	diagnostic := semanticDiagnostic(ASTLine{Line: 8, Column: 0, EndColumn: 0}, "TEST", "message")
	if diagnostic.Column != 1 || diagnostic.EndColumn != 2 {
		t.Fatalf("semanticDiagnostic columns = %d-%d", diagnostic.Column, diagnostic.EndColumn)
	}
	if tupleNamesFromASTLine(ASTLine{Text: "not a tuple"}) != nil {
		t.Fatal("non-tuple line returned tuple names")
	}
	if duplicates := duplicateNames([]string{"_", "_", "fast", "fast"}); len(duplicates) != 1 || duplicates[0] != "fast" {
		t.Fatalf("duplicates = %#v", duplicates)
	}
	if count, ok := semanticTupleReturnCount("not a call"); count != 0 || ok {
		t.Fatalf("semanticTupleReturnCount = %d/%v", count, ok)
	}
	if inferSemanticValueKind("   ") != SemanticValueUnknown {
		t.Fatal("blank expression should infer unknown")
	}
	if calls := semanticFunctionCalls("ta.sma(close"); len(calls) != 0 {
		t.Fatalf("malformed semantic calls = %#v", calls)
	}
}

func diagnosticsText(diagnostics []Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Code+": "+diagnostic.Message)
	}
	return strings.Join(parts, "\n")
}
