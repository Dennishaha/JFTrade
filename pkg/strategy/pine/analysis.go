package pine

import (
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type AnalysisOptions struct {
	IncludeAST      bool
	IncludeSemantic bool
}

type AnalysisResult struct {
	OK                   bool
	NormalizedScript     string
	Program              *strategyir.Program
	Requirements         strategyir.Requirements
	Warnings             []string
	Diagnostics          []Diagnostic
	Features             []string
	AST                  *AST
	Semantic             *SemanticSummary
	Visuals              []PineVisualMetadata
	Declarations         []SemanticDeclaration
	CollectionOperations []SemanticCollectionOperation
	ObjectOperations     []SemanticObjectOperation
}

func AnalyzeScript(script string, options AnalysisOptions) AnalysisResult {
	trimmed := strings.TrimSpace(script)
	result := AnalysisResult{
		NormalizedScript: trimmed,
		Warnings:         []string{},
		Diagnostics:      []Diagnostic{},
		Features:         SupportedFeatureIDs(),
	}
	lines := tokenizeScript(script)
	ast, astDiagnostics := parseAST(lines)
	result.Diagnostics = append(result.Diagnostics, astDiagnostics...)
	if options.IncludeAST {
		result.AST = ast
	}
	semantic, semanticDiagnostics := analyzeSemantics(ast)
	result.Diagnostics = append(result.Diagnostics, semanticDiagnostics...)
	result.Visuals = semantic.Visuals
	result.Declarations = semantic.Declarations
	result.CollectionOperations = semantic.CollectionOperations
	result.ObjectOperations = semantic.ObjectOperations
	if options.IncludeSemantic || options.IncludeAST {
		result.Semantic = semantic
	}
	if len(lines) == 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Severity:  DiagnosticSeverityError,
			Code:      "PINE_EMPTY_SCRIPT",
			Message:   "pine script is required",
			Line:      1,
			Column:    1,
			EndLine:   1,
			EndColumn: 1,
		})
		return result
	}
	if diagnosticError(result.Diagnostics) != nil {
		return result
	}
	compilation, err := compileLoweredAST(script, lines, ast)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diagnosticFromError(err))
		return result
	}
	markExecutableSemanticSurface(semantic, compilation.Program)
	result.Declarations = semantic.Declarations
	result.ObjectOperations = semantic.ObjectOperations
	result.Program = compilation.Program
	result.Warnings = compilation.Warnings
	for _, warning := range compilation.Warnings {
		result.Diagnostics = append(result.Diagnostics, diagnosticFromWarning(warning))
	}
	requirements, err := strategyir.PlanRequirements(compilation.Program)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diagnosticFromError(err))
		return result
	}
	result.Requirements = requirements
	result.OK = true
	return result
}

func markExecutableSemanticSurface(summary *SemanticSummary, program *strategyir.Program) {
	if summary == nil || program == nil {
		return
	}
	typeLines := map[int]bool{}
	methodLines := map[int]bool{}
	for _, definition := range program.Types {
		typeLines[definition.Range.StartLine] = true
	}
	for _, definition := range program.Methods {
		methodLines[definition.Range.StartLine] = true
	}
	for index := range summary.Declarations {
		declaration := &summary.Declarations[index]
		executable := (declaration.Kind == "type" && typeLines[declaration.Line]) ||
			(declaration.Kind == "method" && methodLines[declaration.Line])
		if executable {
			declaration.Executable = true
			declaration.Reason = ""
			declaration.UnsupportedReason = ""
		}
	}
	objectLines := map[int]bool{}
	var collect func([]strategyir.Statement)
	collect = func(statements []strategyir.Statement) {
		for _, statement := range statements {
			switch typed := statement.(type) {
			case *strategyir.ObjectStmt:
				objectLines[typed.Range.StartLine] = true
			case *strategyir.IfStmt:
				collect(typed.Then)
				collect(typed.Else)
			case *strategyir.LoopStmt:
				collect(typed.Body)
			}
		}
	}
	for _, hook := range program.Hooks {
		collect(hook.Statements)
	}
	for index := range summary.ObjectOperations {
		operation := &summary.ObjectOperations[index]
		if objectLines[operation.Line] {
			operation.Executable = true
			operation.Reason = ""
		}
	}
}

