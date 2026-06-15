package pine

import (
	"fmt"
	"regexp"
	"strings"
)

type SemanticValueKind string

const (
	SemanticValueUnknown SemanticValueKind = "unknown"
	SemanticValueConst   SemanticValueKind = "const"
	SemanticValueSimple  SemanticValueKind = "simple"
	SemanticValueSeries  SemanticValueKind = "series"
	SemanticValueObject  SemanticValueKind = "object"
)

type SemanticSummary struct {
	Symbols              []SemanticSymbol              `json:"symbols"`
	TupleBindings        []SemanticTupleBinding        `json:"tupleBindings,omitempty"`
	FunctionCalls        []SemanticFunctionCall        `json:"functionCalls,omitempty"`
	Declarations         []SemanticDeclaration         `json:"declarations,omitempty"`
	CollectionOperations []SemanticCollectionOperation `json:"collectionOperations,omitempty"`
	ObjectOperations     []SemanticObjectOperation     `json:"objectOperations,omitempty"`
	Visuals              []PineVisualMetadata          `json:"visuals,omitempty"`
}

type SemanticSymbol struct {
	Name       string            `json:"name"`
	Line       int               `json:"line"`
	Scope      string            `json:"scope"`
	ValueKind  SemanticValueKind `json:"valueKind"`
	Assignment string            `json:"assignment"`
}

type SemanticTupleBinding struct {
	Line        int      `json:"line"`
	Names       []string `json:"names"`
	Expression  string   `json:"expression"`
	ReturnCount int      `json:"returnCount"`
	Supported   bool     `json:"supported"`
}

type SemanticFunctionCall struct {
	Line       int               `json:"line"`
	Name       string            `json:"name"`
	ArgCount   int               `json:"argCount"`
	Signature  string            `json:"signature,omitempty"`
	ReturnKind SemanticValueKind `json:"returnKind"`
	Supported  bool              `json:"supported"`
}

type SemanticDeclaration struct {
	Line              int                 `json:"line"`
	Kind              string              `json:"kind"`
	Name              string              `json:"name,omitempty"`
	ExportedKind      string              `json:"exportedKind,omitempty"`
	Namespace         string              `json:"namespace,omitempty"`
	Alias             string              `json:"alias,omitempty"`
	Call              string              `json:"call,omitempty"`
	TypeArgs          string              `json:"typeArgs,omitempty"`
	Signature         string              `json:"signature,omitempty"`
	Receiver          *SemanticParameter  `json:"receiver,omitempty"`
	Parameters        []SemanticParameter `json:"parameters,omitempty"`
	Fields            []SemanticParameter `json:"fields,omitempty"`
	ImportPath        string              `json:"importPath,omitempty"`
	Version           string              `json:"version,omitempty"`
	Executable        bool                `json:"executable"`
	Reason            string              `json:"reason,omitempty"`
	UnsupportedReason string              `json:"unsupportedReason,omitempty"`
}

type SemanticParameter struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Default string `json:"default,omitempty"`
}

type SemanticCollectionOperation struct {
	Line       int      `json:"line"`
	Namespace  string   `json:"namespace"`
	Operation  string   `json:"operation"`
	Call       string   `json:"call"`
	TypeArgs   string   `json:"typeArgs,omitempty"`
	Signature  string   `json:"signature,omitempty"`
	Target     string   `json:"target,omitempty"`
	Arguments  []string `json:"arguments,omitempty"`
	Mutates    bool     `json:"mutates"`
	Supported  bool     `json:"supported"`
	Executable bool     `json:"executable"`
	Reason     string   `json:"reason,omitempty"`
}

type SemanticObjectOperation struct {
	Line       int      `json:"line"`
	Kind       string   `json:"kind"`
	Type       string   `json:"type,omitempty"`
	Method     string   `json:"method,omitempty"`
	Call       string   `json:"call"`
	Signature  string   `json:"signature,omitempty"`
	Target     string   `json:"target,omitempty"`
	Arguments  []string `json:"arguments,omitempty"`
	Supported  bool     `json:"supported"`
	Executable bool     `json:"executable"`
	Reason     string   `json:"reason,omitempty"`
}

type PineVisualMetadata struct {
	Line      int               `json:"line"`
	Kind      string            `json:"kind"`
	Call      string            `json:"call"`
	Variable  string            `json:"variable,omitempty"`
	Target    string            `json:"target,omitempty"`
	Title     string            `json:"title,omitempty"`
	Arguments []string          `json:"arguments,omitempty"`
	NamedArgs map[string]string `json:"namedArgs,omitempty"`
	Text      string            `json:"text"`
}

type semanticSignature struct {
	minArgs    int
	maxArgs    int
	signature  string
	returnKind SemanticValueKind
}

type collectionOperationSignature struct {
	minArgs   int
	maxArgs   int
	signature string
}

