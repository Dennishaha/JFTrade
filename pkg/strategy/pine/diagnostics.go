package pine

import (
	"fmt"
	"strings"
)

type DiagnosticSeverity string

const (
	DiagnosticSeverityError   DiagnosticSeverity = "error"
	DiagnosticSeverityWarning DiagnosticSeverity = "warning"
	DiagnosticSeverityInfo    DiagnosticSeverity = "info"
)

type Diagnostic struct {
	Severity  DiagnosticSeverity `json:"severity"`
	Code      string             `json:"code"`
	Message   string             `json:"message"`
	Line      int                `json:"line"`
	Column    int                `json:"column"`
	EndLine   int                `json:"endLine"`
	EndColumn int                `json:"endColumn"`
}

func diagnosticForLine(severity DiagnosticSeverity, code string, message string, line parsedLine) Diagnostic {
	column := line.indent + 1
	endColumn := column + len(strings.TrimRight(line.trimmed, "\r\n"))
	if endColumn <= column {
		endColumn = column + 1
	}
	return Diagnostic{
		Severity:  severity,
		Code:      code,
		Message:   message,
		Line:      line.number,
		Column:    column,
		EndLine:   line.number,
		EndColumn: endColumn,
	}
}

func diagnosticError(diagnostics []Diagnostic) error {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == DiagnosticSeverityError {
			return fmt.Errorf("pine line %d: %s", diagnostic.Line, diagnostic.Message)
		}
	}
	return nil
}