func SupportedFeatureIDs() []string {
	registry := CapabilityRegistry()
	out := make([]string, 0, len(registry))
	for _, capability := range registry {
		if capability.Status == CapabilityUnsupported {
			continue
		}
		out = append(out, capability.ID)
	}
	return out
}

func diagnosticFromWarning(warning string) Diagnostic {
	line := 1
	message := warning
	if parsedLine, parsedMessage, ok := parsePineLineMessage(warning); ok {
		line = parsedLine
		message = parsedMessage
	}
	return Diagnostic{
		Severity:  DiagnosticSeverityWarning,
		Code:      "PINE_WARNING",
		Message:   message,
		Line:      line,
		Column:    1,
		EndLine:   line,
		EndColumn: 1,
	}
}

func diagnosticFromError(err error) Diagnostic {
	line := 1
	message := err.Error()
	if parsedLine, parsedMessage, ok := parsePineLineMessage(message); ok {
		line = parsedLine
		message = parsedMessage
	}
	return Diagnostic{
		Severity:  DiagnosticSeverityError,
		Code:      diagnosticCodeForCompileMessage(message),
		Message:   message,
		Line:      line,
		Column:    1,
		EndLine:   line,
		EndColumn: 1,
	}
}

func diagnosticCodeForCompileMessage(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(lower, "recursive user-defined function"):
		return "PINE_UDF_RECURSIVE_UNSUPPORTED"
	case strings.Contains(lower, "nested user-defined functions") ||
		strings.Contains(lower, "user-defined functions are supported only at top level"):
		return "PINE_UDF_NESTED_UNSUPPORTED"
	case strings.Contains(lower, "user-defined function") &&
		(strings.Contains(lower, "expects") ||
			strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "duplicate") ||
			strings.Contains(lower, "conflicts") ||
			strings.Contains(lower, "requires") ||
			strings.Contains(lower, "must have")):
		return "PINE_UDF_SIGNATURE_UNSUPPORTED"
	case strings.Contains(lower, "nested for loops deeper") ||
		strings.Contains(lower, "dynamic loop nesting exceeds"):
		return "PINE_LOOP_NESTING_UNSUPPORTED"
	case strings.Contains(lower, "for loop step cannot be 0") ||
		strings.Contains(lower, "for loop step does not reach the end value") ||
		strings.Contains(lower, "for loop expands to more than") ||
		strings.Contains(lower, "for loop exceeded") ||
		strings.Contains(lower, "while loop exceeded") ||
		strings.Contains(lower, "loop bound must be an integer"):
		return "PINE_LOOP_LIMIT_UNSUPPORTED"
	case strings.Contains(lower, "loop variable") && strings.Contains(lower, "read-only"):
		return "PINE_LOOP_VARIABLE_READONLY"
	case strings.Contains(lower, "oca_name") || strings.Contains(lower, "oca_type"):
		return "PINE_ORDER_OCA_UNSUPPORTED"
	case strings.Contains(lower, "qty or qty_percent"):
		return "PINE_ORDER_QTY_CONFLICT"
	case strings.Contains(lower, "trail with stop/limit") ||
		strings.Contains(lower, "accepts trail_points or trail_price, not both"):
		return "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED"
	case strings.Contains(lower, "strategy.exit advanced exit semantics"):
		return "PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED"
	case strings.Contains(lower, "broker emulator") ||
		strings.Contains(lower, "partial fill") ||
		strings.Contains(lower, "intrabar"):
		return "PINE_BROKER_EMULATOR_OUT_OF_SCOPE"
	default:
		return "PINE_COMPILE_ERROR"
	}
}

func parsePineLineMessage(value string) (int, string, bool) {
	prefix := "pine line "
	_, after, ok := strings.Cut(value, prefix)
	if !ok {
		return 0, "", false
	}
	rest := after
	before, after, ok := strings.Cut(rest, ":")
	if !ok {
		return 0, "", false
	}
	line, err := strconv.Atoi(strings.TrimSpace(before))
	if err != nil || line <= 0 {
		return 0, "", false
	}
	return line, strings.TrimSpace(after), true
}