var semanticFunctionSignatures = map[string]semanticSignature{
	"input.int":            {minArgs: 1, maxArgs: 8, signature: "input.int(defval, title?)", returnKind: SemanticValueSimple},
	"input.float":          {minArgs: 1, maxArgs: 8, signature: "input.float(defval, title?)", returnKind: SemanticValueSimple},
	"input.bool":           {minArgs: 1, maxArgs: 8, signature: "input.bool(defval, title?)", returnKind: SemanticValueSimple},
	"input.string":         {minArgs: 1, maxArgs: 8, signature: "input.string(defval, title?)", returnKind: SemanticValueSimple},
	"input.source":         {minArgs: 1, maxArgs: 8, signature: "input.source(defval, title?)", returnKind: SemanticValueSeries},
	"input.time":           {minArgs: 1, maxArgs: 8, signature: "input.time(defval, title?)", returnKind: SemanticValueSimple},
	"input.timeframe":      {minArgs: 1, maxArgs: 8, signature: "input.timeframe(defval, title?)", returnKind: SemanticValueSimple},
	"input.color":          {minArgs: 1, maxArgs: 8, signature: "input.color(defval, title?)", returnKind: SemanticValueSimple},
	"ta.ema":               {minArgs: 2, maxArgs: 2, signature: "ta.ema(source, length)", returnKind: SemanticValueSeries},
	"ta.sma":               {minArgs: 2, maxArgs: 2, signature: "ta.sma(source, length)", returnKind: SemanticValueSeries},
	"ta.rma":               {minArgs: 2, maxArgs: 2, signature: "ta.rma(source, length)", returnKind: SemanticValueSeries},
	"ta.wma":               {minArgs: 2, maxArgs: 2, signature: "ta.wma(source, length)", returnKind: SemanticValueSeries},
	"ta.hma":               {minArgs: 2, maxArgs: 2, signature: "ta.hma(source, length)", returnKind: SemanticValueSeries},
	"ta.vwma":              {minArgs: 2, maxArgs: 2, signature: "ta.vwma(source, length)", returnKind: SemanticValueSeries},
	"ta.rsi":               {minArgs: 2, maxArgs: 2, signature: "ta.rsi(source, length)", returnKind: SemanticValueSeries},
	"ta.atr":               {minArgs: 1, maxArgs: 1, signature: "ta.atr(length)", returnKind: SemanticValueSeries},
	"ta.macd":              {minArgs: 4, maxArgs: 4, signature: "ta.macd(source, fast, slow, signal)", returnKind: SemanticValueObject},
	"ta.bb":                {minArgs: 3, maxArgs: 3, signature: "ta.bb(source, length, mult)", returnKind: SemanticValueObject},
	"ta.bbw":               {minArgs: 3, maxArgs: 3, signature: "ta.bbw(source, length, mult)", returnKind: SemanticValueSeries},
	"ta.cog":               {minArgs: 2, maxArgs: 2, signature: "ta.cog(source, length)", returnKind: SemanticValueSeries},
	"ta.vwap":              {minArgs: 0, maxArgs: 3, signature: "ta.vwap(source?, anchor?, stdev_mult?)", returnKind: SemanticValueSeries},
	"ta.supertrend":        {minArgs: 2, maxArgs: 2, signature: "ta.supertrend(factor, atrPeriod)", returnKind: SemanticValueObject},
	"ta.kc":                {minArgs: 3, maxArgs: 4, signature: "ta.kc(source, length, mult, useTrueRange?)", returnKind: SemanticValueObject},
	"str.length":           {minArgs: 1, maxArgs: 1, signature: "str.length(source)", returnKind: SemanticValueSimple},
	"str.contains":         {minArgs: 2, maxArgs: 2, signature: "str.contains(source, needle)", returnKind: SemanticValueSimple},
	"str.pos":              {minArgs: 2, maxArgs: 2, signature: "str.pos(source, needle)", returnKind: SemanticValueSimple},
	"str.substring":        {minArgs: 2, maxArgs: 3, signature: "str.substring(source, begin, end?)", returnKind: SemanticValueSimple},
	"str.replace":          {minArgs: 3, maxArgs: 3, signature: "str.replace(source, target, replacement)", returnKind: SemanticValueSimple},
	"str.upper":            {minArgs: 1, maxArgs: 1, signature: "str.upper(source)", returnKind: SemanticValueSimple},
	"str.lower":            {minArgs: 1, maxArgs: 1, signature: "str.lower(source)", returnKind: SemanticValueSimple},
	"str.format":           {minArgs: 1, maxArgs: 16, signature: "str.format(format, ...)", returnKind: SemanticValueSimple},
	"timeframe.change":     {minArgs: 1, maxArgs: 1, signature: "timeframe.change(timeframe)", returnKind: SemanticValueSimple},
	"timeframe.in_seconds": {minArgs: 0, maxArgs: 1, signature: "timeframe.in_seconds(timeframe?)", returnKind: SemanticValueSimple},
	"request.security":     {minArgs: 3, maxArgs: 5, signature: "request.security(syminfo.tickerid, timeframe, expression, gaps?, lookahead?)", returnKind: SemanticValueSeries},
	"strategy.entry":       {minArgs: 2, maxArgs: 12, signature: "strategy.entry(id, direction, ...)", returnKind: SemanticValueUnknown},
	"strategy.order":       {minArgs: 2, maxArgs: 12, signature: "strategy.order(id, direction, ...)", returnKind: SemanticValueUnknown},
	"strategy.close":       {minArgs: 1, maxArgs: 8, signature: "strategy.close(id, ...)", returnKind: SemanticValueUnknown},
	"strategy.exit":        {minArgs: 2, maxArgs: 16, signature: "strategy.exit(id, from_entry, ...)", returnKind: SemanticValueUnknown},
}

var collectionCallPattern = regexp.MustCompile(`(?i)\b(array|map|matrix)\.([A-Za-z_][A-Za-z0-9_]*)(<[^>]+>)?\s*\(`)
var objectCallPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
var objectFieldCollectionCallPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

var collectionOperationSignatures = map[string]collectionOperationSignature{
	"array.new_float":                       {minArgs: 0, maxArgs: 2, signature: "array.new_float(size?, initial_value?)"},
	"array.new_int":                         {minArgs: 0, maxArgs: 2, signature: "array.new_int(size?, initial_value?)"},
	"array.new_bool":                        {minArgs: 0, maxArgs: 2, signature: "array.new_bool(size?, initial_value?)"},
	"array.new_string":                      {minArgs: 0, maxArgs: 2, signature: "array.new_string(size?, initial_value?)"},
	"array.new":                             {minArgs: 0, maxArgs: 2, signature: "array.new<T>(size?, initial_value?)"},
	"array.get":                             {minArgs: 2, maxArgs: 2, signature: "array.get(id, index)"},
	"array.set":                             {minArgs: 3, maxArgs: 3, signature: "array.set(id, index, value)"},
	"array.push":                            {minArgs: 2, maxArgs: 2, signature: "array.push(id, value)"},
	"array.pop":                             {minArgs: 1, maxArgs: 1, signature: "array.pop(id)"},
	"array.shift":                           {minArgs: 1, maxArgs: 1, signature: "array.shift(id)"},
	"array.unshift":                         {minArgs: 2, maxArgs: 2, signature: "array.unshift(id, value)"},
	"array.insert":                          {minArgs: 3, maxArgs: 3, signature: "array.insert(id, index, value)"},
	"array.remove":                          {minArgs: 2, maxArgs: 2, signature: "array.remove(id, index)"},
	"array.first":                           {minArgs: 1, maxArgs: 1, signature: "array.first(id)"},
	"array.last":                            {minArgs: 1, maxArgs: 1, signature: "array.last(id)"},
	"array.size":                            {minArgs: 1, maxArgs: 1, signature: "array.size(id)"},
	"array.clear":                           {minArgs: 1, maxArgs: 1, signature: "array.clear(id)"},
	"array.abs":                             {minArgs: 1, maxArgs: 1, signature: "array.abs(id)"},
	"array.binary_search_leftmost":          {minArgs: 2, maxArgs: 2, signature: "array.binary_search_leftmost(id, value)"},
	"array.binary_search_rightmost":         {minArgs: 2, maxArgs: 2, signature: "array.binary_search_rightmost(id, value)"},
	"array.percentrank":                     {minArgs: 2, maxArgs: 2, signature: "array.percentrank(id, index)"},
	"array.percentile_nearest_rank":         {minArgs: 2, maxArgs: 2, signature: "array.percentile_nearest_rank(id, percentage)"},
	"array.percentile_linear_interpolation": {minArgs: 2, maxArgs: 2, signature: "array.percentile_linear_interpolation(id, percentage)"},
	"array.stdev":                           {minArgs: 1, maxArgs: 1, signature: "array.stdev(id)"},
	"array.variance":                        {minArgs: 1, maxArgs: 1, signature: "array.variance(id)"},
	"array.covariance":                      {minArgs: 2, maxArgs: 2, signature: "array.covariance(id, other)"},
	"map.new":                               {minArgs: 0, maxArgs: 0, signature: "map.new<K, V>()"},
	"map.get":                               {minArgs: 2, maxArgs: 2, signature: "map.get(id, key)"},
	"map.put":                               {minArgs: 3, maxArgs: 3, signature: "map.put(id, key, value)"},
	"map.remove":                            {minArgs: 2, maxArgs: 2, signature: "map.remove(id, key)"},
	"map.contains":                          {minArgs: 2, maxArgs: 2, signature: "map.contains(id, key)"},
	"map.size":                              {minArgs: 1, maxArgs: 1, signature: "map.size(id)"},
	"map.clear":                             {minArgs: 1, maxArgs: 1, signature: "map.clear(id)"},
	"matrix.new":                            {minArgs: 2, maxArgs: 3, signature: "matrix.new<T>(rows, columns, initial_value?)"},
	"matrix.get":                            {minArgs: 3, maxArgs: 3, signature: "matrix.get(id, row, column)"},
	"matrix.set":                            {minArgs: 4, maxArgs: 4, signature: "matrix.set(id, row, column, value)"},
	"matrix.rows":                           {minArgs: 1, maxArgs: 1, signature: "matrix.rows(id)"},
	"matrix.columns":                        {minArgs: 1, maxArgs: 1, signature: "matrix.columns(id)"},
}

