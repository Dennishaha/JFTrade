package pine

import (
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type NodeKind string

const (
	NodeKindVersion         NodeKind = "version"
	NodeKindStrategy        NodeKind = "strategy"
	NodeKindAssignment      NodeKind = "assignment"
	NodeKindTupleAssignment NodeKind = "tuple_assignment"
	NodeKindIf              NodeKind = "if"
	NodeKindElse            NodeKind = "else"
	NodeKindCall            NodeKind = "call"
	NodeKindUnsupported     NodeKind = "unsupported"
)

type AST struct {
	SourceFormat string    `json:"sourceFormat"`
	Lines        []ASTLine `json:"lines"`
}

type ASTLine struct {
	Kind       NodeKind                  `json:"kind"`
	Line       int                       `json:"line"`
	Column     int                       `json:"column"`
	EndLine    int                       `json:"endLine"`
	EndColumn  int                       `json:"endColumn"`
	Indent     int                       `json:"indent"`
	Text       string                    `json:"text"`
	Name       string                    `json:"name,omitempty"`
	Expression string                    `json:"expression,omitempty"`
	Mode       strategyir.AssignmentMode `json:"mode,omitempty"`
}

func parseAST(lines []parsedLine) (*AST, []Diagnostic) {
	ast := &AST{SourceFormat: SourceFormatPineV6, Lines: make([]ASTLine, 0, len(lines))}
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		astLine := ASTLine{
			Kind:      classifyASTLine(line.trimmed),
			Line:      line.number,
			Column:    line.indent + 1,
			EndLine:   line.number,
			EndColumn: line.indent + 1 + len(line.trimmed),
			Indent:    line.indent,
			Text:      line.trimmed,
		}
		if astLine.EndColumn <= astLine.Column {
			astLine.EndColumn = astLine.Column + 1
		}
		if match := assignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			astLine.Kind = NodeKindAssignment
			astLine.Name = strings.TrimSpace(match[2])
			astLine.Expression = strings.TrimSpace(match[4])
			switch {
			case strings.TrimSpace(match[1]) != "":
				astLine.Mode = strategyir.AssignmentModeVar
			case strings.TrimSpace(match[3]) == ":=":
				astLine.Mode = strategyir.AssignmentModeReassign
			default:
				astLine.Mode = strategyir.AssignmentModeLet
			}
		} else if match := tupleAssignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			astLine.Kind = NodeKindTupleAssignment
			astLine.Name = strings.TrimSpace(match[1])
			astLine.Expression = strings.TrimSpace(match[5])
		}
		if diagnostic, ok := unsupportedSyntaxDiagnostic(line); ok {
			astLine.Kind = NodeKindUnsupported
			diagnostics = append(diagnostics, diagnostic)
		}
		ast.Lines = append(ast.Lines, astLine)
	}
	return ast, diagnostics
}

func classifyASTLine(trimmed string) NodeKind {
	lower := strings.ToLower(strings.TrimSpace(trimmed))
	switch {
	case strings.HasPrefix(lower, "//@version"):
		return NodeKindVersion
	case strings.HasPrefix(lower, "strategy("):
		return NodeKindStrategy
	case strings.HasPrefix(lower, "if "):
		return NodeKindIf
	case lower == "else" || lower == "else:":
		return NodeKindElse
	case strings.Contains(lower, "("):
		return NodeKindCall
	default:
		return NodeKindUnsupported
	}
}
