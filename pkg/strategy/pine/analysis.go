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
		"syntax.const_varip",
		"syntax.reassign",
		"syntax.udf_expression",
		"syntax.for_static_unroll",
		"expression.history_ref_1",
		"expression.history_ref_n",
		"expression.ternary",
		"expression.na_nz",
		"expression.strict_bool",
		"expression.input_defaults",
		"expression.input_symbol_session",
		"expression.math_namespace",
		"expression.string_namespace",
		"expression.format_constants",
		"expression.account_equity",
		"expression.time_variables",
		"expression.derived_sources",
		"expression.timestamp",
		"expression.barstate_session",
		"expression.pine_constants",
		"indicator.ma",
		"indicator.ma_source_aware",
		"indicator.source_aware_core",
		"indicator.rsi",
		"indicator.macd",
		"indicator.atr",
		"indicator.cci",
		"indicator.bollinger",
		"indicator.williams_r",
		"indicator.rolling_window",
		"indicator.sum",
		"indicator.cross",
		"indicator.cum_stoch_extrema_bars",
		"indicator.vwap_mfi_dmi_supertrend",
		"indicator.stateful_events",
		"indicator.sar",
		"request.security.mtf_ma_subset",
		"request.security.mtf_sources",
		"request.security.mtf_ma_source_aware",
		"request.security.timeframe_multipliers",
		"request.security.htf_history",
		"expression.input_timeframe",
		"expression.barmerge_constants",
		"visual.noop_calls",
		"alert.alertcondition_noop",
		"order.strategy_order_net",
		"order.qty_percent",
		"order.close_all",
		"order.exit_quantity",
		"order.exit_bracket",
		"order.exit_price_expressions",
		"order.pending_stop",
		"order.cancel_pending",
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