func analyzeSemantics(ast *AST) (*SemanticSummary, []Diagnostic) {
	summary := &SemanticSummary{
		Symbols:              []SemanticSymbol{},
		TupleBindings:        []SemanticTupleBinding{},
		FunctionCalls:        []SemanticFunctionCall{},
		Declarations:         []SemanticDeclaration{},
		CollectionOperations: []SemanticCollectionOperation{},
		ObjectOperations:     []SemanticObjectOperation{},
		Visuals:              []PineVisualMetadata{},
	}
	if ast == nil {
		return summary, nil
	}
	diagnostics := make([]Diagnostic, 0)
	activeTypeIndex := -1
	activeTypeIndent := 0
	activeTypeFields := map[string]bool(nil)
	activeTypeRegistered := false
	typeDeclarations := map[string]SemanticDeclaration{}
	methodDeclarations := map[string][]SemanticDeclaration{}
	methodSignatures := map[string]bool{}
	objectTypes := map[string]string{}
	collectionNamespaces := map[string]string{}
	importAliases := map[string]bool{}
	for _, line := range ast.Lines {
		if activeTypeIndex >= 0 && line.Indent <= activeTypeIndent {
			activeTypeIndex = -1
			activeTypeFields = nil
			activeTypeRegistered = false
		}
		collectionOperations := semanticCollectionOperations(line, collectionNamespaces)
		objectOperations := semanticObjectOperations(line, typeDeclarations, methodDeclarations, objectTypes)
		switch line.Kind {
		case NodeKindAssignment:
			valueKind := inferSemanticValueKind(line.Expression)
			if operation, ok := assignedObjectConstructor(objectOperations, line.Name); ok {
				valueKind = SemanticValueObject
				objectTypes[strings.ToLower(line.Name)] = operation.Type
			}
			summary.Symbols = append(summary.Symbols, SemanticSymbol{
				Name:       line.Name,
				Line:       line.Line,
				Scope:      semanticScope(line.Indent),
				ValueKind:  valueKind,
				Assignment: string(line.Mode),
			})
		case NodeKindTupleAssignment:
			names := tupleNamesFromASTLine(line)
			duplicates := duplicateNames(names)
			if len(duplicates) > 0 {
				diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_TUPLE", fmt.Sprintf("tuple assignment repeats %s", strings.Join(duplicates, ", "))))
			}
			returnCount, supported := semanticTupleReturnCount(line.Expression)
			summary.TupleBindings = append(summary.TupleBindings, SemanticTupleBinding{
				Line:        line.Line,
				Names:       names,
				Expression:  line.Expression,
				ReturnCount: returnCount,
				Supported:   supported,
			})
			for _, name := range names {
				summary.Symbols = append(summary.Symbols, SemanticSymbol{
					Name:       name,
					Line:       line.Line,
					Scope:      semanticScope(line.Indent),
					ValueKind:  inferTupleElementKind(line.Expression),
					Assignment: string(line.Mode),
				})
			}
		case NodeKindDeclaration:
			declaration := semanticDeclaration(line)
			summary.Declarations = append(summary.Declarations, declaration)
			diagnostics = append(diagnostics, semanticDeclarationDiagnostics(line, declaration)...)
			diagnostics = append(diagnostics, semanticImportAliasDiagnostics(line, declaration, importAliases)...)
			if declaration.Kind == "type" {
				typeKey := strings.ToLower(strings.TrimSpace(declaration.Name))
				_, duplicate := typeDeclarations[typeKey]
				if typeKey != "" && duplicate {
					diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("type %s is already declared", declaration.Name)))
				}
				activeTypeRegistered = typeKey != "" && !duplicate
				if activeTypeRegistered {
					typeDeclarations[typeKey] = declaration
				}
				activeTypeIndex = len(summary.Declarations) - 1
				activeTypeIndent = line.Indent
				activeTypeFields = map[string]bool{}
			}
			if declaration.Kind == "method" {
				registryDiagnostics, register := semanticMethodRegistryDiagnostics(line, declaration, typeDeclarations, methodSignatures)
				diagnostics = append(diagnostics, registryDiagnostics...)
				if register {
					key := semanticMethodKey(declaration.Receiver.Type, declaration.Name)
					methodDeclarations[key] = append(methodDeclarations[key], declaration)
					methodSignatures[semanticMethodSignatureKey(declaration)] = true
				}
			}
		case NodeKindCollection:
			if line.Name != "" {
				summary.Symbols = append(summary.Symbols, SemanticSymbol{
					Name:       line.Name,
					Line:       line.Line,
					Scope:      semanticScope(line.Indent),
					ValueKind:  inferCollectionValueKind(line.Text),
					Assignment: string(line.Mode),
				})
			}
			if declaration, ok := semanticCollectionDeclaration(line); ok {
				summary.Declarations = append(summary.Declarations, declaration)
				diagnostics = append(diagnostics, semanticCollectionDeclarationDiagnostics(line, declaration, collectionOperations)...)
				if declaration.Namespace != "" && line.Name != "" {
					collectionNamespaces[strings.ToLower(line.Name)] = declaration.Namespace
				}
			}
			if operation, ok := assignedCollectionConstructor(collectionOperations, line.Name); ok {
				collectionNamespaces[strings.ToLower(line.Name)] = operation.Namespace
			}
		case NodeKindVisual:
			summary.Visuals = append(summary.Visuals, semanticVisualMetadata(line)...)
		case NodeKindUnsupported:
			if activeTypeIndex >= 0 && line.Indent > activeTypeIndent {
				if field, ok := semanticTypeField(line.Text); ok {
					fieldName := strings.ToLower(field.Name)
					if activeTypeFields[fieldName] {
						diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("type %s repeats field %s", summary.Declarations[activeTypeIndex].Name, field.Name)))
					}
					activeTypeFields[fieldName] = true
					summary.Declarations[activeTypeIndex].Fields = append(summary.Declarations[activeTypeIndex].Fields, field)
					summary.Declarations[activeTypeIndex].Signature = objectConstructorSignature(summary.Declarations[activeTypeIndex])
					if activeTypeRegistered {
						typeDeclarations[strings.ToLower(summary.Declarations[activeTypeIndex].Name)] = summary.Declarations[activeTypeIndex]
					}
				}
			}
		}
		if len(collectionOperations) > 0 {
			summary.CollectionOperations = append(summary.CollectionOperations, collectionOperations...)
			diagnostics = append(diagnostics, collectionOperationDiagnostics(line, collectionOperations)...)
			if collectionOperationsContainNonExecutable(collectionOperations) && line.Kind != NodeKindCollection {
				diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_COLLECTION_UNSUPPORTED", "Pine collection namespaces array/matrix/map are not executable in this JFTrade Pine v6 version"))
			}
		}
		if line.Kind != NodeKindVisual {
			summary.Visuals = append(summary.Visuals, semanticVisualMetadata(line)...)
		}
		summary.ObjectOperations = append(summary.ObjectOperations, objectOperations...)
		diagnostics = append(diagnostics, objectOperationDiagnostics(line, objectOperations, typeDeclarations, methodDeclarations)...)
		for _, call := range semanticFunctionCalls(line.Text) {
			if signature, ok := semanticFunctionSignatures[call.name]; ok {
				summary.FunctionCalls = append(summary.FunctionCalls, SemanticFunctionCall{
					Line:       line.Line,
					Name:       call.name,
					ArgCount:   call.argCount,
					Signature:  signature.signature,
					ReturnKind: signature.returnKind,
					Supported:  true,
				})
				if call.argCount < signature.minArgs || call.argCount > signature.maxArgs {
					diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_SIGNATURE", fmt.Sprintf("%s expects %s", call.name, signature.signature)))
				}
			}
		}
	}
	return summary, diagnostics
}

