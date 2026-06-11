package pine

import (
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type AnalysisOptions struct {
	IncludeAST bool
}

type AnalysisResult struct {
	OK               bool
	NormalizedScript string
	Program          *strategyir.Program
	Requirements     strategyir.Requirements
	Warnings         []string
	Diagnostics      []Diagnostic
	Features         []string
	AST              *AST
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
	compilation, err := compileLoweredAST(script, ast)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diagnosticFromError(err))
		return result
	}
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

func SupportedFeatureIDs() []string {
	return []string{
		"metadata.version6",
		"metadata.strategy",
		"syntax.if_else",
		"syntax.assignment",
		"syntax.var",
		"syntax.reassign",
		"expression.history_ref_1",
		"expression.ternary",
		"expression.na_nz",
		"expression.strict_bool",
		"indicator.ma",
		"indicator.rsi",
		"indicator.macd",
		"indicator.atr",
		"indicator.cci",
		"indicator.bollinger",
		"request.security.mtf_ma_subset",
		"strategy.entry_close_exit_subset",
	}
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
		Code:      "PINE_COMPILE_ERROR",
		Message:   message,
		Line:      line,
		Column:    1,
		EndLine:   line,
		EndColumn: 1,
	}
}

func parsePineLineMessage(value string) (int, string, bool) {
	prefix := "pine line "
	index := strings.Index(value, prefix)
	if index < 0 {
		return 0, "", false
	}
	rest := value[index+len(prefix):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return 0, "", false
	}
	line, err := strconv.Atoi(strings.TrimSpace(rest[:colon]))
	if err != nil || line <= 0 {
		return 0, "", false
	}
	return line, strings.TrimSpace(rest[colon+1:]), true
}
