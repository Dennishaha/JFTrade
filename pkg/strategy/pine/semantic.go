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
	"input.int":                          {minArgs: 1, maxArgs: 8, signature: "input.int(defval, title?)", returnKind: SemanticValueSimple},
	"input.float":                        {minArgs: 1, maxArgs: 8, signature: "input.float(defval, title?)", returnKind: SemanticValueSimple},
	"input.bool":                         {minArgs: 1, maxArgs: 8, signature: "input.bool(defval, title?)", returnKind: SemanticValueSimple},
	"input.string":                       {minArgs: 1, maxArgs: 8, signature: "input.string(defval, title?)", returnKind: SemanticValueSimple},
	"input.source":                       {minArgs: 1, maxArgs: 8, signature: "input.source(defval, title?)", returnKind: SemanticValueSeries},
	"input.time":                         {minArgs: 1, maxArgs: 8, signature: "input.time(defval, title?)", returnKind: SemanticValueSimple},
	"input.timeframe":                    {minArgs: 1, maxArgs: 8, signature: "input.timeframe(defval, title?)", returnKind: SemanticValueSimple},
	"input.color":                        {minArgs: 1, maxArgs: 8, signature: "input.color(defval, title?)", returnKind: SemanticValueSimple},
	"math.abs":                           {minArgs: 1, maxArgs: 1, signature: "math.abs(number)", returnKind: SemanticValueSimple},
	"math.min":                           {minArgs: 2, maxArgs: 16, signature: "math.min(number1, number2, ...)", returnKind: SemanticValueSimple},
	"math.max":                           {minArgs: 2, maxArgs: 16, signature: "math.max(number1, number2, ...)", returnKind: SemanticValueSimple},
	"math.avg":                           {minArgs: 2, maxArgs: 16, signature: "math.avg(number1, number2, ...)", returnKind: SemanticValueSimple},
	"math.round":                         {minArgs: 1, maxArgs: 2, signature: "math.round(number, precision?)", returnKind: SemanticValueSimple},
	"math.round_to_mintick":              {minArgs: 1, maxArgs: 1, signature: "math.round_to_mintick(number)", returnKind: SemanticValueSimple},
	"math.floor":                         {minArgs: 1, maxArgs: 1, signature: "math.floor(number)", returnKind: SemanticValueSimple},
	"math.ceil":                          {minArgs: 1, maxArgs: 1, signature: "math.ceil(number)", returnKind: SemanticValueSimple},
	"math.sqrt":                          {minArgs: 1, maxArgs: 1, signature: "math.sqrt(number)", returnKind: SemanticValueSimple},
	"math.pow":                           {minArgs: 2, maxArgs: 2, signature: "math.pow(base, exponent)", returnKind: SemanticValueSimple},
	"math.log":                           {minArgs: 1, maxArgs: 1, signature: "math.log(number)", returnKind: SemanticValueSimple},
	"math.sign":                          {minArgs: 1, maxArgs: 1, signature: "math.sign(number)", returnKind: SemanticValueSimple},
	"ta.ema":                             {minArgs: 2, maxArgs: 2, signature: "ta.ema(source, length)", returnKind: SemanticValueSeries},
	"ta.sma":                             {minArgs: 2, maxArgs: 2, signature: "ta.sma(source, length)", returnKind: SemanticValueSeries},
	"ta.rma":                             {minArgs: 2, maxArgs: 2, signature: "ta.rma(source, length)", returnKind: SemanticValueSeries},
	"ta.wma":                             {minArgs: 2, maxArgs: 2, signature: "ta.wma(source, length)", returnKind: SemanticValueSeries},
	"ta.hma":                             {minArgs: 2, maxArgs: 2, signature: "ta.hma(source, length)", returnKind: SemanticValueSeries},
	"ta.vwma":                            {minArgs: 2, maxArgs: 2, signature: "ta.vwma(source, length)", returnKind: SemanticValueSeries},
	"ta.rsi":                             {minArgs: 2, maxArgs: 2, signature: "ta.rsi(source, length)", returnKind: SemanticValueSeries},
	"ta.atr":                             {minArgs: 1, maxArgs: 1, signature: "ta.atr(length)", returnKind: SemanticValueSeries},
	"ta.cci":                             {minArgs: 2, maxArgs: 2, signature: "ta.cci(source, length)", returnKind: SemanticValueSeries},
	"ta.highest":                         {minArgs: 1, maxArgs: 2, signature: "ta.highest(source?, length)", returnKind: SemanticValueSeries},
	"ta.lowest":                          {minArgs: 1, maxArgs: 2, signature: "ta.lowest(source?, length)", returnKind: SemanticValueSeries},
	"ta.highestbars":                     {minArgs: 2, maxArgs: 2, signature: "ta.highestbars(source, length)", returnKind: SemanticValueSeries},
	"ta.lowestbars":                      {minArgs: 2, maxArgs: 2, signature: "ta.lowestbars(source, length)", returnKind: SemanticValueSeries},
	"ta.change":                          {minArgs: 1, maxArgs: 2, signature: "ta.change(source, length?)", returnKind: SemanticValueSeries},
	"ta.mom":                             {minArgs: 2, maxArgs: 2, signature: "ta.mom(source, length)", returnKind: SemanticValueSeries},
	"ta.roc":                             {minArgs: 2, maxArgs: 2, signature: "ta.roc(source, length)", returnKind: SemanticValueSeries},
	"ta.range":                           {minArgs: 2, maxArgs: 2, signature: "ta.range(source, length)", returnKind: SemanticValueSeries},
	"ta.mode":                            {minArgs: 2, maxArgs: 2, signature: "ta.mode(source, length)", returnKind: SemanticValueSeries},
	"ta.rising":                          {minArgs: 2, maxArgs: 2, signature: "ta.rising(source, length)", returnKind: SemanticValueSeries},
	"ta.falling":                         {minArgs: 2, maxArgs: 2, signature: "ta.falling(source, length)", returnKind: SemanticValueSeries},
	"ta.stdev":                           {minArgs: 2, maxArgs: 2, signature: "ta.stdev(source, length)", returnKind: SemanticValueSeries},
	"ta.variance":                        {minArgs: 2, maxArgs: 2, signature: "ta.variance(source, length)", returnKind: SemanticValueSeries},
	"ta.cum":                             {minArgs: 1, maxArgs: 1, signature: "ta.cum(source)", returnKind: SemanticValueSeries},
	"ta.wpr":                             {minArgs: 1, maxArgs: 1, signature: "ta.wpr(length)", returnKind: SemanticValueSeries},
	"ta.mfi":                             {minArgs: 2, maxArgs: 2, signature: "ta.mfi(source, length)", returnKind: SemanticValueSeries},
	"ta.stoch":                           {minArgs: 4, maxArgs: 4, signature: "ta.stoch(source, high, low, length)", returnKind: SemanticValueSeries},
	"ta.tr":                              {minArgs: 0, maxArgs: 1, signature: "ta.tr(handle_na?)", returnKind: SemanticValueSeries},
	"ta.barssince":                       {minArgs: 1, maxArgs: 1, signature: "ta.barssince(condition)", returnKind: SemanticValueSeries},
	"ta.valuewhen":                       {minArgs: 3, maxArgs: 3, signature: "ta.valuewhen(condition, source, occurrence)", returnKind: SemanticValueSeries},
	"ta.crossover":                       {minArgs: 2, maxArgs: 2, signature: "ta.crossover(source1, source2)", returnKind: SemanticValueSimple},
	"ta.crossunder":                      {minArgs: 2, maxArgs: 2, signature: "ta.crossunder(source1, source2)", returnKind: SemanticValueSimple},
	"ta.cross":                           {minArgs: 2, maxArgs: 2, signature: "ta.cross(source1, source2)", returnKind: SemanticValueSimple},
	"ta.macd":                            {minArgs: 4, maxArgs: 4, signature: "ta.macd(source, fast, slow, signal)", returnKind: SemanticValueObject},
	"ta.bb":                              {minArgs: 3, maxArgs: 3, signature: "ta.bb(source, length, mult)", returnKind: SemanticValueObject},
	"ta.dmi":                             {minArgs: 2, maxArgs: 2, signature: "ta.dmi(diLength, adxSmoothing)", returnKind: SemanticValueObject},
	"ta.bbw":                             {minArgs: 3, maxArgs: 3, signature: "ta.bbw(source, length, mult)", returnKind: SemanticValueSeries},
	"ta.cog":                             {minArgs: 2, maxArgs: 2, signature: "ta.cog(source, length)", returnKind: SemanticValueSeries},
	"ta.vwap":                            {minArgs: 0, maxArgs: 3, signature: "ta.vwap(source?, anchor?, stdev_mult?)", returnKind: SemanticValueSeries},
	"ta.supertrend":                      {minArgs: 2, maxArgs: 2, signature: "ta.supertrend(factor, atrPeriod)", returnKind: SemanticValueObject},
	"ta.sar":                             {minArgs: 3, maxArgs: 3, signature: "ta.sar(start, inc, max)", returnKind: SemanticValueSeries},
	"ta.linreg":                          {minArgs: 3, maxArgs: 3, signature: "ta.linreg(source, length, offset)", returnKind: SemanticValueSeries},
	"ta.pivothigh":                       {minArgs: 2, maxArgs: 3, signature: "ta.pivothigh(source?, leftbars, rightbars)", returnKind: SemanticValueSeries},
	"ta.pivotlow":                        {minArgs: 2, maxArgs: 3, signature: "ta.pivotlow(source?, leftbars, rightbars)", returnKind: SemanticValueSeries},
	"ta.kc":                              {minArgs: 3, maxArgs: 4, signature: "ta.kc(source, length, mult, useTrueRange?)", returnKind: SemanticValueObject},
	"ta.kcw":                             {minArgs: 3, maxArgs: 4, signature: "ta.kcw(source, length, mult, useTrueRange?)", returnKind: SemanticValueSeries},
	"ta.alma":                            {minArgs: 4, maxArgs: 4, signature: "ta.alma(source, length, offset, sigma)", returnKind: SemanticValueSeries},
	"ta.cmo":                             {minArgs: 2, maxArgs: 2, signature: "ta.cmo(source, length)", returnKind: SemanticValueSeries},
	"ta.tsi":                             {minArgs: 3, maxArgs: 3, signature: "ta.tsi(source, shortLength, longLength)", returnKind: SemanticValueSeries},
	"ta.correlation":                     {minArgs: 3, maxArgs: 3, signature: "ta.correlation(source1, source2, length)", returnKind: SemanticValueSeries},
	"ta.dev":                             {minArgs: 2, maxArgs: 2, signature: "ta.dev(source, length)", returnKind: SemanticValueSeries},
	"ta.median":                          {minArgs: 2, maxArgs: 2, signature: "ta.median(source, length)", returnKind: SemanticValueSeries},
	"ta.percentile_linear_interpolation": {minArgs: 3, maxArgs: 3, signature: "ta.percentile_linear_interpolation(source, length, percentage)", returnKind: SemanticValueSeries},
	"ta.percentile_nearest_rank":         {minArgs: 3, maxArgs: 3, signature: "ta.percentile_nearest_rank(source, length, percentage)", returnKind: SemanticValueSeries},
	"ta.percentrank":                     {minArgs: 2, maxArgs: 2, signature: "ta.percentrank(source, length)", returnKind: SemanticValueSeries},
	"ta.swma":                            {minArgs: 1, maxArgs: 1, signature: "ta.swma(source)", returnKind: SemanticValueSeries},
	"str.length":                         {minArgs: 1, maxArgs: 1, signature: "str.length(source)", returnKind: SemanticValueSimple},
	"str.tostring":                       {minArgs: 1, maxArgs: 2, signature: "str.tostring(value, format?)", returnKind: SemanticValueSimple},
	"str.contains":                       {minArgs: 2, maxArgs: 2, signature: "str.contains(source, needle)", returnKind: SemanticValueSimple},
	"str.pos":                            {minArgs: 2, maxArgs: 2, signature: "str.pos(source, needle)", returnKind: SemanticValueSimple},
	"str.substring":                      {minArgs: 2, maxArgs: 3, signature: "str.substring(source, begin, end?)", returnKind: SemanticValueSimple},
	"str.replace":                        {minArgs: 3, maxArgs: 3, signature: "str.replace(source, target, replacement)", returnKind: SemanticValueSimple},
	"str.upper":                          {minArgs: 1, maxArgs: 1, signature: "str.upper(source)", returnKind: SemanticValueSimple},
	"str.lower":                          {minArgs: 1, maxArgs: 1, signature: "str.lower(source)", returnKind: SemanticValueSimple},
	"str.format":                         {minArgs: 1, maxArgs: 16, signature: "str.format(format, ...)", returnKind: SemanticValueSimple},
	"timeframe.change":                   {minArgs: 1, maxArgs: 1, signature: "timeframe.change(timeframe)", returnKind: SemanticValueSimple},
	"timeframe.in_seconds":               {minArgs: 0, maxArgs: 1, signature: "timeframe.in_seconds(timeframe?)", returnKind: SemanticValueSimple},
	"request.security":                   {minArgs: 3, maxArgs: 5, signature: "request.security(syminfo.tickerid, timeframe, expression, gaps?, lookahead?)", returnKind: SemanticValueSeries},
	"strategy.entry":                     {minArgs: 2, maxArgs: 12, signature: "strategy.entry(id, direction, ...)", returnKind: SemanticValueUnknown},
	"strategy.order":                     {minArgs: 2, maxArgs: 12, signature: "strategy.order(id, direction, ...)", returnKind: SemanticValueUnknown},
	"strategy.close":                     {minArgs: 1, maxArgs: 8, signature: "strategy.close(id, ...)", returnKind: SemanticValueUnknown},
	"strategy.exit":                      {minArgs: 2, maxArgs: 16, signature: "strategy.exit(id, from_entry, ...)", returnKind: SemanticValueUnknown},
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

//nolint:funlen
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