func collectionOperationsContainNonExecutable(operations []SemanticCollectionOperation) bool {
	for _, operation := range operations {
		if !operation.Executable {
			return true
		}
	}
	return false
}

func semanticDeclaration(line ASTLine) SemanticDeclaration {
	lower := strings.ToLower(strings.TrimSpace(line.Text))
	kind := "declaration"
	name := ""
	var receiver *SemanticParameter
	parameters := []SemanticParameter(nil)
	importPath := ""
	version := ""
	alias := ""
	exportedKind := ""
	switch {
	case strings.HasPrefix(lower, "type "):
		kind = "type"
		name = firstDeclarationName(line.Text[len("type "):])
	case strings.HasPrefix(lower, "method "):
		kind = "method"
		name, receiver, parameters = parseMethodDeclaration(line.Text[len("method "):])
	case strings.HasPrefix(lower, "import "):
		kind = "import"
		importPath, alias = parseImportDeclaration(line.Text[len("import "):])
		name = importPath
		version = importVersion(importPath)
	case strings.HasPrefix(lower, "export "):
		kind = "export"
		name, exportedKind, receiver, parameters = parseExportDeclaration(line.Text[len("export "):])
	case strings.HasPrefix(lower, "library("):
		kind = "library"
		name = firstStringArgument(line.Text)
	}
	reason := "parse-only; runtime declaration execution is not enabled"
	declaration := SemanticDeclaration{
		Line:              line.Line,
		Kind:              kind,
		Name:              name,
		ExportedKind:      exportedKind,
		Alias:             alias,
		Receiver:          receiver,
		Parameters:        parameters,
		ImportPath:        importPath,
		Version:           version,
		Executable:        false,
		Reason:            reason,
		UnsupportedReason: reason,
	}
	declaration.Signature = semanticDeclarationSignature(declaration)
	return declaration
}

func semanticDeclarationSignature(declaration SemanticDeclaration) string {
	switch declaration.Kind {
	case "type":
		return objectConstructorSignature(declaration)
	case "method":
		return objectMethodSignature(declaration)
	case "import":
		signature := "import " + strings.TrimSpace(declaration.ImportPath)
		if strings.TrimSpace(declaration.Alias) != "" {
			signature += " as " + strings.TrimSpace(declaration.Alias)
		}
		return signature
	case "library":
		return `library("` + strings.ReplaceAll(declaration.Name, `"`, `\"`) + `")`
	case "export":
		if declaration.ExportedKind == "method" {
			return "export " + objectMethodSignature(declaration)
		}
		if len(declaration.Parameters) > 0 {
			params := make([]string, 0, len(declaration.Parameters))
			for _, parameter := range declaration.Parameters {
				params = append(params, semanticParameterSignature(parameter))
			}
			return "export " + declaration.Name + "(" + strings.Join(params, ", ") + ")"
		}
		if strings.TrimSpace(declaration.ExportedKind) != "" {
			return "export " + declaration.ExportedKind + " " + declaration.Name
		}
		return "export " + declaration.Name
	default:
		return declaration.Name
	}
}

func semanticDeclarationDiagnostics(line ASTLine, declaration SemanticDeclaration) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	switch declaration.Kind {
	case "method":
		seen := map[string]bool{}
		for _, parameter := range declaration.Parameters {
			name := strings.ToLower(strings.TrimSpace(parameter.Name))
			if name == "" {
				continue
			}
			if seen[name] {
				diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("method %s repeats parameter %s", declaration.Name, parameter.Name)))
				continue
			}
			seen[name] = true
		}
	}
	return diagnostics
}

func parseExportDeclaration(value string) (string, string, *SemanticParameter, []SemanticParameter) {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "method "):
		name, receiver, parameters := parseMethodDeclaration(trimmed[len("method "):])
		return name, "method", receiver, parameters
	case strings.HasPrefix(lower, "type "):
		return firstDeclarationName(trimmed[len("type "):]), "type", nil, nil
	default:
		name, _, parameters := parseMethodDeclaration(trimmed)
		return name, "function", nil, parameters
	}
}

func semanticMethodRegistryDiagnostics(line ASTLine, declaration SemanticDeclaration, typeDeclarations map[string]SemanticDeclaration, methodSignatures map[string]bool) ([]Diagnostic, bool) {
	if declaration.Kind != "method" {
		return nil, false
	}
	if declaration.Receiver == nil {
		return []Diagnostic{semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("method %s requires a receiver parameter", declaration.Name))}, false
	}
	receiverType := strings.TrimSpace(declaration.Receiver.Type)
	if receiverType == "" {
		return []Diagnostic{semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("method %s requires a typed receiver as its first parameter", declaration.Name))}, false
	}
	if !isKnownMethodReceiverType(receiverType, typeDeclarations) {
		return []Diagnostic{semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("method %s receiver type %s is not declared", declaration.Name, receiverType))}, false
	}
	signatureKey := semanticMethodSignatureKey(declaration)
	if methodSignatures[signatureKey] {
		return []Diagnostic{semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("method %s is already declared for receiver %s with this signature", declaration.Name, receiverType))}, false
	}
	return nil, true
}

func isKnownMethodReceiverType(receiverType string, typeDeclarations map[string]SemanticDeclaration) bool {
	normalized := normalizeSemanticType(receiverType)
	if normalized == "" {
		return false
	}
	if _, ok := typeDeclarations[normalized]; ok {
		return true
	}
	if namespace, _ := collectionTypeAnnotationInfo(receiverType); namespace != "" {
		return true
	}
	switch normalized {
	case "bool", "int", "float", "string", "color", "line", "linefill", "label", "box", "table", "polyline", "chart.point":
		return true
	default:
		return false
	}
}

func semanticImportAliasDiagnostics(line ASTLine, declaration SemanticDeclaration, importAliases map[string]bool) []Diagnostic {
	if declaration.Kind != "import" || declaration.Alias == "" {
		return nil
	}
	alias := strings.ToLower(strings.TrimSpace(declaration.Alias))
	if alias == "" {
		return nil
	}
	if importAliases[alias] {
		return []Diagnostic{semanticDiagnostic(line, "PINE_SEMANTIC_DECLARATION", fmt.Sprintf("import alias %s is already declared", declaration.Alias))}
	}
	importAliases[alias] = true
	return nil
}

func parseImportDeclaration(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ""
	}
	parts := strings.Fields(trimmed)
	for index := 0; index < len(parts)-1; index++ {
		if !strings.EqualFold(parts[index], "as") {
			continue
		}
		path := strings.TrimSpace(strings.Join(parts[:index], " "))
		alias := strings.TrimSpace(parts[index+1])
		return path, alias
	}
	return trimmed, ""
}

func parseMethodDeclaration(value string) (string, *SemanticParameter, []SemanticParameter) {
	trimmed := strings.TrimSpace(value)
	open := strings.Index(trimmed, "(")
	if open < 0 {
		return firstDeclarationName(trimmed), nil, nil
	}
	name := firstDeclarationName(trimmed[:open])
	close := matchingParen(trimmed, open)
	if close < 0 {
		return name, nil, nil
	}
	parameters := parseSemanticParameters(trimmed[open+1 : close])
	if len(parameters) == 0 {
		return name, nil, nil
	}
	receiver := parameters[0]
	return name, &receiver, parameters
}

func parseSemanticParameters(value string) []SemanticParameter {
	args := splitSemanticParameterArguments(value)
	parameters := make([]SemanticParameter, 0, len(args))
	for _, arg := range args {
		parameter := parseSemanticParameter(arg)
		if parameter.Name == "" && parameter.Type == "" {
			continue
		}
		parameters = append(parameters, parameter)
	}
	return parameters
}

func splitSemanticParameterArguments(value string) []string {
	parts := []string{}
	start := 0
	depth := 0
	angleDepth := 0
	inString := byte(0)
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
			if inString == 0 {
				inString = ch
			} else if inString == ch {
				inString = 0
			}
			continue
		}
		if inString != 0 {
			continue
		}
		switch ch {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case '<':
			if angleDepth > 0 || isCollectionGenericOpen(value, index) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if depth == 0 && angleDepth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	tail := strings.TrimSpace(value[start:])
	if tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func isCollectionGenericOpen(value string, index int) bool {
	start := index
	for start > 0 {
		ch := value[start-1]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			start--
			continue
		}
		break
	}
	switch strings.ToLower(value[start:index]) {
	case "array", "map", "matrix":
		return true
	default:
		return false
	}
}

func parseSemanticParameter(value string) SemanticParameter {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return SemanticParameter{}
	}
	defaultValue := ""
	if index := strings.Index(cleaned, "="); index >= 0 {
		defaultValue = strings.TrimSpace(cleaned[index+1:])
		cleaned = strings.TrimSpace(cleaned[:index])
	}
	fields := strings.Fields(cleaned)
	switch len(fields) {
	case 0:
		return SemanticParameter{}
	case 1:
		return SemanticParameter{Name: strings.Trim(fields[0], ","), Default: defaultValue}
	default:
		return SemanticParameter{
			Type:    strings.Trim(strings.Join(fields[:len(fields)-1], " "), ","),
			Name:    strings.Trim(fields[len(fields)-1], ","),
			Default: defaultValue,
		}
	}
}

func semanticTypeField(value string) (SemanticParameter, bool) {
	field := parseSemanticParameter(value)
	if field.Type == "" || field.Name == "" {
		return SemanticParameter{}, false
	}
	return field, true
}

func importVersion(importPath string) string {
	parts := strings.Split(strings.TrimSpace(importPath), "/")
	if len(parts) == 0 {
		return ""
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return ""
	}
	for _, ch := range last {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return last
}

func semanticCollectionDeclaration(line ASTLine) (SemanticDeclaration, bool) {
	namespace, call, typeArgs := collectionCallInfo(line.Text)
	annotationNamespace, annotationTypeArgs := collectionTypeAnnotationInfo(line.Type)
	if namespace == "" {
		namespace = annotationNamespace
	}
	if typeArgs == "" {
		typeArgs = annotationTypeArgs
	}
	operation := strings.TrimPrefix(call, namespace+".")
	if namespace == "" || (call != "" && !strings.HasPrefix(operation, "new")) {
		return SemanticDeclaration{}, false
	}
	executable := false
	if operation != "" {
		executable = collectionOperationExecutable(namespace, operation)
	}
	reason := "parse-only; runtime collection execution is not enabled"
	if executable {
		reason = ""
	}
	signature := call
	if signature == "" {
		signature = namespace
	}
	if typeArgs != "" {
		signature += "<" + typeArgs + ">"
	}
	return SemanticDeclaration{
		Line:              line.Line,
		Kind:              "collection",
		Name:              line.Name,
		Namespace:         namespace,
		Call:              call,
		TypeArgs:          typeArgs,
		Signature:         signature,
		Executable:        executable,
		Reason:            reason,
		UnsupportedReason: reason,
	}, true
}

func collectionTypeAnnotationInfo(annotation string) (string, string) {
	trimmed := normalizeTypeAnnotation(annotation)
	if trimmed == "" {
		return "", ""
	}
	lower := strings.ToLower(trimmed)
	for _, namespace := range []string{"array", "map", "matrix"} {
		if lower == namespace {
			return namespace, ""
		}
		prefix := namespace + "<"
		if strings.HasPrefix(lower, prefix) && strings.HasSuffix(trimmed, ">") {
			return namespace, strings.TrimSpace(trimmed[len(namespace)+1 : len(trimmed)-1])
		}
	}
	return "", ""
}

func semanticCollectionDeclarationDiagnostics(line ASTLine, declaration SemanticDeclaration, operations []SemanticCollectionOperation) []Diagnostic {
	annotationNamespace, annotationTypeArgs := collectionTypeAnnotationInfo(line.Type)
	constructor, hasConstructor := assignedCollectionConstructor(operations, line.Name)
	diagnostics := collectionTypeArgumentDiagnostics(line, "type annotation", annotationNamespace, annotationTypeArgs)
	if !hasConstructor {
		return diagnostics
	}
	if annotationNamespace != "" && constructor.Namespace != "" && annotationNamespace != constructor.Namespace {
		diagnostics = append(diagnostics, semanticDiagnostic(
			line,
			"PINE_SEMANTIC_COLLECTION_TYPE",
			fmt.Sprintf("%s declaration cannot be initialized with %s", annotationNamespace, constructor.Call),
		))
		return diagnostics
	}
	annotationArgs := collectionTypeArguments(annotationTypeArgs)
	constructorArgs := collectionConstructorTypeArguments(constructor)
	if collectionTypeArgumentsHaveExpectedArity(annotationNamespace, annotationArgs) &&
		collectionTypeArgumentsHaveExpectedArity(constructor.Namespace, constructorArgs) &&
		!equalCollectionTypeArguments(annotationArgs, constructorArgs) {
		diagnostics = append(diagnostics, semanticDiagnostic(
			line,
			"PINE_SEMANTIC_COLLECTION_TYPE",
			fmt.Sprintf("%s type arguments <%s> do not match %s element types <%s>", declaration.Name, strings.Join(annotationArgs, ", "), constructor.Call, strings.Join(constructorArgs, ", ")),
		))
	}
	return diagnostics
}

func semanticCollectionOperations(line ASTLine, collectionNamespaces map[string]string) []SemanticCollectionOperation {
	matches := collectionCallPattern.FindAllStringSubmatchIndex(line.Text, -1)
	operations := make([]SemanticCollectionOperation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 8 {
			continue
		}
		namespace := strings.ToLower(line.Text[match[2]:match[3]])
		operation := strings.ToLower(line.Text[match[4]:match[5]])
		typeArgs := ""
		if match[6] >= 0 && match[7] >= 0 {
			typeArgs = strings.TrimSpace(strings.Trim(line.Text[match[6]:match[7]], "<>"))
		}
		open := match[1] - 1
		close := matchingParen(line.Text, open)
		args := []string{}
		if close > open {
			args = splitArguments(line.Text[open+1 : close])
		}
		operations = append(operations, SemanticCollectionOperation{
			Line:       line.Line,
			Namespace:  namespace,
			Operation:  operation,
			Call:       namespace + "." + operation,
			TypeArgs:   typeArgs,
			Signature:  collectionOperationSignatureText(namespace + "." + operation),
			Target:     collectionOperationTarget(operation, args, line.Name),
			Arguments:  args,
			Mutates:    collectionOperationMutates(operation),
			Supported:  collectionOperationSupported(namespace + "." + operation),
			Executable: collectionOperationExecutable(namespace, operation),
			Reason:     collectionOperationReason(namespace, operation),
		})
	}
	operations = append(operations, semanticCollectionMethodOperations(line, collectionNamespaces)...)
	return operations
}

func semanticCollectionMethodOperations(line ASTLine, collectionNamespaces map[string]string) []SemanticCollectionOperation {
	if len(collectionNamespaces) == 0 {
		return nil
	}
	matches := objectCallPattern.FindAllStringSubmatchIndex(line.Text, -1)
	operations := make([]SemanticCollectionOperation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		target := strings.TrimSpace(line.Text[match[2]:match[3]])
		namespace, ok := collectionNamespaces[strings.ToLower(target)]
		if !ok {
			continue
		}
		operation := strings.ToLower(strings.TrimSpace(line.Text[match[4]:match[5]]))
		if strings.HasPrefix(operation, "new") {
			continue
		}
		open := match[1] - 1
		close := matchingParen(line.Text, open)
		if close < open {
			continue
		}
		args := []string{target}
		args = append(args, splitArguments(line.Text[open+1:close])...)
		call := namespace + "." + operation
		operations = append(operations, SemanticCollectionOperation{
			Line:       line.Line,
			Namespace:  namespace,
			Operation:  operation,
			Call:       call,
			Signature:  collectionOperationSignatureText(call),
			Target:     target,
			Arguments:  args,
			Mutates:    collectionOperationMutates(operation),
			Supported:  collectionOperationSupported(call),
			Executable: collectionOperationExecutable(namespace, operation),
			Reason:     collectionOperationReason(namespace, operation),
		})
	}
	return operations
}

func semanticObjectOperations(line ASTLine, typeDeclarations map[string]SemanticDeclaration, methodDeclarations map[string][]SemanticDeclaration, objectTypes map[string]string) []SemanticObjectOperation {
	matches := objectCallPattern.FindAllStringSubmatchIndex(line.Text, -1)
	operations := make([]SemanticObjectOperation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		receiver := strings.TrimSpace(line.Text[match[2]:match[3]])
		method := strings.TrimSpace(line.Text[match[4]:match[5]])
		open := match[1] - 1
		close := matchingParen(line.Text, open)
		if close < open {
			continue
		}
		args := splitArguments(line.Text[open+1 : close])
		if strings.EqualFold(method, "new") {
			declaration, ok := typeDeclarations[strings.ToLower(receiver)]
			if !ok {
				continue
			}
			operations = append(operations, SemanticObjectOperation{
				Line:       line.Line,
				Kind:       "constructor",
				Type:       declaration.Name,
				Call:       declaration.Name + ".new",
				Signature:  objectConstructorSignature(declaration),
				Target:     line.Name,
				Arguments:  args,
				Supported:  true,
				Executable: false,
				Reason:     "parse-only; runtime object execution is not enabled",
			})
			continue
		}
		objectType, ok := objectTypes[strings.ToLower(receiver)]
		if !ok {
			continue
		}
		declarations := methodDeclarations[semanticMethodKey(objectType, method)]
		declaration, ok := resolveSemanticMethodDeclaration(declarations, len(args))
		if !ok {
			continue
		}
		operations = append(operations, SemanticObjectOperation{
			Line:       line.Line,
			Kind:       "method",
			Type:       objectType,
			Method:     declaration.Name,
			Call:       receiver + "." + declaration.Name,
			Signature:  objectMethodSignature(declaration),
			Target:     receiver,
			Arguments:  args,
			Supported:  true,
			Executable: false,
			Reason:     "parse-only; runtime object execution is not enabled",
		})
	}
	return operations
}

func assignedObjectConstructor(operations []SemanticObjectOperation, name string) (SemanticObjectOperation, bool) {
	for _, operation := range operations {
		if operation.Kind == "constructor" && operation.Target == name && operation.Type != "" {
			return operation, true
		}
	}
	return SemanticObjectOperation{}, false
}

func semanticMethodKey(receiverType string, method string) string {
	return normalizeSemanticType(receiverType) + "." + strings.ToLower(strings.TrimSpace(method))
}

func semanticMethodSignatureKey(declaration SemanticDeclaration) string {
	types := make([]string, 0, len(declaration.Parameters))
	for _, parameter := range declaration.Parameters {
		parameterType := normalizeSemanticType(parameter.Type)
		if parameterType == "" {
			parameterType = "?"
		}
		types = append(types, parameterType)
	}
	receiverType := ""
	if declaration.Receiver != nil {
		receiverType = declaration.Receiver.Type
	}
	return semanticMethodKey(receiverType, declaration.Name) + "(" + strings.Join(types, ",") + ")"
}

func normalizeSemanticType(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
}

func resolveSemanticMethodDeclaration(declarations []SemanticDeclaration, argCount int) (SemanticDeclaration, bool) {
	for _, declaration := range declarations {
		parameters := declaration.Parameters
		if len(parameters) > 0 {
			parameters = parameters[1:]
		}
		if argCount >= requiredSemanticParameterCount(parameters) && argCount <= len(parameters) {
			return declaration, true
		}
	}
	if len(declarations) > 0 {
		return declarations[0], true
	}
	return SemanticDeclaration{}, false
}

func objectConstructorSignature(declaration SemanticDeclaration) string {
	fields := make([]string, 0, len(declaration.Fields))
	for _, field := range declaration.Fields {
		fields = append(fields, semanticParameterSignature(field))
	}
	return declaration.Name + ".new(" + strings.Join(fields, ", ") + ")"
}

func objectMethodSignature(declaration SemanticDeclaration) string {
	parameters := make([]string, 0, len(declaration.Parameters))
	for _, parameter := range declaration.Parameters {
		parameters = append(parameters, semanticParameterSignature(parameter))
	}
	return declaration.Name + "(" + strings.Join(parameters, ", ") + ")"
}

func semanticParameterSignature(parameter SemanticParameter) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(parameter.Type) != "" {
		parts = append(parts, strings.TrimSpace(parameter.Type))
	}
	if strings.TrimSpace(parameter.Name) != "" {
		parts = append(parts, strings.TrimSpace(parameter.Name))
	}
	signature := strings.Join(parts, " ")
	if strings.TrimSpace(parameter.Default) != "" {
		if signature != "" {
			signature += " = "
		}
		signature += strings.TrimSpace(parameter.Default)
	}
	return signature
}

func objectOperationDiagnostics(line ASTLine, operations []SemanticObjectOperation, typeDeclarations map[string]SemanticDeclaration, methodDeclarations map[string][]SemanticDeclaration) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, operation := range operations {
		minArgs, maxArgs, ok := objectOperationArgBounds(operation, typeDeclarations, methodDeclarations)
		if !ok {
			continue
		}
		argCount := len(operation.Arguments)
		if argCount < minArgs || argCount > maxArgs {
			diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_OBJECT_SIGNATURE", fmt.Sprintf("%s expects %s", operation.Call, operation.Signature)))
		}
	}
	return diagnostics
}

func objectOperationArgBounds(operation SemanticObjectOperation, typeDeclarations map[string]SemanticDeclaration, methodDeclarations map[string][]SemanticDeclaration) (int, int, bool) {
	switch operation.Kind {
	case "constructor":
		declaration, ok := typeDeclarations[strings.ToLower(operation.Type)]
		if !ok {
			return 0, 0, false
		}
		return requiredSemanticParameterCount(declaration.Fields), len(declaration.Fields), true
	case "method":
		declarations := methodDeclarations[semanticMethodKey(operation.Type, operation.Method)]
		declaration, ok := semanticMethodDeclarationForOperation(declarations, operation)
		if !ok {
			return 0, 0, false
		}
		parameters := declaration.Parameters
		if len(parameters) > 0 {
			parameters = parameters[1:]
		}
		return requiredSemanticParameterCount(parameters), len(parameters), true
	default:
		return 0, 0, false
	}
}

func semanticMethodDeclarationForOperation(declarations []SemanticDeclaration, operation SemanticObjectOperation) (SemanticDeclaration, bool) {
	for _, declaration := range declarations {
		if objectMethodSignature(declaration) == operation.Signature {
			return declaration, true
		}
	}
	return resolveSemanticMethodDeclaration(declarations, len(operation.Arguments))
}

func requiredSemanticParameterCount(parameters []SemanticParameter) int {
	count := 0
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Default) == "" {
			count++
		}
	}
	return count
}

func collectionOperationDiagnostics(line ASTLine, operations []SemanticCollectionOperation) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, operation := range operations {
		signature, ok := collectionOperationSignatures[operation.Call]
		if !ok {
			continue
		}
		argCount := len(operation.Arguments)
		if argCount < signature.minArgs || argCount > signature.maxArgs {
			diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_COLLECTION_SIGNATURE", fmt.Sprintf("%s expects %s", operation.Call, signature.signature)))
		}
		if strings.HasPrefix(operation.Operation, "new") {
			diagnostics = append(diagnostics, collectionTypeArgumentDiagnostics(line, operation.Call, operation.Namespace, operation.TypeArgs)...)
		}
	}
	return diagnostics
}

func collectionTypeArgumentDiagnostics(line ASTLine, source string, namespace string, typeArgs string) []Diagnostic {
	if strings.TrimSpace(typeArgs) == "" {
		return nil
	}
	expected, ok := collectionTypeArgumentCount(namespace)
	if !ok {
		return nil
	}
	actual := len(collectionTypeArguments(typeArgs))
	if actual == expected {
		return nil
	}
	return []Diagnostic{semanticDiagnostic(
		line,
		"PINE_SEMANTIC_COLLECTION_TYPE",
		fmt.Sprintf("%s requires %d type argument(s), got %d", source, expected, actual),
	)}
}

func collectionTypeArgumentCount(namespace string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(namespace)) {
	case "array", "matrix":
		return 1, true
	case "map":
		return 2, true
	default:
		return 0, false
	}
}

func collectionTypeArguments(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := make([]string, 0, 2)
	start := 0
	angleDepth := 0
	otherDepth := 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(', '[':
			otherDepth++
		case ')', ']':
			if otherDepth > 0 {
				otherDepth--
			}
		case ',':
			if angleDepth == 0 && otherDepth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func collectionConstructorTypeArguments(operation SemanticCollectionOperation) []string {
	if strings.TrimSpace(operation.TypeArgs) != "" {
		return collectionTypeArguments(operation.TypeArgs)
	}
	if operation.Namespace != "array" || !strings.HasPrefix(operation.Operation, "new_") {
		return nil
	}
	elementType := strings.TrimPrefix(operation.Operation, "new_")
	switch elementType {
	case "bool", "float", "int", "string":
		return []string{elementType}
	default:
		return nil
	}
}

func equalCollectionTypeArguments(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if normalizeCollectionTypeArgument(left[index]) != normalizeCollectionTypeArgument(right[index]) {
			return false
		}
	}
	return true
}

func collectionTypeArgumentsHaveExpectedArity(namespace string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	expected, ok := collectionTypeArgumentCount(namespace)
	return ok && len(args) == expected
}

func normalizeCollectionTypeArgument(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
}

func collectionOperationSignatureText(call string) string {
	if signature, ok := collectionOperationSignatures[call]; ok {
		return signature.signature
	}
	return ""
}

func collectionOperationSupported(call string) bool {
	_, ok := collectionOperationSignatures[call]
	return ok
}

func collectionOperationExecutable(namespace string, operation string) bool {
	return executableCollectionOperations[namespace][operation]
}

func collectionOperationReason(namespace string, operation string) string {
	if collectionOperationExecutable(namespace, operation) {
		return ""
	}
	return "parse-only; runtime collection execution is not enabled"
}

func assignedCollectionConstructor(operations []SemanticCollectionOperation, name string) (SemanticCollectionOperation, bool) {
	for _, operation := range operations {
		if strings.HasPrefix(operation.Operation, "new") && operation.Target == name && operation.Namespace != "" {
			return operation, true
		}
	}
	return SemanticCollectionOperation{}, false
}

func collectionCallInfo(text string) (string, string, string) {
	match := collectionCallPattern.FindStringSubmatch(text)
	if match == nil {
		return "", "", ""
	}
	namespace := strings.ToLower(match[1])
	method := strings.ToLower(match[2])
	typeArgs := strings.Trim(match[3], "<>")
	return namespace, namespace + "." + method, strings.TrimSpace(typeArgs)
}

func inferCollectionValueKind(text string) SemanticValueKind {
	for _, operation := range semanticCollectionOperations(ASTLine{Text: text}, nil) {
		if strings.HasPrefix(operation.Operation, "new") {
			return SemanticValueObject
		}
	}
	return SemanticValueUnknown
}

func collectionOperationTarget(operation string, args []string, assignedName string) string {
	if strings.HasPrefix(operation, "new") {
		return assignedName
	}
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func collectionOperationMutates(operation string) bool {
	switch operation {
	case "push", "pop", "shift", "unshift", "insert", "remove", "clear", "set", "fill", "put":
		return true
	default:
		return strings.HasPrefix(operation, "new")
	}
}

func firstDeclarationName(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	name := strings.Trim(fields[0], "()")
	if index := strings.Index(name, "("); index >= 0 {
		name = name[:index]
	}
	return name
}

func semanticVisualMetadata(line ASTLine) []PineVisualMetadata {
	calls := semanticVisualCalls(line.Text)
	metadata := make([]PineVisualMetadata, 0, len(calls))
	for _, call := range calls {
		namedArgs := visualNamedArgs(call.args)
		metadata = append(metadata, PineVisualMetadata{
			Line:      line.Line,
			Kind:      visualMetadataKind(call.name),
			Call:      call.name,
			Variable:  visualMetadataVariable(line, call.name),
			Target:    visualMetadataTarget(call.name, call.args),
			Title:     visualMetadataTitle(call.name, call.args, namedArgs),
			Arguments: call.args,
			NamedArgs: namedArgs,
			Text:      line.Text,
		})
	}
	return metadata
}

type semanticVisualCall struct {
	name string
	args []string
}

func semanticVisualCalls(text string) []semanticVisualCall {
	calls := make([]semanticVisualCall, 0)
	index := 0
	for index < len(text) {
		open := strings.Index(text[index:], "(")
		if open < 0 {
			break
		}
		open += index
		nameStart := open
		for nameStart > 0 {
			ch := text[nameStart-1]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' {
				nameStart--
				continue
			}
			break
		}
		name := strings.ToLower(strings.TrimSpace(text[nameStart:open]))
		close := matchingParen(text, open)
		if close < 0 {
			break
		}
		if isVisualCallName(name) {
			calls = append(calls, semanticVisualCall{name: name, args: splitArguments(text[open+1 : close])})
		}
		index = close + 1
	}
	return calls
}

func isVisualCallName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "plot", "plotshape", "plotchar", "hline", "bgcolor", "barcolor", "fill", "alertcondition":
		return true
	default:
		return isDrawingVisualCallName(lower) ||
			strings.HasPrefix(lower, "table.")
	}
}

func isDrawingVisualCallName(name string) bool {
	for _, namespace := range []string{"label", "line", "box"} {
		prefix := namespace + "."
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		operation := strings.TrimPrefix(name, prefix)
		return operation == "new" ||
			operation == "delete" ||
			operation == "copy" ||
			strings.HasPrefix(operation, "set_") ||
			strings.HasPrefix(operation, "get_")
	}
	return false
}

func visualMetadataVariable(line ASTLine, call string) string {
	if line.Name == "" || !strings.Contains(strings.ToLower(line.Expression), strings.ToLower(call)+"(") {
		return ""
	}
	return line.Name
}

func visualNamedArgs(args []string) map[string]string {
	named := map[string]string{}
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		named[strings.ToLower(key)] = value
	}
	if len(named) == 0 {
		return nil
	}
	return named
}

func visualMetadataKind(call string) string {
	lower := strings.ToLower(strings.TrimSpace(call))
	switch {
	case lower == "alertcondition":
		return "alert"
	case strings.HasPrefix(lower, "label."), strings.HasPrefix(lower, "line."), strings.HasPrefix(lower, "box."):
		return "drawing"
	case strings.HasPrefix(lower, "table."):
		return "table"
	case lower == "bgcolor" || lower == "barcolor" || lower == "fill":
		return "color"
	default:
		return "plot"
	}
}

func visualMetadataTarget(call string, args []string) string {
	lower := strings.ToLower(strings.TrimSpace(call))
	switch {
	case len(args) == 0:
		return ""
	case lower == "label.new" && len(args) >= 3:
		return args[2]
	default:
		if _, value, ok := splitNamedArg(args[0]); ok {
			return value
		}
		return args[0]
	}
}

func visualMetadataTitle(call string, args []string, namedArgs map[string]string) string {
	for _, key := range []string{"title", "text", "message"} {
		if value := namedArgs[key]; value != "" {
			return unquote(value)
		}
	}
	lower := strings.ToLower(strings.TrimSpace(call))
	switch {
	case lower == "alertcondition" && len(args) >= 2:
		return unquote(args[1])
	case lower == "plot" && len(args) >= 2:
		return unquote(args[1])
	case lower == "hline" && len(args) >= 2:
		return unquote(args[1])
	case lower == "label.new" && len(args) >= 3:
		return unquote(args[2])
	default:
		return ""
	}
}

func semanticScope(indent int) string {
	if indent > 0 {
		return "block"
	}
	return "global"
}

func semanticDiagnostic(line ASTLine, code string, message string) Diagnostic {
	column := line.Column
	if column <= 0 {
		column = 1
	}
	endColumn := line.EndColumn
	if endColumn <= column {
		endColumn = column + 1
	}
	return Diagnostic{
		Severity:  DiagnosticSeverityError,
		Code:      code,
		Message:   message,
		Line:      line.Line,
		Column:    column,
		EndLine:   line.EndLine,
		EndColumn: endColumn,
	}
}

func tupleNamesFromASTLine(line ASTLine) []string {
	match := generalTuplePattern.FindStringSubmatch(line.Text)
	if match == nil {
		return nil
	}
	names := splitArguments(match[1])
	for index := range names {
		names[index] = strings.TrimSpace(names[index])
	}
	return names
}

func duplicateNames(names []string) []string {
	seen := map[string]bool{}
	duplicates := make([]string, 0)
	for _, name := range names {
		if name == "_" {
			continue
		}
		if seen[name] {
			duplicates = append(duplicates, name)
			continue
		}
		seen[name] = true
	}
	return duplicates
}

func semanticTupleReturnCount(expression string) (int, bool) {
	trimmed := strings.TrimSpace(expression)
	if args, ok := requestSecurityCallArgs(trimmed); ok {
		if lowered, tupleOK := lowerSupportedRequestSecurityTupleGeneral(args); tupleOK {
			return len(lowered), true
		}
	}
	if len(trimmed) >= 2 && trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']' {
		values := splitArguments(trimmed[1 : len(trimmed)-1])
		return len(values), len(values) >= 2 && len(values) <= 8
	}
	name, args, ok := parseTACall(trimmed)
	if !ok {
		if lowered := replaceSupportedRequestSecurity(trimmed); lowered != trimmed {
			lowered = stripWrappingParens(lowered)
			if callName, callArgs, callOK := parseFunctionCallText(lowered); callOK {
				name, args, ok = callName, callArgs, true
			}
		}
	}
	if !ok {
		return 0, false
	}
	switch strings.ToLower(name) {
	case "macd", "bb", "dmi", "kc", "bollinger":
		return 3, true
	case "supertrend":
		return 2, true
	default:
		return len(args), false
	}
}

func inferTupleElementKind(expression string) SemanticValueKind {
	if strings.Contains(strings.ToLower(expression), "request.security(") {
		return SemanticValueSeries
	}
	return inferSemanticValueKind(expression)
}

func inferSemanticValueKind(expression string) SemanticValueKind {
	trimmed := strings.TrimSpace(stripWrappingParens(expression))
	if trimmed == "" {
		return SemanticValueUnknown
	}
	if isSimpleAliasExpression(trimmed) {
		return SemanticValueConst
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "ta.") ||
		strings.Contains(lower, "request.security(") ||
		historyReferencePattern.MatchString(trimmed) ||
		containsSeriesIdentifier(trimmed) {
		if strings.Contains(lower, "ta.macd(") || strings.Contains(lower, "ta.bb(") ||
			strings.Contains(lower, "ta.supertrend(") || strings.Contains(lower, "ta.kc(") ||
			strings.Contains(lower, "ta.dmi(") {
			return SemanticValueObject
		}
		return SemanticValueSeries
	}
	if strings.Contains(lower, "input.") || strings.Contains(lower, "timestamp(") || strings.Contains(lower, "math.") {
		return SemanticValueSimple
	}
	return SemanticValueUnknown
}

func containsSeriesIdentifier(expression string) bool {
	for _, source := range []string{"open", "high", "low", "close", "volume", "hl2", "hlc3", "ohlc4", "time", "bar_index"} {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(source) + `\b`)
		if pattern.MatchString(expression) {
			return true
		}
	}
	return false
}

type semanticCall struct {
	name     string
	argCount int
}

func semanticFunctionCalls(text string) []semanticCall {
	calls := make([]semanticCall, 0)
	index := 0
	for index < len(text) {
		open := strings.Index(text[index:], "(")
		if open < 0 {
			break
		}
		open += index
		nameStart := open
		for nameStart > 0 {
			ch := text[nameStart-1]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' {
				nameStart--
				continue
			}
			break
		}
		name := strings.ToLower(strings.TrimSpace(text[nameStart:open]))
		close := matchingParen(text, open)
		if close < 0 {
			break
		}
		if name != "" && strings.Contains(name, ".") {
			args := splitArguments(text[open+1 : close])
			calls = append(calls, semanticCall{name: name, argCount: len(args)})
		}
		index = close + 1
	}
	return calls
}

func parseFunctionCallText(expression string) (string, []string, bool) {
	trimmed := strings.TrimSpace(expression)
	open := strings.Index(trimmed, "(")
	if open <= 0 {
		return "", nil, false
	}
	close := matchingParen(trimmed, open)
	if close != len(trimmed)-1 {
		return "", nil, false
	}
	return strings.ToLower(strings.TrimSpace(trimmed[:open])), splitArguments(trimmed[open+1 : close]), true
}
